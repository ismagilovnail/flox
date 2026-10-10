package pgqueue_test

import (
	"context"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

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
		  organization_id text NOT NULL DEFAULT '`+defaultOrg+`',
		  status text NOT NULL DEFAULT 'queued',
		  next_attempt_at timestamptz NOT NULL DEFAULT now(),
		  claimed_at timestamptz, batch_id text,
		  attempts integer NOT NULL DEFAULT 0, last_error text NOT NULL DEFAULT '', dead_at timestamptz)`); err != nil {
		t.Fatalf("creating %s: %v", table, err)
	}
	// As migration 00066's fair claim index.
	if _, err := pool.Exec(context.Background(), `CREATE INDEX ON `+table+` (organization_id, next_attempt_at, id)
		WHERE status = 'queued' AND batch_id IS NULL`); err != nil {
		t.Fatalf("indexing %s: %v", table, err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DROP TABLE `+table) })
	return pool, table
}

// defaultOrg owns rows enqueue adds; fairness tests name their own.
const defaultOrg = "01TESTORGANIZATION00000000"

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

// Audit 2026-10-06, Q2: an outage backs off but never dead-letters; a
// batch the server keeps rejecting is set aside after MaxRejectedAttempts.
func TestFailBacksOffAndDeadLettersOnlyRejections(t *testing.T) {
	pool, table := queueTable(t)
	ctx := context.Background()

	state := func(id string) (status string, attempts int, wait time.Duration, lastError string, dead bool) {
		var next time.Time
		var deadAt *time.Time
		if err := pool.QueryRow(ctx, `SELECT status, attempts, next_attempt_at, last_error, dead_at FROM `+table+` WHERE id = $1`, id).
			Scan(&status, &attempts, &next, &lastError, &deadAt); err != nil {
			t.Fatalf("reading row: %v", err)
		}
		return status, attempts, time.Until(next), lastError, deadAt != nil
	}

	enqueue(t, pool, table, 2)
	var transient, rejected string
	if err := pool.QueryRow(ctx, `SELECT min(id), max(id) FROM `+table).Scan(&transient, &rejected); err != nil {
		t.Fatalf("ids: %v", err)
	}

	for i := 1; i <= 12; i++ {
		dead, err := pgqueue.Fail(ctx, pool, table, []string{transient}, false, "connection refused")
		if err != nil || dead {
			t.Fatalf("transient attempt %d: dead=%v err=%v, want retried", i, dead, err)
		}
		_, _, wait, _, _ := state(transient)
		if want := min(10*time.Second<<(i-1), 10*time.Minute); wait > want+time.Second || wait < want-5*time.Second {
			t.Fatalf("transient attempt %d: next try in %s, want about %s", i, wait.Round(time.Second), want)
		}
	}
	if status, attempts, _, _, _ := state(transient); status != "queued" || attempts != 12 {
		t.Fatalf("after 12 transient failures: %s/%d, want still queued", status, attempts)
	}

	for i := 1; i <= pgqueue.MaxRejectedAttempts; i++ {
		dead, err := pgqueue.Fail(ctx, pool, table, []string{rejected}, true, "Code: 53. DB::Exception: Type mismatch")
		if err != nil {
			t.Fatalf("rejected attempt %d: %v", i, err)
		}
		if want := i == pgqueue.MaxRejectedAttempts; dead != want {
			t.Fatalf("rejected attempt %d: dead=%v, want %v", i, dead, want)
		}
	}
	status, _, _, lastError, deadAt := state(rejected)
	if status != "dead" || !deadAt || lastError != "Code: 53. DB::Exception: Type mismatch" {
		t.Fatalf("rejected batch: status=%s dead_at=%v last_error=%q, want dead with its error kept", status, deadAt, lastError)
	}

	// A dead batch is never claimed again on its own.
	if _, err := pool.Exec(ctx, `UPDATE `+table+` SET next_attempt_at = now() - interval '1 hour'`); err != nil {
		t.Fatalf("making everything due: %v", err)
	}
	_, rows, err := pgqueue.ClaimBatch(ctx, pool, table, 10)
	if err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	for _, r := range rows {
		if r.ID == rejected {
			t.Fatal("a dead-lettered batch was claimed")
		}
	}
}
