package pixel_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/pixel"
)

func TestCreateGetUpdateDelete(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pixel.NewService(pixel.NewRepository(pool))

	created, err := svc.Create(ctx, orgID, pixel.CreateInput{
		Name: "Facebook — Sweeps conversions", Provider: pixel.ProviderFacebook, PixelID: "1029384756102938",
		Events: []string{"PWA_INSTALL", "CPA_ACCEPT"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Status != pixel.StatusActive {
		t.Fatalf("Status = %q, want active (the DB default)", created.Status)
	}
	if len(created.Events) != 2 {
		t.Fatalf("Events = %v, want 2 entries", created.Events)
	}

	got, err := svc.Get(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != created.Name {
		t.Fatalf("Get returned %+v, want the just-created row", got)
	}

	newName := "Facebook — Sweeps (renamed)"
	updated, err := svc.Update(ctx, orgID, created.ID, pixel.UpdateInput{Name: &newName})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != newName {
		t.Fatalf("Name = %q, want %q", updated.Name, newName)
	}
	if len(updated.Events) != 2 {
		t.Fatalf("Events after a name-only update = %v, want unchanged (2 entries)", updated.Events)
	}

	if err := svc.Delete(ctx, orgID, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, orgID, created.ID); err == nil {
		t.Fatal("Get after Delete succeeded, want not-found")
	}
}

func TestCreateAllowsEmptyPixelID(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pixel.NewService(pixel.NewRepository(pool))
	created, err := svc.Create(ctx, orgID, pixel.CreateInput{
		Name: "Generic — no id yet", Provider: pixel.ProviderGeneric, PixelID: "",
		Events: []string{"CPA_HOLD"},
	})
	if err != nil {
		t.Fatalf("Create with an empty pixelId failed, want success (pixelId is optional): %v", err)
	}
	if created.PixelID != "" {
		t.Fatalf("PixelID = %q, want empty", created.PixelID)
	}
}

func TestCreateRejectsInvalidShapes(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pixel.NewService(pixel.NewRepository(pool))

	if _, err := svc.Create(ctx, orgID, pixel.CreateInput{
		Name: "Bad Provider", Provider: "myspace", Events: []string{"TG_JOIN"},
	}); err == nil {
		t.Fatal("Create with an unrecognized provider succeeded, want a validation error")
	}
	if _, err := svc.Create(ctx, orgID, pixel.CreateInput{
		Name: "No Events", Provider: pixel.ProviderTikTok, Events: nil,
	}); err == nil {
		t.Fatal("Create with zero events succeeded, want a validation error")
	}
	if _, err := svc.Create(ctx, orgID, pixel.CreateInput{
		Name: "Bad Event", Provider: pixel.ProviderTikTok, Events: []string{"NOT_A_REAL_EVENT"},
	}); err == nil {
		t.Fatal("Create with an unrecognized event type succeeded, want a validation error")
	}
	if _, err := svc.Create(ctx, orgID, pixel.CreateInput{
		Name: "X", Provider: pixel.ProviderTikTok, Events: []string{"TG_JOIN"},
	}); err == nil {
		t.Fatal("Create with a 1-character name succeeded, want a validation error")
	}
}

func TestPauseActivateTransitions(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pixel.NewService(pixel.NewRepository(pool))
	created, err := svc.Create(ctx, orgID, pixel.CreateInput{
		Name: "Toggle Pixel", Provider: pixel.ProviderSnapchat, Events: []string{"TG_JOIN"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	paused, err := svc.Pause(ctx, orgID, created.ID)
	if err != nil || paused.Status != pixel.StatusPaused {
		t.Fatalf("Pause: got %+v, err %v", paused, err)
	}
	if again, err := svc.Pause(ctx, orgID, created.ID); err != nil || again.Status != pixel.StatusPaused {
		t.Fatalf("Pause (idempotent): got %+v, err %v", again, err)
	}

	active, err := svc.Activate(ctx, orgID, created.ID)
	if err != nil || active.Status != pixel.StatusActive {
		t.Fatalf("Activate: got %+v, err %v", active, err)
	}

	archived := pixel.StatusArchived
	if _, err := svc.Update(ctx, orgID, created.ID, pixel.UpdateInput{Status: &archived}); err != nil {
		t.Fatalf("archiving via Update: %v", err)
	}
	if _, err := svc.Pause(ctx, orgID, created.ID); err == nil {
		t.Fatal("Pause on an archived pixel succeeded, want a conflict error")
	}
	if _, err := svc.Activate(ctx, orgID, created.ID); err == nil {
		t.Fatal("Activate on an archived pixel succeeded, want a conflict error")
	}
}

func TestDuplicateKeepsStatus(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pixel.NewService(pixel.NewRepository(pool))
	created, err := svc.Create(ctx, orgID, pixel.CreateInput{
		Name: "Original", Provider: pixel.ProviderTwitter, PixelID: "abc123",
		Events: []string{"PWA_INSTALL", "TG_JOIN"},
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
	if dup.Status != pixel.StatusPaused {
		t.Fatalf("Status = %q, want paused (copied from the source, not reset)", dup.Status)
	}
	if dup.PixelID != created.PixelID {
		t.Fatalf("PixelID = %q, want %q (copied verbatim, no server-computed value here)", dup.PixelID, created.PixelID)
	}
	if len(dup.Events) != 2 {
		t.Fatalf("Events = %v, want 2 entries copied from the source", dup.Events)
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)

	svc := pixel.NewService(pixel.NewRepository(pool))
	created, err := svc.Create(ctx, orgA, pixel.CreateInput{
		Name: "Org A Pixel", Provider: pixel.ProviderGeneric, Events: []string{"TG_JOIN"},
	})
	if err != nil {
		t.Fatalf("creating for org A: %v", err)
	}

	t.Run("get", func(t *testing.T) {
		if _, err := svc.Get(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B fetched org A's pixel, want not-found")
		}
	})
	t.Run("update", func(t *testing.T) {
		name := "hijacked"
		if _, err := svc.Update(ctx, orgB, created.ID, pixel.UpdateInput{Name: &name}); err == nil {
			t.Fatal("org B updated org A's pixel, want not-found")
		}
	})
	t.Run("delete", func(t *testing.T) {
		if err := svc.Delete(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B deleted org A's pixel, want not-found")
		}
	})
	t.Run("list", func(t *testing.T) {
		pixels, err := svc.List(ctx, orgB)
		if err != nil {
			t.Fatalf("listing as org B: %v", err)
		}
		if len(pixels) != 0 {
			t.Fatalf("org B saw %d of org A's pixels, want 0", len(pixels))
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
