package pixelsend

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPostgresStoreOnTable is NewPostgresStore over a private copy of
// pixel_deliveries (testpg.PrivateTable).
func NewPostgresStoreOnTable(db *pgxpool.Pool, table string) *PostgresStore {
	return &PostgresStore{db: db, table: table}
}

// WithClock fixes the Deliverer's clock.
func (d *Deliverer) WithClock(now func() time.Time) *Deliverer {
	d.now = now
	return d
}
