package campaign_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/campaign"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
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

// TestExternalCampaignID covers the ad-spend sync match column (§74/
// §27-COST, migration 00019): round-trips through Create/Update, and
// ListByExternalID scopes to (org, trafficSource) and returns every
// matching campaign (no uniqueness constraint — two campaigns can
// deliberately share one ad-platform campaign id).
func TestExternalCampaignID(t *testing.T) {
	ctx := context.Background()
	pool := mustPool(t)
	orgID := seedOrg(t, ctx, pool)
	sourceID := seedTrafficSource(t, ctx, pool, orgID)

	repo := campaign.NewRepository(pool)
	svc := campaign.NewService(repo)

	created, err := svc.Create(ctx, orgID, campaign.CreateInput{
		TrafficSourceID:    sourceID,
		Name:               "With External ID",
		FallbackURL:        "https://example.com/fallback",
		ExternalCampaignID: "  fb_campaign_123  ",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ExternalCampaignID != "fb_campaign_123" {
		t.Fatalf("ExternalCampaignID = %q, want trimmed fb_campaign_123", created.ExternalCampaignID)
	}

	found, err := repo.ListByExternalID(ctx, orgID, sourceID, "fb_campaign_123")
	if err != nil {
		t.Fatalf("ListByExternalID: %v", err)
	}
	if len(found) != 1 || found[0].ID != created.ID {
		t.Fatalf("ListByExternalID = %+v, want exactly the created campaign", found)
	}

	// A second campaign deliberately sharing the same external id: both
	// come back, no uniqueness constraint silently drops one.
	second, err := svc.Create(ctx, orgID, campaign.CreateInput{
		TrafficSourceID:    sourceID,
		Name:               "Shares The Same External ID",
		FallbackURL:        "https://example.com/fallback",
		ExternalCampaignID: "fb_campaign_123",
	})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}
	found, err = repo.ListByExternalID(ctx, orgID, sourceID, "fb_campaign_123")
	if err != nil {
		t.Fatalf("ListByExternalID after second create: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("ListByExternalID = %d campaigns, want 2 (%q and %q)", len(found), created.ID, second.ID)
	}

	// Scoped to trafficSourceID: a different source's campaign with the
	// same external id never shows up here.
	otherSource := seedTrafficSource(t, ctx, pool, orgID)
	if _, err := svc.Create(ctx, orgID, campaign.CreateInput{
		TrafficSourceID:    otherSource,
		Name:               "Different Source, Same External ID",
		FallbackURL:        "https://example.com/fallback",
		ExternalCampaignID: "fb_campaign_123",
	}); err != nil {
		t.Fatalf("Create under other source: %v", err)
	}
	found, err = repo.ListByExternalID(ctx, orgID, sourceID, "fb_campaign_123")
	if err != nil {
		t.Fatalf("ListByExternalID after other-source create: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("ListByExternalID leaked a campaign from a different traffic source: got %d, want 2", len(found))
	}

	// Update also round-trips it.
	newExternalID := "fb_campaign_456"
	updated, err := svc.Update(ctx, orgID, created.ID, campaign.UpdateInput{ExternalCampaignID: &newExternalID})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.ExternalCampaignID != "fb_campaign_456" {
		t.Fatalf("ExternalCampaignID after Update = %q, want fb_campaign_456", updated.ExternalCampaignID)
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
