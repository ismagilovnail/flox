package costsync_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/campaign"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/cost"
	"github.com/ismagilovnail/flox/apps/internal/costsync"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// fakeConnectionLister stands in for *adaccount.Repository's own
// ListAllConnections — this package's own adaccount_test package already
// covers that method against real Postgres; the scheduler's tests only need
// to exercise what it does with whatever list comes back.
type fakeConnectionLister struct {
	refs []adaccount.ConnectionRef
	err  error
}

func (f *fakeConnectionLister) ListAllConnections(ctx context.Context) ([]adaccount.ConnectionRef, error) {
	return f.refs, f.err
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, nil))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestSchedulerRunOnceSyncsEveryConnection(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	sourceA := seedTrafficSource(t, ctx, pool, orgA, "facebook_ads")
	campaignA := seedCampaign(t, ctx, pool, orgA, sourceA, "fb_camp_a")

	orgB := seedOrg(t, ctx, pool)
	sourceB := seedTrafficSource(t, ctx, pool, orgB, "tiktok_ads")
	campaignB := seedCampaign(t, ctx, pool, orgB, sourceB, "tt_camp_b")

	adAccountRepo := adaccount.NewRepository(pool)
	if _, err := adaccount.NewService(adAccountRepo).Connect(ctx, orgA, sourceA, adaccount.ConnectInput{
		AdAccountID: "act_a", AccessToken: "token-a-1234567890",
	}); err != nil {
		t.Fatalf("connecting org A: %v", err)
	}
	if _, err := adaccount.NewService(adAccountRepo).Connect(ctx, orgB, sourceB, adaccount.ConnectInput{
		AdAccountID: "adv_b", AccessToken: "token-b-1234567890",
	}); err != nil {
		t.Fatalf("connecting org B: %v", err)
	}

	day := time.Now().UTC().Truncate(24 * time.Hour)
	fbProvider := &fakeProvider{records: []adaccount.DailyCampaignSpendRecord{
		{Date: day, ExternalCampaignID: "fb_camp_a", Amount: 12, Currency: "USD"},
	}}
	ttProvider := &fakeProvider{records: []adaccount.DailyCampaignSpendRecord{
		{Date: day, ExternalCampaignID: "tt_camp_b", Amount: 34, Currency: "USD"},
	}}

	costSvc := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool))
	svc := costsync.NewService(adAccountRepo, campaign.NewRepository(pool), costSvc, costsync.Providers{
		FacebookAds: fbProvider,
		TikTokAds:   ttProvider,
	})

	// A fake lister scoped to just this test's two connections — using the
	// real ListAllConnections here would see every connection any other
	// test in this package has left connected, making the "how many did it
	// attempt" assertion below flaky.
	lister := &fakeConnectionLister{refs: []adaccount.ConnectionRef{
		{OrganizationID: orgA, TrafficSourceID: sourceA},
		{OrganizationID: orgB, TrafficSourceID: sourceB},
	}}

	scheduler := costsync.NewScheduler(svc, lister, discardLogger())
	n, err := scheduler.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 2 {
		t.Fatalf("RunOnce attempted %d connections, want 2", n)
	}

	entriesA, err := costSvc.List(ctx, orgA, campaignA, cost.ListFilter{From: day, To: day})
	if err != nil {
		t.Fatalf("List for org A: %v", err)
	}
	if len(entriesA) != 1 || entriesA[0].Amount != 12 {
		t.Fatalf("org A cost entries = %+v, want one entry of amount 12", entriesA)
	}

	entriesB, err := costSvc.List(ctx, orgB, campaignB, cost.ListFilter{From: day, To: day})
	if err != nil {
		t.Fatalf("List for org B: %v", err)
	}
	if len(entriesB) != 1 || entriesB[0].Amount != 34 {
		t.Fatalf("org B cost entries = %+v, want one entry of amount 34", entriesB)
	}
}

func TestSchedulerRunOnceContinuesPastAFailingConnection(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgGood := seedOrg(t, ctx, pool)
	sourceGood := seedTrafficSource(t, ctx, pool, orgGood, "facebook_ads")
	campaignGood := seedCampaign(t, ctx, pool, orgGood, sourceGood, "fb_camp_good")

	adAccountRepo := adaccount.NewRepository(pool)
	if _, err := adaccount.NewService(adAccountRepo).Connect(ctx, orgGood, sourceGood, adaccount.ConnectInput{
		AdAccountID: "act_good", AccessToken: "token-good-1234567890",
	}); err != nil {
		t.Fatalf("connecting good org: %v", err)
	}

	day := time.Now().UTC().Truncate(24 * time.Hour)
	fbProvider := &fakeProvider{records: []adaccount.DailyCampaignSpendRecord{
		{Date: day, ExternalCampaignID: "fb_camp_good", Amount: 99, Currency: "USD"},
	}}

	costSvc := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool))
	svc := costsync.NewService(adAccountRepo, campaign.NewRepository(pool), costSvc, costsync.Providers{
		FacebookAds: fbProvider,
	})

	// The second ref points at a traffic source that was never connected —
	// Sync will error on it (TestSyncErrorsWhenNotConnected already covers
	// that error itself); the scheduler must still process the first ref.
	lister := &fakeConnectionLister{refs: []adaccount.ConnectionRef{
		{OrganizationID: idgen.New(), TrafficSourceID: idgen.New()},
		{OrganizationID: orgGood, TrafficSourceID: sourceGood},
	}}

	scheduler := costsync.NewScheduler(svc, lister, discardLogger())
	n, err := scheduler.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 2 {
		t.Fatalf("RunOnce attempted %d connections, want 2", n)
	}

	entries, err := costSvc.List(ctx, orgGood, campaignGood, cost.ListFilter{From: day, To: day})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 || entries[0].Amount != 99 {
		t.Fatalf("good org's cost entries = %+v, want one entry of amount 99 despite the other connection failing", entries)
	}
}

func TestSchedulerRunLoopStopsOnContextCancel(t *testing.T) {
	lister := &fakeConnectionLister{}
	svc := costsync.NewService(adaccount.NewRepository(nil), campaign.NewRepository(nil), cost.NewService(cost.NewRepository(nil), nil), costsync.Providers{})
	scheduler := costsync.NewScheduler(svc, lister, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		scheduler.RunLoop(ctx, time.Hour)
		close(done)
	}()

	// RunLoop's immediate first tick runs against an empty lister (no
	// connections, no DB call needed), so it returns almost instantly; the
	// loop then blocks on its hour-long ticker until ctx is canceled.
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunLoop did not return after context cancellation")
	}
}
