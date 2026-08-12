package campaign_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/api/internal/campaign"
	"github.com/ismagilovnail/flox/apps/api/internal/idgen"
)

// TestCrossTenantIsolation is the DoD requirement CLAUDE.md calls out
// explicitly for API phases: "org A must receive zero rows from org B for
// every list/analytics endpoint." Runs against a real Postgres — set
// DATABASE_URL (matching .env.example / infra/docker-compose.dev.yml) to
// run it; skipped otherwise so `go test ./...` doesn't require a live DB.
func TestCrossTenantIsolation(t *testing.T) {
	ctx := context.Background()
	pool := mustPool(t)

	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)
	sourceA := seedTrafficSource(t, ctx, pool, orgA)
	sourceB := seedTrafficSource(t, ctx, pool, orgB)

	repo := campaign.NewRepository(pool)
	svc := campaign.NewService(repo)

	created, err := svc.Create(ctx, orgA, campaign.CreateInput{
		TrafficSourceID: sourceA,
		Name:            "Org A Campaign",
		FallbackURL:     "https://example.com/fallback",
	})
	if err != nil {
		t.Fatalf("creating campaign for org A: %v", err)
	}

	t.Run("list", func(t *testing.T) {
		result, err := svc.List(ctx, orgB, campaign.ListFilter{})
		if err != nil {
			t.Fatalf("listing as org B: %v", err)
		}
		if len(result.Campaigns) != 0 || result.Total != 0 {
			t.Fatalf("org B saw %d of org A's campaigns, want 0", len(result.Campaigns))
		}
	})

	t.Run("get by id", func(t *testing.T) {
		_, err := svc.Get(ctx, orgB, created.ID)
		if err == nil {
			t.Fatal("org B fetched org A's campaign by id, want not-found")
		}
	})

	t.Run("update", func(t *testing.T) {
		name := "hijacked"
		_, err := svc.Update(ctx, orgB, created.ID, campaign.UpdateInput{Name: &name})
		if err == nil {
			t.Fatal("org B updated org A's campaign, want not-found")
		}
	})

	t.Run("delete", func(t *testing.T) {
		if err := svc.Delete(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B deleted org A's campaign, want not-found")
		}
	})

	t.Run("cannot attach a campaign to another org's traffic source", func(t *testing.T) {
		_, err := svc.Create(ctx, orgA, campaign.CreateInput{
			TrafficSourceID: sourceB,
			Name:            "Cross-org attempt",
			FallbackURL:     "https://example.com/fallback",
		})
		if err == nil {
			t.Fatal("org A created a campaign against org B's traffic source, want validation error")
		}
	})

	// Org A's own view is unaffected by any of the above.
	result, err := svc.List(ctx, orgA, campaign.ListFilter{})
	if err != nil {
		t.Fatalf("listing as org A: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("org A has %d campaigns, want 1", result.Total)
	}
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
	// Registered before any seedOrg cleanup, so t.Cleanup's LIFO order runs
	// the org-delete cleanups first and closes the pool last — a plain
	// `defer pool.Close()` in the test body would close it before
	// t.Cleanup callbacks run at all, silently no-op'ing every cleanup
	// query below.
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

func seedTrafficSource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID string) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO traffic_sources (id, organization_id, name, type) VALUES ($1, $2, $3, $4)`,
		id, orgID, "Test Source", "Facebook",
	)
	if err != nil {
		t.Fatalf("seeding traffic source: %v", err)
	}
	return id
}
