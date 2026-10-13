package cost_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
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

func TestSpendByCampaignSumsAcrossCampaignsAndSources(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignA := seedCampaign(t, ctx, pool, orgID)
	campaignB := seedCampaign(t, ctx, pool, orgID)
	sourceID := seedTrafficSource(t, ctx, pool, orgID)

	svc := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool))
	day1 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)

	if _, err := svc.Upsert(ctx, orgID, campaignA, cost.UpsertInput{EntryDate: day1, Amount: 100, Currency: "USD"}); err != nil {
		t.Fatalf("upsert A day1: %v", err)
	}
	if _, err := svc.Upsert(ctx, orgID, campaignA, cost.UpsertInput{TrafficSourceID: &sourceID, EntryDate: day2, Amount: 50, Currency: "USD"}); err != nil {
		t.Fatalf("upsert A day2 (source-scoped): %v", err)
	}
	if _, err := svc.Upsert(ctx, orgID, campaignB, cost.UpsertInput{EntryDate: day1, Amount: 20, Currency: "USD"}); err != nil {
		t.Fatalf("upsert B day1: %v", err)
	}

	spend, err := svc.SpendByCampaign(ctx, orgID, day1, day2)
	if err != nil {
		t.Fatalf("SpendByCampaign: %v", err)
	}
	byCampaign := map[string]cost.CampaignSpend{}
	for _, s := range spend {
		byCampaign[s.CampaignID] = s
	}
	if byCampaign[campaignA].AmountUSD != 150 {
		t.Fatalf("campaign A spend = %v, want 150 (100 + 50 across both sources)", byCampaign[campaignA].AmountUSD)
	}
	if byCampaign[campaignB].AmountUSD != 20 {
		t.Fatalf("campaign B spend = %v, want 20", byCampaign[campaignB].AmountUSD)
	}
}

func TestSpendBySourceRollsCampaignsUpToTheirSource(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	srcShared := seedTrafficSource(t, ctx, pool, orgID)
	srcOther := seedTrafficSource(t, ctx, pool, orgID)
	insertCampaign := func(sourceID string) string {
		id := idgen.New()
		if _, err := pool.Exec(ctx,
			`INSERT INTO campaigns (id, organization_id, traffic_source_id, name, fallback_url) VALUES ($1, $2, $3, $4, $5)`,
			id, orgID, sourceID, "Test Campaign", "https://example.com/fallback",
		); err != nil {
			t.Fatalf("seeding campaign: %v", err)
		}
		return id
	}
	campA, campB, campC := insertCampaign(srcShared), insertCampaign(srcShared), insertCampaign(srcOther)

	svc := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool))
	day := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	for camp, amount := range map[string]float64{campA: 100, campB: 20, campC: 7} {
		if _, err := svc.Upsert(ctx, orgID, camp, cost.UpsertInput{EntryDate: day, Amount: amount, Currency: "USD"}); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}

	spend, err := svc.SpendBySource(ctx, orgID, day, day)
	if err != nil {
		t.Fatalf("SpendBySource: %v", err)
	}
	bySource := map[string]float64{}
	for _, s := range spend {
		bySource[s.TrafficSourceID] = s.AmountUSD
	}
	if len(bySource) != 2 || bySource[srcShared] != 120 || bySource[srcOther] != 7 {
		t.Fatalf("spend by source = %v, want shared=120 (two campaigns summed) other=7", bySource)
	}

	// Another org's source must never show up.
	otherOrg := seedOrg(t, ctx, pool)
	if got, err := svc.SpendBySource(ctx, otherOrg, day, day); err != nil || len(got) != 0 {
		t.Fatalf("other org spend by source = %v, %v, want empty", got, err)
	}
}

