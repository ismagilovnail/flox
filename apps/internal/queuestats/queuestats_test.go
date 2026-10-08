package queuestats

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

// A private table shaped like event_queue: the dev-stack worker claims due
// rows from the real queues, which would move this test's numbers.
func TestRefreshCountsStatusesAndOldestDueAge(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(db.Close) // not defer: the table cleanup below needs the pool
	table := "queuestats_test_" + strings.ToLower(idgen.New())
	if _, err := db.Exec(ctx, `CREATE TABLE `+table+` (id text, status text, next_attempt_at timestamptz)`); err != nil {
		t.Fatalf("creating table: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(context.Background(), `DROP TABLE `+table) })
	if _, err := db.Exec(ctx, `INSERT INTO `+table+` VALUES
		('a', 'queued', now() - interval '90 seconds'),
		('b', 'queued', now() + interval '1 hour'),
		('c', 'processing', now()),
		('d', 'dead', now() - interval '1 day')`); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	q := queue{table, "status", []string{"queued", "processing", "dead"}, []string{"queued"}}
	if err := refresh(ctx, db, q); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	for status, want := range map[string]float64{"queued": 2, "processing": 1, "dead": 1} {
		if got := testutil.ToFloat64(metrics.QueueRows.WithLabelValues(table, status)); got != want {
			t.Errorf("queue_rows{status=%q} = %v, want %v", status, got, want)
		}
	}
	// Only the due queued row counts: not the future one, not the dead one.
	if age := testutil.ToFloat64(metrics.QueueOldestDueAgeSeconds.WithLabelValues(table)); age < 89 || age > 120 {
		t.Errorf("oldest due age = %v s, want about 90", age)
	}
}
