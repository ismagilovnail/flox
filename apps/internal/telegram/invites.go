package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/ismagilovnail/flox/apps/internal/integrations"
)

// Per-click invite links (docs/telegram.md "Per-click invite links"): a
// destination of /tgj/{botId}?click={click_id} sends each click to its own
// invite link of the bot's channel, so a join through it is TG_JOIN on that
// click without a /start. Telegram makes the links and the redirect may
// not call it (CLAUDE.md #9): the worker keeps a pool (InvitePool), the
// tracker claims from it (InviteRedirect).

const (
	// InvitePoolSize is how many free links the worker keeps per bot.
	InvitePoolSize = 50
	// invitesPerRun bounds the links created per bot in one run, to stay
	// well inside Telegram's rate limits; the next run tops up the rest.
	invitesPerRun = 20
	// InviteLinkTTL is how long a claimed link stays valid: after it the
	// worker revokes it (a join that late is no longer this ad's).
	InviteLinkTTL = 30 * 24 * time.Hour
	// revokesPerRun bounds the revocations in one run.
	revokesPerRun = 100
	// InvitePoolInterval is how often the worker tops up and revokes.
	InvitePoolInterval = time.Minute
	// clickLag is how long after a claim a click may still be missing from
	// ClickHouse (events are flushed in batches): a join through the link
	// before then is retried rather than dropped.
	clickLag = 10 * time.Minute
)

// inviteLinkName labels the pool's links in the channel's admin list.
const inviteLinkName = "FLOX click"

// ClickInviteLink is the destination URL for a bot's channel with a link of
// its own per click (base: TELEGRAM_WEBHOOK_BASE_URL, the tracker).
func ClickInviteLink(base, botID string) string {
	return base + "/tgj/" + botID + "?click={click_id}"
}

// claimInvite returns clickID's invite link of bot's channel: the one it
// already has, else a free one from the pool, claimed now. found is false
// for an unknown bot or one without a channel; link is "" when the pool is
// empty — channel is then the channel's shared link.
func (r *Repository) claimInvite(ctx context.Context, botID, clickID string) (link, channel string, found bool, err error) {
	var claimed *string
	err = r.db.QueryRow(ctx, `
		WITH bot AS (
		  SELECT channel_invite_link FROM telegram_bots WHERE id = $1 AND channel_id IS NOT NULL
		), existing AS (
		  SELECT invite_link FROM telegram_invite_links WHERE bot_id = $1 AND click_id = $2 LIMIT 1
		), claimed AS (
		  UPDATE telegram_invite_links SET click_id = $2, claimed_at = now()
		  WHERE invite_link = (
		    SELECT invite_link FROM telegram_invite_links
		    WHERE bot_id = $1 AND click_id IS NULL AND NOT EXISTS (SELECT 1 FROM existing)
		    ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED)
		  RETURNING invite_link
		)
		SELECT bot.channel_invite_link, coalesce((SELECT invite_link FROM existing), (SELECT invite_link FROM claimed))
		FROM bot`, botID, clickID).Scan(&channel, &claimed)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("telegram: claiming invite link: %w", err)
	}
	if claimed != nil {
		link = *claimed
	}
	return link, channel, true, nil
}

// channelLink is bot's channel's shared link (found: a bot with a channel).
func (r *Repository) channelLink(ctx context.Context, botID string) (string, bool, error) {
	var link string
	err := r.db.QueryRow(ctx, `SELECT channel_invite_link FROM telegram_bots WHERE id = $1 AND channel_id IS NOT NULL`, botID).Scan(&link)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("telegram: reading channel link: %w", err)
	}
	return link, true, nil
}

// inviteClaim is a claimed link as the webhook sees it.
type inviteClaim struct {
	ClickID   string
	ClaimedAt time.Time
	Joined    bool
}

// invitedClick is the claim of link among bot's links (found false: not a
// claimed link of this bot).
func (r *Repository) invitedClick(ctx context.Context, b webhookBot, link string) (inviteClaim, bool, error) {
	var c inviteClaim
	err := r.db.QueryRow(ctx, `
		SELECT click_id, claimed_at, joined_at IS NOT NULL FROM telegram_invite_links
		WHERE invite_link = $1 AND bot_id = $2 AND click_id IS NOT NULL`, link, b.ID).Scan(&c.ClickID, &c.ClaimedAt, &c.Joined)
	if errors.Is(err, pgx.ErrNoRows) {
		return inviteClaim{}, false, nil
	}
	if err != nil {
		return inviteClaim{}, false, fmt.Errorf("telegram: reading invite link: %w", err)
	}
	return c, true, nil
}

