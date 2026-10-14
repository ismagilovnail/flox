package analytics

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/cost"
	"github.com/ismagilovnail/flox/apps/internal/fx"
)

// maxReportDims bounds how many dimensions one report can group by at
// once — not requested by any spec section, a deliberate, documented
// guard against a client asking for an 11-way cross-product that would
// return an enormous, useless row count, the same spirit as
// maxRangeDays in analytics.go.
const maxReportDims = 4

// DimDay is the report builder's "День" pseudo-dimension: not a
// chstore.ReportDimension (no column to group by — it buckets event_at), but
// accepted anywhere in ReportInput.Dims. It turns on per-day grouping on top
// of the other dimensions and counts toward maxReportDims.
const DimDay chstore.ReportDimension = "day"

// splitDay removes DimDay from dims and reports whether it was there.
func splitDay(dims []chstore.ReportDimension) (rest []chstore.ReportDimension, hasDay bool) {
	rest = make([]chstore.ReportDimension, 0, len(dims))
	for _, d := range dims {
		if d == DimDay {
			hasDay = true
			continue
		}
		rest = append(rest, d)
	}
	return rest, hasDay
}

// scope is the timezone a request's days are cut in. The zero value is UTC.
type scope struct {
	loc  *time.Location
	name string
}

// resolveScope validates an IANA zone name ("" and "UTC" mean UTC).
func resolveScope(name string) (scope, error) {
	if name == "" || name == "UTC" {
		return scope{loc: time.UTC}, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return scope{}, apierror.Validation("unknown timezone", map[string]string{"timezone": name})
	}
	return scope{loc: loc, name: name}, nil
}

// bounds converts the caller's inclusive day range (UTC-midnight dates, see
// handler.go) into the UTC instants [from 00:00, to+1d 00:00) in sc's zone.
func (sc scope) bounds(from, to time.Time) (time.Time, time.Time) {
	f := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, sc.loc).UTC()
	t := time.Date(to.Year(), to.Month(), to.Day()+1, 0, 0, 0, 0, sc.loc).UTC()
	return f, t
}

// MetricValues is the full §50 metrics-registry row every report/
// timeseries/breakdown/dashboard endpoint returns — ports
// apps/web/src/features/analytics/aggregate.ts's deriveMetrics 1:1 so the
// two never drift on formulas (that file's own client-side version is
// deleted once apps/web switches to this endpoint, not kept as a second
// implementation — CLAUDE.md's "no duplicate business logic"). Pointer
// fields are nil exactly when CLAUDE.md #6 says they must be: no cost
// data on file (Cost/Profit/ROI/ROAS/CPC/CPA), or the requested
// dimensions can't be answered by the conversion report at all
// (Conversions/Revenue too, in that case — see
// chstore.SupportedByConversions; os/browser/language are answered through
// a click_id join, so today that is only a defensive guard).
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
	EPC          *float64

	// PWA / CPA funnel counters (§50 CPA FUNNEL). PwaInstalls comes from
	// tracking_events and is always answerable; the CPA counts are nil
	// exactly when Conversions is (conversion_events can't answer the
	// requested dimensions). Purchases = CPA_ACCEPT (the first purchase,
	// FTD), Repurchases = CPA_REDEP, Registrations = CPA_HOLD, Declines =
	// CPA_DECLINE (chargebacks included), Trash = CPA_TRASH.
	PwaInstalls   uint64
	Registrations *uint64
	Purchases     *uint64
	Repurchases   *uint64
	Declines      *uint64
	Trash         *uint64

	// Step rates in percent and cost per step. Nil when the denominator is
	// zero (or, for costs, when there is no cost on file) — never a
	// fabricated 0 and never an error (CLAUDE.md #6, #12).
	ClickToInstall  *float64
	InstallToReg    *float64
	RegToPurchase   *float64
	CostPerInstall  *float64
	CostPerReg      *float64
	CostPerPurchase *float64

	// Traffic quality (audit #3). Blocked is SOURCE_FILTER — clicks no
	// destination took; Bots is every click the classifier flagged is_bot,
	// including bots that were still redirected to a fallback. The rates are
	// shares of ALL traffic that reached the tracker (Clicks + Blocked), in
	// percent; nil when there was none.
	Blocked     uint64
	Bots        uint64
	BlockedRate *float64
	BotRate     *float64
}

