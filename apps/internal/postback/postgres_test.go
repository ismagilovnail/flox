package postback_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/postback"
)

func TestPostgresStoreEnqueueAndClaim(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID)
	sourceID := seedPostback(t, ctx, pool, orgID, networkID, "click-1", "CPA_ACCEPT")

	store := postback.NewPostgresStore(pool)
	id, err := store.Enqueue(ctx, postback.EnqueueInput{
		OrganizationID: orgID, NetworkID: networkID, SourcePostbackID: sourceID,
		ClickID: "click-1", Status: event.CpaAccept, URL: "https://net.example/pb?click_id=click-1",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if id == "" {
		t.Fatal("Enqueue returned an empty id")
	}

	claimed, err := store.ClaimDue(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed = %d, want 1", len(claimed))
	}
	d := claimed[0]
	if d.ID != id || d.OrganizationID != orgID || d.NetworkID != networkID || d.ClickID != "click-1" || d.Status != event.CpaAccept {
		t.Fatalf("claimed delivery = %+v, want matching the enqueued one", d)
	}
	if d.AttemptCount != 1 {
		t.Fatalf("AttemptCount = %d, want 1 (ClaimDue increments)", d.AttemptCount)
	}

	// A freshly-claimed (now 'processing') delivery is not due again.
	claimedAgain, err := store.ClaimDue(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimDue (second): %v", err)
	}
	if len(claimedAgain) != 0 {
		t.Fatalf("claimed a 'processing' delivery a second time: %+v", claimedAgain)
	}
}

func TestPostgresStoreMarkRetryingIsDueAtNextAttemptAt(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID)
	sourceID := seedPostback(t, ctx, pool, orgID, networkID, "click-2", "CPA_ACCEPT")

	store := postback.NewPostgresStore(pool)
	id, err := store.Enqueue(ctx, postback.EnqueueInput{
		OrganizationID: orgID, NetworkID: networkID, SourcePostbackID: sourceID,
		ClickID: "click-2", Status: event.CpaAccept, URL: "https://net.example/pb",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	claimed, err := store.ClaimDue(ctx, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("ClaimDue: claimed=%d err=%v", len(claimed), err)
	}

	future := time.Now().UTC().Add(time.Hour)
	if err := store.MarkRetrying(ctx, id, 500, "network responded 500", future); err != nil {
		t.Fatalf("MarkRetrying: %v", err)
	}

	// Not due yet.
	stillNotDue, err := store.ClaimDue(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if len(stillNotDue) != 0 {
		t.Fatalf("claimed a delivery whose next_attempt_at is an hour out: %+v", stillNotDue)
	}

	// Push it into the past and it becomes due.
	if err := store.MarkRetrying(ctx, id, 500, "network responded 500", time.Now().UTC().Add(-time.Second)); err != nil {
		t.Fatalf("MarkRetrying (backdated): %v", err)
	}
	dueNow, err := store.ClaimDue(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if len(dueNow) != 1 || dueNow[0].AttemptCount != 2 {
		t.Fatalf("claimed = %+v, want one delivery on its second attempt", dueNow)
	}
}

func TestPostgresStoreMarkSuccessAndDead(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID)
	store := postback.NewPostgresStore(pool)

	src1 := seedPostback(t, ctx, pool, orgID, networkID, "click-3", "CPA_ACCEPT")
	id1, _ := store.Enqueue(ctx, postback.EnqueueInput{OrganizationID: orgID, NetworkID: networkID, SourcePostbackID: src1, ClickID: "click-3", Status: event.CpaAccept, URL: "https://net.example/pb"})
	src2 := seedPostback(t, ctx, pool, orgID, networkID, "click-4", "CPA_ACCEPT")
	id2, _ := store.Enqueue(ctx, postback.EnqueueInput{OrganizationID: orgID, NetworkID: networkID, SourcePostbackID: src2, ClickID: "click-4", Status: event.CpaAccept, URL: "https://net.example/pb"})

	if _, err := store.ClaimDue(ctx, 10); err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}

	if err := store.MarkSuccess(ctx, id1, 200); err != nil {
		t.Fatalf("MarkSuccess: %v", err)
	}
	if err := store.MarkDead(ctx, id2, 500, "exhausted retries"); err != nil {
		t.Fatalf("MarkDead: %v", err)
	}

	// Neither a success nor a dead delivery is ever due again.
	claimed, err := store.ClaimDue(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if len(claimed) != 0 {
		t.Fatalf("claimed terminal deliveries: %+v", claimed)
	}

	var deliveryStatus1, deliveryStatus2 string
	pool.QueryRow(ctx, `SELECT delivery_status FROM postback_deliveries WHERE id = $1`, id1).Scan(&deliveryStatus1)
	pool.QueryRow(ctx, `SELECT delivery_status FROM postback_deliveries WHERE id = $1`, id2).Scan(&deliveryStatus2)
	if deliveryStatus1 != "success" {
		t.Fatalf("delivery 1 status = %q, want success", deliveryStatus1)
	}
	if deliveryStatus2 != "dead" {
		t.Fatalf("delivery 2 status = %q, want dead", deliveryStatus2)
	}
}

func TestPostgresStoreClaimRespectsLimit(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID)
	store := postback.NewPostgresStore(pool)

	for i := 0; i < 5; i++ {
		clickID := "click-limit-" + idgen.New()
		src := seedPostback(t, ctx, pool, orgID, networkID, clickID, "CPA_ACCEPT")
		if _, err := store.Enqueue(ctx, postback.EnqueueInput{
			OrganizationID: orgID, NetworkID: networkID, SourcePostbackID: src,
			ClickID: clickID, Status: event.CpaAccept, URL: "https://net.example/pb",
		}); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}

	claimed, err := store.ClaimDue(ctx, 3)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if len(claimed) != 3 {
		t.Fatalf("claimed = %d, want exactly the limit of 3", len(claimed))
	}
}

func seedNetwork(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID string) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx, `INSERT INTO networks (id, organization_id, name) VALUES ($1, $2, $3)`, id, orgID, "Test Network")
	if err != nil {
		t.Fatalf("seeding network: %v", err)
	}
	return id
}

// seedPostback inserts the minimal incoming postbacks row postback_deliveries'
// source_postback_id FK requires — internal/postback never writes to
// `postbacks` itself, only reads its id back via internal/conversion's
// success path in production.
func seedPostback(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID, networkID, clickID, status string) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO postbacks (id, organization_id, network_id, click_id, status, direction, result)
		 VALUES ($1, $2, $3, $4, $5, 'incoming', 'success')`,
		id, orgID, networkID, clickID, status,
	)
	if err != nil {
		t.Fatalf("seeding source postback: %v", err)
	}
	return id
}

func mustPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connecting to database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func seedOrg(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, id, "Test Org "+id)
	if err != nil {
		t.Fatalf("seeding organization: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, id)
	})
	return id
}
