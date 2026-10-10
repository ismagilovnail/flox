package eventqueue

import "github.com/jackc/pgx/v5/pgxpool"

// NewPostgresQueueOnTable is NewPostgresQueue over another table shaped like
// event_queue (testpg.PrivateTable).
func NewPostgresQueueOnTable(db *pgxpool.Pool, table string) *PostgresQueue {
	return newPostgresQueue(db, table)
}
