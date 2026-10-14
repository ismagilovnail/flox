package analytics_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/analytics"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/event"
)

type fakeRepo struct {
	counts        []chstore.DailyCount
	revenue       []chstore.DailyRevenue
	called        bool
	revenueCalled bool
}

func (r *fakeRepo) DailyCampaignCounts(_ context.Context, _, _ string, _, _ time.Time) ([]chstore.DailyCount, error) {
	r.called = true
	return r.counts, nil
}

func (r *fakeRepo) DailyCampaignRevenue(_ context.Context, _, _ string, _, _ time.Time, _ chstore.DateBasis) ([]chstore.DailyRevenue, error) {
	r.revenueCalled = true
	return r.revenue, nil
}

func (r *fakeRepo) ReportClicks(_ context.Context, _ chstore.ReportQuery) ([]chstore.ClickAgg, error) {
	return nil, nil
}

func (r *fakeRepo) ReportLandViews(_ context.Context, _ chstore.ReportQuery) ([]chstore.LandAgg, error) {
	return nil, nil
}

func (r *fakeRepo) ReportPwaInstalls(_ context.Context, _ chstore.ReportQuery) ([]chstore.InstallAgg, error) {
	return nil, nil
}

func (r *fakeRepo) ReportConversions(_ context.Context, _ chstore.ReportQuery) ([]chstore.ConversionAgg, error) {
	return nil, nil
}

func (r *fakeRepo) FunnelCounts(_ context.Context, _ string, _, _ time.Time, _ []chstore.ReportFilter) (map[string]uint64, error) {
	return nil, nil
}

func (r *fakeRepo) DistinctValues(_ context.Context, _ string, _ chstore.ReportDimension, _, _ time.Time, _ int) ([]string, error) {
	return nil, nil
}

func TestCampaignDailyReturnsRepositoryResult(t *testing.T) {
	repo := &fakeRepo{counts: []chstore.DailyCount{{Type: event.SourceClick, EventCount: 3}}}
	svc := analytics.NewService(repo, nil)

	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	counts, err := svc.CampaignDaily(context.Background(), "org-1", "camp-1", from, to)
	if err != nil {
		t.Fatalf("CampaignDaily: %v", err)
	}
	if !repo.called {
		t.Fatal("repository was not queried")
	}
	if len(counts) != 1 || counts[0].EventCount != 3 {
		t.Fatalf("counts = %+v, want the repository's result passed through", counts)
	}
}

func TestCampaignDailyRejectsMissingOrgOrCampaign(t *testing.T) {
	svc := analytics.NewService(&fakeRepo{}, nil)
	from := time.Now()
	to := from

	if _, err := svc.CampaignDaily(context.Background(), "", "camp-1", from, to); err == nil {
		t.Fatal("expected an error for a missing organization id")
	}
	if _, err := svc.CampaignDaily(context.Background(), "org-1", "", from, to); err == nil {
		t.Fatal("expected an error for a missing campaign id")
	}
}

func TestCampaignDailyRejectsInvertedRange(t *testing.T) {
	svc := analytics.NewService(&fakeRepo{}, nil)
	to := time.Now()
	from := to.AddDate(0, 0, 1) // after `to`

	if _, err := svc.CampaignDaily(context.Background(), "org-1", "camp-1", from, to); err == nil {
		t.Fatal("expected an error when to is before from")
	}
}

func TestCampaignDailyRevenueReturnsRepositoryResult(t *testing.T) {
	repo := &fakeRepo{revenue: []chstore.DailyRevenue{{Type: event.CpaAccept, EventCount: 2, RevenueUSD: 150}}}
	svc := analytics.NewService(repo, nil)

	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	revenue, err := svc.CampaignDailyRevenue(context.Background(), "org-1", "camp-1", from, to, "")
	if err != nil {
		t.Fatalf("CampaignDailyRevenue: %v", err)
	}
	if !repo.revenueCalled {
		t.Fatal("repository was not queried")
	}
	if len(revenue) != 1 || revenue[0].RevenueUSD != 150 {
		t.Fatalf("revenue = %+v, want the repository's result passed through", revenue)
	}
}

func TestCampaignDailyRevenueRejectsSameValidationAsCounts(t *testing.T) {
	svc := analytics.NewService(&fakeRepo{}, nil)
	if _, err := svc.CampaignDailyRevenue(context.Background(), "", "camp-1", time.Now(), time.Now(), ""); err == nil {
		t.Fatal("expected an error for a missing organization id")
	}
}

func TestCampaignDailyRejectsRangeTooWide(t *testing.T) {
	repo := &fakeRepo{}
	svc := analytics.NewService(repo, nil)
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(2, 0, 0) // 2 years

	if _, err := svc.CampaignDaily(context.Background(), "org-1", "camp-1", from, to); err == nil {
		t.Fatal("expected an error for a multi-year range")
	}
	if repo.called {
		t.Fatal("repository must not be queried when validation already failed")
	}
}

func (r *fakeRepo) FlowStageCounts(context.Context, string, time.Time, time.Time, []string) (map[string]map[string]uint64, error) {
	return nil, nil
}
