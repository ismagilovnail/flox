package cost_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/cost"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

func TestUpsertCreatesThenUpdatesInPlace(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID)

	svc := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool))
	day := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

	first, err := svc.Upsert(ctx, orgID, campaignID, cost.UpsertInput{EntryDate: day, Amount: 100, Currency: "USD"})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if first.AmountUSD == nil || *first.AmountUSD != 100 {
		t.Fatalf("AmountUSD = %v, want 100 (USD is always 1:1)", first.AmountUSD)
	}

	second, err := svc.Upsert(ctx, orgID, campaignID, cost.UpsertInput{EntryDate: day, Amount: 250, Currency: "USD"})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("re-upserting the same day created a new row (id %s != %s), want it updated in place", second.ID, first.ID)
	}
	if second.Amount != 250 {
		t.Fatalf("Amount = %v, want 250 after update", second.Amount)
	}

	entries, err := svc.List(ctx, orgID, campaignID, cost.ListFilter{From: day.AddDate(0, 0, -1), To: day.AddDate(0, 0, 1)})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want exactly 1 (upsert must not stack)", len(entries))
	}
}

func TestUpsertNoFXRateStoresNilNotZero(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID)

	svc := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool))
	day := time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)

	entry, err := svc.Upsert(ctx, orgID, campaignID, cost.UpsertInput{EntryDate: day, Amount: 50, Currency: "ZZZ"})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if entry.AmountUSD != nil {
		t.Fatalf("AmountUSD = %v, want nil (no fx_rates row for ZZZ) — CLAUDE.md #6/#7", *entry.AmountUSD)
	}
	if entry.Amount != 50 || entry.Currency != "ZZZ" {
		t.Fatalf("entry stored with wrong original amount/currency: %+v", entry)
	}
}

func TestDailyCampaignSpendFlagsIncompleteFXDay(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID)
	sourceID := seedTrafficSource(t, ctx, pool, orgID)

	svc := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool))
	day := time.Date(2026, 1, 17, 0, 0, 0, 0, time.UTC)

	// One converted (USD) and one unconverted (no fx_rates row) entry on
	// the same day, via the source-scoped and campaign-wide identity keys
	// respectively — both must be visible in the day's aggregate.
	if _, err := svc.Upsert(ctx, orgID, campaignID, cost.UpsertInput{TrafficSourceID: &sourceID, EntryDate: day, Amount: 100, Currency: "USD"}); err != nil {
		t.Fatalf("upsert converted entry: %v", err)
	}
	if _, err := svc.Upsert(ctx, orgID, campaignID, cost.UpsertInput{EntryDate: day, Amount: 20, Currency: "ZZZ"}); err != nil {
		t.Fatalf("upsert unconverted entry: %v", err)
	}

	spend, err := svc.DailyCampaignSpend(ctx, orgID, campaignID, day, day)
	if err != nil {
		t.Fatalf("DailyCampaignSpend: %v", err)
	}
	if len(spend) != 1 {
		t.Fatalf("spend days = %d, want 1", len(spend))
	}
	if spend[0].AmountUSD != 100 {
		t.Fatalf("AmountUSD = %v, want 100 (only the converted entry sums; the ZZZ one contributes nothing, not an error)", spend[0].AmountUSD)
	}
	if spend[0].AllConverted {
		t.Fatal("AllConverted = true, want false — the ZZZ entry has no USD value on file")
	}
}

func TestUpsertRecordsManualSource(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID)

	svc := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool))
	day := time.Date(2026, 1, 19, 0, 0, 0, 0, time.UTC)

	entry, err := svc.Upsert(ctx, orgID, campaignID, cost.UpsertInput{EntryDate: day, Amount: 10, Currency: "USD"})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if entry.Source != cost.SourceManual {
		t.Fatalf("Source = %q, want manual — the HTTP-reachable path must never write anything else", entry.Source)
	}
}

