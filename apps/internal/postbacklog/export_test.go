package postbacklog

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPostgresQueueOnTable is NewPostgresQueue over another table shaped like
// postback_attempt_queue (testpg.PrivateTable).
func NewPostgresQueueOnTable(db *pgxpool.Pool, logger *slog.Logger, table string) *PostgresQueue {
	return newPostgresQueue(db, logger, table)
}
