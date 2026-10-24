// Package fx keeps fx_rates filled (§50-FX, CLAUDE.md #7) and repairs the
// USD values that were stored while a rate was missing.
//
// Without rates every non-USD payout and spend has a NULL USD value and
// drops out of Revenue, Cost and ROI (tech audit AN1). apps/worker runs a
// Syncer on a timer:
//
//  1. pull reference rates from a Provider (internal/fx/ecb by default —
//     behind an interface, CLAUDE.md #11) and upsert them;
//  2. fill postbacks.usd_value and cost_entries.amount_usd that are still
//     NULL and now have a rate.
//
// A rate is looked up as "the latest published on or before the event date,
// at most MaxStaleDays old" (conversion.PostgresFX uses the same rule):
// reference rates are not published on weekends and holidays, and an event
// that happens before today's fixing uses yesterday's — the rate in force
// at the time, never a later one.
package fx

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

// MaxStaleDays is how far back a lookup may reach for the latest rate:
// covers a long holiday weekend, not a provider that stopped publishing.
const MaxStaleDays = 7

// Rate is one currency's value in USD on one date.
type Rate struct {
	Currency  string    // ISO 4217, upper case
	Date      time.Time // UTC midnight
	RateToUSD float64   // 1 unit of Currency = RateToUSD USD
}

// Provider fetches reference rates published on or after since.
type Provider interface {
	Rates(ctx context.Context, since time.Time) ([]Rate, error)
}

// Store is fx_rates plus the USD repair over the tables that need it.
type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

// LatestDate is the newest rate_date on file; ok=false when the table is
// empty.
func (s *Store) LatestDate(ctx context.Context) (time.Time, bool, error) {
	var d *time.Time
	if err := s.db.QueryRow(ctx, `SELECT max(rate_date) FROM fx_rates`).Scan(&d); err != nil {
		return time.Time{}, false, fmt.Errorf("fx: reading latest rate date: %w", err)
	}
	if d == nil {
		return time.Time{}, false, nil
	}
	return d.UTC(), true, nil
}

// OldestMissingDate is the earliest event date whose USD value is still
// missing, so the first sync fetches far enough back to repair it.
func (s *Store) OldestMissingDate(ctx context.Context) (time.Time, bool, error) {
	var d *time.Time
	err := s.db.QueryRow(ctx, `
		SELECT least(
		  (SELECT min((event_at AT TIME ZONE 'UTC')::date) FROM postbacks
		   WHERE usd_value IS NULL AND revenue IS NOT NULL AND currency <> ''),
		  (SELECT min(entry_date) FROM cost_entries WHERE amount_usd IS NULL))`,
	).Scan(&d)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("fx: finding oldest missing usd value: %w", err)
	}
	if d == nil {
		return time.Time{}, false, nil
	}
	return d.UTC(), true, nil
}

