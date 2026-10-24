// Package clicklimit makes the tracker's per-IP click limit visible (audit
// REL-2). A visitor over the limit is sent on unrecorded (audit SEC-4),
// so no report counts them; Counter adds them up in apps/tracker per
// campaign and UTC day and writes the totals to ip_limited_clicks, and
// Handler tells the campaign page how many there were.
package clicklimit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

const (
	// FlushInterval is how often apps/tracker writes its counts.
	FlushInterval = 30 * time.Second
	// keepDays is how long a day's count is kept.
	keepDays = 90
	// WindowDays is the period the campaign page's warning looks at.
	WindowDays = 7
)

type key struct {
	org, campaign string
	day           time.Time
}

// Counter adds up limited clicks in memory — no database call on the
// redirect (CLAUDE.md #9) — until Flush. A nil Counter counts nothing.
type Counter struct {
	DB     *pgxpool.Pool
	Logger *slog.Logger

	mu     sync.Mutex
	counts map[key]int64
	pruned time.Time
}

// Add counts one click of a campaign over the limit at t.
func (c *Counter) Add(orgID, campaignID string, t time.Time) {
	if c == nil || orgID == "" || campaignID == "" {
		return
	}
	k := key{org: orgID, campaign: campaignID, day: t.UTC().Truncate(24 * time.Hour)}
	c.mu.Lock()
	if c.counts == nil {
		c.counts = map[key]int64{}
	}
	c.counts[k]++
	c.mu.Unlock()
}

// Flush writes the counts so far, added to what is stored. On failure they
// are kept for the next Flush.
func (c *Counter) Flush(ctx context.Context) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	batch := c.counts
	c.counts = nil
	c.mu.Unlock()
	if len(batch) == 0 {
		return nil
	}
	orgs, campaigns, days, clicks := make([]string, 0, len(batch)), make([]string, 0, len(batch)), make([]time.Time, 0, len(batch)), make([]int64, 0, len(batch))
	for k, n := range batch {
		orgs, campaigns, days, clicks = append(orgs, k.org), append(campaigns, k.campaign), append(days, k.day), append(clicks, n)
	}
	// Only organizations that still exist: one deleted meanwhile must not
	// fail the others' counts on the foreign key.
	_, err := c.DB.Exec(ctx, `
		INSERT INTO ip_limited_clicks (organization_id, campaign_id, day, clicks)
		SELECT b.org, b.campaign, b.day, b.clicks
		FROM unnest($1::text[], $2::text[], $3::date[], $4::bigint[]) AS b(org, campaign, day, clicks)
		WHERE EXISTS (SELECT 1 FROM organizations o WHERE o.id = b.org)
		ON CONFLICT (organization_id, campaign_id, day)
		DO UPDATE SET clicks = ip_limited_clicks.clicks + EXCLUDED.clicks, updated_at = now()`,
		orgs, campaigns, days, clicks)
	if err != nil {
		c.mu.Lock()
		if c.counts == nil {
			c.counts = map[key]int64{}
		}
		for k, n := range batch {
			c.counts[k] += n
		}
		c.mu.Unlock()
		return fmt.Errorf("clicklimit: writing %d counts: %w", len(batch), err)
	}
	return nil
}

// prune deletes counts older than keepDays, at most once an hour.
func (c *Counter) prune(ctx context.Context, now time.Time) error {
	if now.Sub(c.pruned) < time.Hour {
		return nil
	}
	c.pruned = now
	_, err := c.DB.Exec(ctx, `DELETE FROM ip_limited_clicks WHERE day < current_date - $1::int`, keepDays)
	if err != nil {
		return fmt.Errorf("clicklimit: deleting old counts: %w", err)
	}
	return nil
}

// RunLoop flushes every interval until ctx ends, then once more, so a
// clean shutdown keeps what it counted.
func (c *Counter) RunLoop(ctx context.Context, interval time.Duration) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			final, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := c.Flush(final); err != nil {
				c.Logger.Error("limited clicks: final flush", "error", err)
			}
			cancel()
			return
		case now := <-tick.C:
			if err := c.Flush(ctx); err != nil && ctx.Err() == nil {
				c.Logger.Error("limited clicks: flush", "error", err)
			}
			if err := c.prune(ctx, now); err != nil && ctx.Err() == nil {
				c.Logger.Error("limited clicks: prune", "error", err)
			}
		}
	}
}

// Summary is a campaign's limited clicks over the last WindowDays UTC
// days, today included.
type Summary struct {
	WindowDays int        `json:"windowDays"`
	Clicks     int64      `json:"clicks"`
	LastDay    *time.Time `json:"lastDay"`
}

// Diagnostics is the campaign page's warnings.
type Diagnostics struct {
	IPLimited Summary `json:"ipLimited"`
}

// Handler serves GET /campaigns/{campaignId}/diagnostics.
type Handler struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

func NewHandler(db *pgxpool.Pool, logger *slog.Logger) *Handler {
	return &Handler{db: db, logger: logger}
}

func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.diagnostics)
}

// Read is a campaign's diagnostics; apierror.NotFound for a campaign not
// in the organization.
func Read(ctx context.Context, db *pgxpool.Pool, orgID, campaignID string) (Diagnostics, error) {
	var exists bool
	if err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM campaigns WHERE organization_id = $1 AND id = $2)`,
		orgID, campaignID).Scan(&exists); err != nil {
		return Diagnostics{}, fmt.Errorf("clicklimit: finding the campaign: %w", err)
	}
	if !exists {
		return Diagnostics{}, apierror.NotFound("campaign not found")
	}
	s := Summary{WindowDays: WindowDays}
	err := db.QueryRow(ctx, `
		SELECT coalesce(sum(clicks), 0), max(day) FROM ip_limited_clicks
		WHERE organization_id = $1 AND campaign_id = $2 AND day > (now() AT TIME ZONE 'UTC')::date - $3::int`,
		orgID, campaignID, WindowDays).Scan(&s.Clicks, &s.LastDay)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Diagnostics{}, fmt.Errorf("clicklimit: reading counts: %w", err)
	}
	return Diagnostics{IPLimited: s}, nil
}

func (h *Handler) diagnostics(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	d, err := Read(r.Context(), h.db, orgID, chi.URLParam(r, "campaignId"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(d)
}
