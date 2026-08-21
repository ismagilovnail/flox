// Package eventqueue is the durable link in §43's pipeline — "Tracker ->
// Event Queue -> Worker -> ClickHouse" — between apps/tracker's async
// event writer and apps/worker's ClickHouse flush loop. Postgres-backed
// (STACK has no message broker), following the same "FOR UPDATE SKIP
// LOCKED job queue" pattern internal/postback already established for
// outgoing deliveries.
//
// Unlike internal/postback's queue, a claimed batch here is DELETED once it
// is durably in ClickHouse, not marked terminal and kept — this table is
// disposable transit, not an audit ledger.
package eventqueue

import (
	"context"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
)

// QueuedEvent is one row claimed from the queue.
type QueuedEvent struct {
	ID    string
	Event event.Event
}

// Producer is what apps/tracker's event writer needs — see Sink (sink.go),
// which adapts this to eventbuf.Sink.
type Producer interface {
	// EnqueueBatch durably persists every event in batch. Called from
	// eventbuf.Writer's own flush goroutine, already off the redirect's
	// critical path (CLAUDE.md #9) — this does not need its own additional
	// buffering.
	EnqueueBatch(ctx context.Context, events []event.Event) error
}

// Consumer is what apps/worker's flush loop needs.
type Consumer interface {
	// ClaimDue atomically claims up to limit due rows (FOR UPDATE SKIP
	// LOCKED), marking them 'processing' so a second worker replica can't
	// also claim them.
	ClaimDue(ctx context.Context, limit int) ([]QueuedEvent, error)

	// Delete permanently removes rows once their batch is durably in
	// ClickHouse.
	Delete(ctx context.Context, ids []string) error

	// Requeue puts claimed rows back to 'queued', due again at
	// next_attempt_at — used when a ClickHouse batch insert fails, so the
	// whole batch is retried together rather than row by row.
	Requeue(ctx context.Context, ids []string, next time.Time) error
}
