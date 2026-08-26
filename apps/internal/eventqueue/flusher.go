package eventqueue

import (
	"context"
	"log/slog"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

// flushRetryDelay is how long a failed ClickHouse batch waits before the
// next attempt. Fixed, not exponential: unlike internal/postback's outgoing
// deliveries (money-adjacent, dead-lettered after MaxAttempts), a delayed
// analytics batch has no per-item deadline — retrying the same batch every
// flushRetryDelay until ClickHouse recovers is simpler and sufficient, and
// nothing here should ever give up and drop events permanently.
const flushRetryDelay = 10 * time.Second

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

	insertStart := time.Now()
	insertErr := f.ch.InsertBatch(ctx, events)
	metrics.EventProcessingLatencySeconds.Observe(time.Since(insertStart).Seconds())

	if insertErr != nil {
		f.logger.Error("clickhouse batch insert failed, requeueing", "error", insertErr, "count", len(claimed))
		if reqErr := f.consumer.Requeue(ctx, ids, time.Now().UTC().Add(flushRetryDelay)); reqErr != nil {
			f.logger.Error("requeueing failed event batch", "error", reqErr)
		}
		metrics.EventsRequeuedTotal.Add(float64(len(claimed)))
		return len(claimed), nil
	}

	if err := f.consumer.Delete(ctx, ids); err != nil {
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
