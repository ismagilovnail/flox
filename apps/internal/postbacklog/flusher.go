package postbacklog

import (
	"context"
	"log/slog"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
)

// flushRetryDelay mirrors internal/eventqueue's: a failed ClickHouse batch
// retries on a fixed delay, unbounded — this is a secondary audit log, not
// a money-correctness path, so there is no dead-letter state here either.
const flushRetryDelay = 10 * time.Second

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

	if err := f.ch.InsertPostbackAttempts(ctx, attempts); err != nil {
		f.logger.Error("clickhouse postback_events batch insert failed, requeueing", "error", err, "count", len(claimed))
		if reqErr := f.consumer.Requeue(ctx, ids, time.Now().UTC().Add(flushRetryDelay)); reqErr != nil {
			f.logger.Error("requeueing failed postback attempt batch", "error", reqErr)
		}
		return len(claimed), nil
	}

	if err := f.consumer.Delete(ctx, ids); err != nil {
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
