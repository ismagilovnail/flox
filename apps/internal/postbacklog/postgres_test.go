package postbacklog_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/postbacklog"
)

func TestPostgresQueueEnqueueClaimAndDelete(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	q := postbacklog.NewPostgresQueue(pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	clickID := idgen.New()
	q.EnqueueAttempt(ctx, chstore.PostbackAttempt{OrganizationID: orgID, Direction: "incoming", ClickID: clickID, Result: "success"})
	q.EnqueueAttempt(ctx, chstore.PostbackAttempt{OrganizationID: orgID, Direction: "outgoing", ClickID: clickID, Result: "success"})

	claimed, err := q.ClaimDue(ctx, 100)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	var mine []postbacklog.QueuedAttempt
	for _, c := range claimed {
		if c.Attempt.ClickID == clickID {
			mine = append(mine, c)
		}
	}
	if len(mine) != 2 {
		t.Fatalf("claimed = %d matching attempts, want 2", len(mine))
	}

	directions := map[string]bool{}
	for _, c := range mine {
		directions[c.Attempt.Direction] = true
		if c.Attempt.OrganizationID != orgID {
			t.Fatalf("claimed attempt organization_id = %q, want %q", c.Attempt.OrganizationID, orgID)
		}
	}
	if !directions["incoming"] || !directions["outgoing"] {
		t.Fatalf("directions = %v, want both incoming and outgoing", directions)
	}

	ids := make([]string, len(mine))
	for i, c := range mine {
		ids[i] = c.ID
	}
	if err := q.Delete(ctx, ids); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM postback_attempt_queue WHERE payload->>'ClickID' = $1`, clickID).Scan(&count); err != nil {
		t.Fatalf("counting remaining rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("rows remaining after Delete = %d, want 0", count)
	}
}

func TestPostgresQueueRequeue(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	q := postbacklog.NewPostgresQueue(pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	clickID := idgen.New()

	q.EnqueueAttempt(ctx, chstore.PostbackAttempt{OrganizationID: orgID, Direction: "incoming", ClickID: clickID})
	claimed, err := q.ClaimDue(ctx, 100)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	var id string
	for _, c := range claimed {
		if c.Attempt.ClickID == clickID {
			id = c.ID
		}
	}
	if id == "" {
		t.Fatal("did not claim the enqueued attempt")
	}

	if err := q.Requeue(ctx, []string{id}, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("Requeue: %v", err)
	}
	notDue, err := q.ClaimDue(ctx, 1000)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	for _, c := range notDue {
		if c.ID == id {
			t.Fatal("claimed an attempt requeued an hour into the future")
		}
	}

	if err := q.Requeue(ctx, []string{id}, time.Now().UTC().Add(-time.Second)); err != nil {
		t.Fatalf("Requeue (backdated): %v", err)
	}
	due, err := q.ClaimDue(ctx, 1000)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	found := false
	for _, c := range due {
		if c.ID == id {
			found = true
		}
	}
	if !found {
		t.Fatal("requeued attempt with a past next_attempt_at was not claimed")
	}
	_ = q.Delete(ctx, []string{id})
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
