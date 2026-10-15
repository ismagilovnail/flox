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
// destination of /tgj/{channelId}?click={click_id} sends each click to its
// own invite link of the channel, so a join through it is TG_JOIN on that
// click without a /start. Telegram makes the links and the redirect may
// not call it (CLAUDE.md #9): the worker keeps a pool (InvitePool), the
// tracker claims from it (InviteRedirect).

const (
	// InvitePoolSize is how many free links the worker keeps per channel.
	InvitePoolSize = 50
	// invitesPerRun bounds the links created per channel in one run, to stay
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

// ClickInviteLink is the destination URL for a channel with a link of its
// own per click (base: TELEGRAM_WEBHOOK_BASE_URL, the tracker).
func ClickInviteLink(base, channelID string) string {
	return base + "/tgj/" + channelID + "?click={click_id}"
}

// claimInvite returns clickID's invite link of a channel: the one it
// already has, else a free one from the channel's pool, claimed now. id is
// the channel's id or — the form Phase 30.29 published — a bot's, meaning
// its first channel. found is false for no such channel; link is "" when
// the pool is empty — shared is then the channel's shared link.
func (r *Repository) claimInvite(ctx context.Context, id, clickID string) (link, shared string, found bool, err error) {
	var claimed *string
	err = r.db.QueryRow(ctx, `
		WITH ch AS (
		  SELECT id, invite_link FROM telegram_channels WHERE id = $1 OR bot_id = $1
		  ORDER BY created_at, id LIMIT 1
		), existing AS (
		  SELECT l.invite_link FROM telegram_invite_links l, ch WHERE l.channel_id = ch.id AND l.click_id = $2 LIMIT 1
		), claimed AS (
		  UPDATE telegram_invite_links SET click_id = $2, claimed_at = now()
		  WHERE invite_link = (
		    SELECT l.invite_link FROM telegram_invite_links l, ch
		    WHERE l.channel_id = ch.id AND l.click_id IS NULL AND NOT EXISTS (SELECT 1 FROM existing)
		    ORDER BY l.created_at LIMIT 1 FOR UPDATE OF l SKIP LOCKED)
		  RETURNING invite_link
		)
		SELECT ch.invite_link, coalesce((SELECT invite_link FROM existing), (SELECT invite_link FROM claimed))
		FROM ch`, id, clickID).Scan(&shared, &claimed)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("telegram: claiming invite link: %w", err)
	}
	if claimed != nil {
		link = *claimed
	}
	return link, shared, true, nil
}

// channelLink is a channel's shared link (id: as claimInvite's).
func (r *Repository) channelLink(ctx context.Context, id string) (string, bool, error) {
	var link string
	err := r.db.QueryRow(ctx, `
		SELECT invite_link FROM telegram_channels WHERE id = $1 OR bot_id = $1
		ORDER BY created_at, id LIMIT 1`, id).Scan(&link)
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

// invitedClick is the claim of link among c's links (found false: not a
// claimed link of this channel).
func (r *Repository) invitedClick(ctx context.Context, c webhookChannel, link string) (inviteClaim, bool, error) {
	var claim inviteClaim
	err := r.db.QueryRow(ctx, `
		SELECT click_id, claimed_at, joined_at IS NOT NULL FROM telegram_invite_links
		WHERE invite_link = $1 AND channel_id = $2 AND click_id IS NOT NULL`, link, c.ID).Scan(&claim.ClickID, &claim.ClaimedAt, &claim.Joined)
	if errors.Is(err, pgx.ErrNoRows) {
		return inviteClaim{}, false, nil
	}
	if err != nil {
		return inviteClaim{}, false, fmt.Errorf("telegram: reading invite link: %w", err)
	}
	return claim, true, nil
}

// recordInviteJoin marks link's join and reports whether it was not marked
// yet — one TG_JOIN per link, so per click.
func (r *Repository) recordInviteJoin(ctx context.Context, c webhookChannel, link string, at time.Time) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE telegram_invite_links SET joined_at = $3
		WHERE invite_link = $1 AND channel_id = $2 AND click_id IS NOT NULL AND joined_at IS NULL`, link, c.ID, at)
	if err != nil {
		return false, fmt.Errorf("telegram: recording invite join: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// poolChannel is a channel whose pool the worker keeps.
type poolChannel struct {
	ID, BotID, OrganizationID string
	TokenEncrypted            string
	TgChatID                  int64
	Free                      int
}

// poolChannels is every channel and its free links — across organizations
// (the worker's job), or one organization's when orgID is set (tests share
// the dev database).
func (r *Repository) poolChannels(ctx context.Context, orgID string) ([]poolChannel, error) {
	rows, err := r.db.Query(ctx, `
		SELECT c.id, c.bot_id, c.organization_id, b.token_encrypted, c.tg_chat_id,
		       (SELECT count(*) FROM telegram_invite_links l WHERE l.channel_id = c.id AND l.click_id IS NULL)
		FROM telegram_channels c JOIN telegram_bots b ON b.id = c.bot_id
		WHERE $1 = '' OR c.organization_id = $1
		ORDER BY c.id`, orgID)
	if err != nil {
		return nil, fmt.Errorf("telegram: listing channels for invite links: %w", err)
	}
	defer rows.Close()
	var out []poolChannel
	for rows.Next() {
		var c poolChannel
		if err := rows.Scan(&c.ID, &c.BotID, &c.OrganizationID, &c.TokenEncrypted, &c.TgChatID, &c.Free); err != nil {
			return nil, fmt.Errorf("telegram: scanning channel for invite links: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) addInvite(ctx context.Context, c poolChannel, link string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO telegram_invite_links (invite_link, bot_id, channel_id, organization_id) VALUES ($1, $2, $3, $4)
		ON CONFLICT (invite_link) DO NOTHING`, link, c.BotID, c.ID, c.OrganizationID)
	if err != nil {
		return fmt.Errorf("telegram: storing invite link: %w", err)
	}
	return nil
}

