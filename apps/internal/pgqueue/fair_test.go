package pgqueue_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/pgqueue"
)

// enqueueFor adds n rows for org, the first created age ago and each later
// one a second newer.
func enqueueFor(t *testing.T, pool *pgxpool.Pool, table, org string, n int, age time.Duration) []string {
	t.Helper()
	var out []string
	for i := 0; i < n; i++ {
		id := idgen.New()
		at := time.Now().Add(-age).Add(time.Duration(i) * time.Second)
		if _, err := pool.Exec(context.Background(),
			`INSERT INTO `+table+` (id, organization_id, next_attempt_at) VALUES ($1, $2, $3)`, id, org, at); err != nil {
			t.Fatalf("enqueue for %s: %v", org, err)
		}
		out = append(out, id)
	}
	return out
}

func orgsOf(t *testing.T, pool *pgxpool.Pool, table string, rows []pgqueue.Row) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, r := range rows {
		var org string
		if err := pool.QueryRow(context.Background(), `SELECT organization_id FROM `+table+` WHERE id = $1`, r.ID).Scan(&org); err != nil {
			t.Fatal(err)
		}
		out[org]++
	}
	return out
}

// One organization's backlog, older than everyone else's rows, no longer
// fills the batch: each organization gets its share, the flood the rest.
func TestFairClaimDoesNotLetOneOrganizationFillTheBatch(t *testing.T) {
	pool, table := queueTable(t)
	ctx := context.Background()
	const flood, small, tiny = "01FLOOD0000000000000000000", "01SMALL0000000000000000000", "01TINY00000000000000000000"
	floodIDs := enqueueFor(t, pool, table, flood, 100, time.Hour)
	enqueueFor(t, pool, table, small, 3, time.Minute)
	enqueueFor(t, pool, table, tiny, 2, time.Minute)

	_, rows, err := pgqueue.ClaimBatch(ctx, pool, table, 10)
	if err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	got := orgsOf(t, pool, table, rows)
	if len(rows) != 10 || got[small] != 3 || got[tiny] != 2 || got[flood] != 5 {
		t.Fatalf("batch = %v (%d rows), want small 3, tiny 2 and the flood the other 5", got, len(rows))
	}
	// Within an organization, oldest first.
	claimed := map[string]bool{}
	for _, r := range rows {
		claimed[r.ID] = true
	}
	for _, id := range floodIDs[:5] {
		if !claimed[id] {
			t.Fatalf("the flood's 5 oldest rows were not the ones claimed")
		}
	}
}

// With more organizations than one claim's limit, the starting point moves:
// every organization is reached within a few claims, not only the first
// ones by id.
func TestFairClaimRotatesThroughOrganizations(t *testing.T) {
	pool, table := queueTable(t)
	ctx := context.Background()
	orgs := []string{"01ORGA0000000000000000000A", "01ORGB0000000000000000000B", "01ORGC0000000000000000000C",
		"01ORGD0000000000000000000D", "01ORGE0000000000000000000E"}
	for _, org := range orgs {
		enqueueFor(t, pool, table, org, 10, time.Minute)
	}

	var rot pgqueue.Rotation
	served := map[string]bool{}
	for claim := 0; claim < 3; claim++ {
		_, rows, err := pgqueue.ClaimBatchRotating(ctx, pool, table, 2, &rot)
		if err != nil || len(rows) != 2 {
			t.Fatalf("claim %d: %d rows, %v", claim, len(rows), err)
		}
		for org := range orgsOf(t, pool, table, rows) {
			served[org] = true
		}
	}
	if len(served) != len(orgs) {
		t.Fatalf("3 claims of 2 served %d organizations, want all 5", len(served))
	}
}
