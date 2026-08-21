// Package postback is Phase 24's outgoing postback engine (§46): it
// delivers a macro-resolved URL to a network for every conversion
// internal/conversion records, off the incoming postback request's
// critical path (CLAUDE.md #9's spirit — never block on an outbound
// partner call), with exponential backoff and a dead-letter state after
// repeated failure.
//
// The queue is `postback_deliveries` (migration 00014), a durable Postgres
// table, not a broker — the STACK has none, and a job queue's own state
// (was this delivered?) is not cache-appropriate data anyway. Enqueue
// happens synchronously inside internal/conversion.Service.Record (a local
// Postgres insert, not an outbound call, so it doesn't violate #9);
// Deliverer.PollLoop, running in apps/worker, is what actually calls out.
package postback

import (
	"context"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
)

// DeliveryStatus is §46's exact status vocabulary.
type DeliveryStatus string

const (
	StatusQueued     DeliveryStatus = "queued"
	StatusProcessing DeliveryStatus = "processing"
	StatusSuccess    DeliveryStatus = "success"
	StatusFailed     DeliveryStatus = "failed"
	StatusRetrying   DeliveryStatus = "retrying"
	StatusDead       DeliveryStatus = "dead"
)

// MaxAttempts is how many delivery attempts run before a delivery is
// dead-lettered. Not specified by §46; chosen so a network's multi-hour
// outage still gets delivered (see Backoff's total span, ~21 hours) without
// retrying forever.
const MaxAttempts = 8

// Backoff is the exponential schedule between attempts, capped at its last
// entry. NextAttemptDelay(1) (the wait after the FIRST failed attempt) is
// Backoff[0], and so on.
var Backoff = []time.Duration{
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
	6 * time.Hour,
	12 * time.Hour,
}

// NextAttemptDelay returns how long to wait before the NEXT try, given
// attemptCount attempts already made.
func NextAttemptDelay(attemptCount int) time.Duration {
	i := attemptCount - 1
	if i < 0 {
		i = 0
	}
	if i >= len(Backoff) {
		i = len(Backoff) - 1
	}
	return Backoff[i]
}

// Delivery is one queue row, as claimed by Store.ClaimDue.
type Delivery struct {
	ID               string
	OrganizationID   string
	NetworkID        string
	SourcePostbackID string
	ClickID          string
	Status           event.Type
	URL              string
	// AttemptCount already reflects the attempt in progress — ClaimDue
	// increments it as part of claiming, so a caller deciding "have I hit
	// MaxAttempts" never has to add one itself.
	AttemptCount int
}

// EnqueueInput is what Store.Enqueue persists.
type EnqueueInput struct {
	OrganizationID   string
	NetworkID        string
	SourcePostbackID string
	ClickID          string
	Status           event.Type
	URL              string
}

// Store is the durable queue backing this package. PostgresStore
// (postgres.go) is the only implementation.
type Store interface {
	// Enqueue adds one delivery in StatusQueued, due immediately.
	Enqueue(ctx context.Context, in EnqueueInput) (id string, err error)

	// ClaimDue atomically claims up to limit due deliveries (StatusQueued
	// or StatusRetrying with next_attempt_at <= now), marking them
	// StatusProcessing and incrementing AttemptCount, so a second worker
	// replica polling concurrently can never also claim the same row
	// (Postgres FOR UPDATE SKIP LOCKED).
	ClaimDue(ctx context.Context, limit int) ([]Delivery, error)

	// MarkSuccess records a successful delivery (a 2xx response).
	MarkSuccess(ctx context.Context, id string, responseStatusCode int) error

	// MarkRetrying records a failed attempt that will be tried again at
	// nextAttemptAt. responseStatusCode is 0 when the failure never got an
	// HTTP response at all (timeout, connection refused, invalid URL).
	MarkRetrying(ctx context.Context, id string, responseStatusCode int, message string, nextAttemptAt time.Time) error

	// MarkDead records a failed attempt that exhausted MaxAttempts.
	MarkDead(ctx context.Context, id string, responseStatusCode int, message string) error
}

// AttemptRecord is what Deliverer reports to the postback attempt audit
// log (internal/postbacklog, §48's postback_events) after every dispatch
// attempt — success, retrying, and dead alike.
type AttemptRecord struct {
	OrganizationID     string
	NetworkID          string
	ClickID            string
	Status             event.Type
	Result             DeliveryStatus
	Message            string
	AttemptCount       int
	ResponseStatusCode int
	URL                string
	OccurredAt         time.Time
}

// AttemptLogger is the narrow slice of internal/postbacklog this package
// needs — same decoupled, no-error-return pattern as everywhere else in
// this codebase (EventSink, DeliveryEnqueuer, conversion.AttemptLogger): a
// delivery attempt's own outcome (already durably recorded via Store) must
// never be affected by this secondary audit log's queue insert stumbling.
type AttemptLogger interface {
	LogAttempt(ctx context.Context, rec AttemptRecord)
}
