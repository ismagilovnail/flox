package pgqueue

import (
	"context"
	"log/slog"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

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

// Consumer is what a Flusher needs from a queue (Queue implements it).
type Consumer[T any] interface {
	// ClaimDue claims one batch — a whole earlier batch due for retry, or
	// up to limit fresh rows — marked 'processing' so no other worker
	// takes it. Every returned row carries the same BatchID.
	ClaimDue(ctx context.Context, limit int) ([]Item[T], error)
	// Delete removes rows once their batch is durably in ClickHouse.
	Delete(ctx context.Context, ids []string) error
	// Fail records a failed insert: retried with backoff, or — when the
	// server keeps rejecting it (chstore.Rejected) — dead-lettered.
	// Reports whether it was.
	Fail(ctx context.Context, ids []string, rejected bool, lastError string) (dead bool, err error)
}

// FlusherConfig says what a Flusher flushes.
type FlusherConfig[T any] struct {
	// Queue names the queue in metrics and logs: "event_queue".
	Queue string
	// What names one row in logs: "event".
	What string
	// Insert writes one batch to ClickHouse.
	Insert func(ctx context.Context, batch []T) error

	// Optional per-queue metrics.
	OnInsert    func(took time.Duration)
	OnPersisted func(n int)
	OnRequeued  func(n int)
}

// Flusher claims due rows and batch-writes them to ClickHouse, run by
// apps/worker — one implementation for every queue (tech audit CQ2).
type Flusher[T any] struct {
	consumer Consumer[T]
	cfg      FlusherConfig[T]
	logger   *slog.Logger
}

func NewFlusher[T any](consumer Consumer[T], cfg FlusherConfig[T], logger *slog.Logger) *Flusher[T] {
	return &Flusher[T]{consumer: consumer, cfg: cfg, logger: logger}
}

// RunOnce claims up to limit due rows and attempts one ClickHouse batch
// insert for all of them together — batching the insert, not just the
// claim, is what makes this fast at volume. A failed insert sends the
// WHOLE batch back (never a partial retry: ClickHouse's batch API sends one
// block, so a failure means none of it landed) and is logged, never
// returned as an error that would stall the poll loop's pacing.
func (f *Flusher[T]) RunOnce(ctx context.Context, limit int) (int, error) {
	claimed, err := f.consumer.ClaimDue(ctx, limit)
	if err != nil {
		return 0, err
	}
	if len(claimed) == 0 {
		return 0, nil
	}

	batch := make([]T, len(claimed))
	ids := make([]string, len(claimed))
	for i, c := range claimed {
		batch[i] = c.Value
		ids[i] = c.ID
	}

	// Once claimed, the batch is finished on a context the shutdown signal
	// cannot cancel: cancelling the insert and the requeue with it would
	// leave the rows 'processing' on every deploy (tech audit Q1).
	// PollLoop stops claiming new batches; this one completes or goes back.
	insertCtx, cancelInsert := detached(ctx, insertTimeout)
	// The batch's id makes the insert idempotent: a retry of a batch that
	// partly or fully landed adds nothing (tech audit D2).
	insertCtx = chstore.WithBatchID(insertCtx, claimed[0].BatchID)
	insertStart := time.Now()
	insertErr := f.cfg.Insert(insertCtx, batch)
	cancelInsert()
	if f.cfg.OnInsert != nil {
		f.cfg.OnInsert(time.Since(insertStart))
	}

	cleanupCtx, cancelCleanup := detached(ctx, cleanupTimeout)
	defer cancelCleanup()

	if insertErr != nil {
		// Backoff on every failure; dead-letter only what the ClickHouse
		// server itself keeps rejecting (tech audit Q2).
		rejected := chstore.Rejected(insertErr)
		dead, failErr := f.consumer.Fail(cleanupCtx, ids, rejected, insertErr.Error())
		switch {
		case failErr != nil:
			f.logger.Error("recording failed "+f.cfg.What+" batch", "error", failErr, "insert_error", insertErr)
		case dead:
			metrics.QueueDeadLetteredTotal.WithLabelValues(f.cfg.Queue).Add(float64(len(claimed)))
			f.logger.Error("clickhouse keeps rejecting a "+f.cfg.What+" batch, dead-lettered",
				"error", insertErr, "count", len(claimed), "batch_id", claimed[0].BatchID)
			return len(claimed), nil
		}
		f.logger.Error("clickhouse "+f.cfg.What+" batch insert failed, retrying later",
			"error", insertErr, "rejected", rejected, "count", len(claimed), "batch_id", claimed[0].BatchID)
		if f.cfg.OnRequeued != nil {
			f.cfg.OnRequeued(len(claimed))
		}
		return len(claimed), nil
	}

	if err := f.consumer.Delete(cleanupCtx, ids); err != nil {
		f.logger.Error("deleting flushed "+f.cfg.What+" rows", "error", err)
	}
	if f.cfg.OnPersisted != nil {
		f.cfg.OnPersisted(len(claimed))
	}
	return len(claimed), nil
}

// PollLoop runs RunOnce until ctx is done, mirroring
// internal/postback.Deliverer.PollLoop's pacing: a full batch polls again
// immediately to drain a backlog, anything less waits idle.
func (f *Flusher[T]) PollLoop(ctx context.Context, batchSize int, idle time.Duration) {
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := f.RunOnce(ctx, batchSize)
		if err != nil {
			f.logger.Error(f.cfg.What+" flush poll failed", "error", err)
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
