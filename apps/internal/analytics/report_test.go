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

	lastConvQuery  *chstore.ReportQuery // nil unless ReportConversions was actually called
	lastClickQuery *chstore.ReportQuery
	lastFunnelArgs struct {
		from, to time.Time
		filters  []chstore.ReportFilter
	}
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
func (r *reportFakeRepo) ReportClicks(_ context.Context, q chstore.ReportQuery) ([]chstore.ClickAgg, error) {
	r.lastClickQuery = &q
	return r.clicks, nil
}
func (r *reportFakeRepo) ReportLandViews(context.Context, chstore.ReportQuery) ([]chstore.LandAgg, error) {
	return r.land, nil
}
func (r *reportFakeRepo) ReportConversions(_ context.Context, q chstore.ReportQuery) ([]chstore.ConversionAgg, error) {
	r.lastConvQuery = &q
	return r.conv, nil
}
func (r *reportFakeRepo) FunnelCounts(_ context.Context, _ string, from, to time.Time, filters []chstore.ReportFilter) (map[string]uint64, error) {
	r.lastFunnelArgs.from, r.lastFunnelArgs.to, r.lastFunnelArgs.filters = from, to, filters
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
	bySource   []cost.SourceSpend

	campaignDay []cost.CampaignDaySpend
	sourceDay   []cost.SourceDaySpend
	byDay       []cost.DailySpend
}

