package analytics

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
)

// maxReportDims bounds how many dimensions one report can group by at
// once — not requested by any spec section, a deliberate, documented
// guard against a client asking for an 11-way cross-product that would
// return an enormous, useless row count, the same spirit as
// maxRangeDays in analytics.go.
const maxReportDims = 4

// MetricValues is the full §50 metrics-registry row every report/
// timeseries/breakdown/dashboard endpoint returns — ports
// apps/web/src/features/analytics/aggregate.ts's deriveMetrics 1:1 so the
// two never drift on formulas (that file's own client-side version is
// deleted once apps/web switches to this endpoint, not kept as a second
// implementation — CLAUDE.md's "no duplicate business logic"). Pointer
// fields are nil exactly when CLAUDE.md #6 says they must be: no cost
// data on file (Cost/Profit/ROI/ROAS/CPC/CPA), or the requested
// dimensions can't be resolved against conversion_events' narrower
// column set (Conversions/Revenue too, in that case — see
// chstore.SupportedByConversions).
type MetricValues struct {
	Clicks       uint64
	UniqueClicks uint64
	Conversions  *uint64
	Revenue      *float64
	Cost         *float64
	Profit       *float64
	ROI          *float64
	ROAS         *float64
	CTR          float64
	CVR          float64
	CPC          *float64
	CPA          *float64
	EPC          float64
}

// ReportRow is one output row of Report/Timeseries/Breakdown/Dashboard —
// Day is set (DimValues empty) for a timeseries row, DimValues is set
// (Day nil) for a report/breakdown row; nothing in Phase 35's own API
// surface needs both on the same row (see chstore.ReportQuery's identical
// note).
type ReportRow struct {
	Day       *time.Time
	DimValues map[chstore.ReportDimension]string
	Metrics   MetricValues
}

// ReportInput is Report's request shape — mirrors
// apps/web/src/features/analytics/report-controls.tsx's ReportControlsState
// almost exactly (Timezone/Compare aren't wired yet: no query in this
// package is timezone-aware beyond ClickHouse's own UTC storage, and
// period-over-period comparison is a follow-up, not silently faked here).
type ReportInput struct {
	Dims     []chstore.ReportDimension
	Filters  []chstore.ReportFilter
	From, To time.Time
	Sort     SortSpec
}

// SortSpec sorts the final, fully-derived rows — after merging and
// deriving every metric, the same order aggregate.ts's own client-side
// sort ran in, since a sort by a derived metric (revenue, roi, ...) can't
// happen any earlier than that. Key is a dimension name or a MetricValues
// field name (case-insensitive: "revenue", "campaign", ...); an unknown
// Key sorts nothing (defensive default, not an error — a client sending
// a stale/typo'd sort key shouldn't 500 the whole report).
type SortSpec struct {
	Key string
	Dir string // "asc" | "desc"
}

func (s *Service) validateReportInput(dims []chstore.ReportDimension, filters []chstore.ReportFilter, from, to time.Time) error {
	if to.Before(from) {
		return apierror.Validation("to must not be before from", map[string]string{"to": "before from"})
	}
	if to.Sub(from) > maxRangeDays*24*time.Hour {
		return apierror.Validation("date range too wide", map[string]string{"to": "more than 366 days after from"})
	}
	if len(dims) > maxReportDims {
		return apierror.Validation(fmt.Sprintf("too many dimensions, max %d", maxReportDims), map[string]string{"dimensions": "too many"})
	}
	for _, d := range dims {
		if !chstore.ValidDimension(d) {
			return apierror.Validation("unknown dimension", map[string]string{"dimensions": string(d) + " is not a supported dimension"})
		}
	}
	for _, f := range filters {
		if !chstore.ValidDimension(f.Dimension) {
			return apierror.Validation("unknown filter dimension", map[string]string{"filters": string(f.Dimension) + " is not a supported dimension"})
		}
	}
	return nil
}