// recordInviteJoin marks link's join and reports whether it was not marked
// yet — one TG_JOIN per link, so per click.
func (r *Repository) recordInviteJoin(ctx context.Context, b webhookBot, link string, at time.Time) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE telegram_invite_links SET joined_at = $3
		WHERE invite_link = $1 AND bot_id = $2 AND click_id IS NOT NULL AND joined_at IS NULL`, link, b.ID, at)
	if err != nil {
		return false, fmt.Errorf("telegram: recording invite join: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// freeInvites is how many free links each bot has.
func (r *Repository) freeInvites(ctx context.Context, orgID string) (map[string]int, error) {
	rows, err := r.db.Query(ctx, `
		SELECT bot_id, count(*) FROM telegram_invite_links
		WHERE organization_id = $1 AND click_id IS NULL GROUP BY bot_id`, orgID)
	if err != nil {
		return nil, fmt.Errorf("telegram: counting free invite links: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("telegram: scanning free invite links: %w", err)
		}
		out[id] = n
	}
	return out, rows.Err()
}

// poolBot is a bot whose pool the worker keeps.
type poolBot struct {
	ID, OrganizationID string
	TokenEncrypted     string
	ChannelID          int64
	Free               int
}

// poolBots is every bot with a channel and its free links — across
// organizations (the worker's job), or one organization's when orgID is
// set (tests share the dev database).
func (r *Repository) poolBots(ctx context.Context, orgID string) ([]poolBot, error) {
	rows, err := r.db.Query(ctx, `
		SELECT b.id, b.organization_id, b.token_encrypted, b.channel_id,
		       (SELECT count(*) FROM telegram_invite_links l WHERE l.bot_id = b.id AND l.click_id IS NULL)
		FROM telegram_bots b
		WHERE b.channel_id IS NOT NULL AND ($1 = '' OR b.organization_id = $1)
		ORDER BY b.id`, orgID)
	if err != nil {
		return nil, fmt.Errorf("telegram: listing bots for invite links: %w", err)
	}
	defer rows.Close()
	var out []poolBot
	for rows.Next() {
		var b poolBot
		if err := rows.Scan(&b.ID, &b.OrganizationID, &b.TokenEncrypted, &b.ChannelID, &b.Free); err != nil {
			return nil, fmt.Errorf("telegram: scanning bot for invite links: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *Repository) addInvite(ctx context.Context, b poolBot, link string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO telegram_invite_links (invite_link, bot_id, organization_id) VALUES ($1, $2, $3)
		ON CONFLICT (invite_link) DO NOTHING`, link, b.ID, b.OrganizationID)
	if err != nil {
		return fmt.Errorf("telegram: storing invite link: %w", err)
	}
	return nil
}

