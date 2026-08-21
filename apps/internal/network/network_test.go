package network_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/network"
)

func TestCreateGetUpdateDelete(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := network.NewService(network.NewRepository(pool))

	created, err := svc.Create(ctx, orgID, network.CreateInput{
		Name:        "AffTrust CPA",
		PostbackURL: "https://afftrust.example/postback?click_id={click_id}",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Status != network.StatusActive {
		t.Fatalf("Status = %q, want active (the DB default)", created.Status)
	}

	got, err := svc.Get(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != created.Name {
		t.Fatalf("Get returned %+v, want the just-created row", got)
	}

	newName := "AffTrust CPA (renamed)"
	updated, err := svc.Update(ctx, orgID, created.ID, network.UpdateInput{Name: &newName})
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

func TestCreateRejectsInvalidPostbackURL(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := network.NewService(network.NewRepository(pool))
	_, err := svc.Create(ctx, orgID, network.CreateInput{Name: "Bad Network", PostbackURL: "not-a-url"})
	if err == nil {
		t.Fatal("Create with an invalid postback URL succeeded, want a validation error")
	}
}

func TestPauseActivateTransitions(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := network.NewService(network.NewRepository(pool))
	created, err := svc.Create(ctx, orgID, network.CreateInput{Name: "Toggle Network", PostbackURL: "https://example.com/pb"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	paused, err := svc.Pause(ctx, orgID, created.ID)
	if err != nil || paused.Status != network.StatusPaused {
		t.Fatalf("Pause: got %+v, err %v", paused, err)
	}
	if again, err := svc.Pause(ctx, orgID, created.ID); err != nil || again.Status != network.StatusPaused {
		t.Fatalf("Pause (idempotent): got %+v, err %v", again, err)
	}

	active, err := svc.Activate(ctx, orgID, created.ID)
	if err != nil || active.Status != network.StatusActive {
		t.Fatalf("Activate: got %+v, err %v", active, err)
	}

	archived := network.StatusArchived
	if _, err := svc.Update(ctx, orgID, created.ID, network.UpdateInput{Status: &archived}); err != nil {
		t.Fatalf("archiving via Update: %v", err)
	}
	if _, err := svc.Pause(ctx, orgID, created.ID); err == nil {
		t.Fatal("Pause on an archived network succeeded, want a conflict error")
	}
	if _, err := svc.Activate(ctx, orgID, created.ID); err == nil {
		t.Fatal("Activate on an archived network succeeded, want a conflict error")
	}
}

func TestDuplicateKeepsStatus(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := network.NewService(network.NewRepository(pool))
	created, err := svc.Create(ctx, orgID, network.CreateInput{Name: "Original", PostbackURL: "https://example.com/pb"})
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
	if dup.Status != network.StatusPaused {
		t.Fatalf("Status = %q, want paused (copied from the source, not reset)", dup.Status)
	}
}

// TestDeleteCascadesToOffers proves the opposite of trafficsource's own
// delete-conflict test: offers.network_id CASCADEs (00003), so deleting a
// network deletes its offers too, rather than being blocked by them —
// a deliberate schema choice this phase inherits, not one it makes.
func TestDeleteCascadesToOffers(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := network.NewService(network.NewRepository(pool))
	created, err := svc.Create(ctx, orgID, network.CreateInput{Name: "Cascading Network", PostbackURL: "https://example.com/pb"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	offerID := idgen.New()
	_, err = pool.Exec(ctx,
		`INSERT INTO offers (id, organization_id, network_id, name, payout) VALUES ($1, $2, $3, $4, $5)`,
		offerID, orgID, created.ID, "Cascaded Offer", 10,
	)
	if err != nil {
		t.Fatalf("seeding offer: %v", err)
	}

	if err := svc.Delete(ctx, orgID, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM offers WHERE id = $1)`, offerID).Scan(&exists); err != nil {
		t.Fatalf("checking offer existence: %v", err)
	}
	if exists {
		t.Fatal("offer still exists after its network was deleted, want it cascade-deleted")
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)

	svc := network.NewService(network.NewRepository(pool))
	created, err := svc.Create(ctx, orgA, network.CreateInput{Name: "Org A Network", PostbackURL: "https://example.com/pb"})
	if err != nil {
		t.Fatalf("creating for org A: %v", err)
	}

	t.Run("get", func(t *testing.T) {
		if _, err := svc.Get(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B fetched org A's network, want not-found")
		}
	})
	t.Run("update", func(t *testing.T) {
		name := "hijacked"
		if _, err := svc.Update(ctx, orgB, created.ID, network.UpdateInput{Name: &name}); err == nil {
			t.Fatal("org B updated org A's network, want not-found")
		}
	})
	t.Run("delete", func(t *testing.T) {
		if err := svc.Delete(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B deleted org A's network, want not-found")
		}
	})
	t.Run("list", func(t *testing.T) {
		networks, err := svc.List(ctx, orgB)
		if err != nil {
			t.Fatalf("listing as org B: %v", err)
		}
		if len(networks) != 0 {
			t.Fatalf("org B saw %d of org A's networks, want 0", len(networks))
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
