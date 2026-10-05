package analytics_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/analytics"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/cost"
)

// reportFakeRepo is a fully scripted analytics.Repository for report.go's
// own unit tests — canned per-key aggregates, no real ClickHouse. The
// three chstore.EventStore report methods proper (grouping, filtering,
// the SQL itself) are covered against a real database by
// internal/chstore/report_test.go; what belongs here is
// report.go's own merge/derive/sort logic, which needs none of that.
type reportFakeRepo struct {
	clicks []chstore.ClickAgg
	land   []chstore.LandAgg
	conv   []chstore.ConversionAgg
	funnel map[string]uint64
	values []string

	lastConvQuery   *chstore.ReportQuery // nil unless ReportConversions was actually called
	lastValuesDim   chstore.ReportDimension
	lastValuesTo    time.Time
	lastValuesLimit int
}

func (r *reportFakeRepo) DailyCampaignCounts(context.Context, string, string, time.Time, time.Time) ([]chstore.DailyCount, error) {
	return nil, nil
}
func (r *reportFakeRepo) DailyCampaignRevenue(context.Context, string, string, time.Time, time.Time) ([]chstore.DailyRevenue, error) {
	return nil, nil
}
func (r *reportFakeRepo) ReportClicks(context.Context, chstore.ReportQuery) ([]chstore.ClickAgg, error) {
	return r.clicks, nil
}
func (r *reportFakeRepo) ReportLandViews(context.Context, chstore.ReportQuery) ([]chstore.LandAgg, error) {
	return r.land, nil
}
func (r *reportFakeRepo) ReportConversions(_ context.Context, q chstore.ReportQuery) ([]chstore.ConversionAgg, error) {
	r.lastConvQuery = &q
	return r.conv, nil
}
func (r *reportFakeRepo) FunnelCounts(context.Context, string, time.Time, time.Time) (map[string]uint64, error) {
	return r.funnel, nil
}
func (r *reportFakeRepo) DistinctValues(_ context.Context, _ string, dim chstore.ReportDimension, _, to time.Time, limit int) ([]string, error) {
	r.lastValuesDim = dim
	r.lastValuesTo = to
	r.lastValuesLimit = limit
	return r.values, nil
}

type fakeCost struct {
	byCampaign []cost.CampaignSpend
	byDay      []cost.DailySpend
}

func (c *fakeCost) SpendByCampaign(context.Context, string, time.Time, time.Time) ([]cost.CampaignSpend, error) {
	return c.byCampaign, nil
}
func (c *fakeCost) DailySpendForOrg(context.Context, string, time.Time, time.Time) ([]cost.DailySpend, error) {
	return c.byDay, nil
}

func dimKey(dim chstore.ReportDimension, value string) chstore.ReportKey {
	return chstore.ReportKey{DimValues: map[chstore.ReportDimension]string{dim: value}}
}

