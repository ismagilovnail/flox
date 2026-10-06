package postbacklog

import (
	"context"
	"log/slog"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

// A failed batch is retried with exponential backoff and, when the
// ClickHouse server keeps rejecting it, dead-lettered — see pgqueue.Fail
// (audit 2026-10-06, Q2).

// insertTimeout / cleanupTimeout bound a claimed batch's work once it is
// detached from the poll loop's context — same reasoning as
// eventqueue.Flusher.RunOnce.
const (
	insertTimeout  = 30 * time.Second
	cleanupTimeout = 10 * time.Second
)

func detached(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}

// ClickHouseSink is the narrow slice of chstore.EventStore Flusher needs.
type ClickHouseSink interface {
	InsertPostbackAttempts(ctx context.Context, attempts []chstore.PostbackAttempt) error
}

// Flusher claims due attempts and batch-writes them to ClickHouse, run by
// apps/worker — the same shape as internal/eventqueue.Flusher.
type Flusher struct {
	consumer Consumer
	ch       ClickHouseSink
	logger   *slog.Logger
}

func NewFlusher(consumer Consumer, ch ClickHouseSink, logger *slog.Logger) *Flusher {
	return &Flusher{consumer: consumer, ch: ch, logger: logger}
}

func (f *Flusher) RunOnce(ctx context.Context, limit int) (int, error) {
	claimed, err := f.consumer.ClaimDue(ctx, limit)
	if err != nil {
		return 0, err
	}
	if len(claimed) == 0 {
		return 0, nil
	}

	attempts := make([]chstore.PostbackAttempt, len(claimed))
	ids := make([]string, len(claimed))
	for i, c := range claimed {
		attempts[i] = c.Attempt
		ids[i] = c.ID
	}

	// A claimed batch finishes even if ctx is cancelled mid-flight (audit
	// 2026-10-06, Q1) — see eventqueue.Flusher.RunOnce.
	insertCtx, cancelInsert := detached(ctx, insertTimeout)
	insertCtx = chstore.WithBatchID(insertCtx, claimed[0].BatchID) // idempotent retry, see eventqueue
	insertErr := f.ch.InsertPostbackAttempts(insertCtx, attempts)
	cancelInsert()

	cleanupCtx, cancelCleanup := detached(ctx, cleanupTimeout)
	defer cancelCleanup()

	if err := insertErr; err != nil {
		// Backoff and dead-lettering as in eventqueue.Flusher.RunOnce.
		rejected := chstore.Rejected(err)
		dead, failErr := f.consumer.Fail(cleanupCtx, ids, rejected, err.Error())
		switch {
		case failErr != nil:
			f.logger.Error("recording failed postback attempt batch", "error", failErr, "insert_error", err)
		case dead:
			metrics.QueueDeadLetteredTotal.WithLabelValues("postback_attempt_queue").Add(float64(len(claimed)))
			f.logger.Error("clickhouse keeps rejecting a postback_events batch, dead-lettered",
				"error", err, "count", len(claimed), "batch_id", claimed[0].BatchID)
		default:
			f.logger.Error("clickhouse postback_events batch insert failed, retrying later",
				"error", err, "rejected", rejected, "count", len(claimed))
		}
		return len(claimed), nil
	}

	if err := f.consumer.Delete(cleanupCtx, ids); err != nil {
		f.logger.Error("deleting flushed postback attempts", "error", err)
	}
	return len(claimed), nil
}

// PollLoop mirrors internal/eventqueue.Flusher.PollLoop and
// internal/postback.Deliverer.PollLoop's pacing exactly.
func (f *Flusher) PollLoop(ctx context.Context, batchSize int, idle time.Duration) {
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := f.RunOnce(ctx, batchSize)
		if err != nil {
			f.logger.Error("postback attempt flush poll failed", "error", err)
			n = 0
		}
		if n == batchSize {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(idle):
		}
	}
}
