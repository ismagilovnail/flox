package analytics_test

import (
	"context"
	"errors"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/analytics"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/cost"
)

type fakeOwners map[string]string

func (f fakeOwners) Owners(context.Context, string) (map[string]string, error) { return f, nil }
func (f fakeOwners) OwnerNames(context.Context, string) ([]analytics.Buyer, error) {
	return []analytics.Buyer{{ID: "ann", Name: "Ann"}, {ID: "bob", Name: "Bob"}}, nil
}

var owners = fakeOwners{"camp-a1": "ann", "camp-a2": "ann", "camp-b": "bob", "camp-x": ""}

func buyerRepo() *reportFakeRepo {
	return &reportFakeRepo{
		clicks: []chstore.ClickAgg{
			{Key: dimKey(chstore.DimCampaign, "camp-a1"), Clicks: 100, UniqueClicks: 80},
			{Key: dimKey(chstore.DimCampaign, "camp-a2"), Clicks: 300, UniqueClicks: 200},
			{Key: dimKey(chstore.DimCampaign, "camp-b"), Clicks: 50, UniqueClicks: 50},
			{Key: dimKey(chstore.DimCampaign, "camp-x"), Clicks: 10, UniqueClicks: 10},
		},
		conv: []chstore.ConversionAgg{
			{Key: dimKey(chstore.DimCampaign, "camp-a1"), Conversions: 4, RevenueUSD: 400},
			{Key: dimKey(chstore.DimCampaign, "camp-a2"), Conversions: 6, RevenueUSD: 200},
			{Key: dimKey(chstore.DimCampaign, "camp-b"), Conversions: 1, RevenueUSD: 50},
		},
	}
}

var buyerRange = analytics.ReportInput{
	From: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	To:   time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC),
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// Audit TEAM-1: by buyer, campaigns add up per current owner — the base
// counts and money are summed, and CR/ROI/EPC come from the sums, never
// from averaging the campaigns'.
func TestReportByBuyerAddsCampaignsUp(t *testing.T) {
	repo := buyerRepo()
	spend := &fakeCost{byCampaign: []cost.CampaignSpend{
		{CampaignID: "camp-a1", AmountUSD: 100, AllConverted: true},
		{CampaignID: "camp-a2", AmountUSD: 200, AllConverted: true},
		// bob's campaign has no spend: his cost is unknown, not 0.
	}}
	svc := analytics.NewService(repo, spend).WithOwners(owners)

	in := buyerRange
	in.Dims = []chstore.ReportDimension{chstore.DimBuyer}
	rows, err := svc.Report(context.Background(), "org", in)
	if err != nil {
		t.Fatal(err)
	}
	if q := repo.lastClickQuery; q == nil || !slices.Equal(q.Dims, []chstore.ReportDimension{chstore.DimCampaign}) {
		t.Fatalf("ClickHouse was asked for %+v, want by campaign", q)
	}
	by := map[string]analytics.MetricValues{}
	for _, r := range rows {
		by[r.DimValues[chstore.DimBuyer]] = r.Metrics
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %+v, want ann, bob and the unowned", rows)
	}
	ann := by["ann"]
	if ann.Clicks != 400 || ann.UniqueClicks != 280 || *ann.Conversions != 10 || *ann.Revenue != 600 || *ann.Cost != 300 {
		t.Errorf("ann = %+v, want the sum of her two campaigns", ann)
	}
	// ROI from the sums: (600-300)/300 = 100%, not the average of 300% and 0%.
	if ann.ROI == nil || !near(*ann.ROI, 100) || !near(ann.CVR, 10.0/400*100) {
		t.Errorf("ann ROI %v CVR %v, want 100 and 2.5 — from her sums", ann.ROI, ann.CVR)
	}
	if bob := by["bob"]; bob.Clicks != 50 || bob.Cost != nil || bob.ROI != nil {
		t.Errorf("bob = %+v, want 50 clicks and no cost (none entered)", bob)
	}
	if none := by[""]; none.Clicks != 10 {
		t.Errorf("unowned = %+v, want camp-x's 10 clicks", none)
	}
}