func TestSpendByDayPerCampaignAndPerSource(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	src := seedTrafficSource(t, ctx, pool, orgID)
	insertCampaign := func() string {
		id := idgen.New()
		if _, err := pool.Exec(ctx,
			`INSERT INTO campaigns (id, organization_id, traffic_source_id, name, fallback_url) VALUES ($1, $2, $3, $4, $5)`,
			id, orgID, src, "Test Campaign", "https://example.com/fallback",
		); err != nil {
			t.Fatalf("seeding campaign: %v", err)
		}
		return id
	}
	campA, campB := insertCampaign(), insertCampaign()
	svc := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool))
	d1 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	for _, e := range []struct {
		camp string
		day  time.Time
		amt  float64
	}{{campA, d1, 10}, {campA, d2, 20}, {campB, d1, 5}} {
		if _, err := svc.Upsert(ctx, orgID, e.camp, cost.UpsertInput{EntryDate: e.day, Amount: e.amt, Currency: "USD"}); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}

	byCamp, err := svc.CampaignSpendByDay(ctx, orgID, d1, d2)
	if err != nil {
		t.Fatalf("CampaignSpendByDay: %v", err)
	}
	got := map[string]float64{}
	for _, r := range byCamp {
		got[r.CampaignID+"|"+r.Day.Format("2006-01-02")] = r.AmountUSD
	}
	if len(got) != 3 || got[campA+"|2026-02-01"] != 10 || got[campA+"|2026-02-02"] != 20 || got[campB+"|2026-02-01"] != 5 {
		t.Fatalf("campaign/day spend = %v", got)
	}

	bySrc, err := svc.SourceSpendByDay(ctx, orgID, d1, d2)
	if err != nil {
		t.Fatalf("SourceSpendByDay: %v", err)
	}
	gotSrc := map[string]float64{}
	for _, r := range bySrc {
		gotSrc[r.TrafficSourceID+"|"+r.Day.Format("2006-01-02")] = r.AmountUSD
	}
	if len(gotSrc) != 2 || gotSrc[src+"|2026-02-01"] != 15 || gotSrc[src+"|2026-02-02"] != 20 {
		t.Fatalf("source/day spend = %v, want 15 on day 1 (two campaigns) and 20 on day 2", gotSrc)
	}
}

func TestSpendByCampaignEmptyForOrgWithNoEntries(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool))
	spend, err := svc.SpendByCampaign(ctx, orgID, time.Now().AddDate(0, -1, 0), time.Now())
	if err != nil {
		t.Fatalf("SpendByCampaign: %v", err)
	}
	// A brand-new org with zero cost_entries must get zero rows back, not
	// a row with AmountUSD=0 — "no data at all" and "confirmed zero spend"
	// are different things (CLAUDE.md #6), and it's the caller's job
	// (apps/internal/analytics.Service) to treat "campaign absent from
	// this slice" as null cost, not fabricate a zero.
	if len(spend) != 0 {
		t.Fatalf("spend = %+v, want empty for an org with no cost_entries at all", spend)
	}
}