func TestReportComputesEveryMetricWhenCostIsAvailable(t *testing.T) {
	repo := &reportFakeRepo{
		clicks: []chstore.ClickAgg{{Key: dimKey(chstore.DimCampaign, "camp-a"), Clicks: 100, UniqueClicks: 90}},
		land:   []chstore.LandAgg{{Key: dimKey(chstore.DimCampaign, "camp-a"), LandViews: 40}},
		conv:   []chstore.ConversionAgg{{Key: dimKey(chstore.DimCampaign, "camp-a"), Conversions: 10, RevenueUSD: 500}},
	}
	costSrc := &fakeCost{byCampaign: []cost.CampaignSpend{{CampaignID: "camp-a", AmountUSD: 200, AllConverted: true}}}
	svc := analytics.NewService(repo, costSrc)

	rows, err := svc.Report(context.Background(), "org-1", analytics.ReportInput{
		Dims: []chstore.ReportDimension{chstore.DimCampaign},
		From: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	m := rows[0].Metrics

	if m.Clicks != 100 || m.UniqueClicks != 90 {
		t.Fatalf("Clicks/UniqueClicks = %d/%d, want 100/90", m.Clicks, m.UniqueClicks)
	}
	if m.Conversions == nil || *m.Conversions != 10 {
		t.Fatalf("Conversions = %v, want 10", m.Conversions)
	}
	if m.Revenue == nil || *m.Revenue != 500 {
		t.Fatalf("Revenue = %v, want 500", m.Revenue)
	}
	if m.Cost == nil || *m.Cost != 200 {
		t.Fatalf("Cost = %v, want 200", m.Cost)
	}
	if m.Profit == nil || *m.Profit != 300 {
		t.Fatalf("Profit = %v, want 300 (500 - 200)", m.Profit)
	}
	if m.ROI == nil || *m.ROI != 150 {
		t.Fatalf("ROI = %v, want 150 ((500-200)/200 * 100)", m.ROI)
	}
	if m.ROAS == nil || *m.ROAS != 2.5 {
		t.Fatalf("ROAS = %v, want 2.5 (500/200)", m.ROAS)
	}
	if m.CTR != 40 {
		t.Fatalf("CTR = %v, want 40 (40 land views / 100 clicks * 100)", m.CTR)
	}
	if m.CVR != 10 {
		t.Fatalf("CVR = %v, want 10 (10 conversions / 100 clicks * 100)", m.CVR)
	}
	if m.CPC == nil || *m.CPC != 2 {
		t.Fatalf("CPC = %v, want 2 (200/100)", m.CPC)
	}
	if m.CPA == nil || *m.CPA != 20 {
		t.Fatalf("CPA = %v, want 20 (200/10)", m.CPA)
	}
	if m.EPC != 5 {
		t.Fatalf("EPC = %v, want 5 (500/100)", m.EPC)
	}
}

func TestReportZeroConversionsIsARealZeroNotNull(t *testing.T) {
	// A campaign with real clicks but zero matching conversion rows —
	// ReportConversions legitimately returns nothing for this key (SQL
	// GROUP BY never emits an empty group). Conversions/Revenue must
	// still be a real 0, not nil ("—") — nil is reserved for "this
	// dimension combo can't be answered at all," not "answered, zero."
	repo := &reportFakeRepo{
		clicks: []chstore.ClickAgg{{Key: dimKey(chstore.DimCampaign, "camp-a"), Clicks: 50, UniqueClicks: 50}},
	}
	svc := analytics.NewService(repo, nil)

	rows, err := svc.Report(context.Background(), "org-1", analytics.ReportInput{
		Dims: []chstore.ReportDimension{chstore.DimCampaign},
		From: time.Now().AddDate(0, 0, -7),
		To:   time.Now(),
	})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	m := rows[0].Metrics
	if m.Conversions == nil || *m.Conversions != 0 {
		t.Fatalf("Conversions = %v, want a real 0, not nil", m.Conversions)
	}
	if m.Revenue == nil || *m.Revenue != 0 {
		t.Fatalf("Revenue = %v, want a real 0, not nil", m.Revenue)
	}
	if m.CVR != 0 {
		t.Fatalf("CVR = %v, want 0", m.CVR)
	}
}

func TestReportNullsConversionMetricsForUnsupportedDimension(t *testing.T) {
	// DimOS has no column on conversion_events — conversions/revenue (and
	// everything built from them) must be nil for every row, and
	// ReportConversions must not even be called.
	repo := &reportFakeRepo{
		clicks: []chstore.ClickAgg{{Key: dimKey(chstore.DimOS, "iOS 18"), Clicks: 20, UniqueClicks: 18}},
	}
	svc := analytics.NewService(repo, nil)

	rows, err := svc.Report(context.Background(), "org-1", analytics.ReportInput{
		Dims: []chstore.ReportDimension{chstore.DimOS},
		From: time.Now().AddDate(0, 0, -7),
		To:   time.Now(),
	})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if repo.lastConvQuery != nil {
		t.Fatal("ReportConversions was called for a dimension conversion_events can't group by")
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	m := rows[0].Metrics
	if m.Conversions != nil {
		t.Fatalf("Conversions = %v, want nil", *m.Conversions)
	}
	if m.Revenue != nil {
		t.Fatalf("Revenue = %v, want nil", *m.Revenue)
	}
	if m.CVR != 0 {
		t.Fatalf("CVR = %v, want 0 (clicks-only metric, unaffected by the conversion gap)", m.CVR)
	}
}

func TestReportNullsCostMetricsWhenGroupingIsFinerThanCampaign(t *testing.T) {
	// Grouping by country: cost_entries has no country breakdown, so
	// Cost/Profit/ROI/ROAS/CPC/CPA must all be nil even though revenue
	// itself is fully known (conversion_events DOES have a country
	// column).
	repo := &reportFakeRepo{
		clicks: []chstore.ClickAgg{{Key: dimKey(chstore.DimCountry, "US"), Clicks: 100, UniqueClicks: 90}},
		conv:   []chstore.ConversionAgg{{Key: dimKey(chstore.DimCountry, "US"), Conversions: 5, RevenueUSD: 250}},
	}
	costSrc := &fakeCost{byCampaign: []cost.CampaignSpend{{CampaignID: "camp-a", AmountUSD: 999}}}
	svc := analytics.NewService(repo, costSrc)

	rows, err := svc.Report(context.Background(), "org-1", analytics.ReportInput{
		Dims: []chstore.ReportDimension{chstore.DimCountry},
		From: time.Now().AddDate(0, 0, -7),
		To:   time.Now(),
	})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	m := rows[0].Metrics
	if m.Revenue == nil || *m.Revenue != 250 {
		t.Fatalf("Revenue = %v, want 250 (unaffected by the cost gap)", m.Revenue)
	}
	if m.Cost != nil || m.Profit != nil || m.ROI != nil || m.ROAS != nil || m.CPC != nil || m.CPA != nil {
		t.Fatalf("cost-derived metrics = %+v, want all nil for a country breakdown", m)
	}
}

func TestReportNoDataAtAllStillReturnsOneZeroRow(t *testing.T) {
	// A brand-new organization, no Dims requested: real ClickHouse
	// returns exactly one all-zero row for a GROUP-BY-less aggregate over
	// zero matching rows (count() of nothing is a real 0, not an absent
	// row — verified against a real database in
	// internal/chstore/report_test.go's own TestCrossTenantIsolationInReports).
	// This fake mirrors that same shape rather than the more naive "empty
	// slice in, empty slice out" a dumber fake might return, specifically
	// so this test exercises the same "exactly one zero row" path
	// Service.Dashboard's totals depend on for a real /overview render.
	repo := &reportFakeRepo{
		clicks: []chstore.ClickAgg{{Key: chstore.ReportKey{}}},
		land:   []chstore.LandAgg{{Key: chstore.ReportKey{}}},
		conv:   []chstore.ConversionAgg{{Key: chstore.ReportKey{}}},
	}
	svc := analytics.NewService(repo, &fakeCost{})

	rows, err := svc.Report(context.Background(), "org-1", analytics.ReportInput{
		From: time.Now().AddDate(0, 0, -7),
		To:   time.Now(),
	})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want exactly 1 all-zero row", len(rows))
	}
	m := rows[0].Metrics
	if m.Clicks != 0 || m.UniqueClicks != 0 || m.CTR != 0 || m.CVR != 0 || m.EPC != 0 {
		t.Fatalf("Metrics = %+v, want every field zero", m)
	}
	if m.Conversions == nil || *m.Conversions != 0 {
		t.Fatalf("Conversions = %v, want a real 0", m.Conversions)
	}
}

func TestReportRejectsTooManyDimensions(t *testing.T) {
	svc := analytics.NewService(&reportFakeRepo{}, nil)
	dims := []chstore.ReportDimension{chstore.DimCampaign, chstore.DimCountry, chstore.DimDevice, chstore.DimOS, chstore.DimBrowser}

	_, err := svc.Report(context.Background(), "org-1", analytics.ReportInput{
		Dims: dims,
		From: time.Now().AddDate(0, 0, -7),
		To:   time.Now(),
	})
	if err == nil {
		t.Fatal("Report with 5 dimensions succeeded, want a validation error")
	}
}

func TestReportRejectsUnknownDimension(t *testing.T) {
	svc := analytics.NewService(&reportFakeRepo{}, nil)
	_, err := svc.Report(context.Background(), "org-1", analytics.ReportInput{
		Dims: []chstore.ReportDimension{"pwa"}, // one of the 3 deferred dimensions
		From: time.Now().AddDate(0, 0, -7),
		To:   time.Now(),
	})
	if err == nil {
		t.Fatal("Report with dimension \"pwa\" succeeded, want a validation error (deferred, no backing column)")
	}
}

func TestReportSortsByMetricDescending(t *testing.T) {
	repo := &reportFakeRepo{
		clicks: []chstore.ClickAgg{
			{Key: dimKey(chstore.DimCampaign, "low"), Clicks: 10},
			{Key: dimKey(chstore.DimCampaign, "high"), Clicks: 100},
			{Key: dimKey(chstore.DimCampaign, "mid"), Clicks: 50},
		},
	}
	svc := analytics.NewService(repo, nil)

	rows, err := svc.Report(context.Background(), "org-1", analytics.ReportInput{
		Dims: []chstore.ReportDimension{chstore.DimCampaign},
		From: time.Now().AddDate(0, 0, -7),
		To:   time.Now(),
		Sort: analytics.SortSpec{Key: "clicks", Dir: "desc"},
	})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	got := []string{rows[0].DimValues[chstore.DimCampaign], rows[1].DimValues[chstore.DimCampaign], rows[2].DimValues[chstore.DimCampaign]}
	want := []string{"high", "mid", "low"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sort order = %v, want %v", got, want)
		}
	}
}

func TestBreakdownLimitsAndSorts(t *testing.T) {
	repo := &reportFakeRepo{
		clicks: []chstore.ClickAgg{
			{Key: dimKey(chstore.DimCountry, "US"), Clicks: 10},
			{Key: dimKey(chstore.DimCountry, "DE"), Clicks: 30},
			{Key: dimKey(chstore.DimCountry, "FR"), Clicks: 20},
		},
	}
	svc := analytics.NewService(repo, nil)

	rows, err := svc.Breakdown(context.Background(), "org-1", analytics.BreakdownInput{
		Dim:    chstore.DimCountry,
		Metric: "clicks",
		From:   time.Now().AddDate(0, 0, -7),
		To:     time.Now(),
		Limit:  2,
	})
	if err != nil {
		t.Fatalf("Breakdown: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (Limit)", len(rows))
	}
	if rows[0].DimValues[chstore.DimCountry] != "DE" || rows[1].DimValues[chstore.DimCountry] != "FR" {
		t.Fatalf("order = %v, want [DE, FR] (top 2 by clicks)", rows)
	}
}

func TestFunnelZeroFillsEveryStage(t *testing.T) {
	repo := &reportFakeRepo{funnel: map[string]uint64{"SOURCE_CLICK": 100, "CPA_ACCEPT": 3}}
	svc := analytics.NewService(repo, nil)

	steps, err := svc.Funnel(context.Background(), "org-1", time.Now().AddDate(0, 0, -7), time.Now())
	if err != nil {
		t.Fatalf("Funnel: %v", err)
	}
	if len(steps) != 8 {
		t.Fatalf("steps = %d, want 8 (full §43 funnel)", len(steps))
	}
	byStep := map[string]uint64{}
	for _, s := range steps {
		byStep[s.Step] = s.Value
	}
	if byStep["SOURCE_CLICK"] != 100 || byStep["CPA_ACCEPT"] != 3 {
		t.Fatalf("steps = %+v, want SOURCE_CLICK=100 CPA_ACCEPT=3", byStep)
	}
	if byStep["PWA_INSTALL"] != 0 {
		t.Fatalf(`byStep["PWA_INSTALL"] = %d, want 0 (zero-filled, not absent)`, byStep["PWA_INSTALL"])
	}
}

func TestDashboardOmitsTopOffers(t *testing.T) {
	// DashboardResult has no TopOffers field at all. "offer" became a
	// backed dimension (chstore schema/009), but the dashboard card is a
	// separate product decision nobody has made yet — the type keeps
	// apps/web from rendering one by accident until then.
	repo := &reportFakeRepo{
		clicks: []chstore.ClickAgg{{Key: chstore.ReportKey{}, Clicks: 10, UniqueClicks: 9}},
	}
	svc := analytics.NewService(repo, nil)

	result, err := svc.Dashboard(context.Background(), "org-1", time.Now().AddDate(0, 0, -7), time.Now())
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if result.Totals.Clicks != 10 {
		t.Fatalf("Totals.Clicks = %d, want 10", result.Totals.Clicks)
	}
	// Compile-time enforced (no result.TopOffers field exists) — this
	// assertion just documents the three that DO exist.
	_ = result.TopCampaigns
	_ = result.TopCountries
	_ = result.TopFlows
}

func TestDashboardNewOrganizationIsAllZeros(t *testing.T) {
	svc := analytics.NewService(&reportFakeRepo{}, &fakeCost{})

	result, err := svc.Dashboard(context.Background(), "org-1", time.Now().AddDate(0, 0, -7), time.Now())
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if result.Totals.Clicks != 0 || result.Totals.UniqueClicks != 0 {
		t.Fatalf("Totals = %+v, want all-zero for a brand-new organization", result.Totals)
	}
	if len(result.Daily) != 0 || len(result.TopCampaigns) != 0 {
		t.Fatalf("Daily/TopCampaigns = %d/%d, want 0/0 with nothing to break down", len(result.Daily), len(result.TopCampaigns))
	}
}

func TestDimensionValuesPassesThroughAndCaps(t *testing.T) {
	repo := &reportFakeRepo{values: []string{"US", "DE", "GB"}}
	svc := analytics.NewService(repo, nil)

	to := time.Now()
	values, err := svc.DimensionValues(context.Background(), "org-1", analytics.DimensionValuesInput{
		Dim:  chstore.DimCountry,
		From: to.AddDate(0, 0, -7),
		To:   to,
	})
	if err != nil {
		t.Fatalf("DimensionValues: %v", err)
	}
	if len(values) != 3 || values[0] != "US" || values[1] != "DE" || values[2] != "GB" {
		t.Fatalf("values = %v, want the repo's own [US DE GB] passed through unchanged", values)
	}
	if repo.lastValuesDim != chstore.DimCountry {
		t.Fatalf("DistinctValues called with dim=%q, want country", repo.lastValuesDim)
	}
	if repo.lastValuesLimit <= 0 {
		t.Fatalf("DistinctValues called with limit=%d, want a real positive cap", repo.lastValuesLimit)
	}
	// The caller's To is an inclusive "last day" (a date picker's own
	// selection), but chstore.ReportQuery.To is EXCLUSIVE — DimensionValues
	// must add the same +24h groupedReport applies centrally for Report/
	// Timeseries/Breakdown/Dashboard, since this method calls the
	// repository directly. Missing it would silently exclude "today"
	// whenever To is today's date — exactly a fresh page load's own
	// default range, caught live via a real browser pass before this
	// test was added to guard the regression.
	if !repo.lastValuesTo.Equal(to.Add(24 * time.Hour)) {
		t.Fatalf("DistinctValues called with to=%v, want the caller's To (%v) plus 24h (%v)", repo.lastValuesTo, to, to.Add(24*time.Hour))
	}
}

// TestDimensionValuesRejectsUnbackedDimension mirrors
// TestReportRejectsUnknownDimension — the frontend's own fallback to its
// mock pool for the 3 still-unbacked dimensions depends on this coming
// back as a real validation error, not a silent empty list or a panic.
func TestDimensionValuesRejectsUnbackedDimension(t *testing.T) {
	svc := analytics.NewService(&reportFakeRepo{}, nil)
	_, err := svc.DimensionValues(context.Background(), "org-1", analytics.DimensionValuesInput{
		Dim:  "pwa", // one of the 3 deferred dimensions
		From: time.Now().AddDate(0, 0, -7),
		To:   time.Now(),
	})
	if err == nil {
		t.Fatal("DimensionValues with dimension \"pwa\" succeeded, want a validation error (deferred, no backing column)")
	}
}

func TestDimensionValuesRejectsMissingOrganization(t *testing.T) {
	svc := analytics.NewService(&reportFakeRepo{}, nil)
	_, err := svc.DimensionValues(context.Background(), "", analytics.DimensionValuesInput{
		Dim:  chstore.DimCountry,
		From: time.Now().AddDate(0, 0, -7),
		To:   time.Now(),
	})
	if err == nil {
		t.Fatal("DimensionValues with no organization succeeded, want a validation error")
	}
}
