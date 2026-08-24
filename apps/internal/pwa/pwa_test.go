package pwa_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/pwa"
)

func validInput(name string) pwa.CreateInput {
	return pwa.CreateInput{
		Name: name, ShortName: "Sweeps", ThemeColor: "#16a34a", BackgroundColor: "#0a0a0a",
		IconURL: "https://cdn.floxlink.io/pwa/sweeps/icon-512.png", StartURL: "/install/sweeps",
		BounceInAppWebview: true,
	}
}

func TestCreateGetUpdateDelete(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))

	created, err := svc.Create(ctx, orgID, validInput("Sweeps PWA"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Status != pwa.StatusActive {
		t.Fatalf("Status = %q, want active (the DB default)", created.Status)
	}

	got, err := svc.Get(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != created.Name {
		t.Fatalf("Get returned %+v, want the just-created row", got)
	}

	newName := "Sweeps PWA (renamed)"
	updated, err := svc.Update(ctx, orgID, created.ID, pwa.UpdateInput{Name: &newName})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != newName {
		t.Fatalf("Name = %q, want %q", updated.Name, newName)
	}

	if err := svc.Delete(ctx, orgID, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, orgID, created.ID); err == nil {
		t.Fatal("Get after Delete succeeded, want not-found")
	}
}

func TestCreateRejectsInvalidShapes(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))

	bad := validInput("Bad Theme")
	bad.ThemeColor = "green"
	if _, err := svc.Create(ctx, orgID, bad); err == nil {
		t.Fatal("Create with a non-hex theme color succeeded, want a validation error")
	}

	bad = validInput("Bad Icon")
	bad.IconURL = "not-a-url"
	if _, err := svc.Create(ctx, orgID, bad); err == nil {
		t.Fatal("Create with an invalid icon URL succeeded, want a validation error")
	}

	bad = validInput("Bad Short Name")
	bad.ShortName = ""
	if _, err := svc.Create(ctx, orgID, bad); err == nil {
		t.Fatal("Create with an empty short name succeeded, want a validation error")
	}

	bad = validInput("Bad Start URL")
	bad.StartURL = "   "
	if _, err := svc.Create(ctx, orgID, bad); err == nil {
		t.Fatal("Create with a blank start URL succeeded, want a validation error")
	}
}

func TestUpdateAcceptsRelativeStartURL(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))
	created, err := svc.Create(ctx, orgID, validInput("Relative Start"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// startUrl is a device-relative install path (e.g. "/install/sweeps"),
	// not a full URL — it must never be run through isValidURL the way
	// iconUrl is.
	relative := "/install/casino"
	updated, err := svc.Update(ctx, orgID, created.ID, pwa.UpdateInput{StartURL: &relative})
	if err != nil {
		t.Fatalf("Update with a relative start URL: %v", err)
	}
	if updated.StartURL != relative {
		t.Fatalf("StartURL = %q, want %q", updated.StartURL, relative)
	}
}

func TestPauseActivateTransitions(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))
	created, err := svc.Create(ctx, orgID, validInput("Toggle PWA"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	paused, err := svc.Pause(ctx, orgID, created.ID)
	if err != nil || paused.Status != pwa.StatusPaused {
		t.Fatalf("Pause: got %+v, err %v", paused, err)
	}
	if again, err := svc.Pause(ctx, orgID, created.ID); err != nil || again.Status != pwa.StatusPaused {
		t.Fatalf("Pause (idempotent): got %+v, err %v", again, err)
	}

	active, err := svc.Activate(ctx, orgID, created.ID)
	if err != nil || active.Status != pwa.StatusActive {
		t.Fatalf("Activate: got %+v, err %v", active, err)
	}

	archived := pwa.StatusArchived
	if _, err := svc.Update(ctx, orgID, created.ID, pwa.UpdateInput{Status: &archived}); err != nil {
		t.Fatalf("archiving via Update: %v", err)
	}
	if _, err := svc.Pause(ctx, orgID, created.ID); err == nil {
		t.Fatal("Pause on an archived pwa succeeded, want a conflict error")
	}
	if _, err := svc.Activate(ctx, orgID, created.ID); err == nil {
		t.Fatal("Activate on an archived pwa succeeded, want a conflict error")
	}
}

func TestDuplicateKeepsStatus(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))
	created, err := svc.Create(ctx, orgID, validInput("Original"))
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
	if dup.Status != pwa.StatusPaused {
		t.Fatalf("Status = %q, want paused (copied from the source, not reset)", dup.Status)
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))
	created, err := svc.Create(ctx, orgA, validInput("Org A PWA"))
	if err != nil {
		t.Fatalf("creating for org A: %v", err)
	}

	t.Run("get", func(t *testing.T) {
		if _, err := svc.Get(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B fetched org A's pwa, want not-found")
		}
	})
	t.Run("update", func(t *testing.T) {
		name := "hijacked"
		if _, err := svc.Update(ctx, orgB, created.ID, pwa.UpdateInput{Name: &name}); err == nil {
			t.Fatal("org B updated org A's pwa, want not-found")
		}
	})
	t.Run("delete", func(t *testing.T) {
		if err := svc.Delete(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B deleted org A's pwa, want not-found")
		}
	})
	t.Run("list", func(t *testing.T) {
		pwas, err := svc.List(ctx, orgB)
		if err != nil {
			t.Fatalf("listing as org B: %v", err)
		}
		if len(pwas) != 0 {
			t.Fatalf("org B saw %d of org A's pwas, want 0", len(pwas))
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