// expiredInvites is c's links claimed before cutoff, oldest first.
func (r *Repository) expiredInvites(ctx context.Context, c poolChannel, cutoff time.Time, limit int) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT invite_link FROM telegram_invite_links
		WHERE channel_id = $1 AND click_id IS NOT NULL AND claimed_at < $2
		ORDER BY claimed_at LIMIT $3`, c.ID, cutoff, limit)
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

func (r *Repository) deleteInvite(ctx context.Context, c poolChannel, link string) error {
	if _, err := r.db.Exec(ctx, `DELETE FROM telegram_invite_links WHERE invite_link = $1 AND channel_id = $2`, link, c.ID); err != nil {
		return fmt.Errorf("telegram: deleting invite link: %w", err)
	}
	return nil
}

// InviteRedirect serves GET /tgj/{channelId}?click= on apps/tracker: the
// click's own invite link of the channel, claimed from its pool — one indexed
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
	r.Get("/tgj/{id}", h.serve)
}

func (h *InviteRedirect) serve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	clickID := r.URL.Query().Get("click")
	if !idPattern.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	var link, channel string
	var found bool
	var err error
	if clickPattern.MatchString(clickID) {
		link, channel, found, err = h.repo.claimInvite(ctx, id, clickID)
	} else {
		// Opened without a click (the template itself, a typo): the
		// shared link, and no pool link spent on it.
		channel, found, err = h.repo.channelLink(ctx, id)
	}
	if err != nil {
		h.logger.Error("telegram invite redirect", "id", id, "error", err)
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
			h.logger.Warn("telegram invite pool empty, shared channel link used", "id", id)
			if h.Fallbacks != nil {
				h.Fallbacks.Inc()
			}
		}
	}
	// 302, never cached: every click asks again.
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, link, http.StatusFound)
}

// InvitePool is the worker's job: every channel keeps InvitePoolSize free
// links, and claimed links are revoked after
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

// RunOnce tops up and revokes for every channel. One failing (a revoked
// token, the bot no longer admin, Telegram unreachable) is logged and does
// not stop the others; created counts the links added.
func (p *InvitePool) RunOnce(ctx context.Context) (created, revoked int, err error) {
	channels, err := p.Repo.poolChannels(ctx, p.OrganizationID)
	if err != nil {
		return 0, 0, err
	}
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	for _, c := range channels {
		token, err := integrations.Decrypt(p.Key, c.TokenEncrypted)
		if err != nil {
			p.Logger.Warn("telegram invite pool: cannot decrypt bot token", "bot_id", c.BotID, "error", err)
			continue
		}
		n, err := p.topUp(ctx, c, token)
		created += n
		if err != nil {
			p.Logger.Warn("telegram invite pool: creating links", "bot_id", c.BotID, "channel_id", c.ID, "error", err)
		}
		n, err = p.revoke(ctx, c, token, now().Add(-InviteLinkTTL))
		revoked += n
		if err != nil {
			p.Logger.Warn("telegram invite pool: revoking links", "bot_id", c.BotID, "channel_id", c.ID, "error", err)
		}
		if ctx.Err() != nil {
			return created, revoked, ctx.Err()
		}
	}
	return created, revoked, nil
}

func (p *InvitePool) topUp(ctx context.Context, c poolChannel, token string) (int, error) {
	n := min(InvitePoolSize-c.Free, invitesPerRun)
	created := 0
	for range n {
		link, err := p.API.CreateChatInviteLink(ctx, token, c.TgChatID, inviteLinkName)
		if err != nil {
			return created, err
		}
		if err := p.Repo.addInvite(ctx, c, link); err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}

// revoke revokes c's links claimed before cutoff and forgets them. A link
// Telegram refuses to revoke (already revoked, the channel gone) is
// forgotten too; one it could not be asked about is kept for the next run.
func (p *InvitePool) revoke(ctx context.Context, c poolChannel, token string, cutoff time.Time) (int, error) {
	links, err := p.Repo.expiredInvites(ctx, c, cutoff, revokesPerRun)
	if err != nil {
		return 0, err
	}
	revoked := 0
	for _, link := range links {
		if err := p.API.RevokeChatInviteLink(ctx, token, c.TgChatID, link); err != nil {
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				return revoked, err
			}
		}
		if err := p.Repo.deleteInvite(ctx, c, link); err != nil {
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
