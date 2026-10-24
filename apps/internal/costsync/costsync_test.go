package costsync_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/adaccount/yandexdirect"
	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/campaign"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/cost"
	"github.com/ismagilovnail/flox/apps/internal/costsync"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

var testKey = mustTestKey()

func mustTestKey() []byte {
	key, err := adaccount.ParseKey("MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE=")
	if err != nil {
		panic(err)
	}
	return key
}

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

	adAccountRepo := adaccount.NewRepository(pool, testKey)
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

	adAccountRepo := adaccount.NewRepository(pool, testKey)
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

	adAccountRepo := adaccount.NewRepository(pool, testKey)
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

	adAccountRepo := adaccount.NewRepository(pool, testKey)
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

// A sync keeps the ad account's zone (only a real IANA one), synced spend
// reports it, manual spend does not, and connecting another account
// forgets it (tech audit AN4).
func TestSyncRecordsTheAccountTimezone(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	sourceID := seedTrafficSource(t, ctx, pool, orgID, "facebook_ads")
	campaignID := seedCampaign(t, ctx, pool, orgID, sourceID, "fb_camp_1")

	adAccountRepo := adaccount.NewRepository(pool, testKey)
	accounts := adaccount.NewService(adAccountRepo)
	if _, err := accounts.Connect(ctx, orgID, sourceID, adaccount.ConnectInput{AdAccountID: "act_1", AccessToken: "test-token-1234567890"}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	costSvc := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool))
	day := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	provider := &fakeProvider{}
	svc := costsync.NewService(adAccountRepo, campaign.NewRepository(pool), costSvc, costsync.Providers{FacebookAds: provider})
	connectionTZ := func() string {
		t.Helper()
		c, err := adAccountRepo.GetByTrafficSourceID(ctx, orgID, sourceID)
		if err != nil {
			t.Fatal(err)
		}
		return c.Timezone
	}

	provider.records = []adaccount.DailyCampaignSpendRecord{{Date: day, ExternalCampaignID: "fb_camp_1", Amount: 5, Currency: "USD", Timezone: "Mars/Base"}}
	if res, err := svc.Sync(ctx, orgID, sourceID, day, day); err != nil || res.AccountTimezone != "" || connectionTZ() != "" {
		t.Fatalf("an invalid zone: result %+v, err %v, stored %q; want nothing stored", res, err, connectionTZ())
	}

	provider.records[0].Timezone = "America/Los_Angeles"
	res, err := svc.Sync(ctx, orgID, sourceID, day, day)
	if err != nil || res.AccountTimezone != "America/Los_Angeles" || connectionTZ() != "America/Los_Angeles" {
		t.Fatalf("result %+v, err %v, stored %q; want America/Los_Angeles", res, err, connectionTZ())
	}

	// Manual spend on another day has no zone of its own.
	if _, err := costSvc.Upsert(ctx, orgID, campaignID, cost.UpsertInput{EntryDate: day.AddDate(0, 0, 5), Amount: 1, Currency: "USD"}); err != nil {
		t.Fatalf("manual entry: %v", err)
	}
	if zones, err := costSvc.AccountTimezones(ctx, orgID, day, day); err != nil || len(zones) != 1 || zones[0] != "America/Los_Angeles" {
		t.Fatalf("zones on the synced day = %v, %v", zones, err)
	}
	if zones, err := costSvc.AccountTimezones(ctx, orgID, day.AddDate(0, 0, 5), day.AddDate(0, 0, 5)); err != nil || len(zones) != 0 {
		t.Fatalf("zones on a manual-only day = %v, %v; want none", zones, err)
	}

	if _, err := accounts.Connect(ctx, orgID, sourceID, adaccount.ConnectInput{AdAccountID: "act_2", AccessToken: "test-token-0987654321"}); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if tz := connectionTZ(); tz != "" {
		t.Fatalf("another account kept the old zone %q", tz)
	}
}

func TestSyncKeepsADaySplitByCountry(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	sourceID := seedTrafficSource(t, ctx, pool, orgID, "facebook_ads")
	campaignID := seedCampaign(t, ctx, pool, orgID, sourceID, "fb_split")

	adAccountRepo := adaccount.NewRepository(pool, testKey)
	if _, err := adaccount.NewService(adAccountRepo).Connect(ctx, orgID, sourceID, adaccount.ConnectInput{
		AdAccountID: "act_split", AccessToken: "test-token-1234567890",
	}); err != nil {
		t.Fatalf("connecting ad account: %v", err)
	}
	costSvc := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool))
	split, other := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC)
	if _, err := costSvc.Upsert(ctx, orgID, campaignID, cost.UpsertInput{TrafficSourceID: &sourceID, EntryDate: split, Country: "DE", Amount: 8, Currency: "USD"}); err != nil {
		t.Fatalf("manual DE entry: %v", err)
	}

	svc := costsync.NewService(adAccountRepo, campaign.NewRepository(pool), costSvc, costsync.Providers{FacebookAds: &fakeProvider{records: []adaccount.DailyCampaignSpendRecord{
		{Date: split, ExternalCampaignID: "fb_split", Amount: 20, Currency: "USD"},
		{Date: other, ExternalCampaignID: "fb_split", Amount: 5, Currency: "USD"},
	}}})
	result, err := svc.Sync(ctx, orgID, sourceID, split, other)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.EntriesWritten != 1 || result.EntriesSkippedSplit != 1 {
		t.Fatalf("written/skipped = %d/%d, want 1/1", result.EntriesWritten, result.EntriesSkippedSplit)
	}
	entries, err := costSvc.List(ctx, orgID, campaignID, cost.ListFilter{From: split, To: split})
	if err != nil || len(entries) != 1 || entries[0].Country != "DE" || entries[0].Amount != 8 {
		t.Fatalf("split day = %+v, %v, want only the manual DE 8", entries, err)
	}
}