// ReportRow is one output row of Report/Timeseries/Breakdown/Dashboard —
// Day is set (DimValues empty) for a timeseries row, DimValues is set
// (Day nil) for a report/breakdown row, and both for a report with the
// "day" dimension.
type ReportRow struct {
	Day       *time.Time
	DimValues map[chstore.ReportDimension]string
	Metrics   MetricValues
	// DatedByConversion counts, when dated by click, the row's conversions
	// with no attributed click, which keep their own date.
	DatedByConversion uint64
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
	// Timezone is the IANA zone days are cut in ("" = UTC); also moves the
	// From/To day boundaries, so a report in another zone covers the same
	// local days.
	Timezone string
	// DateBasis dates conversions by their own time or their click's
	// ("" = by conversion; tech audit AN5).
	DateBasis chstore.DateBasis
	// KeepUSD reports money in USD whatever the organization's reporting
	// currency — for alert rules, whose thresholds are USD.
	KeepUSD bool
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
		if d == DimDay {
			continue
		}
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
// validateBasis refuses a date basis other than "", conversion and click.
func validateBasis(b chstore.DateBasis) error {
	if !b.Valid() {
		return apierror.Validation("unknown date basis", map[string]string{"dateBasis": "want conversion or click"})
	}
	return nil
}

func (s *Service) Report(ctx context.Context, organizationID string, in ReportInput) ([]ReportRow, error) {
	if organizationID == "" {
		return nil, apierror.Validation("missing organization", nil)
	}
	if err := s.validateReportInput(in.Dims, in.Filters, in.From, in.To); err != nil {
		return nil, err
	}
	if err := validateBasis(in.DateBasis); err != nil {
		return nil, err
	}
	defer observeLatency("report")()

	sc, err := resolveScope(in.Timezone)
	if err != nil {
		return nil, err
	}
	dims, byDay := splitDay(in.Dims)

	rows, err := s.groupedReport(ctx, organizationID, dims, byDay, in.Filters, in.From, in.To, sc, in.DateBasis, in.KeepUSD)
	if err != nil {
		return nil, err
	}
	sortRows(rows, dims, in.Sort)
	return rows, nil
}

// TimeseriesInput is one metric's daily series — Report's Dims is always
// empty here (a line chart plots one value per day, not per group); the
// response still carries every metric per day, not just the one the
// caller asked about, so apps/web's MetricPicker can switch which field
// it plots without a second request (matches the instant client-side
// switch the mock version already gave for free).
type TimeseriesInput struct {
	Filters   []chstore.ReportFilter
	From, To  time.Time
	Timezone  string // IANA zone days are cut in ("" = UTC)
	DateBasis chstore.DateBasis
}

func (s *Service) Timeseries(ctx context.Context, organizationID string, in TimeseriesInput) ([]ReportRow, error) {
	if organizationID == "" {
		return nil, apierror.Validation("missing organization", nil)
	}
	if err := s.validateReportInput(nil, in.Filters, in.From, in.To); err != nil {
		return nil, err
	}
	if err := validateBasis(in.DateBasis); err != nil {
		return nil, err
	}
	defer observeLatency("timeseries")()

	sc, err := resolveScope(in.Timezone)
	if err != nil {
		return nil, err
	}
	rows, err := s.groupedReport(ctx, organizationID, nil, true, in.Filters, in.From, in.To, sc, in.DateBasis, false)
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
	Dim       chstore.ReportDimension
	Metric    string
	Filters   []chstore.ReportFilter
	From, To  time.Time
	Limit     int
	Timezone  string // IANA zone days are cut in ("" = UTC)
	DateBasis chstore.DateBasis
}

func (s *Service) Breakdown(ctx context.Context, organizationID string, in BreakdownInput) ([]ReportRow, error) {
	if organizationID == "" {
		return nil, apierror.Validation("missing organization", nil)
	}
	dims := []chstore.ReportDimension{in.Dim}
	if err := s.validateReportInput(dims, in.Filters, in.From, in.To); err != nil {
		return nil, err
	}
	if err := validateBasis(in.DateBasis); err != nil {
		return nil, err
	}
	defer observeLatency("breakdown")()

	sc, err := resolveScope(in.Timezone)
	if err != nil {
		return nil, err
	}
	rows, err := s.groupedReport(ctx, organizationID, dims, false, in.Filters, in.From, in.To, sc, in.DateBasis, false)
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

// maxDimensionValues bounds the report builder's filter-value dropdown —
// a client picking from a real list needs "the most common ones," not
// every value that ever occurred; DistinctValues (chstore) already
// orders by frequency descending, so this cap keeps the common case
// (dozens of countries, not thousands of campaign ids) from turning
// into an unusably long dropdown.
const maxDimensionValues = 50

// DimensionValuesInput is DimensionValues' request shape — one
// dimension, one date range, no filters (a "what values exist at all"
// query is deliberately unfiltered by other dimensions; scoping it to
// the report's other active filters would be a real feature, not this
// endpoint's job — see docs/analytics-report-api.md).
type DimensionValuesInput struct {
	Dim      chstore.ReportDimension
	From, To time.Time
}

// DimensionValues answers "what real values has this dimension actually
// taken" — backs the report builder's filter-value dropdown
// (apps/web/src/features/analytics/report-controls.tsx). Rejects a
// dimension outside chstore.ValidDimension with the same validation error
// Report/Breakdown/Timeseries give.
func (s *Service) DimensionValues(ctx context.Context, organizationID string, in DimensionValuesInput) ([]string, error) {
	if organizationID == "" {
		return nil, apierror.Validation("missing organization", nil)
	}
	if err := s.validateReportInput([]chstore.ReportDimension{in.Dim}, nil, in.From, in.To); err != nil {
		return nil, err
	}
	defer observeLatency("dimension_values")()

	// +24h: In/To is the caller's inclusive last day (a UI date picker's
	// own "to" selection), but chstore.ReportQuery.To is EXCLUSIVE — the
	// same adjustment groupedReport makes centrally for Report/
	// Timeseries/Breakdown/Dashboard, repeated here since this method
	// calls the repository directly rather than through groupedReport.
	// Missing this would silently exclude "today" itself whenever To is
	// today's date, exactly the range a fresh page load defaults to.
	return s.repo.DistinctValues(ctx, organizationID, in.Dim, in.From, in.To.Add(24*time.Hour), maxDimensionValues)
}

// groupedReport is the shared core Report/Timeseries/Breakdown/Dashboard
// all build on: three ClickHouse queries (click_events, tracking_events,
// conversion_events) with the same dims/byDay/filters/range, merged by
// group key, plus a Postgres cost lookup joined in when the requested
// dims resolve to campaign granularity or coarser — see costEligible.
// Nothing here sorts or limits; that's each public method's own job,
// since only they know what "sorted by" and "top-N" mean for their
// specific request.
func (s *Service) groupedReport(ctx context.Context, organizationID string, dims []chstore.ReportDimension, byDay bool, filters []chstore.ReportFilter, from, to time.Time, sc scope, basis chstore.DateBasis, keepUSD bool) ([]ReportRow, error) {
	qFrom, qTo := sc.bounds(from, to)
	q := chstore.ReportQuery{
		OrganizationID: organizationID,
		Dims:           dims,
		ByDay:          byDay,
		Filters:        filters,
		Timezone:       sc.name,
		// To is exclusive (chstore.ReportQuery's own doc) — from/to arrive
		// here as day-precision dates (handler.go's own parsing); bounds
		// turns them into the UTC instants of that zone's local midnights,
		// so the whole last requested day is included.
		From:      qFrom,
		To:        qTo,
		DateBasis: basis,
	}

	// Revenue is converted per conversion day: dated by click, a
	// conversion can land any day up to now, and a zone can move the first
	// day back across UTC midnight — hence the wider range.
	conv := fx.USD
	if !keepUSD {
		last := to.AddDate(0, 0, 1)
		if now := time.Now().AddDate(0, 0, 1); now.After(last) {
			last = now
		}
		var err error
		if conv, err = s.conversion(ctx, organizationID, from.AddDate(0, 0, -1), last); err != nil {
			return nil, fmt.Errorf("analytics: loading the reporting currency: %w", err)
		}
	}
	if !conv.Identity() {
		days, factors := conv.Days()
		q.Money = &chstore.MoneyFactors{Days: days, Factors: factors}
	}

	clickAggs, err := s.repo.ReportClicks(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("analytics: querying click report: %w", err)
	}
	landAggs, err := s.repo.ReportLandViews(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("analytics: querying land-view report: %w", err)
	}

	installAggs, err := s.repo.ReportPwaInstalls(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("analytics: querying pwa-install report: %w", err)
	}

	convSupported := chstore.SupportedByConversions(dims) && chstore.FiltersSupportedByConversions(q.Filters)
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
		acc.blocked, acc.bots = a.Blocked, a.Bots
		acc.uniqueClicks = a.UniqueClicks
	}
	for _, a := range landAggs {
		acc := get(a.Key)
		acc.landViews = a.LandViews
	}
	for _, a := range installAggs {
		acc := get(a.Key)
		acc.pwaInstalls = a.PwaInstalls
	}
	if convSupported {
		// Default every row seen so far (from clicks/land) to a real zero
		// — a group with clicks but genuinely no conversions must render
		// as "0", not "—" (nil means "conversion_events can't answer
		// this dimension combo at all," a property of the whole request,
		// not of any one group; see conversionColumns' own doc). Only
		// !convSupported groups stay nil, for every row, below.
		for _, acc := range merged {
			acc.zeroConversions()
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
			registrations, purchases, repurchases := a.Registrations, a.Purchases, a.Repurchases
			acc.registrations, acc.purchases, acc.repurchases = &registrations, &purchases, &repurchases
			declines, trash := a.Declines, a.Trash
			acc.declines, acc.trash = &declines, &trash
			acc.datedByConversion = a.DatedByConversion
		}
	}

	if s.cost != nil {
		// A campaign (or source) with spend but no events in the range is
		// still a row: leaving it out hid money spent on no traffic, and
		// made the rows not add up to the total. Only without filters — a
		// filtered report has no way to tell which spend matches them.
		var addRow func(map[chstore.ReportDimension]string) *rowAccumulator
		if len(filters) == 0 {
			addRow = func(dimValues map[chstore.ReportDimension]string) *rowAccumulator {
				acc := get(chstore.ReportKey{DimValues: dimValues})
				if convSupported && acc.conversions == nil {
					acc.zeroConversions()
				}
				return acc
			}
		}
		src := s.cost
		if !conv.Identity() {
			src = convertedCost{inner: s.cost, conv: conv}
		}
		if err := attachCost(ctx, src, organizationID, dims, byDay, filters, from, to, merged, addRow); err != nil {
			return nil, err
		}
	}
	for _, acc := range merged {
		sanitizeMoney(acc)
	}

	rows := make([]ReportRow, 0, len(order))
	for _, k := range order {
		acc := merged[k]
		rows = append(rows, ReportRow{
			Day:               acc.day,
			DimValues:         acc.dimValues,
			Metrics:           deriveMetrics(acc),
			DatedByConversion: acc.datedByConversion,
		})
	}
	return rows, nil
}

// costEligible reports whether dims resolves to campaign, traffic-source
// or country granularity (alone) or coarser — the only shapes cost_entries
// (Postgres, campaign+day[+country] grained, rolled up to a source through
// the campaign) can answer. No device/os/... breakdown exists to join
// against.
func costEligible(dims []chstore.ReportDimension) bool {
	if len(dims) == 0 {
		return true
	}
	return len(dims) == 1 && (dims[0] == chstore.DimCampaign || dims[0] == chstore.DimSource || dims[0] == chstore.DimCountry)
}

// attachCost fills acc.costUSD (and leaves it nil where CLAUDE.md #6
// requires) for every row groupedReport has already built. addRow, when
// not nil, creates the row for a campaign or source that has spend but no
// row yet (per-period reports only).
func attachCost(ctx context.Context, src CostSource, organizationID string, dims []chstore.ReportDimension, byDay bool, filters []chstore.ReportFilter, from, to time.Time, merged map[string]*rowAccumulator, addRow func(map[chstore.ReportDimension]string) *rowAccumulator) error {
	if !costEligible(dims) {
		return nil // every acc.costUSD stays nil — no dimension breakdown cost_entries can answer
	}
	if len(dims) == 1 && dims[0] == chstore.DimCountry {
		return attachCountryCost(ctx, src, organizationID, byDay, filters, from, to, merged, addRow)
	}

	if byDay {
		return attachDailyCost(ctx, src, organizationID, dims, from, to, merged)
	}

	if len(dims) == 0 {
		// One grand total across every campaign — sum SpendByCampaign's
		// own per-campaign rows rather than adding a third cost query
		// shape for "just the org total."
		spend, err := src.SpendByCampaign(ctx, organizationID, from, to)
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

	if dims[0] == chstore.DimSource {
		bySource, err := src.SpendBySource(ctx, organizationID, from, to)
		if err != nil {
			return fmt.Errorf("analytics: querying spend by source: %w", err)
		}
		bySourceID := make(map[string]float64, len(bySource))
		for _, c := range bySource {
			bySourceID[c.TrafficSourceID] = c.AmountUSD
		}
		seen := map[string]bool{}
		for _, acc := range merged {
			id := acc.dimValues[chstore.DimSource]
			seen[id] = true
			if usd, ok := bySourceID[id]; ok {
				v := usd
				acc.costUSD = &v
			}
		}
		if addRow != nil {
			for _, c := range bySource {
				if !seen[c.TrafficSourceID] {
					v := c.AmountUSD
					addRow(map[chstore.ReportDimension]string{chstore.DimSource: c.TrafficSourceID}).costUSD = &v
				}
			}
		}
		return nil
	}

	// dims == [campaign] — per-campaign cost.
	spend, err := src.SpendByCampaign(ctx, organizationID, from, to)
	if err != nil {
		return fmt.Errorf("analytics: querying spend by campaign: %w", err)
	}
	byCampaign := make(map[string]float64, len(spend))
	for _, c := range spend {
		byCampaign[c.CampaignID] = c.AmountUSD
	}
	seen := map[string]bool{}
	for _, acc := range merged {
		campaignID := acc.dimValues[dims[0]]
		seen[campaignID] = true
		if usd, ok := byCampaign[campaignID]; ok {
			v := usd
			acc.costUSD = &v
		}
	}
	if addRow != nil {
		for _, c := range spend {
			if !seen[c.CampaignID] {
				v := c.AmountUSD
				addRow(map[chstore.ReportDimension]string{chstore.DimCampaign: c.CampaignID}).costUSD = &v
			}
		}
	}
	return nil
}

// attachCountryCost fills cost per country (per (country, day) when byDay).
// Spend entered without a country (Country "": every synced day, and manual
// day totals) cannot be split, so a period — or a day, when byDay — that
// holds any of it leaves every country's cost nil: showing the split part
// alone would understate cost and overstate ROI (CLAUDE.md #6). Filters
// narrow the spend only by campaign and traffic source; a report filtered
// by anything else, or by two values of one, gets no cost — which spend
// matches it is unknown.
func attachCountryCost(ctx context.Context, src CostSource, organizationID string, byDay bool, filters []chstore.ReportFilter, from, to time.Time, merged map[string]*rowAccumulator, addRow func(map[chstore.ReportDimension]string) *rowAccumulator) error {
	var scope cost.CountryScope
	for _, f := range filters {
		var field *string
		switch f.Dimension {
		case chstore.DimCampaign:
			field = &scope.CampaignID
		case chstore.DimSource:
			field = &scope.TrafficSourceID
		default:
			return nil
		}
		if *field != "" && *field != f.Value {
			return nil
		}
		*field = f.Value
	}
	spend, err := src.SpendByCountry(ctx, organizationID, from, to, scope)
	if err != nil {
		return fmt.Errorf("analytics: querying spend by country: %w", err)
	}

	dayKey := func(t time.Time) string { return t.Format("2006-01-02") }
	// bucket is the day when byDay, "" for the whole period.
	bucket := func(t time.Time) string {
		if byDay {
			return dayKey(t)
		}
		return ""
	}
	unsplit := map[string]bool{}
	by := map[string]float64{} // country|bucket
	for _, c := range spend {
		if c.Country == "" {
			if c.AmountUSD != 0 {
				unsplit[bucket(c.Day)] = true
			}
			continue
		}
		by[c.Country+"|"+bucket(c.Day)] += c.AmountUSD
	}

	seen := map[string]bool{}
	for _, acc := range merged {
		b := ""
		if byDay {
			if acc.day == nil {
				continue
			}
			b = dayKey(*acc.day)
		}
		k := acc.dimValues[chstore.DimCountry] + "|" + b
		seen[k] = true
		if unsplit[b] {
			continue
		}
		if usd, ok := by[k]; ok {
			v := usd
			acc.costUSD = &v
		}
	}
	// Per-period only, like the campaign and source rows: a country with
	// spend but no traffic is still a row.
	if addRow != nil && !byDay && !unsplit[""] {
		for k, usd := range by {
			if seen[k] {
				continue
			}
			v := usd
			addRow(map[chstore.ReportDimension]string{chstore.DimCountry: strings.TrimSuffix(k, "|")}).costUSD = &v
		}
	}
	return nil
}

// attachDailyCost fills per-day cost: the org's daily total when there is no
// other dimension, or per (campaign|source, day) when grouped by one of them.
func attachDailyCost(ctx context.Context, src CostSource, organizationID string, dims []chstore.ReportDimension, from, to time.Time, merged map[string]*rowAccumulator) error {
	set := func(acc *rowAccumulator, usd float64) {
		v := usd
		acc.costUSD = &v
	}
	dayKey := func(t time.Time) string { return t.Format("2006-01-02") }

	switch {
	case len(dims) == 0:
		spend, err := src.DailySpendForOrg(ctx, organizationID, from, to)
		if err != nil {
			return fmt.Errorf("analytics: querying daily org spend: %w", err)
		}
		byDayUSD := make(map[string]float64, len(spend))
		for _, d := range spend {
			byDayUSD[dayKey(d.Day)] = d.AmountUSD
		}
		for _, acc := range merged {
			if acc.day == nil {
				continue
			}
			if usd, ok := byDayUSD[dayKey(*acc.day)]; ok {
				set(acc, usd)
			}
		}
	case dims[0] == chstore.DimSource:
		spend, err := src.SourceSpendByDay(ctx, organizationID, from, to)
		if err != nil {
			return fmt.Errorf("analytics: querying daily spend by source: %w", err)
		}
		by := make(map[string]float64, len(spend))
		for _, d := range spend {
			by[d.TrafficSourceID+"|"+dayKey(d.Day)] = d.AmountUSD
		}
		for _, acc := range merged {
			if acc.day == nil {
				continue
			}
			if usd, ok := by[acc.dimValues[chstore.DimSource]+"|"+dayKey(*acc.day)]; ok {
				set(acc, usd)
			}
		}
	default: // [campaign]
		spend, err := src.CampaignSpendByDay(ctx, organizationID, from, to)
		if err != nil {
			return fmt.Errorf("analytics: querying daily spend by campaign: %w", err)
		}
		by := make(map[string]float64, len(spend))
		for _, d := range spend {
			by[d.CampaignID+"|"+dayKey(d.Day)] = d.AmountUSD
		}
		for _, acc := range merged {
			if acc.day == nil {
				continue
			}
			if usd, ok := by[acc.dimValues[chstore.DimCampaign]+"|"+dayKey(*acc.day)]; ok {
				set(acc, usd)
			}
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
	blocked, bots                   uint64
	conversions                     *uint64
	revenueUSD                      *float64
	pwaInstalls                     uint64
	registrations, purchases        *uint64
	repurchases, declines, trash    *uint64
	costUSD                         *float64
	datedByConversion               uint64
	// revenueUnknown: revenue met a day with no reporting-currency rate
	// (sanitizeMoney); EPC is then unknown too.
	revenueUnknown bool
}

// zeroConversions makes every conversion_events-backed field a real 0: the
// row's dimensions are answerable there, it just had no conversions.
func (acc *rowAccumulator) zeroConversions() {
	var conversions, registrations, purchases, repurchases, declines, trash uint64
	var revenue float64
	acc.conversions, acc.revenueUSD = &conversions, &revenue
	acc.registrations, acc.purchases, acc.repurchases = &registrations, &purchases, &repurchases
	acc.declines, acc.trash = &declines, &trash
}

// deriveMetrics ports apps/web/src/features/analytics/aggregate.ts's
// deriveMetrics formula for formula, field for field — see that
// function's own doc comment for why nothing here may drift from it.
func deriveMetrics(acc *rowAccumulator) MetricValues {
	m := MetricValues{
		Clicks:        acc.clicks,
		UniqueClicks:  acc.uniqueClicks,
		Conversions:   acc.conversions,
		PwaInstalls:   acc.pwaInstalls,
		Registrations: acc.registrations,
		Purchases:     acc.purchases,
		Repurchases:   acc.repurchases,
		Declines:      acc.declines,
		Trash:         acc.trash,
		Blocked:       acc.blocked,
		Bots:          acc.bots,
	}

	m.BlockedRate = percentOf(acc.blocked, acc.clicks+acc.blocked)
	m.BotRate = percentOf(acc.bots, acc.clicks+acc.blocked)

	m.ClickToInstall = percentOf(acc.pwaInstalls, acc.clicks)
	if acc.registrations != nil {
		m.InstallToReg = percentOf(*acc.registrations, acc.pwaInstalls)
		if acc.purchases != nil {
			m.RegToPurchase = percentOf(*acc.purchases, *acc.registrations)
		}
	}

	if acc.clicks > 0 {
		m.CTR = float64(acc.landViews) / float64(acc.clicks) * 100
	}
	if acc.conversions != nil && acc.clicks > 0 {
		m.CVR = float64(*acc.conversions) / float64(acc.clicks) * 100
	}
	if acc.conversions != nil && acc.clicks > 0 && !acc.revenueUnknown {
		rev := 0.0
		if acc.revenueUSD != nil {
			rev = *acc.revenueUSD
		}
		epc := rev / float64(acc.clicks)
		m.EPC = &epc
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
		if costVal > 0 {
			m.CostPerInstall = perUnit(costVal, acc.pwaInstalls)
			if acc.registrations != nil {
				m.CostPerReg = perUnit(costVal, *acc.registrations)
			}
			if acc.purchases != nil {
				m.CostPerPurchase = perUnit(costVal, *acc.purchases)
			}
		}
	}

	return m
}

// percentOf is num/den in percent, nil when den is zero.
func percentOf(num, den uint64) *float64 {
	if den == 0 {
		return nil
	}
	v := float64(num) / float64(den) * 100
	return &v
}

// perUnit is cost/count, nil when count is zero.
func perUnit(cost float64, count uint64) *float64 {
	if count == 0 {
		return nil
	}
	v := cost / float64(count)
	return &v
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
		if key == "day" {
			di, dj := rows[i].Day, rows[j].Day
			if di == nil || dj == nil {
				return false
			}
			return dir*sign(float64(di.Unix()-dj.Unix())) < 0
		}
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
		return ptrOrNegInf(m.EPC), true
	case "pwainstalls":
		return float64(m.PwaInstalls), true
	case "registrations":
		return u64OrNegInf(m.Registrations), true
	case "purchases":
		return u64OrNegInf(m.Purchases), true
	case "repurchases":
		return u64OrNegInf(m.Repurchases), true
	case "declines":
		return u64OrNegInf(m.Declines), true
	case "trash":
		return u64OrNegInf(m.Trash), true
	case "clicktoinstall":
		return ptrOrNegInf(m.ClickToInstall), true
	case "installtoreg":
		return ptrOrNegInf(m.InstallToReg), true
	case "regtopurchase":
		return ptrOrNegInf(m.RegToPurchase), true
	case "costperinstall":
		return ptrOrNegInf(m.CostPerInstall), true
	case "costperreg":
		return ptrOrNegInf(m.CostPerReg), true
	case "costperpurchase":
		return ptrOrNegInf(m.CostPerPurchase), true
	case "blocked":
		return float64(m.Blocked), true
	case "bots":
		return float64(m.Bots), true
	case "blockedrate":
		return ptrOrNegInf(m.BlockedRate), true
	case "botrate":
		return ptrOrNegInf(m.BotRate), true
	default:
		return 0, false
	}
}

const negInf = -1 << 62

func u64OrNegInf(p *uint64) float64 {
	if p == nil {
		return negInf
	}
	return float64(*p)
}

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

// funnelSteps is §43's canonical funnel order — kept here, not
// re-derived from event.All, since the funnel is a specific
// eight-stage subsequence of the full ~20-type model, not "every type in
// declaration order."
var funnelSteps = []string{
	"SOURCE_CLICK", "LAND_VIEW", "LAND_CLICK", "PWA_VIEW", "PWA_INSTALL",
	"CPA_HOLD", "CPA_ACCEPT", "CPA_REDEP",
}

func (s *Service) Funnel(ctx context.Context, organizationID string, from, to time.Time, filters []chstore.ReportFilter, timezone string) ([]FunnelStep, error) {
	if organizationID == "" {
		return nil, apierror.Validation("missing organization", nil)
	}
	if err := s.validateReportInput(nil, filters, from, to); err != nil {
		return nil, err
	}
	sc, err := resolveScope(timezone)
	if err != nil {
		return nil, err
	}
	defer observeLatency("funnel")()

	qFrom, qTo := sc.bounds(from, to)
	counts, err := s.repo.FunnelCounts(ctx, organizationID, qFrom, qTo, filters)
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
// four "Top X" breakdowns by revenue: campaigns, countries, flows and offers
// (the offer a click was actually routed to — chstore.DimOffer).
type DashboardResult struct {
	Totals MetricValues
	// DatedByConversion: as ReportRow's, for the totals.
	DatedByConversion uint64
	Daily             []ReportRow
	TopCampaigns      []ReportRow
	TopCountries      []ReportRow
	TopFlows          []ReportRow
	TopOffers         []ReportRow
}

const dashboardTopN = 5

// timezone is the IANA zone days are cut in ("" = UTC).
func (s *Service) Dashboard(ctx context.Context, organizationID string, from, to time.Time, timezone string, basis chstore.DateBasis) (DashboardResult, error) {
	if organizationID == "" {
		return DashboardResult{}, apierror.Validation("missing organization", nil)
	}
	if to.Before(from) {
		return DashboardResult{}, apierror.Validation("to must not be before from", map[string]string{"to": "before from"})
	}
	if err := validateBasis(basis); err != nil {
		return DashboardResult{}, err
	}
	defer observeLatency("dashboard")()

	sc, err := resolveScope(timezone)
	if err != nil {
		return DashboardResult{}, err
	}
	totalsRows, err := s.groupedReport(ctx, organizationID, nil, false, nil, from, to, sc, basis, false)
	if err != nil {
		return DashboardResult{}, err
	}
	// groupedReport with no dims/byDay always returns exactly one
	// aggregate row (see chstore's own note: an empty-result COUNT() is
	// still a real row, not an absent one).
	var totals MetricValues
	var datedByConversion uint64
	if len(totalsRows) == 1 {
		totals, datedByConversion = totalsRows[0].Metrics, totalsRows[0].DatedByConversion
	}

	daily, err := s.groupedReport(ctx, organizationID, nil, true, nil, from, to, sc, basis, false)
	if err != nil {
		return DashboardResult{}, err
	}
	sort.Slice(daily, func(i, j int) bool { return daily[i].Day.Before(*daily[j].Day) })

	topCampaigns, err := s.Breakdown(ctx, organizationID, BreakdownInput{Dim: chstore.DimCampaign, Metric: "revenue", From: from, To: to, Limit: dashboardTopN, Timezone: timezone, DateBasis: basis})
	if err != nil {
		return DashboardResult{}, err
	}
	topCountries, err := s.Breakdown(ctx, organizationID, BreakdownInput{Dim: chstore.DimCountry, Metric: "revenue", From: from, To: to, Limit: dashboardTopN, Timezone: timezone, DateBasis: basis})
	if err != nil {
		return DashboardResult{}, err
	}
	topFlows, err := s.Breakdown(ctx, organizationID, BreakdownInput{Dim: chstore.DimFlow, Metric: "revenue", From: from, To: to, Limit: dashboardTopN, Timezone: timezone, DateBasis: basis})
	if err != nil {
		return DashboardResult{}, err
	}

	topOffers, err := s.Breakdown(ctx, organizationID, BreakdownInput{Dim: chstore.DimOffer, Metric: "revenue", From: from, To: to, Limit: dashboardTopN, Timezone: timezone, DateBasis: basis})
	if err != nil {
		return DashboardResult{}, err
	}

	return DashboardResult{
		Totals:            totals,
		DatedByConversion: datedByConversion,
		Daily:             daily,
		TopCampaigns:      topCampaigns,
		TopCountries:      topCountries,
		TopFlows:          topFlows,
		TopOffers:         topOffers,
	}, nil
}

// maxSummaryFlows bounds one flow-summary request — a stream set's flows,
// or a few sets' worth.
const maxSummaryFlows = 100

// FlowStages is one flow's §25 node summary: per event type, see
// chstore.FlowStageCounts for what is counted. Every stage key is present.
type FlowStages struct {
	FlowID string
	Counts map[string]uint64
}

// FlowSummary counts each flow's stages over [from, to] in timezone's days
// (§25 "analytics summary" on the flow builder's nodes). Flows are looked
// up only within organizationID: another organization's flow id simply has
// nothing.
func (s *Service) FlowSummary(ctx context.Context, organizationID string, flowIDs []string, from, to time.Time, timezone string) ([]FlowStages, error) {
	if organizationID == "" {
		return nil, apierror.Validation("missing organization", nil)
	}
	if len(flowIDs) == 0 || len(flowIDs) > maxSummaryFlows {
		return nil, apierror.Validation(fmt.Sprintf("flowIds: send 1 to %d flow ids", maxSummaryFlows), nil)
	}
	if err := s.validateReportInput(nil, nil, from, to); err != nil {
		return nil, err
	}
	sc, err := resolveScope(timezone)
	if err != nil {
		return nil, err
	}
	defer observeLatency("flow_summary")()

	qFrom, qTo := sc.bounds(from, to)
	counts, err := s.repo.FlowStageCounts(ctx, organizationID, qFrom, qTo, flowIDs)
	if err != nil {
		return nil, fmt.Errorf("analytics: querying flow stage counts: %w", err)
	}
	out := make([]FlowStages, len(flowIDs))
	for i, id := range flowIDs {
		c := map[string]uint64{}
		for _, typ := range flowSummaryTypes {
			c[typ] = counts[id][typ]
		}
		out[i] = FlowStages{FlowID: id, Counts: c}
	}
	return out, nil
}

var flowSummaryTypes = []string{
	"SOURCE_CLICK", "LAND_VIEW", "LAND_CLICK", "PWA_VIEW", "PWA_INSTALL",
	"POSTLANDING_VIEW", "POSTLANDING_CLICK", "CPA_ACCEPT", "CPA_REDEP",
}
