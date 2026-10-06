package postback_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/postback"
	"github.com/ismagilovnail/flox/apps/internal/testpg"
)

func TestPostgresStoreReleaseAndReapStuck(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID)
	table := testpg.PrivateTable(t, pool, "postback_deliveries")
	store := postback.NewPostgresStoreOnTable(pool, table)

	// Rows are seeded straight into 'processing' (attempt 1, claimed at the
	// given age): a due 'queued' row can be claimed by a dev-stack worker
	// polling the same table mid-test.
	seed := func(age string) string {
		clickID := idgen.New()
		sourceID := seedPostback(t, ctx, pool, orgID, networkID, clickID, "CPA_ACCEPT")
		id := idgen.New()
		if _, err := pool.Exec(ctx, `
			INSERT INTO `+table+`
			  (id, organization_id, network_id, source_postback_id, click_id, status, url,
			   delivery_status, attempt_count, claimed_at)
			VALUES ($1, $2, $3, $4, $5, 'CPA_ACCEPT', 'https://net.example/pb', 'processing', 1, now() - $6::interval)`,
			id, orgID, networkID, sourceID, clickID, age,
		); err != nil {
			t.Fatalf("seeding claimed delivery: %v", err)
		}
		return id
	}
	state := func(id string) (status string, attempts int, claimed bool) {
		var claimedAt *time.Time
		if err := pool.QueryRow(ctx,
			`SELECT delivery_status, attempt_count, claimed_at FROM `+table+` WHERE id = $1`, id,
		).Scan(&status, &attempts, &claimedAt); err != nil {
			t.Fatalf("reading delivery %s: %v", id, err)
		}
		return status, attempts, claimedAt != nil
	}

	released, reaped, fresh := seed("1 second"), seed("10 minutes"), seed("1 second")

	// Release: never attempted -> back to queued, attempt count undone.
	if err := store.Release(ctx, []string{released}); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if s, n, c := state(released); s != "queued" || n != 0 || c {
		t.Fatalf("after release: status=%s attempts=%d claimed=%v, want queued/0/false", s, n, c)
	}

	// Reap: only claims older than the window, attempt kept, retried.
	n, err := store.ReapStuck(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("ReapStuck: %v", err)
	}
	if n < 1 {
		t.Fatalf("ReapStuck moved %d rows, want at least 1", n)
	}
	if s, a, c := state(reaped); s != "retrying" || a != 1 || c {
		t.Fatalf("after reap: status=%s attempts=%d claimed=%v, want retrying/1/false", s, a, c)
	}
	if s, _, _ := state(fresh); s != "processing" {
		t.Fatalf("a fresh claim was reaped: status=%s", s)
	}
	_ = store.Release(ctx, []string{fresh})
}