// Audit UNI-3 part 1: a Yandex Direct source's spend, through the real
// adapter against a fake Direct (the account's currency, one report),
// lands as yandex_direct cost entries on the campaign mapped to the Direct
// campaign id, in roubles with their USD value at the day's rate.
func TestSyncYandexDirectThroughTheAdapter(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	sourceID := seedTrafficSource(t, ctx, pool, orgID, "yandex_direct")
	campaignID := seedCampaign(t, ctx, pool, orgID, sourceID, "70001")

	adAccountRepo := adaccount.NewRepository(pool, testKey)
	if _, err := adaccount.NewService(adAccountRepo).Connect(ctx, orgID, sourceID, adaccount.ConnectInput{
		AdAccountID: "shop-client", AccessToken: "y0_direct_token",
	}); err != nil {
		t.Fatalf("connecting: %v", err)
	}
	direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer y0_direct_token" || r.Header.Get("Client-Login") != "shop-client" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.URL.Path == "/json/v5/clients" {
			_, _ = w.Write([]byte(`{"result": {"Clients": [{"Currency": "RUB"}]}}`))
			return
		}
		_, _ = w.Write([]byte("Date\tCampaignId\tCost\n2026-10-20\t70001\t5000.00\n2026-10-20\t70002\t10.00\n"))
	}))
	defer direct.Close()

	costSvc := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool))
	svc := costsync.NewService(adAccountRepo, campaign.NewRepository(pool), costSvc, costsync.Providers{
		YandexDirect: &yandexdirect.Adapter{BaseURL: direct.URL},
	})
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	result, err := svc.Sync(ctx, orgID, sourceID, day, day)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.EntriesWritten != 1 || len(result.UnmatchedExternalCampaignIDs) != 1 || result.UnmatchedExternalCampaignIDs[0] != "70002" {
		t.Fatalf("result = %+v", result)
	}
	entries, err := costSvc.List(ctx, orgID, campaignID, cost.ListFilter{From: day, To: day})
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %+v, %v", entries, err)
	}
	e := entries[0]
	if e.Source != cost.SourceYandexDirect || e.Amount != 5000 || e.Currency != "RUB" {
		t.Fatalf("entry = %+v", e)
	}
}

// A sync the platform refuses tells the operator why (503 with the
// reason), and a network failure never shows the request URL, which for
// Facebook carries the access token.
func TestSyncErrorsShowTheReasonButNoURL(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	sourceID := seedTrafficSource(t, ctx, pool, orgID, "facebook_ads")
	adAccountRepo := adaccount.NewRepository(pool, testKey)
	if _, err := adaccount.NewService(adAccountRepo).Connect(ctx, orgID, sourceID, adaccount.ConnectInput{
		AdAccountID: "act_1", AccessToken: "secret-token-1234567890",
	}); err != nil {
		t.Fatal(err)
	}
	costSvc := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool))
	for _, tc := range []struct {
		err  error
		want string
	}{
		{errors.New("facebookads: Invalid OAuth access token"), "Invalid OAuth access token"},
		{fmt.Errorf("calling graph: %w", &url.Error{Op: "Get", URL: "https://graph.example/x?access_token=secret-token-1234567890", Err: errors.New("dial tcp: timeout")}), "dial tcp: timeout"},
	} {
		svc := costsync.NewService(adAccountRepo, campaign.NewRepository(pool), costSvc, costsync.Providers{FacebookAds: &failingProvider{err: tc.err}})
		_, err := svc.Sync(ctx, orgID, sourceID, time.Now(), time.Now())
		var apiErr *apierror.Error
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusServiceUnavailable || !strings.Contains(apiErr.Message, tc.want) || strings.Contains(apiErr.Message, "secret-token") {
			t.Fatalf("%v: got %v", tc.err, err)
		}
	}
}

type failingProvider struct{ err error }

func (f *failingProvider) DailySpendByCampaign(context.Context, adaccount.Credentials, time.Time, time.Time) ([]adaccount.DailyCampaignSpendRecord, error) {
	return nil, f.err
}