// Report is the report builder table's own endpoint — group by 0-4
// dimensions, filter, sort, get back every §50 metric per group.
func (s *Service) Report(ctx context.Context, organizationID string, in ReportInput) ([]ReportRow, error) {
	if organizationID == "" {
		return nil, apierror.Validation("missing organization", nil)
	}
	if err := s.validateReportInput(in.Dims, in.Filters, in.From, in.To); err != nil {
		return nil, err
	}
	defer observeLatency("report")()

	rows, err := s.groupedReport(ctx, organizationID, in.Dims, false, in.Filters, in.From, in.To)
	if err != nil {
		return nil, err
	}
	sortRows(rows, in.Dims, in.Sort)
	return rows, nil
}

// TimeseriesInput is one metric's daily series — Report's Dims is always
// empty here (a line chart plots one value per day, not per group); the
// response still carries every metric per day, not just the one the
// caller asked about, so apps/web's MetricPicker can switch which field
// it plots without a second request (matches the instant client-side
// switch the mock version already gave for free).
type TimeseriesInput struct {
	Filters  []chstore.ReportFilter
	From, To time.Time
}

func (s *Service) Timeseries(ctx context.Context, organizationID string, in TimeseriesInput) ([]ReportRow, error) {
	if organizationID == "" {
		return nil, apierror.Validation("missing organization", nil)
	}
	if err := s.validateReportInput(nil, in.Filters, in.From, in.To); err != nil {
		return nil, err
	}
	defer observeLatency("timeseries")()

	rows, err := s.groupedReport(ctx, organizationID, nil, true, in.Filters, in.From, in.To)
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Day.Before(*rows[j].Day) })
	return rows, nil
}

// BreakdownInput is the bar chart's own endpoint — exactly one dimension,
// sorted by one metric descending, capped at Limit (defaulted to 8,
// matching aggregate.ts's own aggregateByDimension default).
type BreakdownInput struct {
	Dim      chstore.ReportDimension
	Metric   string
	Filters  []chstore.ReportFilter
	From, To time.Time
	Limit    int
}

