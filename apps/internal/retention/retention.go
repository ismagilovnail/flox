// Package retention deletes what FLOX keeps only for a while (tech audit
// 2026-10-06, CH3/DB2; docs/retention.md).
//
// Postgres: postbacks rows that were not recorded as a conversion —
// result duplicate, error or ignored — are a log of what a network sent;
// nothing reads them back (every query and reference works on success
// rows), and replays from a chatty network make them most of the table.
// They go after RETENTION_FAILED_POSTBACKS_DAYS. Success rows are the
// source of truth for revenue and dedup and are never deleted.
//
// The table is not partitioned for this: a unique index on a partitioned
// table must include the partition key, and the dedup key (organization,
// click_id, status, event_ref) has to stay one global constraint
// (CLAUDE.md #3). A batched DELETE over a partial index does the same job.
//
// Sign-in state past its expiry: see ExpiredAuth (auth.go, DB5).
//
// pixel_deliveries rows that are finished (sent or dead-lettered) carry the
// visitor's IP and user agent the platform matched on; they go after
// RETENTION_PIXEL_DELIVERIES_DAYS (FinishedDeliveries; push_deliveries the
// same under RETENTION_PUSH_DELIVERIES_DAYS).
//
// ClickHouse: see chstore.SetPostbackLogTTL.
//
// This is maintenance across every organization, like the queue reapers:
// no organization_id filter, nothing tenant-visible changes but the age of
// a log.
package retention

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

const (
	// Interval: the job runs hourly; a row lives at most this much past
	// its age.
	Interval = time.Hour
	// DefaultBatch bounds one DELETE: short transactions, no long lock on
	// a hot table, WAL in small pieces.
	DefaultBatch = 5000
)

// FailedPostbacks deletes postbacks rows that are not successes once they
// are older than Age.
type FailedPostbacks struct {
	DB     *pgxpool.Pool
	Age    time.Duration
	Batch  int // 0: DefaultBatch
	Logger *slog.Logger
}

// RunOnce deletes every expired row, batch by batch, and returns how many.
func (f *FailedPostbacks) RunOnce(ctx context.Context) (int64, error) {
	batch := f.Batch
	if batch <= 0 {
		batch = DefaultBatch
	}
	var total int64
	for {
		tag, err := f.DB.Exec(ctx, `
			DELETE FROM postbacks WHERE id IN (
				SELECT id FROM postbacks
				WHERE result <> 'success' AND created_at < now() - make_interval(secs => $1)
				LIMIT $2)`,
			f.Age.Seconds(), batch)
		if err != nil {
			return total, fmt.Errorf("retention: deleting failed postbacks: %w", err)
		}
		n := tag.RowsAffected()
		total += n
		metrics.RetentionDeletedTotal.WithLabelValues("postbacks").Add(float64(n))
		if n < int64(batch) {
			return total, nil
		}
	}
}

// RunLoop runs RunOnce every interval until ctx is done.
func (f *FailedPostbacks) RunLoop(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		n, err := f.RunOnce(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			f.Logger.Warn("retention run failed", "error", err)
		case n > 0:
			f.Logger.Info("expired postbacks deleted", "rows", n, "older_than", f.Age.String())
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Delivery queues FinishedDeliveries expires: tables shaped like
// pixel_deliveries (00069) and push_deliveries (00070).
const (
	PixelDeliveriesTable = "pixel_deliveries"
	PushDeliveriesTable  = "push_deliveries"
)

// FinishedDeliveries deletes a delivery queue's finished rows (sent or
// dead-lettered) once they are older than Age. Rows still to send are
// never deleted. Table is one of the constants above, never input.
type FinishedDeliveries struct {
	DB     *pgxpool.Pool
	Table  string
	Age    time.Duration
	Batch  int // 0: DefaultBatch
	Logger *slog.Logger
}

// RunOnce deletes every expired row, batch by batch, and returns how many.
func (p *FinishedDeliveries) RunOnce(ctx context.Context) (int64, error) {
	batch := p.Batch
	if batch <= 0 {
		batch = DefaultBatch
	}
	var total int64
	for {
		tag, err := p.DB.Exec(ctx, `
			DELETE FROM `+p.Table+` WHERE id IN (
				SELECT id FROM `+p.Table+`
				WHERE delivery_status IN ('success', 'dead') AND created_at < now() - make_interval(secs => $1)
				LIMIT $2)`,
			p.Age.Seconds(), batch)
		if err != nil {
			return total, fmt.Errorf("retention: deleting %s: %w", p.Table, err)
		}
		n := tag.RowsAffected()
		total += n
		metrics.RetentionDeletedTotal.WithLabelValues(p.Table).Add(float64(n))
		if n < int64(batch) {
			return total, nil
		}
	}
}

// RunLoop runs RunOnce every interval until ctx is done.
func (p *FinishedDeliveries) RunLoop(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		n, err := p.RunOnce(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			p.Logger.Warn("delivery retention run failed", "table", p.Table, "error", err)
		case n > 0:
			p.Logger.Info("expired deliveries deleted", "table", p.Table, "rows", n, "older_than", p.Age.String())
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
