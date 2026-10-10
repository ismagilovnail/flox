package postbacklog

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/pgqueue"
)

// PostgresQueue implements both Producer and Consumer against
// postback_attempt_queue (migration 00016).
//
// The consumer side — claim, delete, requeue, reap, dead-letter — is
// pgqueue.Queue's (tech audit CQ2); this package adds how attempts get in.
type PostgresQueue struct {
	*pgqueue.Queue[chstore.PostbackAttempt]
	db     *pgxpool.Pool
	logger *slog.Logger
	// table is postback_attempt_queue; tests use a private copy.
	table string
}

// NewPostgresQueue builds a Producer/Consumer. logger is used only by the
// Producer half (EnqueueAttempt has no error return, so a failed insert can
// only be logged, never propagated) — see the package doc for why.
func NewPostgresQueue(db *pgxpool.Pool, logger *slog.Logger) *PostgresQueue {
	return newPostgresQueue(db, logger, "postback_attempt_queue")
}

func newPostgresQueue(db *pgxpool.Pool, logger *slog.Logger, table string) *PostgresQueue {
	return &PostgresQueue{
		Queue:  pgqueue.NewQueue[chstore.PostbackAttempt](db, table, "postbacklog"),
		db:     db,
		logger: logger,
		table:  table,
	}
}

var (
	_ Producer = (*PostgresQueue)(nil)
	_ Consumer = (*PostgresQueue)(nil)
)

func (q *PostgresQueue) EnqueueAttempt(ctx context.Context, attempt chstore.PostbackAttempt) {
	payload, err := json.Marshal(attempt)
	if err != nil {
		q.logger.Error("postbacklog: marshaling attempt", "error", err)
		return
	}
	if _, err := q.db.Exec(ctx,
		`INSERT INTO `+q.table+` (id, organization_id, payload) VALUES ($1, $2, $3)`,
		idgen.New(), attempt.OrganizationID, payload,
	); err != nil {
		q.logger.Error("postbacklog: enqueueing attempt", "error", err,
			"organization_id", attempt.OrganizationID, "direction", attempt.Direction, "click_id", attempt.ClickID)
	}
}
