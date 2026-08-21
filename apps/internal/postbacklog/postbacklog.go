// Package postbacklog carries postback attempts — both directions,
// internal/conversion's incoming outcomes and internal/postback's outgoing
// delivery attempts — into ClickHouse's postback_events table (§48), the
// rich per-attempt log migration 00008 (Phase 17) earmarked for once
// ClickHouse existed.
//
// Structurally a near-duplicate of internal/eventqueue: a Postgres-backed
// FOR UPDATE SKIP LOCKED queue (postback_attempt_queue, migration 00016)
// feeding a batching Flusher, run by apps/worker. Kept separate rather than
// generalized because the payload (chstore.PostbackAttempt) and the events
// pipeline's (event.Event) are genuinely different shapes with nothing
// else to share — see docs/analytics-pipeline.md.
//
// Producer.EnqueueAttempt is best-effort, no error return, same contract
// as conversion.DeliveryEnqueuer: neither an accepted incoming postback nor
// a completed outgoing delivery attempt should ever be reported as failed
// just because this secondary audit log's queue insert stumbled.
package postbacklog

import (
	"context"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
)

// QueuedAttempt is one row claimed from the queue.
type QueuedAttempt struct {
	ID      string
	Attempt chstore.PostbackAttempt
}

// Producer is what internal/conversion and internal/postback call.
type Producer interface {
	EnqueueAttempt(ctx context.Context, attempt chstore.PostbackAttempt)
}

// Consumer is what apps/worker's flush loop needs.
type Consumer interface {
	ClaimDue(ctx context.Context, limit int) ([]QueuedAttempt, error)
	Delete(ctx context.Context, ids []string) error
	Requeue(ctx context.Context, ids []string, next time.Time) error
}
