package telegram

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

// Repository is telegram's Postgres access. The API's queries are scoped
// by organization (§36-TENANCY); the webhook's find a bot by its id and
// then work only within that bot's organization.
type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// newBot is a bot ready to store, its webhook already set.
type newBot struct {
	ID, OrganizationID, UserID string
	TgBotID                    int64
	Username                   string
	TokenEncrypted             string
	WebhookSecretHash          string
	WelcomeText                string
	Channel                    *Channel
}

func (r *Repository) create(ctx context.Context, b newBot) (Bot, error) {
	var channelID *int64
	title, link := "", ""
	if b.Channel != nil {
		channelID, title, link = &b.Channel.ID, b.Channel.Title, b.Channel.InviteLink
	}
	var userID *string
	if b.UserID != "" {
		userID = &b.UserID
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO telegram_bots (id, organization_id, tg_bot_id, username, token_encrypted, webhook_secret_hash,
		                           welcome_text, channel_id, channel_title, channel_invite_link, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		b.ID, b.OrganizationID, b.TgBotID, b.Username, b.TokenEncrypted, b.WebhookSecretHash,
		b.WelcomeText, channelID, title, link, userID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Bot{}, apierror.Conflict("this bot is already connected")
	}
	if err != nil {
		return Bot{}, fmt.Errorf("telegram: storing bot: %w", err)
	}
	return r.Get(ctx, b.OrganizationID, b.ID)
}

const botColumns = `id, username, welcome_text, channel_id, channel_title, channel_invite_link, created_at`

func scanBot(row pgx.Row) (Bot, error) {
	var b Bot
	var channelID *int64
	var title, link string
	if err := row.Scan(&b.ID, &b.Username, &b.WelcomeText, &channelID, &title, &link, &b.CreatedAt); err != nil {
		return Bot{}, err
	}
	if channelID != nil {
		b.Channel = &Channel{ID: *channelID, Title: title, InviteLink: link}
	}
	b.DeepLink = deepLink(b.Username)
	return b, nil
}

func (r *Repository) Get(ctx context.Context, orgID, id string) (Bot, error) {
	b, err := scanBot(r.db.QueryRow(ctx, `SELECT `+botColumns+` FROM telegram_bots WHERE id = $1 AND organization_id = $2`, id, orgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Bot{}, apierror.NotFound("bot not found")
	}
	if err != nil {
		return Bot{}, fmt.Errorf("telegram: reading bot: %w", err)
	}
	return b, nil
}

// List is the organization's bots, newest first.
func (r *Repository) List(ctx context.Context, orgID string) ([]Bot, error) {
	rows, err := r.db.Query(ctx, `SELECT `+botColumns+` FROM telegram_bots WHERE organization_id = $1 ORDER BY created_at DESC, id DESC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("telegram: listing bots: %w", err)
	}
	defer rows.Close()
	out := []Bot{}
	for rows.Next() {
		b, err := scanBot(rows)
		if err != nil {
			return nil, fmt.Errorf("telegram: scanning bot: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// encryptedToken is the bot's stored token, for removing its webhook.
func (r *Repository) encryptedToken(ctx context.Context, orgID, id string) (string, error) {
	var token string
	err := r.db.QueryRow(ctx, `SELECT token_encrypted FROM telegram_bots WHERE id = $1 AND organization_id = $2`, id, orgID).Scan(&token)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apierror.NotFound("bot not found")
	}
	if err != nil {
		return "", fmt.Errorf("telegram: reading bot token: %w", err)
	}
	return token, nil
}

func (r *Repository) Delete(ctx context.Context, orgID, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM telegram_bots WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		return fmt.Errorf("telegram: deleting bot: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("bot not found")
	}
	return nil
}

// webhookBot is what the webhook needs of a bot.
type webhookBot struct {
	ID, OrganizationID string
	SecretHash         string
	Username           string
	WelcomeText        string
	ChannelID          int64 // 0: none
	ChannelTitle       string
	InviteLink         string
}

// webhookBot finds a bot by id alone: the webhook's URL names it, and its
// secret header proves the call is Telegram's (checked by the caller).
func (r *Repository) webhookBot(ctx context.Context, id string) (webhookBot, bool, error) {
	var b webhookBot
	var channelID *int64
	err := r.db.QueryRow(ctx, `
		SELECT id, organization_id, webhook_secret_hash, username, welcome_text, channel_id, channel_title, channel_invite_link
		FROM telegram_bots WHERE id = $1`, id,
	).Scan(&b.ID, &b.OrganizationID, &b.SecretHash, &b.Username, &b.WelcomeText, &channelID, &b.ChannelTitle, &b.InviteLink)
	if errors.Is(err, pgx.ErrNoRows) {
		return webhookBot{}, false, nil
	}
	if err != nil {
		return webhookBot{}, false, fmt.Errorf("telegram: reading bot for webhook: %w", err)
	}
	if channelID != nil {
		b.ChannelID = *channelID
	}
	return b, true, nil
}

// recordStart remembers that tgUser started bot from clickID and reports
// whether that is new for this click (a TG_START to record): the same
// click's /start again is not.
func (r *Repository) recordStart(ctx context.Context, b webhookBot, tgUser int64, clickID string, at time.Time) (bool, error) {
	var isNew bool
	err := r.db.QueryRow(ctx, `
		INSERT INTO telegram_users (bot_id, tg_user_id, organization_id, click_id, started_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (bot_id, tg_user_id) DO UPDATE SET click_id = EXCLUDED.click_id, started_at = EXCLUDED.started_at
		WHERE telegram_users.click_id <> EXCLUDED.click_id
		RETURNING true`, b.ID, tgUser, b.OrganizationID, clickID, at).Scan(&isNew)
	if errors.Is(err, pgx.ErrNoRows) { // same click: nothing changed
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("telegram: recording start: %w", err)
	}
	return true, nil
}

// startedClick is the click of tgUser's last /start of bot, and whether
// that click's join is already recorded. No /start: "".
func (r *Repository) startedClick(ctx context.Context, b webhookBot, tgUser int64) (clickID string, joined bool, err error) {
	var joinedClick string
	err = r.db.QueryRow(ctx, `SELECT click_id, joined_click_id FROM telegram_users WHERE bot_id = $1 AND tg_user_id = $2`,
		b.ID, tgUser).Scan(&clickID, &joinedClick)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("telegram: reading started click: %w", err)
	}
	return clickID, joinedClick == clickID, nil
}

// recordJoin marks tgUser's join of bot's channel for clickID and reports
// whether it was not marked yet — so two deliveries of one join record
// one TG_JOIN.
func (r *Repository) recordJoin(ctx context.Context, b webhookBot, tgUser int64, clickID string, at time.Time) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE telegram_users SET joined_click_id = click_id, joined_at = $4
		WHERE bot_id = $1 AND tg_user_id = $2 AND click_id = $3 AND joined_click_id <> click_id`, b.ID, tgUser, clickID, at)
	if err != nil {
		return false, fmt.Errorf("telegram: recording join: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
