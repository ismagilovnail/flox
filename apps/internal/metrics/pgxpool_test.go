package metrics

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPgxPoolCollector(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}

	c := &pgxPoolCollector{pool: pool, service: "test"}
	if n := testutil.CollectAndCount(c); n != 8 {
		t.Fatalf("collected %d series, want 8 (4 conn states, 3 acquire kinds, wait)", n)
	}
	want := `
# HELP flox_pg_pool_acquires_total Postgres pool acquires: all, those that had to wait for a free connection (empty), and canceled.
# TYPE flox_pg_pool_acquires_total counter
`
	if err := testutil.CollectAndCompare(c, strings.NewReader(want+"flox_pg_pool_acquires_total{kind=\"all\",service=\"test\"} 1\nflox_pg_pool_acquires_total{kind=\"canceled\",service=\"test\"} 0\nflox_pg_pool_acquires_total{kind=\"empty\",service=\"test\"} 1\n"), "flox_pg_pool_acquires_total"); err != nil {
		t.Fatalf("acquire counters after one ping: %v", err)
	}
}
