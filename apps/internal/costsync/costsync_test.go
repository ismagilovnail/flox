package costsync_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/campaign"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/cost"
	"github.com/ismagilovnail/flox/apps/internal/costsync"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// fakeProvider stands in for a real facebookads/tiktokads.Adapter — this
// package's own tests exercise the real HTTP adapters against a fake
// server; costsync's tests exercise everything AROUND the adapter
// (credential lookup, campaign matching, cost_entries writes) against
// the adaccount.CostProvider interface directly, so no HTTP is involved
// here at all.
type fakeProvider struct {
	records  []adaccount.DailyCampaignSpendRecord
	gotCreds adaccount.Credentials
}

func (f *fakeProvider) DailySpendByCampaign(ctx context.Context, creds adaccount.Credentials, from, to time.Time) ([]adaccount.DailyCampaignSpendRecord, error) {
	f.gotCreds = creds
	return f.records, nil
}

func TestSyncWritesMatchedCampaignsAndSkipsUnmatched(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	sourceID := seedTrafficSource(t, ctx, pool, orgID, "facebook_ads")
	campaignID := seedCampaign(t, ctx, pool, orgID, sourceID, "fb_camp_1")

	adAccountRepo := adaccount.NewRepository(pool)
	if _, err := adaccount.NewService(adAccountRepo).Connect(ctx, orgID, sourceID, adaccount.ConnectInput{
		AdAccountID: "act_123", AccessToken: "test-token-1234567890",
	}); err != nil {
		t.Fatalf("connecting ad account: %v", err)
	}

	day := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	provider := &fakeProvider{records: []adaccount.DailyCampaignSpendRecord{
		{Date: day, ExternalCampaignID: "fb_camp_1", Amount: 50, Currency: "USD"},
		{Date: day, ExternalCampaignID: "fb_camp_unknown", Amount: 10, Currency: "USD"},
	}}

	svc := costsync.NewService(adAccountRepo, campaign.NewRepository(pool), cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool)), costsync.Providers{
		FacebookAds: provider,
	})

	result, err := svc.Sync(ctx, orgID, sourceID, day, day)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.RecordsFetched != 2 {
		t.Fatalf("RecordsFetched = %d, want 2", result.RecordsFetched)
	}
	if result.EntriesWritten != 1 {
		t.Fatalf("EntriesWritten = %d, want 1 (only fb_camp_1 matches a campaign)", result.EntriesWritten)
	}
	if len(result.UnmatchedExternalCampaignIDs) != 1 || result.UnmatchedExternalCampaignIDs[0] != "fb_camp_unknown" {
		t.Fatalf("UnmatchedExternalCampaignIDs = %v, want [fb_camp_unknown]", result.UnmatchedExternalCampaignIDs)
	}

	if provider.gotCreds.AdAccountID != "act_123" || provider.gotCreds.AccessToken != "test-token-1234567890" {
		t.Fatalf("provider got credentials %+v, want the connected ones", provider.gotCreds)
	}

	entries, err := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool)).List(ctx, orgID, campaignID, cost.ListFilter{From: day, To: day})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("cost entries for matched campaign = %d, want 1", len(entries))
	}
	if entries[0].Source != cost.SourceFacebookAds {
		t.Fatalf("entry Source = %q, want facebook_ads", entries[0].Source)
	}
	if entries[0].Amount != 50 {
		t.Fatalf("entry Amount = %v, want 50", entries[0].Amount)
	}
}

func TestSyncWritesToEveryCampaignSharingAnExternalID(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	sourceID := seedTrafficSource(t, ctx, pool, orgID, "tiktok_ads")
	campaignA := seedCampaign(t, ctx, pool, orgID, sourceID, "shared_id")
	campaignB := seedCampaign(t, ctx, pool, orgID, sourceID, "shared_id")

	adAccountRepo := adaccount.NewRepository(pool)
	if _, err := adaccount.NewService(adAccountRepo).Connect(ctx, orgID, sourceID, adaccount.ConnectInput{
		AdAccountID: "adv_1", AccessToken: "tiktok-token-1234567890",
	}); err != nil {
		t.Fatalf("connecting ad account: %v", err)
	}

	day := time.Date(2026, 1, 11, 0, 0, 0, 0, time.UTC)
	provider := &fakeProvider{records: []adaccount.DailyCampaignSpendRecord{
		{Date: day, ExternalCampaignID: "shared_id", Amount: 30, Currency: "USD"},
	}}

	costSvc := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool))
	svc := costsync.NewService(adAccountRepo, campaign.NewRepository(pool), costSvc, costsync.Providers{TikTokAds: provider})

	result, err := svc.Sync(ctx, orgID, sourceID, day, day)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.EntriesWritten != 2 {
		t.Fatalf("EntriesWritten = %d, want 2 (both campaigns share the external id)", result.EntriesWritten)
	}

	for _, cID := range []string{campaignA, campaignB} {
		entries, err := costSvc.List(ctx, orgID, cID, cost.ListFilter{From: day, To: day})
		if err != nil {
			t.Fatalf("List for %s: %v", cID, err)
		}
		if len(entries) != 1 || entries[0].Source != cost.SourceTikTokAds {
			t.Fatalf("campaign %s entries = %+v, want exactly one tiktok_ads entry", cID, entries)
		}
	}
}

func TestSyncErrorsWhenNotConnected(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	sourceID := seedTrafficSource(t, ctx, pool, orgID, "facebook_ads")

	adAccountRepo := adaccount.NewRepository(pool)
	svc := costsync.NewService(adAccountRepo, campaign.NewRepository(pool), cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool)), costsync.Providers{
		FacebookAds: &fakeProvider{},
	})

	if _, err := svc.Sync(ctx, orgID, sourceID, time.Now(), time.Now()); err == nil {
		t.Fatal("Sync with no connection succeeded, want an error")
	}
}

func TestSyncErrorsWhenCostIntegrationHasNoProvider(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	sourceID := seedTrafficSource(t, ctx, pool, orgID, "none")

	adAccountRepo := adaccount.NewRepository(pool)
	svc := costsync.NewService(adAccountRepo, campaign.NewRepository(pool), cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool)), costsync.Providers{
		FacebookAds: &fakeProvider{}, TikTokAds: &fakeProvider{},
	})

	if _, err := svc.Sync(ctx, orgID, sourceID, time.Now(), time.Now()); err == nil {
		t.Fatal("Sync against a traffic source with cost_integration=none succeeded, want an error")
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

func seedTrafficSource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID, costIntegration string) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO traffic_sources (id, organization_id, name, type, cost_integration) VALUES ($1, $2, $3, $4, $5)`,
		id, orgID, "Test Source", "Facebook", costIntegration,
	)
	if err != nil {
		t.Fatalf("seeding traffic source: %v", err)
	}
	return id
}

func seedCampaign(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID, trafficSourceID, externalCampaignID string) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO campaigns (id, organization_id, traffic_source_id, name, fallback_url, external_campaign_id) VALUES ($1, $2, $3, $4, $5, $6)`,
		id, orgID, trafficSourceID, "Test Campaign", "https://example.com/fallback", externalCampaignID,
	)
	if err != nil {
		t.Fatalf("seeding campaign: %v", err)
	}
	return id
}
