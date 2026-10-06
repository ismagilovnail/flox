package pgqueue_test

import (
	"context"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/pgqueue"
)

// queueTable creates a private table shaped like event_queue, so a dev-stack
// worker polling the real queues can never take rows from under the test.
func queueTable(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(pool.Close)
	table := "pgqueue_test_" + strings.ToLower(idgen.New())
	if _, err := pool.Exec(context.Background(), `
		CREATE TABLE `+table+` (
		  id text PRIMARY KEY, payload jsonb NOT NULL DEFAULT '{}',
		  status text NOT NULL DEFAULT 'queued',
		  next_attempt_at timestamptz NOT NULL DEFAULT now(),
		  claimed_at timestamptz, batch_id text)`); err != nil {
		t.Fatalf("creating %s: %v", table, err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DROP TABLE `+table) })
	return pool, table
}

func enqueue(t *testing.T, pool *pgxpool.Pool, table string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := pool.Exec(context.Background(), `INSERT INTO `+table+` (id) VALUES ($1)`, idgen.New()); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}
}

func ids(rows []pgqueue.Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	sort.Strings(out)
	return out
}

// Audit 2026-10-06, D2: a retried batch must carry exactly the rows of its
// first attempt — its ClickHouse insert token would otherwise cover
// different rows and ClickHouse would drop the new ones.
func TestRetryClaimsTheSameBatchUnmixed(t *testing.T) {
	pool, table := queueTable(t)
	ctx := context.Background()
	enqueue(t, pool, table, 3)

	batchID, first, err := pgqueue.ClaimBatch(ctx, pool, table, 10)
	if err != nil || len(first) != 3 || batchID == "" {
		t.Fatalf("first claim: batch=%q rows=%d err=%v, want 3 rows under a batch id", batchID, len(first), err)
	}

	// The insert failed: requeue the batch, while new rows arrive.
	if _, err := pool.Exec(ctx, `UPDATE `+table+` SET status = 'queued', claimed_at = NULL WHERE batch_id = $1`, batchID); err != nil {
		t.Fatalf("requeue: %v", err)
	}
	enqueue(t, pool, table, 5)

	retryID, retry, err := pgqueue.ClaimBatch(ctx, pool, table, 10)
	if err != nil {
		t.Fatalf("retry claim: %v", err)
	}
	if retryID != batchID || strings.Join(ids(retry), ",") != strings.Join(ids(first), ",") {
		t.Fatalf("retry = %s %v, want the original batch %s %v and nothing else", retryID, ids(retry), batchID, ids(first))
	}

	freshID, fresh, err := pgqueue.ClaimBatch(ctx, pool, table, 10)
	if err != nil || len(fresh) != 5 || freshID == batchID {
		t.Fatalf("next claim: batch=%q rows=%d err=%v, want the 5 new rows under a new batch", freshID, len(fresh), err)
	}

	if id, rows, err := pgqueue.ClaimBatch(ctx, pool, table, 10); err != nil || id != "" || len(rows) != 0 {
		t.Fatalf("empty queue: batch=%q rows=%d err=%v, want nothing", id, len(rows), err)
	}
}

// Several workers retrying at once must each get whole batches.
func TestConcurrentClaimsNeverSplitABatch(t *testing.T) {
	pool, table := queueTable(t)
	ctx := context.Background()

	// 20 batches of 5, all claimed once and put back for retry.
	want := map[string][]string{}
	for b := 0; b < 20; b++ {
		enqueue(t, pool, table, 5)
		batchID, rows, err := pgqueue.ClaimBatch(ctx, pool, table, 5)
		if err != nil || len(rows) != 5 {
			t.Fatalf("seeding batch %d: rows=%d err=%v", b, len(rows), err)
		}
		want[batchID] = ids(rows)
	}
	if _, err := pool.Exec(ctx, `UPDATE `+table+` SET status = 'queued', claimed_at = NULL`); err != nil {
		t.Fatalf("requeue all: %v", err)
	}

	var mu sync.Mutex
	got := map[string][]string{}
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				batchID, rows, err := pgqueue.ClaimBatch(ctx, pool, table, 5)
				if err != nil {
					t.Errorf("claim: %v", err)
					return
				}
				if len(rows) == 0 {
					return
				}
				mu.Lock()
				got[batchID] = append(got[batchID], ids(rows)...)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(got) != len(want) {
		t.Fatalf("claimed %d batches, want %d", len(got), len(want))
	}
	for batchID, rows := range got {
		sort.Strings(rows)
		if strings.Join(rows, ",") != strings.Join(want[batchID], ",") {
			t.Fatalf("batch %s claimed as %v, want exactly %v (split or mixed)", batchID, rows, want[batchID])
		}
	}
}
