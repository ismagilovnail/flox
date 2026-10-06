// Package reconcile checks that ClickHouse holds exactly the conversions
// Postgres recorded (audit 2026-10-06, §12/§17). Postgres `postbacks` is the
// source of truth; ClickHouse `conversion_events` is a copy that travels
// through the outbox, the worker's queue and its retries — every path this
// audit fixed (a dropped event, a dead-lettered batch, a retried write whose
// commit had in fact landed) shows up here as a difference.
//
// It compares, per (organization, UTC day, status), the number of events
// and their USD sum, the way reports would: an accept or redeposit counts
// its usd_value, a decline its reversal negated (conversion.PostgresStore).
// Rows are bucketed on postbacks.event_at — the analytics event's own time —
// so both sides agree on the day. The newest Settle is left out: those
// events may still be on their way. Differences are published (metrics,
// log) and, unless ReportOnly, repaired event by event (Repair).
package reconcile

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

// Bucket is one compared slice.
type Bucket struct {
	OrganizationID string
	Day            string // YYYY-MM-DD, UTC
	Status         string
}

// Totals is one side of a bucket.
type Totals struct {
	Count int64
	USD   float64
}

// Mismatch is a bucket where the two stores disagree.
type Mismatch struct {
	Bucket
	Postgres, ClickHouse Totals
}

func (m Mismatch) countDiffers() bool { return m.Postgres.Count != m.ClickHouse.Count }

// usdDiffers allows for rounding: Postgres keeps USD as numeric(14,4), the
// event carries the unrounded float.
func (m Mismatch) usdDiffers() bool {
	tolerance := 0.01 + 0.0001*float64(max(m.Postgres.Count, m.ClickHouse.Count))
	return math.Abs(m.Postgres.USD-m.ClickHouse.USD) > tolerance
}

type Reconciler struct {
	pg     *pgxpool.Pool
	ch     driver.Conn
	logger *slog.Logger
	now    func() time.Time

	conversions *conversion.PostgresStore
	events      *chstore.EventStore
	clicks      *chstore.ClickResolver
	// repair: RunOnce also repairs what it finds (Repair); off, it only
	// reports.
	repair bool
}

func New(pg *pgxpool.Pool, ch driver.Conn, logger *slog.Logger) *Reconciler {
	return &Reconciler{
		pg: pg, ch: ch, logger: logger, now: time.Now,
		conversions: conversion.NewPostgresStore(pg),
		events:      chstore.NewEventStore(ch),
		clicks:      chstore.NewClickResolver(ch),
		repair:      true,
	}
}

// ReportOnly turns automatic repair off.
func (r *Reconciler) ReportOnly() *Reconciler {
	r.repair = false
	return r
}

// Compare returns every bucket in [from, to) where the stores disagree, in
// a stable order.
func (r *Reconciler) Compare(ctx context.Context, from, to time.Time) ([]Mismatch, error) {
	pgTotals, err := r.postgres(ctx, from, to)
	if err != nil {
		return nil, err
	}
	chTotals, err := r.clickhouse(ctx, from, to)
	if err != nil {
		return nil, err
	}
	keys := map[Bucket]bool{}
	for k := range pgTotals {
		keys[k] = true
	}
	for k := range chTotals {
		keys[k] = true
	}
	var out []Mismatch
	for k := range keys {
		m := Mismatch{Bucket: k, Postgres: pgTotals[k], ClickHouse: chTotals[k]}
		if m.countDiffers() || m.usdDiffers() {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Bucket, out[j].Bucket
		if a.OrganizationID != b.OrganizationID {
			return a.OrganizationID < b.OrganizationID
		}
		if a.Day != b.Day {
			return a.Day < b.Day
		}
		return a.Status < b.Status
	})
	return out, nil
}