func TestDailySpendForOrgSumsAcrossCampaigns(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignA := seedCampaign(t, ctx, pool, orgID)
	campaignB := seedCampaign(t, ctx, pool, orgID)

	svc := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool))
	day := time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC)

	if _, err := svc.Upsert(ctx, orgID, campaignA, cost.UpsertInput{EntryDate: day, Amount: 30, Currency: "USD"}); err != nil {
		t.Fatalf("upsert A: %v", err)
	}
	if _, err := svc.Upsert(ctx, orgID, campaignB, cost.UpsertInput{EntryDate: day, Amount: 45, Currency: "USD"}); err != nil {
		t.Fatalf("upsert B: %v", err)
	}

	spend, err := svc.DailySpendForOrg(ctx, orgID, day, day)
	if err != nil {
		t.Fatalf("DailySpendForOrg: %v", err)
	}
	if len(spend) != 1 || spend[0].AmountUSD != 75 {
		t.Fatalf("spend = %+v, want exactly 1 day totaling 75 (30 + 45 across both campaigns)", spend)
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

func TestCountrySplitDay(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID)
	svc := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool))
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	for country, amount := range map[string]float64{"de": 30, "FR": 12} {
		if _, err := svc.Upsert(ctx, orgID, campaignID, cost.UpsertInput{EntryDate: day, Country: country, Amount: amount, Currency: "USD"}); err != nil {
			t.Fatalf("upsert %s: %v", country, err)
		}
	}
	// Same country again overwrites, it does not stack.
	if _, err := svc.Upsert(ctx, orgID, campaignID, cost.UpsertInput{EntryDate: day, Country: "DE", Amount: 40, Currency: "USD"}); err != nil {
		t.Fatalf("re-upsert DE: %v", err)
	}
	entries, err := svc.List(ctx, orgID, campaignID, cost.ListFilter{From: day, To: day})
	if err != nil || len(entries) != 2 {
		t.Fatalf("List = %d entries, %v, want 2 (one per country)", len(entries), err)
	}

	t.Run("a day total is refused on a split day", func(t *testing.T) {
		_, err := svc.Upsert(ctx, orgID, campaignID, cost.UpsertInput{EntryDate: day, Amount: 99, Currency: "USD"})
		if !errors.Is(err, cost.ErrSplitConflict) {
			t.Fatalf("err = %v, want ErrSplitConflict", err)
		}
	})
	t.Run("a country is refused on a total day", func(t *testing.T) {
		other := day.AddDate(0, 0, 1)
		if _, err := svc.Upsert(ctx, orgID, campaignID, cost.UpsertInput{EntryDate: other, Amount: 5, Currency: "USD"}); err != nil {
			t.Fatalf("total: %v", err)
		}
		_, err := svc.Upsert(ctx, orgID, campaignID, cost.UpsertInput{EntryDate: other, Country: "DE", Amount: 1, Currency: "USD"})
		if !errors.Is(err, cost.ErrSplitConflict) {
			t.Fatalf("err = %v, want ErrSplitConflict", err)
		}
	})
	t.Run("a bad country code is a validation error", func(t *testing.T) {
		if _, err := svc.Upsert(ctx, orgID, campaignID, cost.UpsertInput{EntryDate: day, Country: "DEU", Amount: 1, Currency: "USD"}); err == nil {
			t.Fatal("DEU accepted, want a validation error")
		}
	})
	t.Run("daily spend sums the countries", func(t *testing.T) {
		spend, err := svc.DailyCampaignSpend(ctx, orgID, campaignID, day, day)
		if err != nil || len(spend) != 1 || spend[0].AmountUSD != 52 {
			t.Fatalf("daily spend = %+v, %v, want one day of 52", spend, err)
		}
	})
	t.Run("spend by country, scoped and tenant-isolated", func(t *testing.T) {
		got, err := svc.SpendByCountry(ctx, orgID, day, day.AddDate(0, 0, 1), cost.CountryScope{CampaignID: campaignID})
		if err != nil {
			t.Fatalf("SpendByCountry: %v", err)
		}
		by := map[string]float64{}
		for _, c := range got {
			by[c.Day.Format("01-02")+" "+c.Country] = c.AmountUSD
		}
		if len(by) != 3 || by["03-01 DE"] != 40 || by["03-01 FR"] != 12 || by["03-02 "] != 5 {
			t.Fatalf("spend by country = %v, want DE 40 / FR 12 on day 1 and an unsplit 5 on day 2", by)
		}
		if got, _ := svc.SpendByCountry(ctx, orgID, day, day, cost.CountryScope{CampaignID: idgen.New()}); len(got) != 0 {
			t.Fatalf("another campaign's scope = %v, want empty", got)
		}
		if got, _ := svc.SpendByCountry(ctx, orgID, day, day, cost.CountryScope{TrafficSourceID: idgen.New()}); len(got) != 0 {
			t.Fatalf("another source's scope = %v, want empty", got)
		}
		if got, _ := svc.SpendByCountry(ctx, seedOrg(t, ctx, pool), day, day, cost.CountryScope{}); len(got) != 0 {
			t.Fatalf("another org = %v, want empty", got)
		}
	})
}

