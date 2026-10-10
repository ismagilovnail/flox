package postbacklog

import (
	"context"
	"log/slog"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/pgqueue"
)

// ClickHouseSink is the narrow slice of chstore.EventStore Flusher needs.
type ClickHouseSink interface {
	InsertPostbackAttempts(ctx context.Context, attempts []chstore.PostbackAttempt) error
}

// Flusher claims due attempts and batch-writes them to ClickHouse, run by
// apps/worker (pgqueue.Flusher).
type Flusher = pgqueue.Flusher[chstore.PostbackAttempt]

func NewFlusher(consumer Consumer, ch ClickHouseSink, logger *slog.Logger) *Flusher {
	return pgqueue.NewFlusher(consumer, pgqueue.FlusherConfig[chstore.PostbackAttempt]{
		Queue:  "postback_attempt_queue",
		What:   "postback attempt",
		Insert: ch.InsertPostbackAttempts,
	}, logger)
}
