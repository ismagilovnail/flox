package network_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/network"
	"github.com/ismagilovnail/flox/apps/internal/pagination"
)

func TestCreateGetUpdateDelete(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := network.NewService(network.NewRepository(pool))

	result, err := svc.Create(ctx, orgID, network.CreateInput{
		Name:        "AffTrust CPA",
		PostbackURL: "https://afftrust.example/postback?click_id={click_id}",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	created := result.Network
	if result.PostbackSecret == "" {
		t.Fatal("Create returned an empty PostbackSecret")
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
	createResult, err := svc.Create(ctx, orgID, network.CreateInput{Name: "Toggle Network", PostbackURL: "https://example.com/pb"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	created := createResult.Network

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
	createResult, err := svc.Create(ctx, orgID, network.CreateInput{Name: "Original", PostbackURL: "https://example.com/pb"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	created := createResult.Network
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
	createResult, err := svc.Create(ctx, orgID, network.CreateInput{Name: "Cascading Network", PostbackURL: "https://example.com/pb"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	created := createResult.Network

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
	createResult, err := svc.Create(ctx, orgA, network.CreateInput{Name: "Org A Network", PostbackURL: "https://example.com/pb"})
	if err != nil {
		t.Fatalf("creating for org A: %v", err)
	}
	created := createResult.Network

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
		networks, _, err := svc.List(ctx, orgB, pagination.Page{Limit: pagination.MaxLimit})
		if err != nil {
			t.Fatalf("listing as org B: %v", err)
		}
		if len(networks) != 0 {
			t.Fatalf("org B saw %d of org A's networks, want 0", len(networks))
		}
	})
}

func TestPostbackSecretShownOnceAndRegenerateInvalidatesTheOld(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := network.NewService(network.NewRepository(pool))

	result, err := svc.Create(ctx, orgID, network.CreateInput{Name: "Secret Network", PostbackURL: "https://example.com/pb"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if result.PostbackSecret == "" {
		t.Fatal("Create returned an empty PostbackSecret")
	}

	firstHash := postbackSecretHash(t, ctx, pool, result.Network.ID)
	if firstHash == "" {
		t.Fatal("postback_secret_hash was never persisted")
	}

	newSecret, err := svc.RegeneratePostbackSecret(ctx, orgID, result.Network.ID)
	if err != nil {
		t.Fatalf("RegeneratePostbackSecret: %v", err)
	}
	if newSecret == "" {
		t.Fatal("RegeneratePostbackSecret returned an empty secret")
	}
	if newSecret == result.PostbackSecret {
		t.Fatal("RegeneratePostbackSecret returned the same secret as Create")
	}

	secondHash := postbackSecretHash(t, ctx, pool, result.Network.ID)
	if secondHash == firstHash {
		t.Fatal("postback_secret_hash did not change after RegeneratePostbackSecret — the old secret would still work")
	}
}

func TestTwoNetworksGetDifferentPostbackSecrets(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := network.NewService(network.NewRepository(pool))

	a, err := svc.Create(ctx, orgID, network.CreateInput{Name: "Network A", PostbackURL: "https://example.com/pb"})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	b, err := svc.Create(ctx, orgID, network.CreateInput{Name: "Network B", PostbackURL: "https://example.com/pb"})
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}
	if a.PostbackSecret == b.PostbackSecret {
		t.Fatal("two different networks got the same postback secret")
	}
}

func postbackSecretHash(t *testing.T, ctx context.Context, pool *pgxpool.Pool, networkID string) string {
	t.Helper()
	var hash string
	if err := pool.QueryRow(ctx, `SELECT postback_secret_hash FROM networks WHERE id = $1`, networkID).Scan(&hash); err != nil {
		t.Fatalf("reading postback_secret_hash: %v", err)
	}
	return hash
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

// TestCurrency: the payout currency a postback with revenue but no currency
// is priced in (audit 2026-10-15, DATA-1).
func TestCurrency(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := network.NewService(network.NewRepository(pool))

	def, err := svc.Create(ctx, orgID, network.CreateInput{Name: "Default Currency", PostbackURL: "https://example.com/pb"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if def.Network.Currency != "USD" {
		t.Errorf("default currency = %q, want USD", def.Network.Currency)
	}

	eur, err := svc.Create(ctx, orgID, network.CreateInput{Name: "Euro Network", PostbackURL: "https://example.com/pb", Currency: " eur "})
	if err != nil {
		t.Fatalf("Create EUR: %v", err)
	}
	if eur.Network.Currency != "EUR" {
		t.Errorf("currency = %q, want EUR (normalized)", eur.Network.Currency)
	}

	for _, bad := range []string{"EURO", "E1R", "$"} {
		if _, err := svc.Create(ctx, orgID, network.CreateInput{Name: "Bad Currency", PostbackURL: "https://example.com/pb", Currency: bad}); err == nil {
			t.Errorf("Create with currency %q succeeded, want a validation error", bad)
		}
		b := bad
		if _, err := svc.Update(ctx, orgID, eur.Network.ID, network.UpdateInput{Currency: &b}); err == nil {
			t.Errorf("Update to currency %q succeeded, want a validation error", bad)
		}
	}

	gbp := "gbp"
	updated, err := svc.Update(ctx, orgID, eur.Network.ID, network.UpdateInput{Currency: &gbp})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Currency != "GBP" {
		t.Errorf("updated currency = %q, want GBP", updated.Currency)
	}

	dup, err := svc.Duplicate(ctx, orgID, eur.Network.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if dup.Currency != "GBP" {
		t.Errorf("duplicate currency = %q, want GBP (copied)", dup.Currency)
	}
}

// Audit 2026-10-15, UX-1/UX-2: the incoming postback URL comes from the
// server (TRACKER_PUBLIC_URL), never a guessed host, and the outgoing URL is
// optional.
func TestIncomingURLAndOptionalOutgoing(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	repo := network.NewRepository(pool)
	svc := network.NewService(repo).WithTrackerURL(" https://track.example/ ")

	res, err := svc.Create(ctx, orgID, network.CreateInput{Name: "No Outgoing"})
	if err != nil {
		t.Fatalf("Create without an outgoing postback URL: %v", err)
	}
	n := res.Network
	if n.PostbackURL != "" {
		t.Errorf("postbackUrl = %q, want empty", n.PostbackURL)
	}
	want := "https://track.example/postback/" + n.ID
	if n.IncomingPostbackURL != want {
		t.Errorf("incoming URL = %q, want %q", n.IncomingPostbackURL, want)
	}
	if got, _ := svc.Get(ctx, orgID, n.ID); got.IncomingPostbackURL != want {
		t.Errorf("Get: incoming URL = %q, want %q", got.IncomingPostbackURL, want)
	}
	list, _, err := svc.List(ctx, orgID, pagination.Page{Limit: 10})
	if err != nil || len(list) == 0 || list[0].IncomingPostbackURL == "" {
		t.Errorf("List: %v %+v, want incoming URLs", err, list)
	}

	if _, err := svc.Create(ctx, orgID, network.CreateInput{Name: "Bad Outgoing", PostbackURL: "not-a-url"}); err == nil {
		t.Error("Create with a malformed outgoing URL succeeded, want a validation error")
	}
	set := "https://partner.example/pb?cid={click_id}"
	if _, err := svc.Update(ctx, orgID, n.ID, network.UpdateInput{PostbackURL: &set}); err != nil {
		t.Fatalf("Update to an outgoing URL: %v", err)
	}
	clear := " "
	cleared, err := svc.Update(ctx, orgID, n.ID, network.UpdateInput{PostbackURL: &clear})
	if err != nil || cleared.PostbackURL != "" {
		t.Errorf("clearing the outgoing URL: %v, %q", err, cleared.PostbackURL)
	}

	// Without TRACKER_PUBLIC_URL there is no URL to show, not a made-up one.
	if got, _ := network.NewService(repo).Get(ctx, orgID, n.ID); got.IncomingPostbackURL != "" {
		t.Errorf("without a tracker URL: incoming URL = %q, want empty", got.IncomingPostbackURL)
	}
}
