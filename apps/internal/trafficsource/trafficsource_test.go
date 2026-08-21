package trafficsource_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/trafficsource"
)

func TestListReturnsOrgsOwnSourcesOrderedByName(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)

	seedSource(t, ctx, pool, orgA, "Zulu Ads", "Facebook")
	seedSource(t, ctx, pool, orgA, "Alpha Ads", "TikTok")
	seedSource(t, ctx, pool, orgB, "Org B Source", "Google")

	repo := trafficsource.NewRepository(pool)
	sources, err := repo.List(ctx, orgA)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("sources = %d, want 2 (org A's own only)", len(sources))
	}
	if sources[0].Name != "Alpha Ads" || sources[1].Name != "Zulu Ads" {
		t.Fatalf("sources = %+v, want alphabetical order", sources)
	}
	for _, s := range sources {
		if s.ID == "" || s.Type == "" || s.Status == "" {
			t.Fatalf("source missing a field: %+v", s)
		}
	}
}

func TestListEmptyIsEmptySliceNotNil(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	repo := trafficsource.NewRepository(pool)
	sources, err := repo.List(ctx, orgID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if sources == nil {
		t.Fatal("List returned nil, want an empty (non-nil) slice so JSON encodes [] not null")
	}
	if len(sources) != 0 {
		t.Fatalf("sources = %d, want 0", len(sources))
	}
}

func TestCreateGetUpdateDelete(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := trafficsource.NewService(trafficsource.NewRepository(pool))

	created, err := svc.Create(ctx, orgID, trafficsource.CreateInput{
		Name:             "Facebook Ads",
		Type:             "Facebook",
		TrackingTemplate: "https://track.example.com/click?clickid={click_id}",
		CostIntegration:  trafficsource.CostIntegrationManual,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Status != trafficsource.StatusActive {
		t.Fatalf("Status = %q, want active (the DB default)", created.Status)
	}

	got, err := svc.Get(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != created.Name {
		t.Fatalf("Get returned %+v, want the just-created row", got)
	}

	newName := "Facebook Ads (renamed)"
	updated, err := svc.Update(ctx, orgID, created.ID, trafficsource.UpdateInput{Name: &newName})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != newName {
		t.Fatalf("Name = %q, want %q", updated.Name, newName)
	}
	if updated.Type != created.Type {
		t.Fatalf("Update touched Type (%q), want it untouched by a name-only patch", updated.Type)
	}

	if err := svc.Delete(ctx, orgID, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, orgID, created.ID); err == nil {
		t.Fatal("Get after Delete succeeded, want not-found")
	}
}

func TestCreateRejectsInvalidTrackingTemplate(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := trafficsource.NewService(trafficsource.NewRepository(pool))
	_, err := svc.Create(ctx, orgID, trafficsource.CreateInput{
		Name:             "Bad Source",
		Type:             "Other",
		TrackingTemplate: "not-a-url",
	})
	if err == nil {
		t.Fatal("Create with an invalid tracking template succeeded, want a validation error")
	}
}

func TestPauseActivateTransitions(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := trafficsource.NewService(trafficsource.NewRepository(pool))
	created, err := svc.Create(ctx, orgID, trafficsource.CreateInput{
		Name: "Toggle Source", Type: "Other", TrackingTemplate: "https://example.com/t",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	paused, err := svc.Pause(ctx, orgID, created.ID)
	if err != nil || paused.Status != trafficsource.StatusPaused {
		t.Fatalf("Pause: got %+v, err %v", paused, err)
	}
	// Idempotent from the target state.
	if again, err := svc.Pause(ctx, orgID, created.ID); err != nil || again.Status != trafficsource.StatusPaused {
		t.Fatalf("Pause (idempotent): got %+v, err %v", again, err)
	}

	active, err := svc.Activate(ctx, orgID, created.ID)
	if err != nil || active.Status != trafficsource.StatusActive {
		t.Fatalf("Activate: got %+v, err %v", active, err)
	}

	archived := trafficsource.StatusArchived
	if _, err := svc.Update(ctx, orgID, created.ID, trafficsource.UpdateInput{Status: &archived}); err != nil {
		t.Fatalf("archiving via Update: %v", err)
	}
	if _, err := svc.Pause(ctx, orgID, created.ID); err == nil {
		t.Fatal("Pause on an archived source succeeded, want a conflict error")
	}
	if _, err := svc.Activate(ctx, orgID, created.ID); err == nil {
		t.Fatal("Activate on an archived source succeeded, want a conflict error")
	}
}

func TestDuplicateKeepsStatus(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := trafficsource.NewService(trafficsource.NewRepository(pool))
	created, err := svc.Create(ctx, orgID, trafficsource.CreateInput{
		Name: "Original", Type: "Push", TrackingTemplate: "https://example.com/t",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Pause(ctx, orgID, created.ID); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	dup, err := svc.Duplicate(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if dup.ID == created.ID {
		t.Fatal("Duplicate returned the same id as the source")
	}
	if dup.Name != "Original (Copy)" {
		t.Fatalf("Name = %q, want %q", dup.Name, "Original (Copy)")
	}
	if dup.Status != trafficsource.StatusPaused {
		t.Fatalf("Status = %q, want paused (copied from the source, not reset)", dup.Status)
	}
}

func TestDeleteConflictsWhenReferencedByACampaign(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := trafficsource.NewService(trafficsource.NewRepository(pool))
	created, err := svc.Create(ctx, orgID, trafficsource.CreateInput{
		Name: "In Use", Type: "Other", TrackingTemplate: "https://example.com/t",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO campaigns (id, organization_id, traffic_source_id, name, fallback_url) VALUES ($1, $2, $3, $4, $5)`,
		idgen.New(), orgID, created.ID, "Referencing Campaign", "https://example.com/fallback",
	)
	if err != nil {
		t.Fatalf("seeding referencing campaign: %v", err)
	}

	if err := svc.Delete(ctx, orgID, created.ID); err == nil {
		t.Fatal("Delete on a referenced traffic source succeeded, want a conflict error")
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)

	svc := trafficsource.NewService(trafficsource.NewRepository(pool))
	created, err := svc.Create(ctx, orgA, trafficsource.CreateInput{
		Name: "Org A Source", Type: "Facebook", TrackingTemplate: "https://example.com/t",
	})
	if err != nil {
		t.Fatalf("creating for org A: %v", err)
	}

	t.Run("get", func(t *testing.T) {
		if _, err := svc.Get(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B fetched org A's traffic source, want not-found")
		}
	})

	t.Run("update", func(t *testing.T) {
		name := "hijacked"
		if _, err := svc.Update(ctx, orgB, created.ID, trafficsource.UpdateInput{Name: &name}); err == nil {
			t.Fatal("org B updated org A's traffic source, want not-found")
		}
	})

	t.Run("delete", func(t *testing.T) {
		if err := svc.Delete(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B deleted org A's traffic source, want not-found")
		}
	})

	t.Run("list", func(t *testing.T) {
		sources, err := svc.List(ctx, orgB)
		if err != nil {
			t.Fatalf("listing as org B: %v", err)
		}
		if len(sources) != 0 {
			t.Fatalf("org B saw %d of org A's traffic sources, want 0", len(sources))
		}
	})
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

func seedSource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID, name, typ string) {
	t.Helper()
	_, err := pool.Exec(ctx,
		`INSERT INTO traffic_sources (id, organization_id, name, type) VALUES ($1, $2, $3, $4)`,
		idgen.New(), orgID, name, typ,
	)
	if err != nil {
		t.Fatalf("seeding traffic source: %v", err)
	}
}