// Upsert stores rates. A re-published rate replaces the stored one.
func (s *Store) Upsert(ctx context.Context, rates []Rate) error {
	if len(rates) == 0 {
		return nil
	}
	currencies := make([]string, len(rates))
	dates := make([]time.Time, len(rates))
	values := make([]float64, len(rates))
	for i, r := range rates {
		currencies[i], dates[i], values[i] = r.Currency, r.Date, r.RateToUSD
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO fx_rates (currency, rate_date, rate_to_usd)
		SELECT * FROM unnest($1::text[], $2::date[], $3::numeric[])
		ON CONFLICT (currency, rate_date) DO UPDATE SET rate_to_usd = EXCLUDED.rate_to_usd`,
		currencies, dates, values,
	)
	if err != nil {
		return fmt.Errorf("fx: upserting %d rates: %w", len(rates), err)
	}
	return nil
}

// Repaired counts the rows RepairUSD filled in.
type Repaired struct {
	Postbacks   int64
	CostEntries int64
	// Reversals counts declines whose chargeback got its USD value from
	// the accept they reverse, now that the accept has one.
	Reversals int64
	// Buckets are the recorded conversions whose USD value changed, by
	// (organization, UTC day, status) — what the ClickHouse copy must be
	// rewritten for (Syncer.OnRepaired).
	Buckets []Bucket
}

// Bucket is one (organization, UTC day of event_at, status) slice of
// recorded conversions — the reconciliation's bucket.
type Bucket struct {
	OrganizationID string
	Day            string // YYYY-MM-DD
	Status         string
}

// RepairUSD fills USD values that are NULL and now have a rate, using the
// same lookup rule as conversion.PostgresFX — by the conversion's event
// time (event_at; the network's event_time when it sent one), as the live
// path does. Rows in a currency no provider publishes stay NULL — "no rate"
// is never turned into zero.
//
// A decline that reversed an accept with no USD value yet has a NULL
// reversal_usd_value; once the accept is repaired the reversal takes the
// same value, so the two still net to zero.
//
// Costs are read from Postgres only, so ROI is correct as soon as this
// runs. Conversions are also summed from their ClickHouse copy, which this
// does not touch: Buckets says which ones to rewrite.
func (s *Store) RepairUSD(ctx context.Context) (Repaired, error) {
	var out Repaired
	buckets := map[Bucket]bool{}
	collect := func(rows pgx.Rows) (int64, error) {
		defer rows.Close()
		var n int64
		for rows.Next() {
			var b Bucket
			var counts bool
			if err := rows.Scan(&b.OrganizationID, &b.Day, &b.Status, &counts); err != nil {
				return n, err
			}
			n++
			if counts {
				buckets[b] = true
			}
		}
		return n, rows.Err()
	}
	// counts: only recorded incoming conversions have a ClickHouse copy.
	const returning = `
		RETURNING p.organization_id, to_char(p.event_at AT TIME ZONE 'UTC', 'YYYY-MM-DD'), p.status,
		          p.result = 'success' AND p.direction = 'incoming'`

	rows, err := s.db.Query(ctx, `
		UPDATE postbacks p
		SET usd_value = round(p.revenue * x.rate_to_usd, 4)
		FROM (
		  SELECT m.id, CASE WHEN upper(m.currency) = 'USD' THEN 1::numeric ELSE (
		    SELECT rate_to_usd FROM fx_rates
		    WHERE currency = upper(m.currency)
		      AND rate_date <= (m.event_at AT TIME ZONE 'UTC')::date
		      AND rate_date >  (m.event_at AT TIME ZONE 'UTC')::date - $1::int
		    ORDER BY rate_date DESC LIMIT 1) END AS rate_to_usd
		  FROM postbacks m
		  WHERE m.usd_value IS NULL AND m.revenue IS NOT NULL AND m.currency <> ''
		) x
		WHERE p.id = x.id AND x.rate_to_usd IS NOT NULL`+returning,
		MaxStaleDays,
	)
	if err != nil {
		return out, fmt.Errorf("fx: repairing postbacks usd_value: %w", err)
	}
	if out.Postbacks, err = collect(rows); err != nil {
		return out, fmt.Errorf("fx: repairing postbacks usd_value: %w", err)
	}

	// The purchase a refund reversed: the row it points at
	// (reverses_postback_id — a repeat purchase, or part of one, audit
	// UNI-4), the same share of its USD value; a reversal from before that
	// column, the click's recorded CPA_ACCEPT (one per click: the dedup
	// key) — the value the record path copied.
	rows, err = s.db.Query(ctx, `
		UPDATE postbacks p
		SET reversal_usd_value = CASE
		      WHEN a.revenue IS NULL OR a.revenue = 0 OR p.reversal_revenue = a.revenue THEN a.usd_value
		      ELSE round(a.usd_value * p.reversal_revenue / a.revenue, 4) END
		FROM postbacks a
		WHERE p.reversal_revenue IS NOT NULL AND p.reversal_usd_value IS NULL
		  AND p.result = 'success' AND p.status IN ('CPA_DECLINE', 'CPA_TRASH')
		  AND a.organization_id = p.organization_id AND a.click_id = p.click_id
		  AND (a.id = p.reverses_postback_id
		       OR (p.reverses_postback_id IS NULL AND a.status = 'CPA_ACCEPT'))
		  AND a.result = 'success' AND a.direction = 'incoming'
		  AND a.usd_value IS NOT NULL`+returning)
	if err != nil {
		return out, fmt.Errorf("fx: repairing postbacks reversal_usd_value: %w", err)
	}
	if out.Reversals, err = collect(rows); err != nil {
		return out, fmt.Errorf("fx: repairing postbacks reversal_usd_value: %w", err)
	}

	tag, err := s.db.Exec(ctx, `
		UPDATE cost_entries c
		SET amount_usd = round(c.amount * x.rate_to_usd, 4)
		FROM (
		  SELECT m.id, CASE WHEN upper(m.currency) = 'USD' THEN 1::numeric ELSE (
		    SELECT rate_to_usd FROM fx_rates
		    WHERE currency = upper(m.currency)
		      AND rate_date <= m.entry_date
		      AND rate_date >  m.entry_date - $1::int
		    ORDER BY rate_date DESC LIMIT 1) END AS rate_to_usd
		  FROM cost_entries m
		  WHERE m.amount_usd IS NULL
		) x
		WHERE c.id = x.id AND x.rate_to_usd IS NOT NULL`,
		MaxStaleDays,
	)
	if err != nil {
		return out, fmt.Errorf("fx: repairing cost_entries amount_usd: %w", err)
	}
	out.CostEntries = tag.RowsAffected()

	for b := range buckets {
		out.Buckets = append(out.Buckets, b)
	}
	sort.Slice(out.Buckets, func(i, j int) bool {
		a, b := out.Buckets[i], out.Buckets[j]
		if a.Day != b.Day {
			return a.Day < b.Day
		}
		if a.OrganizationID != b.OrganizationID {
			return a.OrganizationID < b.OrganizationID
		}
		return a.Status < b.Status
	})
	return out, nil
}

// SyncStore is the part of *Store a Syncer uses.
type SyncStore interface {
	LatestDate(ctx context.Context) (time.Time, bool, error)
	OldestMissingDate(ctx context.Context) (time.Time, bool, error)
	Upsert(ctx context.Context, rates []Rate) error
	RepairUSD(ctx context.Context) (Repaired, error)
}

var _ SyncStore = (*Store)(nil)

// Syncer pulls rates and repairs USD values; apps/worker runs it on a timer.
type Syncer struct {
	provider   Provider
	store      SyncStore
	logger     *slog.Logger
	now        func() time.Time
	onRepaired func(ctx context.Context, buckets []Bucket) error
}

// OnRepaired sets what runs after a repair changed recorded conversions'
// USD values: apps/worker rewrites their ClickHouse copy
// (reconcile.Reconciler.RepairBuckets), so reports pick the values up on
// any date, not only within the hourly reconciliation's last 3 days.
func (s *Syncer) OnRepaired(fn func(ctx context.Context, buckets []Bucket) error) *Syncer {
	s.onRepaired = fn
	return s
}

func NewSyncer(provider Provider, store SyncStore, logger *slog.Logger) *Syncer {
	return &Syncer{provider: provider, store: store, logger: logger, now: time.Now}
}

// WithClock replaces the syncer's clock (tests).
func (s *Syncer) WithClock(now func() time.Time) *Syncer {
	s.now = now
	return s
}

// initialLookbackDays is how far back the first sync reaches when nothing
// older needs repairing.
const initialLookbackDays = 90

// RunOnce fetches what is new since the latest stored rate (re-reading a
// few days, in case a rate was corrected), reaching further back when an
// older event still lacks a USD value, then repairs.
func (s *Syncer) RunOnce(ctx context.Context) error {
	today := s.now().UTC().Truncate(24 * time.Hour)
	since := today.AddDate(0, 0, -initialLookbackDays)
	if latest, ok, err := s.store.LatestDate(ctx); err != nil {
		return err
	} else if ok {
		since = latest.AddDate(0, 0, -MaxStaleDays)
	}
	if oldest, ok, err := s.store.OldestMissingDate(ctx); err != nil {
		return err
	} else if ok && oldest.AddDate(0, 0, -MaxStaleDays).Before(since) {
		since = oldest.AddDate(0, 0, -MaxStaleDays)
	}

	rates, err := s.provider.Rates(ctx, since)
	if err != nil {
		return fmt.Errorf("fx: fetching rates since %s: %w", since.Format(time.DateOnly), err)
	}
	if err := s.store.Upsert(ctx, rates); err != nil {
		return err
	}
	repaired, err := s.store.RepairUSD(ctx)
	if err != nil {
		return err
	}
	if s.onRepaired != nil && len(repaired.Buckets) > 0 {
		// Postgres is already repaired; a failure here leaves the
		// ClickHouse copy to the hourly reconciliation (last 3 days) or
		// make reconcile — the next RepairUSD finds nothing to return.
		if err := s.onRepaired(ctx, repaired.Buckets); err != nil {
			return fmt.Errorf("fx: rewriting %d repaired conversion buckets in clickhouse: %w", len(repaired.Buckets), err)
		}
	}

	if latest, ok, err := s.store.LatestDate(ctx); err == nil && ok {
		metrics.FXLatestRateAgeSeconds.Set(s.now().Sub(latest).Seconds())
	}
	s.logger.Info("fx rates synced",
		"since", since.Format(time.DateOnly), "rates", len(rates),
		"repaired_postbacks", repaired.Postbacks, "repaired_reversals", repaired.Reversals,
		"repaired_cost_entries", repaired.CostEntries, "rewritten_buckets", len(repaired.Buckets))
	return nil
}

// RunLoop syncs immediately, then every interval, until ctx is done.
func (s *Syncer) RunLoop(ctx context.Context, interval time.Duration) {
	tick := func() {
		if err := s.RunOnce(ctx); err != nil {
			metrics.FXSyncTotal.WithLabelValues("error").Inc()
			if ctx.Err() == nil {
				s.logger.Error("fx rate sync failed", "error", err)
			}
			return
		}
		metrics.FXSyncTotal.WithLabelValues("ok").Inc()
	}
	tick()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick()
		}
	}
}

// Combine is a Provider over several, each the source of its own
// currencies (internal/fx/ecb for what the ECB publishes, internal/fx/cbr
// for the rouble and CIS currencies — audit DATA-8). One failing does not
// hold back the others' rates: the failure is logged and the next sync
// fetches its days again; only when every provider fails is it an error.
func Combine(logger *slog.Logger, providers map[string]Provider) Provider {
	return &combined{logger: logger, providers: providers}
}

type combined struct {
	logger    *slog.Logger
	providers map[string]Provider
}

func (c *combined) Rates(ctx context.Context, since time.Time) ([]Rate, error) {
	names := make([]string, 0, len(c.providers))
	for name := range c.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []Rate
	var failed []string
	var last error
	for _, name := range names {
		rates, err := c.providers[name].Rates(ctx, since)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			c.logger.Warn("fx provider failed; the others' rates are still stored", "provider", name, "error", err)
			failed, last = append(failed, name), err
			continue
		}
		out = append(out, rates...)
	}
	if len(failed) == len(names) {
		return nil, fmt.Errorf("fx: every provider failed (%s): %w", strings.Join(failed, ", "), last)
	}
	return out, nil
}
