// Package queuestats publishes the state of apps/worker's Postgres queues
// (tech audit §14): open rows by status and how long the oldest due
// row has been waiting. Depth alone misses a stalled queue that happens to
// be small; the age does not.
package queuestats

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

// queue describes one table: its status column and the statuses that are
// still open (postback_deliveries keeps its delivered rows forever, so
// counting every status would scan an ever-growing table).
type queue struct {
	name, statusColumn string
	open               []string
	dueStatuses        []string
}

var queues = []queue{
	{"event_queue", "status", []string{"queued", "processing", "dead"}, []string{"queued"}},
	{"postback_attempt_queue", "status", []string{"queued", "processing", "dead"}, []string{"queued"}},
	{"postback_deliveries", "delivery_status", []string{"queued", "retrying", "processing"}, []string{"queued", "retrying"}},
	{"pixel_deliveries", "delivery_status", []string{"queued", "retrying", "processing"}, []string{"queued", "retrying"}},
	{"push_deliveries", "delivery_status", []string{"queued", "retrying", "processing"}, []string{"queued", "retrying"}},
}

// Poll refreshes the gauges every interval until ctx is done, starting now.
func Poll(ctx context.Context, db *pgxpool.Pool, interval time.Duration, logger *slog.Logger) {
	tick := func() {
		for _, q := range queues {
			if err := refresh(ctx, db, q); err != nil && ctx.Err() == nil {
				logger.Error("polling queue stats", "queue", q.name, "error", err)
			}
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

func refresh(ctx context.Context, db *pgxpool.Pool, q queue) error {
	rows, err := db.Query(ctx, fmt.Sprintf(`SELECT %[1]s, count(*) FROM %[2]s WHERE %[1]s = ANY($1) GROUP BY %[1]s`,
		q.statusColumn, q.name), q.open)
	if err != nil {
		return err
	}
	counts := map[string]float64{}
	for rows.Next() {
		var status string
		var n int64
		if err := rows.Scan(&status, &n); err != nil {
			rows.Close()
			return err
		}
		counts[status] = float64(n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, status := range q.open {
		metrics.QueueRows.WithLabelValues(q.name, status).Set(counts[status])
	}

	var age float64
	if err := db.QueryRow(ctx, fmt.Sprintf(`
		SELECT COALESCE(EXTRACT(EPOCH FROM now() - min(next_attempt_at)), 0)::float8
		FROM %s WHERE %s = ANY($1) AND next_attempt_at <= now()`, q.name, q.statusColumn), q.dueStatuses).Scan(&age); err != nil {
		return err
	}
	metrics.QueueOldestDueAgeSeconds.WithLabelValues(q.name).Set(age)
	return nil
}
