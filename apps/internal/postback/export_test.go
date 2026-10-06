package postback

import "github.com/jackc/pgx/v5/pgxpool"

// NewPostgresStoreOnTable is NewPostgresStore over another table shaped like
// postback_deliveries (testpg.PrivateTable).
func NewPostgresStoreOnTable(db *pgxpool.Pool, table string) *PostgresStore {
	return &PostgresStore{db: db, table: table}
}