func (c *fakeCost) SpendByCampaign(context.Context, string, time.Time, time.Time) ([]cost.CampaignSpend, error) {
	return c.byCampaign, nil
}
func (c *fakeCost) CampaignSpendByDay(context.Context, string, time.Time, time.Time) ([]cost.CampaignDaySpend, error) {
	return c.campaignDay, nil
}
func (c *fakeCost) SourceSpendByDay(context.Context, string, time.Time, time.Time) ([]cost.SourceDaySpend, error) {
	return c.sourceDay, nil
}
func (c *fakeCost) SpendBySource(context.Context, string, time.Time, time.Time) ([]cost.SourceSpend, error) {
	return c.bySource, nil
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

func TestReportQueriesConversionsForClickOnlyDimension(t *testing.T) {
	// DimOS has no column on conversion_events, but the store answers it
	// through a click_id join — so conversions/revenue are real values, and
	// ReportConversions IS called (with the os dimension, filter included).
	repo := &reportFakeRepo{
		clicks: []chstore.ClickAgg{{Key: dimKey(chstore.DimOS, "linux"), Clicks: 20, UniqueClicks: 18}},
		conv:   []chstore.ConversionAgg{{Key: dimKey(chstore.DimOS, "linux"), Conversions: 4, RevenueUSD: 80}},
	}
	svc := analytics.NewService(repo, nil)

	rows, err := svc.Report(context.Background(), "org-1", analytics.ReportInput{
		Dims:    []chstore.ReportDimension{chstore.DimOS},
		Filters: []chstore.ReportFilter{{Dimension: chstore.DimBrowser, Value: "Chrome"}},
		From:    time.Now().AddDate(0, 0, -7),
		To:      time.Now(),
	})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if repo.lastConvQuery == nil {
		t.Fatal("ReportConversions was not called for os/browser, which the click join can answer")
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	m := rows[0].Metrics
	if m.Conversions == nil || *m.Conversions != 4 || m.Revenue == nil || *m.Revenue != 80 {
		t.Fatalf("Conversions/Revenue = %v / %v, want 4 / 80", m.Conversions, m.Revenue)
	}
	if m.CVR != 20 {
		t.Fatalf("CVR = %v, want 20 (4 conversions / 20 clicks)", m.CVR)
	}
}

func TestReportCostBySourceSumsTheSourcesCampaigns(t *testing.T) {
	// Grouped by source, cost is the source's total (all its campaigns),
	// not any one campaign's.
	repo := &reportFakeRepo{
		clicks: []chstore.ClickAgg{{Key: dimKey(chstore.DimSource, "src-fb"), Clicks: 100, UniqueClicks: 90}},
		conv:   []chstore.ConversionAgg{{Key: dimKey(chstore.DimSource, "src-fb"), Conversions: 5, RevenueUSD: 300}},
	}
	costSrc := &fakeCost{
		byCampaign: []cost.CampaignSpend{{CampaignID: "camp-a", AmountUSD: 60}, {CampaignID: "camp-b", AmountUSD: 40}},
		bySource:   []cost.SourceSpend{{TrafficSourceID: "src-fb", AmountUSD: 100}},
	}
	svc := analytics.NewService(repo, costSrc)

	rows, err := svc.Report(context.Background(), "org-1", analytics.ReportInput{
		Dims: []chstore.ReportDimension{chstore.DimSource},
		From: time.Now().AddDate(0, 0, -7),
		To:   time.Now(),
	})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	m := rows[0].Metrics
	if m.Cost == nil || *m.Cost != 100 || m.Profit == nil || *m.Profit != 200 {
		t.Fatalf("Cost/Profit = %v / %v, want 100 / 200", m.Cost, m.Profit)
	}
}

func dayKey(day time.Time, dim chstore.ReportDimension, value string) chstore.ReportKey {
	d := day
	return chstore.ReportKey{Day: &d, DimValues: map[chstore.ReportDimension]string{dim: value}}
}

func TestReportDayDimensionGroupsByDayAndKeepsOtherDims(t *testing.T) {
	d1 := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	repo := &reportFakeRepo{
		clicks: []chstore.ClickAgg{
			{Key: dayKey(d1, chstore.DimCampaign, "camp-a"), Clicks: 10},
			{Key: dayKey(d2, chstore.DimCampaign, "camp-a"), Clicks: 30},
		},
		conv: []chstore.ConversionAgg{{Key: dayKey(d2, chstore.DimCampaign, "camp-a"), Conversions: 3, RevenueUSD: 90}},
	}
	costSrc := &fakeCost{campaignDay: []cost.CampaignDaySpend{
		{CampaignID: "camp-a", Day: d1, AmountUSD: 5},
		{CampaignID: "camp-a", Day: d2, AmountUSD: 20},
	}}
	svc := analytics.NewService(repo, costSrc)

	rows, err := svc.Report(context.Background(), "org-1", analytics.ReportInput{
		Dims: []chstore.ReportDimension{chstore.DimCampaign, analytics.DimDay},
		From: d1, To: d2,
		Sort: analytics.SortSpec{Key: "day", Dir: "desc"},
	})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if repo.lastClickQuery == nil || !repo.lastClickQuery.ByDay || len(repo.lastClickQuery.Dims) != 1 || repo.lastClickQuery.Dims[0] != chstore.DimCampaign {
		t.Fatalf("click query = %+v, want ByDay with only the campaign dimension (day is not a column)", repo.lastClickQuery)
	}
	if len(rows) != 2 || rows[0].Day == nil || !rows[0].Day.Equal(d2) || !rows[1].Day.Equal(d1) {
		t.Fatalf("rows = %+v, want 2 rows sorted by day descending", rows)
	}
	if rows[0].DimValues[chstore.DimCampaign] != "camp-a" {
		t.Fatalf("campaign dim lost on a day row: %+v", rows[0].DimValues)
	}
	m0, m1 := rows[0].Metrics, rows[1].Metrics
	if m0.Cost == nil || *m0.Cost != 20 || m0.Profit == nil || *m0.Profit != 70 {
		t.Fatalf("day 2 cost/profit = %v / %v, want 20 / 70 (that day's own spend)", m0.Cost, m0.Profit)
	}
	if m1.Cost == nil || *m1.Cost != 5 {
		t.Fatalf("day 1 cost = %v, want 5", m1.Cost)
	}
}

func TestReportDayCountsTowardTheDimensionLimit(t *testing.T) {
	svc := analytics.NewService(&reportFakeRepo{}, nil)
	_, err := svc.Report(context.Background(), "org-1", analytics.ReportInput{
		Dims: []chstore.ReportDimension{chstore.DimCampaign, chstore.DimCountry, chstore.DimDevice, chstore.DimOS, analytics.DimDay},
		From: time.Now().AddDate(0, 0, -1), To: time.Now(),
	})
	if err == nil {
		t.Fatal("5 dimensions (4 + day) succeeded, want the max-4 validation error")
	}
}

func TestReportTimezoneMovesDayBoundaries(t *testing.T) {
	repo := &reportFakeRepo{}
	svc := analytics.NewService(repo, nil)
	from := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	_, err := svc.Report(context.Background(), "org-1", analytics.ReportInput{
		Dims: []chstore.ReportDimension{analytics.DimDay},
		From: from, To: from, Timezone: "America/New_York",
	})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	q := repo.lastClickQuery
	// New York is UTC-4 in August: local 10 Aug 00:00 = 04:00Z, to-exclusive = 11 Aug 04:00Z.
	if q == nil || q.Timezone != "America/New_York" ||
		!q.From.Equal(time.Date(2026, 8, 10, 4, 0, 0, 0, time.UTC)) || !q.To.Equal(time.Date(2026, 8, 11, 4, 0, 0, 0, time.UTC)) {
		t.Fatalf("query = %+v, want New York local-midnight bounds 10 Aug 04:00Z .. 11 Aug 04:00Z", q)
	}

	if _, err := svc.Report(context.Background(), "org-1", analytics.ReportInput{From: from, To: from, Timezone: "Mars/Olympus"}); err == nil {
		t.Fatal("unknown timezone succeeded, want a validation error")
	}
}

func TestFunnelPassesFiltersAndTimezone(t *testing.T) {
	repo := &reportFakeRepo{funnel: map[string]uint64{"SOURCE_CLICK": 7}}
	svc := analytics.NewService(repo, nil)
	day := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	filters := []chstore.ReportFilter{{Dimension: chstore.DimCampaign, Value: "camp-a"}}
	steps, err := svc.Funnel(context.Background(), "org-1", day, day, filters, "")
	if err != nil {
		t.Fatalf("Funnel: %v", err)
	}
	if len(repo.lastFunnelArgs.filters) != 1 || repo.lastFunnelArgs.filters[0].Value != "camp-a" {
		t.Fatalf("funnel filters = %+v, want the campaign filter to reach the store", repo.lastFunnelArgs.filters)
	}
	if !repo.lastFunnelArgs.to.Equal(day.Add(24 * time.Hour)) {
		t.Fatalf("funnel to = %v, want exclusive end of the last day", repo.lastFunnelArgs.to)
	}
	if steps[0].Value != 7 {
		t.Fatalf("steps[0] = %+v, want 7", steps[0])
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
		Dims: []chstore.ReportDimension{"postlanding"}, // one of the 2 deferred dimensions
		From: time.Now().AddDate(0, 0, -7),
		To:   time.Now(),
	})
	if err == nil {
		t.Fatal("Report with dimension \"postlanding\" succeeded, want a validation error (deferred, no backing column)")
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

	steps, err := svc.Funnel(context.Background(), "org-1", time.Now().AddDate(0, 0, -7), time.Now(), nil, "")
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
// mock pool for the 2 still-unbacked dimensions depends on this coming
// back as a real validation error, not a silent empty list or a panic.
func TestDimensionValuesRejectsUnbackedDimension(t *testing.T) {
	svc := analytics.NewService(&reportFakeRepo{}, nil)
	_, err := svc.DimensionValues(context.Background(), "org-1", analytics.DimensionValuesInput{
		Dim:  "postlanding", // one of the 2 deferred dimensions
		From: time.Now().AddDate(0, 0, -7),
		To:   time.Now(),
	})
	if err == nil {
		t.Fatal("DimensionValues with dimension \"postlanding\" succeeded, want a validation error (deferred, no backing column)")
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
