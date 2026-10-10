// Package offercap works out which offers have reached their daily
// conversion cap, so routing can stop sending them traffic.
//
// It runs OUTSIDE the redirect: a Monitor refreshes a snapshot in the
// background and the tracker reads it lock-free (CLAUDE.md #9 — no analytics
// query on the hot path). The decision itself stays in internal/routing,
// which only ever sees the resulting set (routing.RequestContext.CappedOffers).
//
// "Day" is the organization's calendar day in its Settings timezone, so a cap
// resets at the same midnight the marketer sees in the reports. A conversion
// is counted once, on the day its click first reached CPA_HOLD or CPA_ACCEPT.
//
// Failure mode: when a refresh fails the previous snapshot is kept — an
// analytics outage must neither block traffic nor suddenly lift every cap.
package offercap

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"
	_ "time/tzdata" // zone names resolve without a system tzdata (distroless images)

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
)

// DefaultInterval is how often the snapshot is rebuilt. It is also the
// worst-case overshoot window: a cap can be exceeded by whatever converts
// within one interval.
const DefaultInterval = 30 * time.Second

// Limit is one active offer that has a daily cap.
type Limit struct {
	OrganizationID string
	OfferID        string
	Cap            int
	// Since is the start of "today" in the organization's timezone.
	Since time.Time
}

// Source lists the capped offers.
type Source interface {
	Limits(ctx context.Context, now time.Time) ([]Limit, error)
}

// Counter counts new conversions per offer since each target's Since. The
// result is keyed by organizationID + "/" + offerID.
type Counter interface {
	NewConversionsByOffer(ctx context.Context, targets []chstore.OfferConversionTarget) (map[string]uint64, error)
}

type Monitor struct {
	source   Source
	counter  Counter
	logger   *slog.Logger
	interval time.Duration
	snapshot atomic.Pointer[map[string]struct{}]
	// statuses is every capped offer's count, by organization, from the
	// same refresh as snapshot (the offers list shows it).
	statuses atomic.Pointer[map[string][]Status]
}

func NewMonitor(source Source, counter Counter, logger *slog.Logger) *Monitor {
	m := &Monitor{source: source, counter: counter, logger: logger, interval: DefaultInterval}
	empty := map[string]struct{}{}
	m.snapshot.Store(&empty)
	noStatuses := map[string][]Status{}
	m.statuses.Store(&noStatuses)
	return m
}

// Capped returns the current set of capped offer IDs. The map is replaced
// wholesale on each refresh and must not be modified. A nil Monitor has no
// caps, so callers need not special-case "not configured".
//
// One set for every organization, with no organization_id in the key, and
// still tenant-safe (tech audit M1): offer IDs are ULIDs,
// unique across organizations, and routing only ever looks up the offers
// of the campaign it routes, which its own organization's config loaded.
// Another organization's capped offer can never be one of them. Keyed by
// anything not globally unique, this would need the organization in it.
func (m *Monitor) Capped() map[string]struct{} {
	if m == nil {
		return nil
	}
	return *m.snapshot.Load()
}

// Run refreshes immediately and then every interval until ctx is done.
func (m *Monitor) Run(ctx context.Context) {
	t := time.NewTicker(m.interval)
	defer t.Stop()
	for {
		if err := m.Refresh(ctx, time.Now()); err != nil && ctx.Err() == nil {
			m.logger.Warn("offer cap refresh failed, keeping the previous snapshot", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Refresh rebuilds the snapshot as of now.
func (m *Monitor) Refresh(ctx context.Context, now time.Time) error {
	statuses, err := Statuses(ctx, m.source, m.counter, now)
	if err != nil {
		return err
	}
	capped := map[string]struct{}{}
	byOrg := map[string][]Status{}
	for _, s := range statuses {
		if s.Reached() {
			capped[s.OfferID] = struct{}{}
		}
		byOrg[s.OrganizationID] = append(byOrg[s.OrganizationID], s)
	}
	m.snapshot.Store(&capped)
	m.statuses.Store(&byOrg)
	return nil
}

// StatusesFor returns organizationID's capped offers with today's counts as
// of the last refresh. A nil Monitor (no ClickHouse) has none.
func (m *Monitor) StatusesFor(organizationID string) []Status {
	if m == nil {
		return nil
	}
	return (*m.statuses.Load())[organizationID]
}

// Status is one capped offer's count of new conversions today.
type Status struct {
	Limit
	Count uint64
}

// Reached reports whether the offer has used up its daily cap.
func (s Status) Reached() bool { return s.Count >= uint64(s.Cap) }

// Statuses counts today's conversions for every active offer with a cap.
// Routing (Monitor) and alerts (internal/alert) both read caps through it.
func Statuses(ctx context.Context, source Source, counter Counter, now time.Time) ([]Status, error) {
	limits, err := source.Limits(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("listing capped offers: %w", err)
	}
	if len(limits) == 0 {
		return nil, nil
	}
	targets := make([]chstore.OfferConversionTarget, len(limits))
	for i, l := range limits {
		targets[i] = chstore.OfferConversionTarget{OrganizationID: l.OrganizationID, OfferID: l.OfferID, Since: l.Since}
	}
	counts, err := counter.NewConversionsByOffer(ctx, targets)
	if err != nil {
		return nil, fmt.Errorf("counting conversions: %w", err)
	}
	out := make([]Status, len(limits))
	for i, l := range limits {
		out[i] = Status{Limit: l, Count: counts[l.OrganizationID+"/"+l.OfferID]}
	}
	return out, nil
}

// StartOfDay is midnight of now's calendar day in the named zone (UTC when
// the name is empty or unknown).
func StartOfDay(now time.Time, timezone string) time.Time {
	loc, err := time.LoadLocation(timezone)
	if err != nil || timezone == "" {
		loc = time.UTC
	}
	y, mo, d := now.In(loc).Date()
	return time.Date(y, mo, d, 0, 0, 0, 0, loc)
}

// PostgresSource reads active offers with a cap, joined to their
// organization's timezone. Offers are tenant-scoped rows: each Limit carries
// its organization_id, and the counter filters on it.
type PostgresSource struct{ db *pgxpool.Pool }

func NewPostgresSource(db *pgxpool.Pool) *PostgresSource { return &PostgresSource{db: db} }

func (s *PostgresSource) Limits(ctx context.Context, now time.Time) ([]Limit, error) {
	rows, err := s.db.Query(ctx, `
		SELECT o.organization_id, o.id, o.cap, org.timezone
		FROM offers o
		JOIN organizations org ON org.id = o.organization_id
		WHERE o.cap IS NOT NULL AND o.status = 'active'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Limit
	for rows.Next() {
		var l Limit
		var tz string
		if err := rows.Scan(&l.OrganizationID, &l.OfferID, &l.Cap, &tz); err != nil {
			return nil, err
		}
		l.Since = StartOfDay(now, tz)
		out = append(out, l)
	}
	return out, rows.Err()
}