func TestParseCSV(t *testing.T) {
	t.Run("comma file, any column order", func(t *testing.T) {
		lines, errs := cost.ParseCSV(strings.NewReader("Country,Date,Amount,Currency\nde,2026-03-01,10.5,usd\n\n,2026-03-02,7,EUR\n"))
		if len(errs) > 0 {
			t.Fatalf("errs = %v", errs)
		}
		if len(lines) != 2 || lines[0].Line != 2 || lines[0].Country != "de" || lines[0].Amount != 10.5 || lines[1].Line != 4 || lines[1].Country != "" {
			t.Fatalf("lines = %+v", lines)
		}
	})
	t.Run("every bad line is named at once", func(t *testing.T) {
		_, errs := cost.ParseCSV(strings.NewReader("date,country,amount,currency\n2026-03-01,DEU,1,USD\n03/01/2026,DE,1,USD\n2026-03-01,DE,-1,USD\n"))
		if len(errs) != 3 || !strings.HasPrefix(errs["line 2"], "country:") || !strings.HasPrefix(errs["line 4"], "amount:") {
			t.Fatalf("errs = %v, want lines 2, 3 and 4", errs)
		}
	})
	t.Run("semicolon file with a BOM and decimal commas", func(t *testing.T) {
		lines, errs := cost.ParseCSV(strings.NewReader("\uFEFFdate;amount;currency\n2026-03-01;12,75;EUR\n"))
		if len(errs) > 0 || len(lines) != 1 || lines[0].Amount != 12.75 {
			t.Fatalf("lines = %+v, errs = %v", lines, errs)
		}
	})
	for name, tc := range map[string]struct{ csv, key string }{
		"unknown column":  {"date,amount,currency,contry\n", "file"},
		"missing column":  {"date,amount\n", "file"},
		"no rows":         {"date,amount,currency\n", "file"},
		"bad date":        {"date,amount,currency\n2026-03-01,1,USD\n03/01/2026,1,USD\n", "line 3"},
		"bad amount":      {"date,amount,currency\n2026-03-01,ten,USD\n", "line 2"},
		"bad currency":    {"date,amount,currency\n2026-04-03,1,DOLLARS\n", "line 2"},
		"bad country":     {"date,amount,currency,country\n2026-03-01,1,USD,DEU\n", "line 2"},
		"malformed quote": {"date,amount,currency\n2026-03-01,\"1,USD\n", "line 2"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, errs := cost.ParseCSV(strings.NewReader(tc.csv)); errs[tc.key] == "" {
				t.Fatalf("errs = %v, want one under %q", errs, tc.key)
			}
		})
	}
}

func TestImport(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID)
	srcID := seedTrafficSource(t, ctx, pool, orgID)
	svc := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool))
	day := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	parse := func(t *testing.T, csv string) []cost.ImportLine {
		t.Helper()
		lines, errs := cost.ParseCSV(strings.NewReader(csv))
		if len(errs) > 0 {
			t.Fatalf("ParseCSV: %v", errs)
		}
		return lines
	}
	count := func(t *testing.T) int {
		t.Helper()
		entries, err := svc.List(ctx, orgID, campaignID, cost.ListFilter{From: day, To: day.AddDate(0, 0, 5)})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		return len(entries)
	}

	n, err := svc.Import(ctx, orgID, campaignID, parse(t, "date,country,amount,currency,traffic_source_id\n"+
		"2026-04-01,DE,10,USD,\n2026-04-01,FR,5,USD,\n2026-04-02,,7,USD,"+srcID+"\n"))
	if err != nil || n != 3 || count(t) != 3 {
		t.Fatalf("Import = %d, %v; entries = %d, want 3 written", n, err, count(t))
	}
	// Importing again overwrites: still 3 entries.
	if n, err := svc.Import(ctx, orgID, campaignID, parse(t, "date,country,amount,currency\n2026-04-01,DE,11,USD\n")); err != nil || n != 1 || count(t) != 3 {
		t.Fatalf("re-import = %d, %v; entries = %d, want 3", n, err, count(t))
	}

	for name, tc := range map[string]struct{ csv, key string }{
		"duplicate day and country":   {"date,country,amount,currency\n2026-04-03,DE,1,USD\n2026-04-03,de,2,USD\n", "line 3"},
		"total and split in one file": {"date,country,amount,currency\n2026-04-03,DE,1,USD\n2026-04-03,,2,USD\n", "line 3"},
		"another org's source":        {"date,amount,currency,traffic_source_id\n2026-04-03,1,USD," + seedTrafficSource(t, ctx, pool, seedOrg(t, ctx, pool)) + "\n", "line 2"},
		// Line 2 is fine; line 3 meets the split day 04-01 already holds:
		// nothing of the file is written.
		"conflict with what is stored": {"date,amount,currency\n2026-04-04,3,USD\n2026-04-01,9,USD\n", "line 3"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Import(ctx, orgID, campaignID, parse(t, tc.csv))
			var apiErr *apierror.Error
			if !errors.As(err, &apiErr) || apiErr.Fields[tc.key] == "" {
				t.Fatalf("err = %v, want a validation error on %q", err, tc.key)
			}
			if count(t) != 3 {
				t.Fatalf("entries = %d after a refused import, want 3 — the import must be all or nothing", count(t))
			}
		})
	}

	t.Run("another org's campaign", func(t *testing.T) {
		if _, err := svc.Import(ctx, seedOrg(t, ctx, pool), campaignID, parse(t, "date,amount,currency\n2026-04-05,1,USD\n")); err == nil {
			t.Fatal("imported into another org's campaign")
		}
	})
}