func TestReportByBuyerAndCampaignKeepsCampaignRows(t *testing.T) {
	svc := analytics.NewService(buyerRepo(), nil).WithOwners(owners)
	in := buyerRange
	in.Dims = []chstore.ReportDimension{chstore.DimBuyer, chstore.DimCampaign}
	rows, err := svc.Report(context.Background(), "org", in)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want one per campaign", len(rows))
	}
	for _, r := range rows {
		if want := owners[r.DimValues[chstore.DimCampaign]]; r.DimValues[chstore.DimBuyer] != want {
			t.Errorf("campaign %s shows buyer %q, want %q", r.DimValues[chstore.DimCampaign], r.DimValues[chstore.DimBuyer], want)
		}
	}
}

// A buyer filter is "campaign IN that member's campaigns" for every query,
// and the spend is only those campaigns' — the total, too.
func TestBuyerFilterNarrowsQueriesAndSpend(t *testing.T) {
	repo := buyerRepo()
	spend := &fakeCost{byCampaign: []cost.CampaignSpend{
		{CampaignID: "camp-a1", AmountUSD: 100, AllConverted: true},
		{CampaignID: "camp-b", AmountUSD: 999, AllConverted: true},
	}}
	svc := analytics.NewService(repo, spend).WithOwners(owners)
	in := buyerRange
	in.Filters = []chstore.ReportFilter{{Dimension: chstore.DimBuyer, Value: "ann"}}
	rows, err := svc.Report(context.Background(), "org", in)
	if err != nil {
		t.Fatal(err)
	}
	for name, q := range map[string]*chstore.ReportQuery{"clicks": repo.lastClickQuery, "conversions": repo.lastConvQuery} {
		if q == nil || len(q.Filters) != 1 || q.Filters[0].Dimension != chstore.DimCampaign ||
			!slices.Equal(q.Filters[0].In, []string{"camp-a1", "camp-a2"}) {
			t.Errorf("%s query filters = %+v, want campaign IN ann's two", name, q)
		}
	}
	if len(rows) != 1 || rows[0].Metrics.Cost == nil || *rows[0].Metrics.Cost != 100 {
		t.Errorf("total = %+v, want cost 100 — ann's spend only", rows)
	}

	// A member owning nothing: an empty set, which matches nothing.
	in.Filters = []chstore.ReportFilter{{Dimension: chstore.DimBuyer, Value: "carol"}}
	if _, err := svc.Report(context.Background(), "org", in); err != nil {
		t.Fatal(err)
	}
	if f := repo.lastClickQuery.Filters[0]; f.In == nil || len(f.In) != 0 {
		t.Errorf("carol's filter = %+v, want an empty set", f)
	}

	// The funnel takes the same filter.
	if _, err := svc.Funnel(context.Background(), "org", in.From, in.To, []chstore.ReportFilter{{Dimension: chstore.DimBuyer, Value: "bob"}}, ""); err != nil {
		t.Fatal(err)
	}
	if f := repo.lastFunnelArgs.filters; len(f) != 1 || !slices.Equal(f[0].In, []string{"camp-b"}) {
		t.Errorf("funnel filters = %+v, want bob's campaign", f)
	}
}

func TestBuyerValuesAndWithoutOwners(t *testing.T) {
	svc := analytics.NewService(buyerRepo(), nil).WithOwners(owners)
	vals, err := svc.DimensionValues(context.Background(), "org", analytics.DimensionValuesInput{Dim: chstore.DimBuyer, From: buyerRange.From, To: buyerRange.To})
	if err != nil || !slices.Equal(vals, []string{"", "ann", "bob"}) {
		t.Errorf("buyer values = %v, %v; want the owners and the unowned bucket", vals, err)
	}

	in := buyerRange
	in.Dims = []chstore.ReportDimension{chstore.DimBuyer}
	if _, err := analytics.NewService(buyerRepo(), nil).Report(context.Background(), "org", in); err == nil {
		t.Error("a report by buyer without an owner source succeeded")
	}
	failing := analytics.NewService(buyerRepo(), nil).WithOwners(failingOwners{})
	if _, err := failing.Report(context.Background(), "org", in); err == nil {
		t.Error("a report by buyer succeeded with the owners unreadable")
	}
}

type failingOwners struct{}

func (failingOwners) Owners(context.Context, string) (map[string]string, error) {
	return nil, errors.New("postgres down")
}
func (failingOwners) OwnerNames(context.Context, string) ([]analytics.Buyer, error) {
	return nil, errors.New("postgres down")
}
