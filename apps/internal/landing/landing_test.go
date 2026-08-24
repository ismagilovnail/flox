package landing_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/landing"
)

func TestCreateGetUpdateDelete(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := landing.NewService(landing.NewRepository(pool))

	created, err := svc.Create(ctx, orgID, landing.CreateInput{
		Name: "Quiz Lander", Type: landing.TypeInternal, Content: "<h1>Quiz</h1>",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Status != landing.StatusActive {
		t.Fatalf("Status = %q, want active (the DB default)", created.Status)
	}

	got, err := svc.Get(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != created.Name {
		t.Fatalf("Get returned %+v, want the just-created row", got)
	}

	newName := "Quiz Lander (renamed)"
	updated, err := svc.Update(ctx, orgID, created.ID, landing.UpdateInput{Name: &newName})
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

// TestInternalLandingURLIsServerComputed exercises the one piece of real
// business logic this domain has: an internal landing's URL is always
// derived from its name server-side (§28), never trusted from the client,
// and stays in sync as the name changes.
func TestInternalLandingURLIsServerComputed(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := landing.NewService(landing.NewRepository(pool))

	created, err := svc.Create(ctx, orgID, landing.CreateInput{
		Name: "Sweeps Quiz!!", Type: landing.TypeInternal, Content: "<h1>Go</h1>",
		URL: "https://attacker.example/not-trusted", // must be ignored for internal
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if want := "https://cdn.floxlink.io/lnd/sweeps-quiz"; created.URL != want {
		t.Fatalf("URL = %q, want %q (slugified from name, client URL ignored)", created.URL, want)
	}

	renamed := "Brand New Name"
	updated, err := svc.Update(ctx, orgID, created.ID, landing.UpdateInput{Name: &renamed})
	if err != nil {
		t.Fatalf("Update (rename): %v", err)
	}
	if want := "https://cdn.floxlink.io/lnd/brand-new-name"; updated.URL != want {
		t.Fatalf("URL after rename = %q, want %q", updated.URL, want)
	}

	// A status-only update (the shape Pause/Activate/Duplicate's
	// preserve-status follow-up all send) must not touch url/content at
	// all — recomputing on every unrelated PATCH would be wasteful, not
	// just untested.
	paused := landing.StatusPaused
	statusOnly, err := svc.Update(ctx, orgID, created.ID, landing.UpdateInput{Status: &paused})
	if err != nil {
		t.Fatalf("Update (status only): %v", err)
	}
	if statusOnly.URL != updated.URL {
		t.Fatalf("URL changed on a status-only update: got %q, want unchanged %q", statusOnly.URL, updated.URL)
	}
}

func TestCreateRejectsInvalidShapes(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := landing.NewService(landing.NewRepository(pool))

	if _, err := svc.Create(ctx, orgID, landing.CreateInput{Name: "Bad External", Type: landing.TypeExternal, URL: "not-a-url"}); err == nil {
		t.Fatal("Create external with an invalid URL succeeded, want a validation error")
	}
	if _, err := svc.Create(ctx, orgID, landing.CreateInput{Name: "Bad Internal", Type: landing.TypeInternal, Content: "   "}); err == nil {
		t.Fatal("Create internal with blank content succeeded, want a validation error")
	}
	if _, err := svc.Create(ctx, orgID, landing.CreateInput{Name: "X", Type: landing.TypeInternal, Content: "ok"}); err == nil {
		t.Fatal("Create with a 1-character name succeeded, want a validation error")
	}
}

func TestPauseActivateTransitions(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := landing.NewService(landing.NewRepository(pool))
	created, err := svc.Create(ctx, orgID, landing.CreateInput{Name: "Toggle Lander", Type: landing.TypeExternal, URL: "https://example.com/lnd"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	paused, err := svc.Pause(ctx, orgID, created.ID)
	if err != nil || paused.Status != landing.StatusPaused {
		t.Fatalf("Pause: got %+v, err %v", paused, err)
	}
	if again, err := svc.Pause(ctx, orgID, created.ID); err != nil || again.Status != landing.StatusPaused {
		t.Fatalf("Pause (idempotent): got %+v, err %v", again, err)
	}

	active, err := svc.Activate(ctx, orgID, created.ID)
	if err != nil || active.Status != landing.StatusActive {
		t.Fatalf("Activate: got %+v, err %v", active, err)
	}

	archived := landing.StatusArchived
	if _, err := svc.Update(ctx, orgID, created.ID, landing.UpdateInput{Status: &archived}); err != nil {
		t.Fatalf("archiving via Update: %v", err)
	}
	if _, err := svc.Pause(ctx, orgID, created.ID); err == nil {
		t.Fatal("Pause on an archived landing succeeded, want a conflict error")
	}
	if _, err := svc.Activate(ctx, orgID, created.ID); err == nil {
		t.Fatal("Activate on an archived landing succeeded, want a conflict error")
	}
}

func TestDuplicateKeepsStatusAndRecomputesInternalURL(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := landing.NewService(landing.NewRepository(pool))
	created, err := svc.Create(ctx, orgID, landing.CreateInput{Name: "Original", Type: landing.TypeInternal, Content: "<p>hi</p>"})
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
	if dup.Status != landing.StatusPaused {
		t.Fatalf("Status = %q, want paused (copied from the source, not reset)", dup.Status)
	}
	if want := "https://cdn.floxlink.io/lnd/original-copy"; dup.URL != want {
		t.Fatalf("URL = %q, want %q (recomputed for the new name, not copied verbatim)", dup.URL, want)
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)

	svc := landing.NewService(landing.NewRepository(pool))
	created, err := svc.Create(ctx, orgA, landing.CreateInput{Name: "Org A Lander", Type: landing.TypeExternal, URL: "https://example.com/lnd"})
	if err != nil {
		t.Fatalf("creating for org A: %v", err)
	}

	t.Run("get", func(t *testing.T) {
		if _, err := svc.Get(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B fetched org A's landing, want not-found")
		}
	})
	t.Run("update", func(t *testing.T) {
		name := "hijacked"
		if _, err := svc.Update(ctx, orgB, created.ID, landing.UpdateInput{Name: &name}); err == nil {
			t.Fatal("org B updated org A's landing, want not-found")
		}
	})
	t.Run("delete", func(t *testing.T) {
		if err := svc.Delete(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B deleted org A's landing, want not-found")
		}
	})
	t.Run("list", func(t *testing.T) {
		landings, err := svc.List(ctx, orgB)
		if err != nil {
			t.Fatalf("listing as org B: %v", err)
		}
		if len(landings) != 0 {
			t.Fatalf("org B saw %d of org A's landings, want 0", len(landings))
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