func (s *Service) Breakdown(ctx context.Context, organizationID string, in BreakdownInput) ([]ReportRow, error) {
	if organizationID == "" {
		return nil, apierror.Validation("missing organization", nil)
	}
	dims := []chstore.ReportDimension{in.Dim}
	if err := s.validateReportInput(dims, in.Filters, in.From, in.To); err != nil {
		return nil, err
	}
	defer observeLatency("breakdown")()

	rows, err := s.groupedReport(ctx, organizationID, dims, false, in.Filters, in.From, in.To)
	if err != nil {
		return nil, err
	}
	sortRows(rows, dims, SortSpec{Key: in.Metric, Dir: "desc"})

	limit := in.Limit
	if limit <= 0 {
		limit = 8
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

// groupedReport is the shared core Report/Timeseries/Breakdown/Dashboard
// all build on: three ClickHouse queries (click_events, tracking_events,
// conversion_events) with the same dims/byDay/filters/range, merged by
// group key, plus a Postgres cost lookup joined in when the requested
// dims resolve to campaign granularity or coarser — see costEligible.
// Nothing here sorts or limits; that's each public method's own job,
// since only they know what "sorted by" and "top-N" mean for their
// specific request.
func (s *Service) groupedReport(ctx context.Context, organizationID string, dims []chstore.ReportDimension, byDay bool, filters []chstore.ReportFilter, from, to time.Time) ([]ReportRow, error) {
	q := chstore.ReportQuery{
		OrganizationID: organizationID,
		Dims:           dims,
		ByDay:          byDay,
		Filters:        filters,
		From:           from,
		// To is exclusive (chstore.ReportQuery's own doc) — from/to arrive
		// here as day-precision UTC midnights (handler.go's own parsing),
		// so +24h includes the entirety of the last requested day.
		To: to.Add(24 * time.Hour),
	}

	clickAggs, err := s.repo.ReportClicks(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("analytics: querying click report: %w", err)
	}
	landAggs, err := s.repo.ReportLandViews(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("analytics: querying land-view report: %w", err)
	}

	convSupported := chstore.SupportedByConversions(dims)
	var convAggs []chstore.ConversionAgg
	if convSupported {
		convAggs, err = s.repo.ReportConversions(ctx, q)
		if err != nil {
			return nil, fmt.Errorf("analytics: querying conversion report: %w", err)
		}
	}

	merged := map[string]*rowAccumulator{}
	var order []string
	get := func(key chstore.ReportKey) *rowAccumulator {
		k := rowKeyString(key, dims)
		if acc, ok := merged[k]; ok {
			return acc
		}
		acc := &rowAccumulator{day: key.Day, dimValues: key.DimValues}
		merged[k] = acc
		order = append(order, k)
		return acc
	}

	for _, a := range clickAggs {
		acc := get(a.Key)
		acc.clicks = a.Clicks
		acc.uniqueClicks = a.UniqueClicks
	}
	for _, a := range landAggs {
		acc := get(a.Key)
		acc.landViews = a.LandViews
	}
	if convSupported {
		// Default every row seen so far (from clicks/land) to a real zero
		// — a group with clicks but genuinely no conversions must render
		// as "0", not "—" (nil means "conversion_events can't answer
		// this dimension combo at all," a property of the whole request,
		// not of any one group; see conversionColumns' own doc). Only
		// !convSupported groups stay nil, for every row, below.
		for _, acc := range merged {
			zero, zeroF := uint64(0), 0.0
			acc.conversions, acc.revenueUSD = &zero, &zeroF
		}
		for _, a := range convAggs {
			// get() here can legitimately create a new row: a click and
			// its eventual conversion don't have to share the same
			// event_at day if the query range starts mid-journey, so a
			// group can appear in ReportConversions without ever
			// appearing in ReportClicks/ReportLandViews.
			acc := get(a.Key)
			conversions, revenue := a.Conversions, a.RevenueUSD
			acc.conversions = &conversions
			acc.revenueUSD = &revenue
		}
	}

	if s.cost != nil {
		if err := s.attachCost(ctx, organizationID, dims, byDay, from, to, merged); err != nil {
			return nil, err
		}
	}

	rows := make([]ReportRow, 0, len(order))
	for _, k := range order {
		acc := merged[k]
		rows = append(rows, ReportRow{
			Day:       acc.day,
			DimValues: acc.dimValues,
			Metrics:   deriveMetrics(acc),
		})
	}
	return rows, nil
}

// costEligible reports whether dims resolves to campaign granularity or
// coarser — the only shape cost_entries (Postgres, campaign+day grained,
// no country/device/os/... breakdown) can answer. DimSource is
// equivalent to DimCampaign here (same underlying column, see
// chstore.DimSource's own doc) — grouping by it alone is exactly as
// cost-eligible as grouping by campaign alone.
func costEligible(dims []chstore.ReportDimension) bool {
	if len(dims) == 0 {
		return true
	}
	return len(dims) == 1 && (dims[0] == chstore.DimCampaign || dims[0] == chstore.DimSource)
}

// attachCost fills acc.costUSD (and leaves it nil where CLAUDE.md #6
// requires) for every row groupedReport has already built.
func (s *Service) attachCost(ctx context.Context, organizationID string, dims []chstore.ReportDimension, byDay bool, from, to time.Time, merged map[string]*rowAccumulator) error {
	if !costEligible(dims) {
		return nil // every acc.costUSD stays nil — no dimension breakdown cost_entries can answer
	}

	if byDay {
		spend, err := s.cost.DailySpendForOrg(ctx, organizationID, from, to)
		if err != nil {
			return fmt.Errorf("analytics: querying daily org spend: %w", err)
		}
		byDayUSD := make(map[string]float64, len(spend))
		for _, d := range spend {
			byDayUSD[d.Day.Format("2006-01-02")] = d.AmountUSD
		}
		for _, acc := range merged {
			if acc.day == nil {
				continue
			}
			if usd, ok := byDayUSD[acc.day.Format("2006-01-02")]; ok {
				v := usd
				acc.costUSD = &v
			}
		}
		return nil
	}

	if len(dims) == 0 {
		// One grand total across every campaign — sum SpendByCampaign's
		// own per-campaign rows rather than adding a third cost query
		// shape for "just the org total."
		spend, err := s.cost.SpendByCampaign(ctx, organizationID, from, to)
		if err != nil {
			return fmt.Errorf("analytics: querying spend by campaign: %w", err)
		}
		if len(spend) == 0 {
			return nil // no cost_entries at all — stays nil, not a fabricated 0
		}
		var total float64
		for _, c := range spend {
			total += c.AmountUSD
		}
		for _, acc := range merged {
			v := total
			acc.costUSD = &v
		}
		return nil
	}

	// dims == [campaign] or [source] — per-campaign cost, keyed by the
	// same campaign_id value either dimension's column holds.
	spend, err := s.cost.SpendByCampaign(ctx, organizationID, from, to)
	if err != nil {
		return fmt.Errorf("analytics: querying spend by campaign: %w", err)
	}
	byCampaign := make(map[string]float64, len(spend))
	for _, c := range spend {
		byCampaign[c.CampaignID] = c.AmountUSD
	}
	for _, acc := range merged {
		campaignID := acc.dimValues[dims[0]]
		if usd, ok := byCampaign[campaignID]; ok {
			v := usd
			acc.costUSD = &v
		}
	}
	return nil
}

// rowAccumulator is groupedReport's mutable working row, before
// deriveMetrics turns it into the public, immutable MetricValues.
type rowAccumulator struct {
	day       *time.Time
	dimValues map[chstore.ReportDimension]string

	clicks, uniqueClicks, landViews uint64
	conversions                     *uint64
	revenueUSD                      *float64
	costUSD                         *float64
}

// deriveMetrics ports apps/web/src/features/analytics/aggregate.ts's
// deriveMetrics formula for formula, field for field — see that
// function's own doc comment for why nothing here may drift from it.
func deriveMetrics(acc *rowAccumulator) MetricValues {
	m := MetricValues{
		Clicks:       acc.clicks,
		UniqueClicks: acc.uniqueClicks,
		Conversions:  acc.conversions,
	}

	if acc.clicks > 0 {
		m.CTR = float64(acc.landViews) / float64(acc.clicks) * 100
	}
	if acc.conversions != nil && acc.clicks > 0 {
		m.CVR = float64(*acc.conversions) / float64(acc.clicks) * 100
	}
	if acc.conversions != nil && acc.clicks > 0 {
		rev := 0.0
		if acc.revenueUSD != nil {
			rev = *acc.revenueUSD
		}
		m.EPC = rev / float64(acc.clicks)
	}

	if acc.revenueUSD != nil {
		rev := *acc.revenueUSD
		m.Revenue = &rev
	}

	if acc.costUSD != nil {
		costVal := *acc.costUSD
		m.Cost = &costVal

		if m.Revenue != nil {
			profit := *m.Revenue - costVal
			m.Profit = &profit
			if costVal > 0 {
				roi := profit / costVal * 100
				m.ROI = &roi
				roas := *m.Revenue / costVal
				m.ROAS = &roas
			}
		}
		if acc.clicks > 0 && costVal > 0 {
			cpc := costVal / float64(acc.clicks)
			m.CPC = &cpc
		}
		if acc.conversions != nil && *acc.conversions > 0 && costVal > 0 {
			cpa := costVal / float64(*acc.conversions)
			m.CPA = &cpa
		}
	}

	return m
}

// rowKeyString is groupedReport's merge key: day (if present) plus every
// requested dimension's value, in the request's own dims order (fixed
// per call, so the same key always builds the same string) —
// \x1f-separated, a byte that can't appear in any dimension value this
// schema stores (they're all plain request/UA-derived strings, none of
// which legitimately contain a control character).
func rowKeyString(k chstore.ReportKey, dims []chstore.ReportDimension) string {
	var b strings.Builder
	if k.Day != nil {
		b.WriteString(k.Day.Format("2006-01-02"))
	}
	for _, d := range dims {
		b.WriteByte('\x1f')
		b.WriteString(k.DimValues[d])
	}
	return b.String()
}

// sortRows sorts rows in place by a dimension or metric key
// (case-insensitive) — the same "sort by whatever the client asked for,
// dimension or derived metric alike" contract aggregate.ts's own
// client-side sort had. An unrecognized key is a no-op, not an error: a
// stale/mistyped sort param shouldn't 500 an otherwise-valid report.
func sortRows(rows []ReportRow, dims []chstore.ReportDimension, spec SortSpec) {
	key := strings.ToLower(spec.Key)
	if key == "" {
		return
	}
	dir := 1
	if strings.EqualFold(spec.Dir, "desc") {
		dir = -1
	}

	var dimKey chstore.ReportDimension
	isDim := false
	for _, d := range dims {
		if strings.EqualFold(string(d), key) {
			dimKey, isDim = d, true
			break
		}
	}

	less := func(i, j int) bool {
		if isDim {
			return dir*strings.Compare(rows[i].DimValues[dimKey], rows[j].DimValues[dimKey]) < 0
		}
		vi, oki := metricFloat(rows[i].Metrics, key)
		vj, okj := metricFloat(rows[j].Metrics, key)
		if !oki || !okj {
			return false
		}
		if vi == vj {
			return false
		}
		return dir*sign(vi-vj) < 0
	}
	sort.SliceStable(rows, less)
}

func sign(f float64) int {
	if f < 0 {
		return -1
	}
	if f > 0 {
		return 1
	}
	return 0
}

// metricFloat reads one named metric as a float for sorting purposes —
// nil (no cost data, unsupported dimension combo, ...) sorts as
// -infinity, the same "unknown sorts first" convention aggregate.ts's
// own client-side sort used (`?? -Infinity`).
func metricFloat(m MetricValues, key string) (float64, bool) {
	switch key {
	case "clicks":
		return float64(m.Clicks), true
	case "uniqueclicks":
		return float64(m.UniqueClicks), true
	case "conversions":
		if m.Conversions == nil {
			return negInf, true
		}
		return float64(*m.Conversions), true
	case "revenue":
		return ptrOrNegInf(m.Revenue), true
	case "cost":
		return ptrOrNegInf(m.Cost), true
	case "profit":
		return ptrOrNegInf(m.Profit), true
	case "roi":
		return ptrOrNegInf(m.ROI), true
	case "roas":
		return ptrOrNegInf(m.ROAS), true
	case "ctr":
		return m.CTR, true
	case "cvr":
		return m.CVR, true
	case "cpc":
		return ptrOrNegInf(m.CPC), true
	case "cpa":
		return ptrOrNegInf(m.CPA), true
	case "epc":
		return m.EPC, true
	default:
		return 0, false
	}
}

const negInf = -1 << 62

func ptrOrNegInf(p *float64) float64 {
	if p == nil {
		return negInf
	}
	return *p
}

// FunnelStep is one §43 funnel stage's total for the requested range —
// count(DISTINCT click_id), zero-filled for a stage nothing has reached
// yet (a brand-new organization's whole funnel, or one that hasn't hit
// PWA_INSTALL yet) rather than silently omitted.
type FunnelStep struct {
	Step  string
	Value uint64
}

// funnelSteps is the exact §43 order
// apps/web/src/lib/mock/analytics.ts's own FUNNEL_STEPS listed — kept
// here, not re-derived from event.All, since the funnel is a specific
// eight-stage subsequence of the full ~20-type model, not "every type in
// declaration order."
var funnelSteps = []string{
	"SOURCE_CLICK", "LAND_VIEW", "LAND_CLICK", "PWA_VIEW", "PWA_INSTALL",
	"CPA_HOLD", "CPA_ACCEPT", "CPA_REDEP",
}

func (s *Service) Funnel(ctx context.Context, organizationID string, from, to time.Time) ([]FunnelStep, error) {
	if organizationID == "" {
		return nil, apierror.Validation("missing organization", nil)
	}
	if to.Before(from) {
		return nil, apierror.Validation("to must not be before from", map[string]string{"to": "before from"})
	}
	defer observeLatency("funnel")()

	counts, err := s.repo.FunnelCounts(ctx, organizationID, from, to.Add(24*time.Hour))
	if err != nil {
		return nil, fmt.Errorf("analytics: querying funnel counts: %w", err)
	}

	steps := make([]FunnelStep, len(funnelSteps))
	for i, step := range funnelSteps {
		steps[i] = FunnelStep{Step: step, Value: counts[step]} // zero value for an absent key, which is exactly the zero-fill this needs
	}
	return steps, nil
}

// DashboardResult is /overview's own endpoint — KPI totals for the
// requested range, a daily revenue/spend/profit/conversions series, and
// three "Top X" breakdowns. Deliberately NOT four: Top Offers is one of
// the 5 dimensions deferred pending event-pipeline work (see
// chstore.ReportDimension's own doc) — apps/web drops that card rather
// than rendering it against fake data.
type DashboardResult struct {
	Totals       MetricValues
	Daily        []ReportRow
	TopCampaigns []ReportRow
	TopCountries []ReportRow
	TopFlows     []ReportRow
}

const dashboardTopN = 5

func (s *Service) Dashboard(ctx context.Context, organizationID string, from, to time.Time) (DashboardResult, error) {
	if organizationID == "" {
		return DashboardResult{}, apierror.Validation("missing organization", nil)
	}
	if to.Before(from) {
		return DashboardResult{}, apierror.Validation("to must not be before from", map[string]string{"to": "before from"})
	}
	defer observeLatency("dashboard")()

	totalsRows, err := s.groupedReport(ctx, organizationID, nil, false, nil, from, to)
	if err != nil {
		return DashboardResult{}, err
	}
	// groupedReport with no dims/byDay always returns exactly one
	// aggregate row (see chstore's own note: an empty-result COUNT() is
	// still a real row, not an absent one).
	var totals MetricValues
	if len(totalsRows) == 1 {
		totals = totalsRows[0].Metrics
	}

	daily, err := s.groupedReport(ctx, organizationID, nil, true, nil, from, to)
	if err != nil {
		return DashboardResult{}, err
	}
	sort.Slice(daily, func(i, j int) bool { return daily[i].Day.Before(*daily[j].Day) })

	topCampaigns, err := s.Breakdown(ctx, organizationID, BreakdownInput{Dim: chstore.DimCampaign, Metric: "revenue", From: from, To: to, Limit: dashboardTopN})
	if err != nil {
		return DashboardResult{}, err
	}
	topCountries, err := s.Breakdown(ctx, organizationID, BreakdownInput{Dim: chstore.DimCountry, Metric: "revenue", From: from, To: to, Limit: dashboardTopN})
	if err != nil {
		return DashboardResult{}, err
	}
	topFlows, err := s.Breakdown(ctx, organizationID, BreakdownInput{Dim: chstore.DimFlow, Metric: "revenue", From: from, To: to, Limit: dashboardTopN})
	if err != nil {
		return DashboardResult{}, err
	}

	return DashboardResult{
		Totals:       totals,
		Daily:        daily,
		TopCampaigns: topCampaigns,
		TopCountries: topCountries,
		TopFlows:     topFlows,
	}, nil
}