func (r *Reconciler) postgres(ctx context.Context, from, to time.Time) (map[Bucket]Totals, error) {
	rows, err := r.pg.Query(ctx, `
		SELECT organization_id, to_char(event_at AT TIME ZONE 'UTC', 'YYYY-MM-DD'), status,
		       count(*),
		       COALESCE(sum(CASE WHEN status IN ('CPA_DECLINE', 'CPA_TRASH') THEN -reversal_usd_value
		                         ELSE usd_value END), 0)::float8
		FROM postbacks
		WHERE result = 'success' AND direction = 'incoming' AND event_at >= $1 AND event_at < $2
		GROUP BY 1, 2, 3`, from, to)
	if err != nil {
		return nil, fmt.Errorf("reconcile: reading postbacks: %w", err)
	}
	defer rows.Close()
	out := map[Bucket]Totals{}
	for rows.Next() {
		var b Bucket
		var t Totals
		if err := rows.Scan(&b.OrganizationID, &b.Day, &b.Status, &t.Count, &t.USD); err != nil {
			return nil, fmt.Errorf("reconcile: scanning postbacks: %w", err)
		}
		out[b] = t
	}
	return out, rows.Err()
}

func (r *Reconciler) clickhouse(ctx context.Context, from, to time.Time) (map[Bucket]Totals, error) {
	rows, err := r.ch.Query(ctx, `
		SELECT organization_id, toString(toDate(event_at)), type,
		       toInt64(count()), sum(usd_value * has_usd_value)
		FROM conversion_events
		WHERE event_at >= ? AND event_at < ?
		GROUP BY 1, 2, 3`, from, to)
	if err != nil {
		return nil, fmt.Errorf("reconcile: reading conversion_events: %w", err)
	}
	defer rows.Close()
	out := map[Bucket]Totals{}
	for rows.Next() {
		var b Bucket
		var t Totals
		if err := rows.Scan(&b.OrganizationID, &b.Day, &b.Status, &t.Count, &t.USD); err != nil {
			return nil, fmt.Errorf("reconcile: scanning conversion_events: %w", err)
		}
		out[b] = t
	}
	return out, rows.Err()
}

// Window is what one scheduled run compares: the last Lookback, minus the
// newest Settle still in flight.
type Window struct {
	Lookback, Settle time.Duration
}

// maxLoggedMismatches bounds the log lines per run; the metric carries the
// full count.
const maxLoggedMismatches = 20

// RunOnce compares the window ending Settle ago, publishes the result and
// logs the differences.
func (r *Reconciler) RunOnce(ctx context.Context, w Window) ([]Mismatch, error) {
	to := r.now().UTC().Add(-w.Settle)
	from := to.Add(-w.Lookback)
	mismatches, err := r.Compare(ctx, from, to)
	if err != nil {
		metrics.ReconciliationRunsTotal.WithLabelValues("error").Inc()
		return nil, err
	}
	var counts, usd int
	for i, m := range mismatches {
		if m.countDiffers() {
			counts++
		}
		if m.usdDiffers() {
			usd++
		}
		if i < maxLoggedMismatches {
			r.logger.Warn("conversions differ between postgres and clickhouse",
				"organization_id", m.OrganizationID, "day", m.Day, "status", m.Status,
				"postgres_count", m.Postgres.Count, "clickhouse_count", m.ClickHouse.Count,
				"postgres_usd", m.Postgres.USD, "clickhouse_usd", m.ClickHouse.USD)
		}
	}
	if r.repair && len(mismatches) > 0 {
		repaired, err := r.Repair(ctx, mismatches, from, to)
		if err != nil {
			metrics.ReconciliationRunsTotal.WithLabelValues("error").Inc()
			return mismatches, fmt.Errorf("reconcile: repairing: %w", err)
		}
		r.logger.Info("postgres/clickhouse repair done", "reemitted", repaired.Reemitted, "deleted", repaired.Deleted,
			"unmatched", repaired.Unmatched, "skipped", repaired.Skipped)
	}
	metrics.ReconciliationMismatchedBuckets.WithLabelValues("count").Set(float64(counts))
	metrics.ReconciliationMismatchedBuckets.WithLabelValues("usd").Set(float64(usd))
	metrics.ReconciliationLastSuccessSeconds.SetToCurrentTime()
	metrics.ReconciliationRunsTotal.WithLabelValues("ok").Inc()
	r.logger.Info("postgres/clickhouse reconciliation done",
		"from", from, "to", to, "mismatched_buckets", len(mismatches))
	return mismatches, nil
}

// RunLoop reconciles now and then every interval until ctx is done.
func (r *Reconciler) RunLoop(ctx context.Context, interval time.Duration, w Window) {
	tick := func() {
		if _, err := r.RunOnce(ctx, w); err != nil && ctx.Err() == nil {
			r.logger.Error("postgres/clickhouse reconciliation failed", "error", err)
		}
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
