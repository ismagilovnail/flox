package eventqueue

import (
	"context"
	"log/slog"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

// A failed batch is retried with exponential backoff and, when the
// ClickHouse server keeps rejecting it, dead-lettered — see pgqueue.Fail
// (audit 2026-10-06, Q2).

// insertTimeout / cleanupTimeout bound a claimed batch's work once it is
// detached from the poll loop's context (see RunOnce).
const (
	insertTimeout  = 30 * time.Second
	cleanupTimeout = 10 * time.Second
)

// detached returns a context that ignores ctx's cancellation but still has
// its own deadline.
func detached(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}

// ClickHouseSink is the narrow slice of chstore.EventStore Flusher needs —
// enough to stay decoupled from ClickHouse's own client type, the same
// pattern Sink already uses in the other direction (eventbuf -> here).
type ClickHouseSink interface {
	InsertBatch(ctx context.Context, events []event.Event) error
}

// Flusher claims due events and batch-writes them to ClickHouse, run by
// apps/worker.
type Flusher struct {
	consumer Consumer
	ch       ClickHouseSink
	logger   *slog.Logger
}

func NewFlusher(consumer Consumer, ch ClickHouseSink, logger *slog.Logger) *Flusher {
	return &Flusher{consumer: consumer, ch: ch, logger: logger}
}

// RunOnce claims up to limit due events and attempts one ClickHouse batch
// insert for all of them together — batching the insert, not just the
// claim, is what makes this fast at volume. A failed insert requeues the
// WHOLE batch (never a partial retry: ClickHouse's batch API sends one
// block, so a failure means none of it landed) and is logged, never
// returned as an error that would stall the poll loop's pacing.
func (f *Flusher) RunOnce(ctx context.Context, limit int) (int, error) {
	claimed, err := f.consumer.ClaimDue(ctx, limit)
	if err != nil {
		return 0, err
	}
	if len(claimed) == 0 {
		return 0, nil
	}

	events := make([]event.Event, len(claimed))
	ids := make([]string, len(claimed))
	for i, c := range claimed {
		events[i] = c.Event
		ids[i] = c.ID
	}

	// Once claimed, the batch is finished on a context the shutdown signal
	// cannot cancel: cancelling the insert and then the requeue with it left
	// the rows 'processing' forever on every deploy (audit 2026-10-06, Q1).
	// PollLoop stops claiming new batches; this one completes or goes back.
	insertCtx, cancelInsert := detached(ctx, insertTimeout)
	// The batch's id makes the insert idempotent: a retry of a batch that
	// partly or fully landed adds nothing (audit 2026-10-06, D2).
	insertCtx = chstore.WithBatchID(insertCtx, claimed[0].BatchID)
	insertStart := time.Now()
	insertErr := f.ch.InsertBatch(insertCtx, events)
	cancelInsert()
	metrics.EventProcessingLatencySeconds.Observe(time.Since(insertStart).Seconds())

	cleanupCtx, cancelCleanup := detached(ctx, cleanupTimeout)
	defer cancelCleanup()

	if insertErr != nil {
		// Backoff on every failure; dead-letter only what the ClickHouse
		// server itself keeps rejecting (audit 2026-10-06, Q2).
		rejected := chstore.Rejected(insertErr)
		dead, failErr := f.consumer.Fail(cleanupCtx, ids, rejected, insertErr.Error())
		switch {
		case failErr != nil:
			f.logger.Error("recording failed event batch", "error", failErr, "insert_error", insertErr)
		case dead:
			metrics.QueueDeadLetteredTotal.WithLabelValues("event_queue").Add(float64(len(claimed)))
			f.logger.Error("clickhouse keeps rejecting an event batch, dead-lettered",
				"error", insertErr, "count", len(claimed), "batch_id", claimed[0].BatchID)
			return len(claimed), nil
		}
		f.logger.Error("clickhouse batch insert failed, retrying later",
			"error", insertErr, "rejected", rejected, "count", len(claimed), "batch_id", claimed[0].BatchID)
		metrics.EventsRequeuedTotal.Add(float64(len(claimed)))
		return len(claimed), nil
	}

	if err := f.consumer.Delete(cleanupCtx, ids); err != nil {
		f.logger.Error("deleting flushed events", "error", err)
	}
	metrics.EventsPersistedTotal.Add(float64(len(claimed)))
	return len(claimed), nil
}

// PollLoop runs RunOnce until ctx is done, mirroring
// internal/postback.Deliverer.PollLoop's pacing: a full batch polls again
// immediately to drain a backlog, anything less waits idle.
func (f *Flusher) PollLoop(ctx context.Context, batchSize int, idle time.Duration) {
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := f.RunOnce(ctx, batchSize)
		if err != nil {
			f.logger.Error("event flush poll failed", "error", err)
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
