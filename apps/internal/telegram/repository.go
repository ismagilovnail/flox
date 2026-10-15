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
	Channel                    *Channel // optional first channel
}

func (r *Repository) create(ctx context.Context, b newBot) (Bot, error) {
	var userID *string
	if b.UserID != "" {
		userID = &b.UserID
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Bot{}, fmt.Errorf("telegram: storing bot: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `
		INSERT INTO telegram_bots (id, organization_id, tg_bot_id, username, token_encrypted, webhook_secret_hash,
		                           welcome_text, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		b.ID, b.OrganizationID, b.TgBotID, b.Username, b.TokenEncrypted, b.WebhookSecretHash, b.WelcomeText, userID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Bot{}, apierror.Conflict("this bot is already connected")
	}
	if err != nil {
		return Bot{}, fmt.Errorf("telegram: storing bot: %w", err)
	}
	if b.Channel != nil {
		if err := insertChannel(ctx, tx, b.OrganizationID, b.ID, *b.Channel); err != nil {
			return Bot{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Bot{}, fmt.Errorf("telegram: storing bot: %w", err)
	}
	return r.Get(ctx, b.OrganizationID, b.ID)
}

func insertChannel(ctx context.Context, q interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, orgID, botID string, c Channel) error {
	_, err := q.Exec(ctx, `
		INSERT INTO telegram_channels (id, bot_id, organization_id, tg_chat_id, title, invite_link)
		VALUES ($1, $2, $3, $4, $5, $6)`, c.ID, botID, orgID, c.TgChatID, c.Title, c.InviteLink)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return apierror.Conflict("this channel is already added to the bot")
	}
	if err != nil {
		return fmt.Errorf("telegram: storing channel: %w", err)
	}
	return nil
}

const botColumns = `id, username, welcome_text, created_at`

func scanBot(row pgx.Row) (Bot, error) {
	b := Bot{Channels: []Channel{}}
	if err := row.Scan(&b.ID, &b.Username, &b.WelcomeText, &b.CreatedAt); err != nil {
		return Bot{}, err
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
	bots := []Bot{b}
	if err := r.attachChannels(ctx, orgID, bots); err != nil {
		return Bot{}, err
	}
	return bots[0], nil
}

// List is the organization's bots, newest first, each with its channels
// (oldest first).
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("telegram: listing bots: %w", err)
	}
	rows.Close()
	return out, r.attachChannels(ctx, orgID, out)
}

// attachChannels fills the bots' channels with their free invite links.
func (r *Repository) attachChannels(ctx context.Context, orgID string, bots []Bot) error {
	if len(bots) == 0 {
		return nil
	}
	ids := make([]string, len(bots))
	byID := make(map[string]*Bot, len(bots))
	for i := range bots {
		ids[i] = bots[i].ID
		byID[bots[i].ID] = &bots[i]
	}
	rows, err := r.db.Query(ctx, `
		SELECT c.bot_id, c.id, c.tg_chat_id, c.title, c.invite_link, c.created_at,
		       (SELECT count(*) FROM telegram_invite_links l WHERE l.channel_id = c.id AND l.click_id IS NULL)
		FROM telegram_channels c
		WHERE c.organization_id = $1 AND c.bot_id = ANY($2)
		ORDER BY c.created_at, c.id`, orgID, ids)
	if err != nil {
		return fmt.Errorf("telegram: listing channels: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var botID string
		var c Channel
		if err := rows.Scan(&botID, &c.ID, &c.TgChatID, &c.Title, &c.InviteLink, &c.CreatedAt, &c.FreeInviteLinks); err != nil {
			return fmt.Errorf("telegram: scanning channel: %w", err)
		}
		b := byID[botID]
		b.Channels = append(b.Channels, c)
	}
	return rows.Err()
}

// updateWelcome sets the bot's welcome message.
func (r *Repository) updateWelcome(ctx context.Context, orgID, id, text string) error {
	tag, err := r.db.Exec(ctx, `UPDATE telegram_bots SET welcome_text = $3 WHERE id = $1 AND organization_id = $2`, id, orgID, text)
	if err != nil {
		return fmt.Errorf("telegram: updating bot: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("bot not found")
	}
	return nil
}

// addChannel stores a channel of the bot, refusing a bot already at
// MaxChannels. The bot row is locked so two adds cannot both pass the count.
func (r *Repository) addChannel(ctx context.Context, orgID, botID string, c Channel) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("telegram: adding channel: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var n int
	err = tx.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM telegram_channels c WHERE c.bot_id = b.id)
		FROM telegram_bots b WHERE b.id = $1 AND b.organization_id = $2 FOR UPDATE`, botID, orgID).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return apierror.NotFound("bot not found")
	}
	if err != nil {
		return fmt.Errorf("telegram: adding channel: %w", err)
	}
	if n >= MaxChannels {
		return apierror.Validation("too many channels", map[string]string{"channel": fmt.Sprintf("a bot has at most %d channels", MaxChannels)})
	}
	if err := insertChannel(ctx, tx, orgID, botID, c); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("telegram: adding channel: %w", err)
	}
	return nil
}

// deleteChannel removes a channel of the bot with its invite-link pool.
func (r *Repository) deleteChannel(ctx context.Context, orgID, botID, channelID string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM telegram_channels WHERE id = $1 AND bot_id = $2 AND organization_id = $3`, channelID, botID, orgID)
	if err != nil {
		return fmt.Errorf("telegram: deleting channel: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("channel not found")
	}
	return nil
}

// credentials is the bot's stored token and Telegram id, for checking a
// channel it is added to.
func (r *Repository) credentials(ctx context.Context, orgID, id string) (token string, tgBotID int64, err error) {
	err = r.db.QueryRow(ctx, `SELECT token_encrypted, tg_bot_id FROM telegram_bots WHERE id = $1 AND organization_id = $2`, id, orgID).Scan(&token, &tgBotID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, apierror.NotFound("bot not found")
	}
	if err != nil {
		return "", 0, fmt.Errorf("telegram: reading bot token: %w", err)
	}
	return token, tgBotID, nil
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
	Channels           []webhookChannel // oldest first
}

type webhookChannel struct {
	ID         string
	TgChatID   int64
	Title      string
	InviteLink string
}

// channel is the bot's channel with Telegram id chatID.
func (b webhookBot) channel(chatID int64) (webhookChannel, bool) {
	for _, c := range b.Channels {
		if c.TgChatID == chatID {
			return c, true
		}
	}
	return webhookChannel{}, false
}

// webhookBot finds a bot by id alone: the webhook's URL names it, and its
// secret header proves the call is Telegram's (checked by the caller).
func (r *Repository) webhookBot(ctx context.Context, id string) (webhookBot, bool, error) {
	var b webhookBot
	err := r.db.QueryRow(ctx, `
		SELECT id, organization_id, webhook_secret_hash, username, welcome_text
		FROM telegram_bots WHERE id = $1`, id,
	).Scan(&b.ID, &b.OrganizationID, &b.SecretHash, &b.Username, &b.WelcomeText)
	if errors.Is(err, pgx.ErrNoRows) {
		return webhookBot{}, false, nil
	}
	if err != nil {
		return webhookBot{}, false, fmt.Errorf("telegram: reading bot for webhook: %w", err)
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, tg_chat_id, title, invite_link FROM telegram_channels
		WHERE bot_id = $1 ORDER BY created_at, id`, id)
	if err != nil {
		return webhookBot{}, false, fmt.Errorf("telegram: reading bot channels for webhook: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c webhookChannel
		if err := rows.Scan(&c.ID, &c.TgChatID, &c.Title, &c.InviteLink); err != nil {
			return webhookBot{}, false, fmt.Errorf("telegram: scanning bot channel: %w", err)
		}
		b.Channels = append(b.Channels, c)
	}
	return b, true, rows.Err()
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
