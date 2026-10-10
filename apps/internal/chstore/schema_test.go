package chstore_test

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
)

// countingConn counts Exec calls: DDL and the version inserts go through
// Exec, the version read through Query.
type countingConn struct {
	driver.Conn
	execs atomic.Int64
}

func (c *countingConn) Exec(ctx context.Context, query string, args ...any) error {
	c.execs.Add(1)
	return c.Conn.Exec(ctx, query, args...)
}

func TestMigrateRecordsEveryFileAndSkipsThemAfter(t *testing.T) {
	conn := mustConn(t) // migrated once already

	var recorded uint64
	if err := conn.QueryRow(context.Background(),
		`SELECT uniqExact(version) FROM schema_migrations WHERE version IN ('000_drop_phase25_schema', '013_click_lookup', '014_event_id')`,
	).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != 3 {
		t.Fatalf("recorded versions among three known files = %d, want 3", recorded)
	}

	counting := &countingConn{Conn: conn}
	if err := chstore.Migrate(context.Background(), counting); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if n := counting.execs.Load(); n != 1 {
		t.Fatalf("Migrate on a recorded schema ran %d statements, want 1 (the CREATE TABLE IF NOT EXISTS)", n)
	}
}

func TestMigrateLockedRunsConcurrentlyWithoutError(t *testing.T) {
	conn := mustConn(t)
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("needs DATABASE_URL")
	}
	pg, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pg.Close)

	// api, tracker and worker starting together.
	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- chstore.MigrateLocked(context.Background(), pg, conn)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("MigrateLocked: %v", err)
		}
	}
	// The lock is released: another session takes it at once.
	c, err := pg.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Release()
	var got bool
	if err := c.QueryRow(context.Background(), `SELECT pg_try_advisory_lock($1)`, int64(0x666c6f78_63686d67)).Scan(&got); err != nil || !got {
		t.Fatalf("advisory lock still held after MigrateLocked (got %v, err %v)", got, err)
	}
	_, _ = c.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, int64(0x666c6f78_63686d67))
}
