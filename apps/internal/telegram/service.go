package telegram

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/integrations"
)

type Service struct {
	repo   *Repository
	api    BotAPI
	logger *slog.Logger
	// key encrypts bot tokens (INTEGRATION_ENCRYPTION_KEY); webhookBase is
	// the tracker's public https origin (TELEGRAM_WEBHOOK_BASE_URL). Either
	// missing: bots cannot be connected.
	key         []byte
	webhookBase string
}

func NewService(repo *Repository, api BotAPI, key []byte, webhookBase string, logger *slog.Logger) *Service {
	return &Service{repo: repo, api: api, key: key, webhookBase: strings.TrimRight(webhookBase, "/"), logger: logger}
}

// Configured reports whether bots can be connected on this deployment.
func (s *Service) Configured() bool { return len(s.key) > 0 && s.webhookBase != "" }

func (s *Service) List(ctx context.Context, orgID string) ([]Bot, error) {
	bots, err := s.repo.List(ctx, orgID)
	if err != nil {
		return nil, err
	}
	free, err := s.repo.freeInvites(ctx, orgID)
	if err != nil {
		return nil, err
	}
	for i := range bots {
		s.decorate(&bots[i], free[bots[i].ID])
	}
	return bots, nil
}

// decorate adds what depends on the deployment and the pool.
func (s *Service) decorate(b *Bot, free int) {
	if b.Channel != nil && s.webhookBase != "" {
		b.ClickInviteLink = ClickInviteLink(s.webhookBase, b.ID)
	}
	b.FreeInviteLinks = free
}

// Connect checks the token with Telegram, the channel (the bot must be an
// admin that can invite), points the bot's webhook at the tracker and
// stores it. Telegram's own reasons ("Unauthorized", "chat not found")
// come back as field errors.
func (s *Service) Connect(ctx context.Context, orgID, userID string, in ConnectInput) (Bot, error) {
	if !s.Configured() {
		return Bot{}, apierror.Unavailable("Telegram is not configured on this deployment (TELEGRAM_WEBHOOK_BASE_URL, INTEGRATION_ENCRYPTION_KEY)")
	}
	in.Token = strings.TrimSpace(in.Token)
	in.Channel = strings.TrimSpace(in.Channel)
	in.WelcomeText = strings.TrimSpace(in.WelcomeText)
	fields := map[string]string{}
	if !tokenPattern.MatchString(in.Token) {
		fields["token"] = "a bot token from @BotFather (123456:ABC…)"
	}
	if in.Channel != "" && !channelPattern.MatchString(in.Channel) {
		fields["channel"] = "@channelname or the channel's id (-100…)"
	}
	if utf8.RuneCountInString(in.WelcomeText) > MaxWelcomeText {
		fields["welcomeText"] = fmt.Sprintf("at most %d characters", MaxWelcomeText)
	}
	if len(fields) > 0 {
		return Bot{}, apierror.Validation("invalid bot", fields)
	}

	me, err := s.api.GetMe(ctx, in.Token)
	if err != nil {
		return Bot{}, telegramError(err, "token")
	}
	if !me.IsBot || me.Username == "" {
		return Bot{}, apierror.Validation("invalid bot", map[string]string{"token": "not a bot's token"})
	}
	var channel *Channel
	if in.Channel != "" {
		if channel, err = s.checkChannel(ctx, in.Token, in.Channel, me.ID); err != nil {
			return Bot{}, err
		}
	}

	id := idgen.New()
	secret, err := randomSecret()
	if err != nil {
		return Bot{}, err
	}
	encrypted, err := integrations.Encrypt(s.key, in.Token)
	if err != nil {
		return Bot{}, fmt.Errorf("telegram: encrypting token: %w", err)
	}
	if err := s.api.SetWebhook(ctx, in.Token, s.webhookBase+"/tg/"+id, secret, AllowedUpdates); err != nil {
		return Bot{}, telegramError(err, "token")
	}
	b, err := s.repo.create(ctx, newBot{
		ID: id, OrganizationID: orgID, UserID: userID, TgBotID: me.ID, Username: me.Username,
		TokenEncrypted: encrypted, WebhookSecretHash: hashSecret(secret), WelcomeText: in.WelcomeText, Channel: channel,
	})
	if err != nil {
		// Not stored: the webhook just set would point at nothing. A bot
		// already connected (409) keeps its own: setWebhook replaced it, so
		// put nothing back — the operator reconnects it by deleting first.
		if delErr := s.api.DeleteWebhook(ctx, in.Token); delErr != nil {
			s.logger.Warn("telegram: removing webhook of a bot not stored", "error", delErr)
		}
		return Bot{}, err
	}
	s.decorate(&b, 0)
	return b, nil
}

// checkChannel resolves channel and checks botID can invite to it; the
// invite link is the public t.me link, or one the bot creates.
func (s *Service) checkChannel(ctx context.Context, token, channel string, botID int64) (*Channel, error) {
	chat, err := s.api.GetChat(ctx, token, channel)
	if err != nil {
		return nil, telegramError(err, "channel")
	}
	if chat.Type != "channel" && chat.Type != "supergroup" {
		return nil, apierror.Validation("invalid channel", map[string]string{"channel": "a channel or a supergroup"})
	}
	member, err := s.api.GetChatMember(ctx, token, chat.ID, botID)
	if err != nil {
		return nil, telegramError(err, "channel")
	}
	if member.Status != "administrator" || !member.CanInviteUsers {
		return nil, apierror.Validation("invalid channel", map[string]string{"channel": "make the bot an admin of the channel with the right to invite users"})
	}
	link := ""
	if chat.Username != "" {
		link = "https://t.me/" + chat.Username
	} else if link, err = s.api.CreateChatInviteLink(ctx, token, chat.ID, "FLOX"); err != nil {
		return nil, telegramError(err, "channel")
	}
	return &Channel{ID: chat.ID, Title: chat.Title, InviteLink: link}, nil
}

// Delete removes the bot and, as far as Telegram lets it (a revoked token
// cannot), its webhook.
func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	encrypted, err := s.repo.encryptedToken(ctx, orgID, id)
	if err != nil {
		return err
	}
	if token, err := integrations.Decrypt(s.key, encrypted); err != nil {
		s.logger.Warn("telegram: cannot decrypt bot token, webhook left as is", "bot_id", id, "error", err)
	} else if err := s.api.DeleteWebhook(ctx, token); err != nil {
		s.logger.Warn("telegram: removing webhook", "bot_id", id, "error", err)
	}
	return s.repo.Delete(ctx, orgID, id)
}

// telegramError turns Telegram's refusal into a field error the operator
// can act on; anything else (unreachable) is a 503.
func telegramError(err error, field string) error {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apierror.Validation("Telegram refused", map[string]string{field: "Telegram: " + apiErr.Description})
	}
	return apierror.Unavailable("Telegram could not be reached: " + err.Error())
}

func randomSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("telegram: generating webhook secret: %w", err)
	}
	// Telegram allows A-Z a-z 0-9 _ - in secret_token.
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
