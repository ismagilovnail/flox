// Package postbacklog carries postback attempts — both directions,
// internal/conversion's incoming outcomes and internal/postback's outgoing
// delivery attempts — into ClickHouse's postback_events table (§48), the
// rich per-attempt log migration 00008 (Phase 17) earmarked for once
// ClickHouse existed.
//
// A Postgres-backed FOR UPDATE SKIP LOCKED queue (postback_attempt_queue,
// migration 00016) feeding a batching Flusher, run by apps/worker. The
// consumer side and the Flusher are internal/pgqueue's, shared with
// internal/eventqueue (tech audit CQ2); this package adds how attempts get
// in.
//
// Producer.EnqueueAttempt is best-effort, no error return, same contract
// as conversion.DeliveryEnqueuer: neither an accepted incoming postback nor
// a completed outgoing delivery attempt should ever be reported as failed
// just because this secondary audit log's queue insert stumbled.
package postbacklog

import (
	"context"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/pgqueue"
)

// QueuedAttempt is one row claimed from the queue.
type QueuedAttempt = pgqueue.Item[chstore.PostbackAttempt]

// Producer is what internal/conversion and internal/postback call.
type Producer interface {
	EnqueueAttempt(ctx context.Context, attempt chstore.PostbackAttempt)
}

// Consumer is what apps/worker's flush loop needs (pgqueue.Consumer).
type Consumer = pgqueue.Consumer[chstore.PostbackAttempt]
