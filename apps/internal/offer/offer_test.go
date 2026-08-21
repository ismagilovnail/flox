package offer_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/offer"
)

func TestCreateGetUpdateDelete(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID)

	svc := offer.NewService(offer.NewRepository(pool))

	created, err := svc.Create(ctx, orgID, offer.CreateInput{
		NetworkID: networkID,
		Name:      "US Sweeps",
		Countries: []string{"us", " gb "},
		Payout:    12,
		Currency:  "usd",
		Links:     []offer.LinkInput{{Label: "Primary", URL: "https://example.com/click?cid={click_id}"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Status != offer.StatusActive {
		t.Fatalf("Status = %q, want active (the DB default)", created.Status)
	}
	if len(created.Links) != 1 || created.Links[0].ID == "" {
		t.Fatalf("Links = %+v, want one link with a generated id", created.Links)
	}
	if got, want := created.Countries, []string{"US", "GB"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Countries = %v, want normalized+uppercased %v", got, want)
	}
	if created.Currency != "USD" {
		t.Fatalf("Currency = %q, want normalized USD", created.Currency)
	}

	got, err := svc.Get(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Links) != 1 || got.Links[0].URL != created.Links[0].URL {
		t.Fatalf("Get returned links %+v, want the just-created link", got.Links)
	}

	newName := "US Sweeps (renamed)"
	updated, err := svc.Update(ctx, orgID, created.ID, offer.UpdateInput{Name: &newName})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != newName {
		t.Fatalf("Name = %q, want %q", updated.Name, newName)
	}
	if len(updated.Links) != 1 || updated.Links[0].ID != created.Links[0].ID {
		t.Fatalf("a name-only update touched links: %+v, want the original link untouched", updated.Links)
	}

	if err := svc.Delete(ctx, orgID, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, orgID, created.ID); err == nil {
		t.Fatal("Get after Delete succeeded, want not-found")
	}

	var linkCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM offer_links WHERE offer_id = $1`, created.ID).Scan(&linkCount); err != nil {
		t.Fatalf("counting links after delete: %v", err)
	}
	if linkCount != 0 {
		t.Fatalf("offer_links count after delete = %d, want 0 (CASCADE)", linkCount)
	}
}

func TestCreateRejectsNoLinksNoCountriesInvalidNetwork(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID)

	svc := offer.NewService(offer.NewRepository(pool))

	t.Run("no links", func(t *testing.T) {
		_, err := svc.Create(ctx, orgID, offer.CreateInput{
			NetworkID: networkID, Name: "No Links", Countries: []string{"US"}, Payout: 10, Currency: "USD",
		})
		if err == nil {
			t.Fatal("Create with no links succeeded, want a validation error")
		}
	})

	t.Run("no countries", func(t *testing.T) {
		_, err := svc.Create(ctx, orgID, offer.CreateInput{
			NetworkID: networkID, Name: "No Countries", Payout: 10, Currency: "USD",
			Links: []offer.LinkInput{{Label: "Primary", URL: "https://example.com/c"}},
		})
		if err == nil {
			t.Fatal("Create with no countries succeeded, want a validation error")
		}
	})

	t.Run("network from another org", func(t *testing.T) {
		otherOrg := seedOrg(t, ctx, pool)
		otherNetwork := seedNetwork(t, ctx, pool, otherOrg)
		_, err := svc.Create(ctx, orgID, offer.CreateInput{
			NetworkID: otherNetwork, Name: "Cross Org", Countries: []string{"US"}, Payout: 10, Currency: "USD",
			Links: []offer.LinkInput{{Label: "Primary", URL: "https://example.com/c"}},
		})
		if err == nil {
			t.Fatal("Create against another org's network succeeded, want a validation error")
		}
	})

	t.Run("non-positive payout", func(t *testing.T) {
		_, err := svc.Create(ctx, orgID, offer.CreateInput{
			NetworkID: networkID, Name: "Zero Payout", Countries: []string{"US"}, Payout: 0, Currency: "USD",
			Links: []offer.LinkInput{{Label: "Primary", URL: "https://example.com/c"}},
		})
		if err == nil {
			t.Fatal("Create with payout 0 succeeded, want a validation error")
		}
	})
}

func TestUpdateReplacesLinksWholesale(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID)

	svc := offer.NewService(offer.NewRepository(pool))
	created, err := svc.Create(ctx, orgID, offer.CreateInput{
		NetworkID: networkID, Name: "Multi-link", Countries: []string{"DE"}, Payout: 3, Currency: "EUR",
		Links: []offer.LinkInput{
			{Label: "Primary", URL: "https://example.com/primary"},
			{Label: "Backup", URL: "https://example.com/backup"},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	oldIDs := map[string]bool{created.Links[0].ID: true, created.Links[1].ID: true}

	newLinks := []offer.LinkInput{{Label: "Sole Link", URL: "https://example.com/sole"}}
	updated, err := svc.Update(ctx, orgID, created.ID, offer.UpdateInput{Links: &newLinks})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(updated.Links) != 1 || updated.Links[0].Label != "Sole Link" {
		t.Fatalf("Links = %+v, want exactly the replacement set", updated.Links)
	}
	if oldIDs[updated.Links[0].ID] {
		t.Fatal("the replacement link reused an old link's id")
	}

	var oldCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM offer_links WHERE offer_id = $1`, created.ID).Scan(&oldCount); err != nil {
		t.Fatalf("counting links: %v", err)
	}
	if oldCount != 1 {
		t.Fatalf("offer_links row count = %d, want exactly 1 after replace", oldCount)
	}
}

func TestUpdateCapThreeStates(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID)

	svc := offer.NewService(offer.NewRepository(pool))
	cap500 := 500
	created, err := svc.Create(ctx, orgID, offer.CreateInput{
		NetworkID: networkID, Name: "Capped", Countries: []string{"US"}, Payout: 10, Currency: "USD", Cap: &cap500,
		Links: []offer.LinkInput{{Label: "Primary", URL: "https://example.com/c"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Cap == nil || *created.Cap != 500 {
		t.Fatalf("Cap = %v, want 500", created.Cap)
	}

	// Not sent: leave untouched.
	newName := "Capped (renamed)"
	untouched, err := svc.Update(ctx, orgID, created.ID, offer.UpdateInput{Name: &newName})
	if err != nil {
		t.Fatalf("Update (name only): %v", err)
	}
	if untouched.Cap == nil || *untouched.Cap != 500 {
		t.Fatalf("Cap after a name-only update = %v, want unchanged 500", untouched.Cap)
	}

	// Sent as null: clear to uncapped.
	cleared, err := svc.Update(ctx, orgID, created.ID, offer.UpdateInput{Cap: &offer.OptionalCap{Set: true, Value: nil}})
	if err != nil {
		t.Fatalf("Update (cap to null): %v", err)
	}
	if cleared.Cap != nil {
		t.Fatalf("Cap = %v, want nil (uncapped) after an explicit null", cleared.Cap)
	}

	// Sent as a number: set a new cap.
	cap200 := 200
	recapped, err := svc.Update(ctx, orgID, created.ID, offer.UpdateInput{Cap: &offer.OptionalCap{Set: true, Value: &cap200}})
	if err != nil {
		t.Fatalf("Update (cap to 200): %v", err)
	}
	if recapped.Cap == nil || *recapped.Cap != 200 {
		t.Fatalf("Cap = %v, want 200", recapped.Cap)
	}
}

func TestDuplicateKeepsStatusAndCopiesLinks(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID)

	svc := offer.NewService(offer.NewRepository(pool))
	created, err := svc.Create(ctx, orgID, offer.CreateInput{
		NetworkID: networkID, Name: "Original", Countries: []string{"CA"}, Payout: 25, Currency: "USD",
		Links: []offer.LinkInput{{Label: "Primary", URL: "https://example.com/c"}},
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
	if dup.Name != "Original (Copy)" {
		t.Fatalf("Name = %q, want %q", dup.Name, "Original (Copy)")
	}
	if dup.Status != offer.StatusPaused {
		t.Fatalf("Status = %q, want paused (copied, not reset)", dup.Status)
	}
	if len(dup.Links) != 1 || dup.Links[0].ID == created.Links[0].ID {
		t.Fatalf("Links = %+v, want one copied link with a fresh id", dup.Links)
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)
	networkA := seedNetwork(t, ctx, pool, orgA)

	svc := offer.NewService(offer.NewRepository(pool))
	created, err := svc.Create(ctx, orgA, offer.CreateInput{
		NetworkID: networkA, Name: "Org A Offer", Countries: []string{"US"}, Payout: 5, Currency: "USD",
		Links: []offer.LinkInput{{Label: "Primary", URL: "https://example.com/c"}},
	})
	if err != nil {
		t.Fatalf("creating for org A: %v", err)
	}

	t.Run("get", func(t *testing.T) {
		if _, err := svc.Get(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B fetched org A's offer, want not-found")
		}
	})
	t.Run("update", func(t *testing.T) {
		name := "hijacked"
		if _, err := svc.Update(ctx, orgB, created.ID, offer.UpdateInput{Name: &name}); err == nil {
			t.Fatal("org B updated org A's offer, want not-found")
		}
	})
	t.Run("delete", func(t *testing.T) {
		if err := svc.Delete(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B deleted org A's offer, want not-found")
		}
	})
	t.Run("list", func(t *testing.T) {
		offers, err := svc.List(ctx, orgB)
		if err != nil {
			t.Fatalf("listing as org B: %v", err)
		}
		if len(offers) != 0 {
			t.Fatalf("org B saw %d of org A's offers, want 0", len(offers))
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

func seedNetwork(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID string) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO networks (id, organization_id, name) VALUES ($1, $2, $3)`,
		id, orgID, "Test Network",
	)
	if err != nil {
		t.Fatalf("seeding network: %v", err)
	}
	return id
}
