package eventqueue

import (
	"context"
	"log/slog"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/pgqueue"
)

// A failed batch is retried with exponential backoff and, when the
// ClickHouse server keeps rejecting it, dead-lettered — see pgqueue.Fail
// (audit 2026-10-06, Q2).

// ClickHouseSink is the narrow slice of chstore.EventStore Flusher needs —
// enough to stay decoupled from ClickHouse's own client type, the same
// pattern Sink already uses in the other direction (eventbuf -> here).
type ClickHouseSink interface {
	InsertBatch(ctx context.Context, events []event.Event) error
}

// Flusher claims due events and batch-writes them to ClickHouse, run by
// apps/worker (pgqueue.Flusher).
type Flusher = pgqueue.Flusher[event.Event]

func NewFlusher(consumer Consumer, ch ClickHouseSink, logger *slog.Logger) *Flusher {
	return pgqueue.NewFlusher(consumer, pgqueue.FlusherConfig[event.Event]{
		Queue:       "event_queue",
		What:        "event",
		Insert:      ch.InsertBatch,
		OnInsert:    func(took time.Duration) { metrics.EventProcessingLatencySeconds.Observe(took.Seconds()) },
		OnPersisted: func(n int) { metrics.EventsPersistedTotal.Add(float64(n)) },
		OnRequeued:  func(n int) { metrics.EventsRequeuedTotal.Add(float64(n)) },
	}, logger)
}