// expiredInvites is b's links claimed before cutoff, oldest first.
func (r *Repository) expiredInvites(ctx context.Context, b poolBot, cutoff time.Time, limit int) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT invite_link FROM telegram_invite_links
		WHERE bot_id = $1 AND click_id IS NOT NULL AND claimed_at < $2
		ORDER BY claimed_at LIMIT $3`, b.ID, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("telegram: listing expired invite links: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var link string
		if err := rows.Scan(&link); err != nil {
			return nil, err
		}
		out = append(out, link)
	}
	return out, rows.Err()
}

func (r *Repository) deleteInvite(ctx context.Context, b poolBot, link string) error {
	if _, err := r.db.Exec(ctx, `DELETE FROM telegram_invite_links WHERE invite_link = $1 AND bot_id = $2`, link, b.ID); err != nil {
		return fmt.Errorf("telegram: deleting invite link: %w", err)
	}
	return nil
}

// InviteRedirect serves GET /tgj/{botId}?click= on apps/tracker: the click's
// own invite link of the bot's channel, claimed from the pool — one indexed
// Postgres statement, no Telegram call. An empty pool (or no click id)
// falls back to the channel's shared link: the visitor still gets in, and
// the join is attributed only through a /start.
type InviteRedirect struct {
	repo   *Repository
	logger *slog.Logger
	// Fallbacks counts redirects to the shared link because the pool was
	// empty (a metric; nil: not counted).
	Fallbacks interface{ Inc() }
}

func NewInviteRedirect(repo *Repository, logger *slog.Logger) *InviteRedirect {
	return &InviteRedirect{repo: repo, logger: logger}
}

func (h *InviteRedirect) Register(r chi.Router) {
	r.Get("/tgj/{botId}", h.serve)
}

func (h *InviteRedirect) serve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	botID := chi.URLParam(r, "botId")
	clickID := r.URL.Query().Get("click")
	var link, channel string
	var found bool
	var err error
	if clickPattern.MatchString(clickID) {
		link, channel, found, err = h.repo.claimInvite(ctx, botID, clickID)
	} else {
		// Opened without a click (the template itself, a typo): the
		// shared link, and no pool link spent on it.
		channel, found, err = h.repo.channelLink(ctx, botID)
	}
	if err != nil {
		h.logger.Error("telegram invite redirect", "bot_id", botID, "error", err)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	if link == "" {
		link = channel
		if clickPattern.MatchString(clickID) {
			h.logger.Warn("telegram invite pool empty, shared channel link used", "bot_id", botID)
			if h.Fallbacks != nil {
				h.Fallbacks.Inc()
			}
		}
	}
	// 302, never cached: every click asks again.
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, link, http.StatusFound)
}

// InvitePool is the worker's job: every bot with a channel keeps
// InvitePoolSize free links, and claimed links are revoked after
// InviteLinkTTL.
type InvitePool struct {
	Repo   *Repository
	API    BotAPI
	Key    []byte // INTEGRATION_ENCRYPTION_KEY, to read the bots' tokens
	Logger *slog.Logger
	// OrganizationID limits a run to one organization (tests); "" is all.
	OrganizationID string
	Now            func() time.Time
}

// RunOnce tops up and revokes for every bot. One bot failing (a revoked
// token, the bot no longer admin, Telegram unreachable) is logged and does
// not stop the others; created counts the links added.
func (p *InvitePool) RunOnce(ctx context.Context) (created, revoked int, err error) {
	bots, err := p.Repo.poolBots(ctx, p.OrganizationID)
	if err != nil {
		return 0, 0, err
	}
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	for _, b := range bots {
		token, err := integrations.Decrypt(p.Key, b.TokenEncrypted)
		if err != nil {
			p.Logger.Warn("telegram invite pool: cannot decrypt bot token", "bot_id", b.ID, "error", err)
			continue
		}
		c, err := p.topUp(ctx, b, token)
		created += c
		if err != nil {
			p.Logger.Warn("telegram invite pool: creating links", "bot_id", b.ID, "error", err)
		}
		v, err := p.revoke(ctx, b, token, now().Add(-InviteLinkTTL))
		revoked += v
		if err != nil {
			p.Logger.Warn("telegram invite pool: revoking links", "bot_id", b.ID, "error", err)
		}
		if ctx.Err() != nil {
			return created, revoked, ctx.Err()
		}
	}
	return created, revoked, nil
}

func (p *InvitePool) topUp(ctx context.Context, b poolBot, token string) (int, error) {
	n := min(InvitePoolSize-b.Free, invitesPerRun)
	created := 0
	for range n {
		link, err := p.API.CreateChatInviteLink(ctx, token, b.ChannelID, inviteLinkName)
		if err != nil {
			return created, err
		}
		if err := p.Repo.addInvite(ctx, b, link); err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}

// revoke revokes b's links claimed before cutoff and forgets them. A link
// Telegram refuses to revoke (already revoked, the channel gone) is
// forgotten too; one it could not be asked about is kept for the next run.
func (p *InvitePool) revoke(ctx context.Context, b poolBot, token string, cutoff time.Time) (int, error) {
	links, err := p.Repo.expiredInvites(ctx, b, cutoff, revokesPerRun)
	if err != nil {
		return 0, err
	}
	revoked := 0
	for _, link := range links {
		if err := p.API.RevokeChatInviteLink(ctx, token, b.ChannelID, link); err != nil {
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				return revoked, err
			}
		}
		if err := p.Repo.deleteInvite(ctx, b, link); err != nil {
			return revoked, err
		}
		revoked++
	}
	return revoked, nil
}

// RunLoop runs RunOnce every interval until ctx ends.
func (p *InvitePool) RunLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		created, revoked, err := p.RunOnce(ctx)
		if err != nil && ctx.Err() == nil {
			p.Logger.Error("telegram invite pool", "error", err)
		} else if created > 0 || revoked > 0 {
			p.Logger.Info("telegram invite pool", "created", created, "revoked", revoked)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
