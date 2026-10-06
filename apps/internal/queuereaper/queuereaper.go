// Package queuereaper returns abandoned claims to apps/worker's Postgres
// queues (event_queue, postback_attempt_queue, postback_deliveries).
//
// Every queue claims rows by flipping them to 'processing'. A worker that
// dies between the claim and its delete/requeue (SIGKILL, OOM, a node
// going away) leaves them there, and nothing else ever claims a
// 'processing' row — before this package those events never reached
// ClickHouse and those postbacks were never sent (audit 2026-10-06, Q1).
//
// The reaper is the backstop, not the normal path: a clean shutdown
// finishes or requeues its batch itself (see the flushers and
// postback.Deliverer). Anything still claimed after StuckAfter is far past
// any real batch's duration and is handed out again. That can re-deliver a
// batch whose work actually completed just before the crash: for the
// ClickHouse queues the retried insert is a no-op (same batch id, same
// deduplication token — internal/pgqueue, chstore.WithBatchID); a reaped
// outgoing postback may reach the partner twice.
package queuereaper

import (
	"context"
	"log/slog"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

// StuckAfter is how long a claim may stay 'processing' before it is
// considered abandoned. A batch is bounded by its own timeouts (a
// ClickHouse insert, or 20 postback deliveries at 10s each), all well
// under this.
const StuckAfter = 5 * time.Minute

// Interval is how often each queue is checked.
const Interval = time.Minute

// Reaper is implemented by each queue's Postgres store.
type Reaper interface {
	// ReapStuck returns rows claimed longer than olderThan ago to the
	// queue and reports how many it moved.
	ReapStuck(ctx context.Context, olderThan time.Duration) (int64, error)
}

// RunLoop reaps queue every interval until ctx is done, starting
// immediately so claims left by the previous process are released at boot.
func RunLoop(ctx context.Context, queue string, r Reaper, interval, olderThan time.Duration, logger *slog.Logger) {
	tick := func() {
		n, err := r.ReapStuck(ctx, olderThan)
		if err != nil {
			if ctx.Err() == nil {
				logger.Error("reaping stuck queue claims", "queue", queue, "error", err)
			}
			return
		}
		if n > 0 {
			metrics.QueueReapedTotal.WithLabelValues(queue).Add(float64(n))
			logger.Warn("returned abandoned queue claims", "queue", queue, "rows", n, "older_than", olderThan)
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