// TestUpsertFromSync covers the ad-spend sync's write path (§74/§27-COST,
// Phase B): a real ad-network source is recorded and round-trips through
// List, and the method structurally refuses to be used to impersonate a
// manual entry or write a bogus source string.
func TestUpsertFromSync(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID)

	svc := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool))
	day := time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)

	entry, err := svc.UpsertFromSync(ctx, orgID, campaignID, cost.UpsertInput{EntryDate: day, Amount: 42, Currency: "USD"}, cost.SourceFacebookAds)
	if err != nil {
		t.Fatalf("UpsertFromSync: %v", err)
	}
	if entry.Source != cost.SourceFacebookAds {
		t.Fatalf("Source = %q, want facebook_ads", entry.Source)
	}

	entries, err := svc.List(ctx, orgID, campaignID, cost.ListFilter{From: day, To: day})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 || entries[0].Source != cost.SourceFacebookAds {
		t.Fatalf("List = %+v, want exactly one facebook_ads entry", entries)
	}

	t.Run("rejects SourceManual", func(t *testing.T) {
		if _, err := svc.UpsertFromSync(ctx, orgID, campaignID, cost.UpsertInput{EntryDate: day.AddDate(0, 0, 1), Amount: 1, Currency: "USD"}, cost.SourceManual); err == nil {
			t.Fatal("UpsertFromSync accepted SourceManual, want error — a sync must never masquerade as a manual entry")
		}
	})

	t.Run("rejects an invalid source string", func(t *testing.T) {
		if _, err := svc.UpsertFromSync(ctx, orgID, campaignID, cost.UpsertInput{EntryDate: day.AddDate(0, 0, 1), Amount: 1, Currency: "USD"}, cost.Source("bogus")); err == nil {
			t.Fatal("UpsertFromSync accepted an invalid source, want error")
		}
	})
}

func TestCrossTenantIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)
	campaignA := seedCampaign(t, ctx, pool, orgA)

	svc := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool))
	day := time.Date(2026, 1, 18, 0, 0, 0, 0, time.UTC)

	entry, err := svc.Upsert(ctx, orgA, campaignA, cost.UpsertInput{EntryDate: day, Amount: 10, Currency: "USD"})
	if err != nil {
		t.Fatalf("creating entry for org A: %v", err)
	}

	t.Run("cannot create against another org's campaign", func(t *testing.T) {
		_, err := svc.Upsert(ctx, orgB, campaignA, cost.UpsertInput{EntryDate: day, Amount: 10, Currency: "USD"})
		if err == nil {
			t.Fatal("org B created a cost entry against org A's campaign, want validation error")
		}
	})

	t.Run("list", func(t *testing.T) {
		entries, err := svc.List(ctx, orgB, campaignA, cost.ListFilter{From: day.AddDate(0, 0, -1), To: day.AddDate(0, 0, 1)})
		if err != nil {
			t.Fatalf("listing as org B: %v", err)
		}
		if len(entries) != 0 {
			t.Fatalf("org B saw %d of org A's cost entries, want 0", len(entries))
		}
	})

	t.Run("delete", func(t *testing.T) {
		if err := svc.Delete(ctx, orgB, campaignA, entry.ID); err == nil {
			t.Fatal("org B deleted org A's cost entry, want not-found")
		}
	})

	t.Run("daily spend", func(t *testing.T) {
		spend, err := svc.DailyCampaignSpend(ctx, orgB, campaignA, day, day)
		if err != nil {
			t.Fatalf("daily spend as org B: %v", err)
		}
		if len(spend) != 0 {
			t.Fatalf("org B saw %d days of org A's spend, want 0", len(spend))
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

func seedCampaign(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID string) string {
	t.Helper()
	sourceID := seedTrafficSource(t, ctx, pool, orgID)
	id := idgen.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO campaigns (id, organization_id, traffic_source_id, name, fallback_url) VALUES ($1, $2, $3, $4, $5)`,
		id, orgID, sourceID, "Test Campaign", "https://example.com/fallback",
	)
	if err != nil {
		t.Fatalf("seeding campaign: %v", err)
	}
	return id
}
