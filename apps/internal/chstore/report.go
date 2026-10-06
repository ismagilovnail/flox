package chstore

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// ReportDimension is a whitelisted group-by dimension the multi-dimension
// report builder (Phase 35) can request — never an arbitrary client
// string reaching a query verbatim; every method in this file rejects
// anything not in reportClickColumns/reportConversionColumns below before it gets
// near SQL.
//
// 15 dimensions total, matching apps/web/src/features/analytics/registry.ts's
// DIMENSIONS. Of the 16 the UI originally named at Phase 35, 2 — postlanding
// and network — still have no backing column: postlanding because
// apps/tracker doesn't serve that funnel stage yet (recording a
// postlanding "shown" that never was would be fabricated attribution),
// network because nothing carries it. DimStreamSet was a whitelist
// omission (its column always existed); DimLanding (schema/008), DimOffer
// (schema/009) and DimPwa (schema/010) got real columns, the pattern the
// rest would follow.
type ReportDimension string

const (
	DimCampaign ReportDimension = "campaign"
	// DimSource groups by the traffic source of the click's campaign
	// (traffic_source_id, schema/011). Empty is a real bucket: rows from
	// before the column existed that were never backfilled.
	DimSource   ReportDimension = "source"
	DimCountry  ReportDimension = "country"
	DimRegion   ReportDimension = "region"
	DimCity     ReportDimension = "city"
	DimDevice   ReportDimension = "device"
	DimPlatform ReportDimension = "platform"
	DimOS       ReportDimension = "os"
	DimBrowser  ReportDimension = "browser"
	DimLanguage ReportDimension = "language"
	DimFlow     ReportDimension = "flow"
	// DimStreamSet groups by the stream set a click's routing decision
	// matched — empty string is its own real bucket (not missing data)
	// for a campaign-fallback or fully-blocked click, the same way
	// DimFlow's flow_id is empty whenever no flow was reached either;
	// see routing.Engine's own doc comments on when each is set.
	DimStreamSet ReportDimension = "stream_set"
	// DimLanding groups by the landing the visitor was shown. Empty is a
	// real bucket — traffic whose flow had no landing stage (or a skipped
	// one: paused landing), and every row from before landings were
	// served at all. Clicks, landing views/clicks and (via attribution)
	// conversions all carry it, so every metric works by landing.
	DimLanding ReportDimension = "landing"
	// DimOffer groups by the offer a click was actually routed to
	// (routing.RouteResult.OfferID). Empty is a real bucket: redirect
	// flows, fallbacks (including an inactive offer that fell through),
	// blocked clicks, and rows from before offers were attributed.
	DimOffer ReportDimension = "offer"
	// DimPwa groups by the PWA stage a click's routed path went through
	// (apps/tracker/pwa.go). Empty is a real bucket: flows without a PWA
	// stage, skipped stages (paused PWA, non-internal type, behind an
	// external landing), and rows from before PWAs were served.
	DimPwa ReportDimension = "pwa"
)

// reportClickColumns is the whitelist for click_events and tracking_events,
// which share an identical dimension column set (both carry every field
// on the same underlying event.Event struct — see schema/002's own
// comment on why they're still two tables).
var reportClickColumns = map[ReportDimension]string{
	DimCampaign:  "campaign_id",
	DimSource:    "traffic_source_id",
	DimCountry:   "country",
	DimRegion:    "region",
	DimCity:      "city",
	DimDevice:    "device",
	DimPlatform:  "platform",
	DimOS:        "os",
	DimBrowser:   "browser",
	DimLanguage:  "language",
	DimFlow:      "flow_id",
	DimStreamSet: "stream_set_id",
	DimLanding:   "landing_id",
	DimOffer:     "offer_id",
	DimPwa:       "pwa_id",
}

// reportConversionColumns is the narrower whitelist for conversion_events
// (schema/003_conversion_events.sql) — it does not carry os/browser/
// language. Those three (reportConversionViaClickColumns) are answered by
// joining click_events onto each conversion via click_id, see
// reportConversionsJoined; every other dimension reads straight off
// conversion_events with no join.
var reportConversionColumns = map[ReportDimension]string{
	DimCampaign:  "campaign_id",
	DimSource:    "traffic_source_id",
	DimCountry:   "country",
	DimRegion:    "region",
	DimCity:      "city",
	DimDevice:    "device",
	DimPlatform:  "platform",
	DimFlow:      "flow_id",
	DimStreamSet: "stream_set_id",
	DimLanding:   "landing_id",
	DimOffer:     "offer_id",
	DimPwa:       "pwa_id",
}

// reportConversionViaClickColumns are the dimensions conversion_events has
// no column for: a conversion inherits them from the SOURCE_CLICK that
// produced it (same click_id). A conversion with no matching click (an
// unattributed postback, or the click was never flushed) lands in the "" bucket.
var reportConversionViaClickColumns = map[ReportDimension]string{
	DimOS:       "os",
	DimBrowser:  "browser",
	DimLanguage: "language",
}

// ValidDimension reports whether d is one of the 15 supported report
// dimensions — the same check both click- and conversion-side queries can
// always answer, since reportClickColumns is the wider of the two whitelists.
func ValidDimension(d ReportDimension) bool {
	_, ok := reportClickColumns[d]
	return ok
}

// SupportedByConversions reports whether the conversion report can answer
// every dimension in dims: directly off conversion_events, or through the
// click_id join for os/browser/language.
func SupportedByConversions(dims []ReportDimension) bool {
	for _, d := range dims {
		if !conversionDimAnswerable(d) {
			return false
		}
	}
	return true
}

// FiltersSupportedByConversions is SupportedByConversions for filter dimensions.
func FiltersSupportedByConversions(filters []ReportFilter) bool {
	for _, f := range filters {
		if !conversionDimAnswerable(f.Dimension) {
			return false
		}
	}
	return true
}

func conversionDimAnswerable(d ReportDimension) bool {
	if _, ok := reportConversionColumns[d]; ok {
		return true
	}
	_, ok := reportConversionViaClickColumns[d]
	return ok
}

// needsClickJoin reports whether q touches a dimension only click_events has.
func needsClickJoin(q ReportQuery) bool {
	for _, d := range q.Dims {
		if _, ok := reportConversionViaClickColumns[d]; ok {
			return true
		}
	}
	for _, f := range q.Filters {
		if _, ok := reportConversionViaClickColumns[f.Dimension]; ok {
			return true
		}
	}
	return false
}

// ReportFilter is one equals-condition ("dimension = value"), AND-combined
// with every other filter in a request — the same shape
// apps/web/src/features/analytics/aggregate.ts's client-side FilterCondition
// already used against mock data.
type ReportFilter struct {
	Dimension ReportDimension
	Value     string
}

// ReportQuery is the shared shape every method below takes. ByDay and
// Dims are independent — Timeseries-style callers set ByDay and leave
// Dims empty, report-table-style callers set Dims and leave ByDay false;
// nothing stops a caller from setting both, but nothing in Phase 35's own
// API surface does, since no UI screen needs a per-day-per-dimension
// breakdown at once.
type ReportQuery struct {
	OrganizationID string
	Dims           []ReportDimension
	ByDay          bool
	Filters        []ReportFilter
	// Timezone is the IANA zone ByDay buckets days in ("" = UTC). The caller
	// (analytics.Service) resolves it with time.LoadLocation and converts
	// From/To to the matching UTC instants; here it only picks the day
	// boundary, and is re-checked (safeTimezone) before it is inlined.
	Timezone string
	// From/To bound event_at: From inclusive, To EXCLUSIVE — callers pass
	// the day after the last day they want included (Service.Report and
	// friends do this once, centrally, rather than every query method
	// repeating a +24h adjustment).
	From, To time.Time
}

// ReportKey is the group-by identity a row of any of the three query
// methods below resolves to — Day/DimValues are how Service merges rows
// from separate click/land/conversion queries that share a key back into
// one output row.
type ReportKey struct {
	Day       *time.Time
	DimValues map[ReportDimension]string
}

type ClickAgg struct {
	Key          ReportKey
	Clicks       uint64
	UniqueClicks uint64
}

type LandAgg struct {
	Key       ReportKey
	LandViews uint64
}

type ConversionAgg struct {
	Key         ReportKey
	Conversions uint64
	RevenueUSD  float64
	// Registrations / Purchases / Repurchases are the per-status counts of
	// CPA_HOLD / CPA_ACCEPT / CPA_REDEP (§50 cpa_hold, cpa_accept,
	// cpa_redep). Conversions stays accept+redep and Revenue is summed over
	// those two only: a HOLD is not yet confirmed money.
	Registrations uint64
	Purchases     uint64
	Repurchases   uint64
}

// InstallAgg is the PWA_INSTALL count of one group.
type InstallAgg struct {
	Key         ReportKey
	PwaInstalls uint64
}

// buildGroupBy resolves q.Dims against cols, returning the SELECT
// fragment's column list and matching GROUP BY list — in the same order,
// so callers can zip scanned values back onto q.Dims positionally. ok is
// false if q.Dims names something cols doesn't have (SupportedByConversions
// should already have been checked by the caller before reaching here for
// conversion_events; this is the belt to that suspenders).
func buildGroupBy(cols map[ReportDimension]string, dims []ReportDimension) (selectCols []string, ok bool) {
	selectCols = make([]string, 0, len(dims))
	for _, d := range dims {
		col, present := cols[d]
		if !present {
			return nil, false
		}
		selectCols = append(selectCols, col)
	}
	return selectCols, true
}

// buildFilterClause resolves q.Filters against cols the same way, adding
// "AND col = ?" fragments and their args — returns ok=false on the same
// unsupported-dimension condition as buildGroupBy.
func buildFilterClause(cols map[ReportDimension]string, filters []ReportFilter, args []any) (clause string, newArgs []any, ok bool) {
	var b strings.Builder
	for _, f := range filters {
		col, present := cols[f.Dimension]
		if !present {
			return "", nil, false
		}
		fmt.Fprintf(&b, " AND %s = ?", col)
		args = append(args, f.Value)
	}
	return b.String(), args, true
}

// scanKey assembles a ReportKey from one row's already-scanned dimension
// values (in q.Dims order) and day pointer (nil unless q.ByDay).
func scanKey(dims []ReportDimension, values []string, day *time.Time) ReportKey {
	key := ReportKey{Day: day}
	if len(dims) > 0 {
		key.DimValues = make(map[ReportDimension]string, len(dims))
		for i, d := range dims {
			key.DimValues[d] = values[i]
		}
	}
	return key
}

// ReportClicks answers "how many SOURCE_CLICK rows, and how many distinct
// click_id values among them" for q's date range/dims/filters —
// count(DISTINCT click_id) is the whole reason "unique clicks" differs
// from plain "clicks" at all: stickyFlowKeepClickId (§39-STICKY) lets one
// click_id legitimately appear on more than one click_events row across a
// returning visitor's journey (docs/attribution.md's own documented
// reason ByClickID resolves to the earliest occurrence, not the only
// one). SOURCE_FILTER is excluded — matches the one place this codebase
// already computes a real "Clicks" number today,
// campaign-detail-view.tsx's own `.filter((c) => c.type === "SOURCE_CLICK")`.
func (s *EventStore) ReportClicks(ctx context.Context, q ReportQuery) ([]ClickAgg, error) {
	selectCols, ok := buildGroupBy(reportClickColumns, q.Dims)
	if !ok {
		return nil, fmt.Errorf("chstore: unsupported dimension in click report")
	}
	args := []any{q.OrganizationID, q.From, q.To}
	filterClause, args, ok := buildFilterClause(reportClickColumns, q.Filters, args)
	if !ok {
		return nil, fmt.Errorf("chstore: unsupported filter dimension in click report")
	}

	selectExpr, groupByExpr := selectAndGroupByExprs(selectCols, q.ByDay, q.Timezone)

	query := fmt.Sprintf(`
		SELECT %scount() AS clicks, count(DISTINCT click_id) AS unique_clicks
		FROM click_events
		WHERE organization_id = ? AND event_at >= ? AND event_at < ? AND type = 'SOURCE_CLICK'%s
		%s`,
		selectExpr, filterClause, groupByExpr,
	)

	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying click report: %w", err)
	}
	defer rows.Close()

	var out []ClickAgg
	for rows.Next() {
		dimVals, day, dest := scanDest(q.Dims, q.ByDay)
		var clicks, unique uint64
		dest = append(dest, &clicks, &unique)
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("chstore: scanning click report row: %w", err)
		}
		out = append(out, ClickAgg{Key: scanKey(q.Dims, deref(dimVals), day), Clicks: clicks, UniqueClicks: unique})
	}
	return out, rows.Err()
}

// ReportLandViews answers "how many LAND_VIEW rows" for q — the funnel's
// first landing-page stage, and the numerator CTR (landClicks/clicks)
// already used against mock data.
func (s *EventStore) ReportLandViews(ctx context.Context, q ReportQuery) ([]LandAgg, error) {
	selectCols, ok := buildGroupBy(reportClickColumns, q.Dims)
	if !ok {
		return nil, fmt.Errorf("chstore: unsupported dimension in land-view report")
	}
	args := []any{q.OrganizationID, q.From, q.To}
	filterClause, args, ok := buildFilterClause(reportClickColumns, q.Filters, args)
	if !ok {
		return nil, fmt.Errorf("chstore: unsupported filter dimension in land-view report")
	}

	selectExpr, groupByExpr := selectAndGroupByExprs(selectCols, q.ByDay, q.Timezone)

	query := fmt.Sprintf(`
		SELECT %scount() AS land_views
		FROM tracking_events
		WHERE organization_id = ? AND event_at >= ? AND event_at < ? AND type = 'LAND_VIEW'%s
		%s`,
		selectExpr, filterClause, groupByExpr,
	)

	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying land-view report: %w", err)
	}
	defer rows.Close()

	var out []LandAgg
	for rows.Next() {
		dimVals, day, dest := scanDest(q.Dims, q.ByDay)
		var landViews uint64
		dest = append(dest, &landViews)
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("chstore: scanning land-view report row: %w", err)
		}
		out = append(out, LandAgg{Key: scanKey(q.Dims, deref(dimVals), day), LandViews: landViews})
	}
	return out, rows.Err()
}

// ReportPwaInstalls answers "how many PWA_INSTALL rows" for q — the
// pwaInstalls metric. Same table and dimensions as ReportLandViews.
func (s *EventStore) ReportPwaInstalls(ctx context.Context, q ReportQuery) ([]InstallAgg, error) {
	selectCols, ok := buildGroupBy(reportClickColumns, q.Dims)
	if !ok {
		return nil, fmt.Errorf("chstore: unsupported dimension in pwa-install report")
	}
	args := []any{q.OrganizationID, q.From, q.To}
	filterClause, args, ok := buildFilterClause(reportClickColumns, q.Filters, args)
	if !ok {
		return nil, fmt.Errorf("chstore: unsupported filter dimension in pwa-install report")
	}

	selectExpr, groupByExpr := selectAndGroupByExprs(selectCols, q.ByDay, q.Timezone)

	query := fmt.Sprintf(`
		SELECT %scount() AS pwa_installs
		FROM tracking_events
		WHERE organization_id = ? AND event_at >= ? AND event_at < ? AND type = 'PWA_INSTALL'%s
		%s`,
		selectExpr, filterClause, groupByExpr,
	)

	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying pwa-install report: %w", err)
	}
	defer rows.Close()

	var out []InstallAgg
	for rows.Next() {
		dimVals, day, dest := scanDest(q.Dims, q.ByDay)
		var installs uint64
		dest = append(dest, &installs)
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("chstore: scanning pwa-install report row: %w", err)
		}
		out = append(out, InstallAgg{Key: scanKey(q.Dims, deref(dimVals), day), PwaInstalls: installs})
	}
	return out, rows.Err()
}

// ReportConversions answers "how many CPA_ACCEPT+CPA_REDEP rows, and how
// much USD revenue" for q — total_deposits per docs/FLOX-master-prompt-v3.md
// §50 ("total_deposits = cpa_accept + cpa_redep"), the same definition
// campaign-detail-view.tsx's own "Conversions" stat already uses
// (CPA_HOLD excluded: not yet confirmed; CPA_DECLINE/CPA_TRASH excluded:
// rejected). Returns an error if q.Dims isn't fully supported by
// conversion_events' narrower column set — see SupportedByConversions;
// Service.Report checks that itself before calling this, so this is a
// second, defensive check, not the primary guard.
func (s *EventStore) ReportConversions(ctx context.Context, q ReportQuery) ([]ConversionAgg, error) {
	if !SupportedByConversions(q.Dims) {
		return nil, fmt.Errorf("chstore: dimension not supported by conversion_events")
	}
	if needsClickJoin(q) {
		return s.reportConversionsJoined(ctx, q)
	}
	selectCols, ok := buildGroupBy(reportConversionColumns, q.Dims)
	if !ok {
		return nil, fmt.Errorf("chstore: unsupported dimension in conversion report")
	}
	args := []any{q.OrganizationID, q.From, q.To}
	filterClause, args, ok := buildFilterClause(reportConversionColumns, q.Filters, args)
	if !ok {
		return nil, fmt.Errorf("chstore: unsupported filter dimension in conversion report")
	}

	selectExpr, groupByExpr := selectAndGroupByExprs(selectCols, q.ByDay, q.Timezone)

	query := fmt.Sprintf(`
		SELECT %scountIf(type IN ('CPA_ACCEPT', 'CPA_REDEP')) AS conversions,
			sumIf(usd_value * has_usd_value, type IN ('CPA_ACCEPT', 'CPA_REDEP')) AS revenue_usd,
			countIf(type = 'CPA_HOLD') AS registrations,
			countIf(type = 'CPA_ACCEPT') AS purchases,
			countIf(type = 'CPA_REDEP') AS repurchases
		FROM conversion_events
		WHERE organization_id = ? AND event_at >= ? AND event_at < ? AND type IN ('CPA_HOLD', 'CPA_ACCEPT', 'CPA_REDEP')%s
		%s`,
		selectExpr, filterClause, groupByExpr,
	)

	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying conversion report: %w", err)
	}
	defer rows.Close()

	var out []ConversionAgg
	for rows.Next() {
		dimVals, day, dest := scanDest(q.Dims, q.ByDay)
		var conversions, registrations, purchases, repurchases uint64
		var revenueUSD float64
		dest = append(dest, &conversions, &revenueUSD, &registrations, &purchases, &repurchases)
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("chstore: scanning conversion report row: %w", err)
		}
		out = append(out, ConversionAgg{
			Key: scanKey(q.Dims, deref(dimVals), day), Conversions: conversions, RevenueUSD: revenueUSD,
			Registrations: registrations, Purchases: purchases, Repurchases: repurchases,
		})
	}
	return out, rows.Err()
}

// reportConversionsJoined is ReportConversions for a query that groups or
// filters by os/browser/language. Each conversion is joined to the
// SOURCE_CLICK carrying its click_id (any() collapses the repeated rows a
// sticky returning visitor leaves under one click_id). The click side is
// deliberately NOT bounded by the report's date range — a conversion can
// land days after its click — but is limited to the click_ids of the
// conversions in range, so it never scans the org's whole click history.
// The LEFT JOIN keeps conversions with no matching click (empty values).
func (s *EventStore) reportConversionsJoined(ctx context.Context, q ReportQuery) ([]ConversionAgg, error) {
	qualify := func(d ReportDimension) (string, bool) {
		if col, ok := reportConversionColumns[d]; ok {
			return "c." + col, true
		}
		if col, ok := reportConversionViaClickColumns[d]; ok {
			return "k." + col, true
		}
		return "", false
	}

	selectCols := make([]string, 0, len(q.Dims)+1)
	for _, d := range q.Dims {
		col, ok := qualify(d)
		if !ok {
			return nil, fmt.Errorf("chstore: unsupported dimension in conversion report")
		}
		selectCols = append(selectCols, col)
	}

	const convTypes = "('CPA_HOLD', 'CPA_ACCEPT', 'CPA_REDEP')"
	// Args in placeholder order: click subquery (org, inner org/from/to), then outer WHERE.
	args := []any{q.OrganizationID, q.OrganizationID, q.From, q.To, q.OrganizationID, q.From, q.To}
	var filterClause strings.Builder
	for _, f := range q.Filters {
		col, ok := qualify(f.Dimension)
		if !ok {
			return nil, fmt.Errorf("chstore: unsupported filter dimension in conversion report")
		}
		fmt.Fprintf(&filterClause, " AND %s = ?", col)
		args = append(args, f.Value)
	}

	all := make([]string, 0, len(selectCols)+1)
	if q.ByDay {
		all = append(all, dayExpr("c.event_at", q.Timezone))
	}
	all = append(all, selectCols...)
	selectExpr, groupByExpr := "", ""
	if len(all) > 0 {
		selectExpr = strings.Join(all, ", ") + ", "
		groupByExpr = "GROUP BY " + strings.Join(all, ", ")
	}

	query := fmt.Sprintf(`
		SELECT %scountIf(c.type IN ('CPA_ACCEPT', 'CPA_REDEP')) AS conversions,
			sumIf(c.usd_value * c.has_usd_value, c.type IN ('CPA_ACCEPT', 'CPA_REDEP')) AS revenue_usd,
			countIf(c.type = 'CPA_HOLD') AS registrations,
			countIf(c.type = 'CPA_ACCEPT') AS purchases,
			countIf(c.type = 'CPA_REDEP') AS repurchases
		FROM conversion_events AS c
		LEFT JOIN (
			SELECT click_id, any(os) AS os, any(browser) AS browser, any(language) AS language
			FROM click_events
			WHERE organization_id = ? AND type = 'SOURCE_CLICK' AND click_id IN (
				SELECT click_id FROM conversion_events
				WHERE organization_id = ? AND event_at >= ? AND event_at < ? AND type IN %s
			)
			GROUP BY click_id
		) AS k ON c.click_id = k.click_id
		WHERE c.organization_id = ? AND c.event_at >= ? AND c.event_at < ? AND c.type IN %s%s
		%s`,
		selectExpr, convTypes, convTypes, filterClause.String(), groupByExpr,
	)

	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying joined conversion report: %w", err)
	}
	defer rows.Close()

	var out []ConversionAgg
	for rows.Next() {
		dimVals, day, dest := scanDest(q.Dims, q.ByDay)
		var conversions, registrations, purchases, repurchases uint64
		var revenueUSD float64
		dest = append(dest, &conversions, &revenueUSD, &registrations, &purchases, &repurchases)
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("chstore: scanning joined conversion report row: %w", err)
		}
		out = append(out, ConversionAgg{
			Key: scanKey(q.Dims, deref(dimVals), day), Conversions: conversions, RevenueUSD: revenueUSD,
			Registrations: registrations, Purchases: purchases, Repurchases: repurchases,
		})
	}
	return out, rows.Err()
}

// FunnelCounts answers the §43 funnel — count(DISTINCT click_id) per
// stage, from whichever of the three event tables actually carries that
// stage's type. Fully computable with today's schema: unlike the 5
// deferred dimensions, no funnel stage needs a column that doesn't exist
// yet. Returns a map so a stage with zero events (a brand-new
// organization, or one that hasn't reached PWA_INSTALL yet) is present
// with 0 rather than silently absent — Service.Funnel zero-fills any
// FUNNEL_STEPS entry this map doesn't have, for the same reason.
func (s *EventStore) FunnelCounts(ctx context.Context, organizationID string, from, to time.Time, filters []ReportFilter) (map[string]uint64, error) {
	out := make(map[string]uint64, 8)

	// Filters apply to the visit as a whole: a funnel step counts a click_id
	// only if its SOURCE_CLICK matches every filter. That makes every stage
	// answerable with every dimension (click_events carries them all),
	// including conversions and PWA/landing steps whose own tables lack the
	// column. The click itself may predate the window (a conversion can land
	// days later), so the subquery is deliberately not date-bounded.
	clickFilter, clickArgs, ok := buildFilterClause(reportClickColumns, filters, nil)
	if !ok {
		return nil, fmt.Errorf("chstore: unsupported filter dimension in funnel")
	}
	visitFilter, visitArgs := "", []any(nil)
	if len(filters) > 0 {
		visitFilter = ` AND click_id IN (SELECT click_id FROM click_events WHERE organization_id = ? AND type = 'SOURCE_CLICK'` + clickFilter + `)`
		visitArgs = append([]any{organizationID}, clickArgs...)
	}

	clickRows, err := s.conn.Query(ctx, `
		SELECT type, count(DISTINCT click_id)
		FROM click_events
		WHERE organization_id = ? AND event_at >= ? AND event_at < ? AND type = 'SOURCE_CLICK'`+clickFilter+`
		GROUP BY type`, append([]any{organizationID, from, to}, clickArgs...)...)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying funnel click counts: %w", err)
	}
	if err := scanFunnelCounts(clickRows, out); err != nil {
		return nil, err
	}

	trackingRows, err := s.conn.Query(ctx, `
		SELECT type, count(DISTINCT click_id)
		FROM tracking_events
		WHERE organization_id = ? AND event_at >= ? AND event_at < ?
			AND type IN ('LAND_VIEW', 'LAND_CLICK', 'PWA_VIEW', 'PWA_INSTALL')`+visitFilter+`
		GROUP BY type`, append([]any{organizationID, from, to}, visitArgs...)...)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying funnel tracking counts: %w", err)
	}
	if err := scanFunnelCounts(trackingRows, out); err != nil {
		return nil, err
	}

	conversionRows, err := s.conn.Query(ctx, `
		SELECT type, count(DISTINCT click_id)
		FROM conversion_events
		WHERE organization_id = ? AND event_at >= ? AND event_at < ?
			AND type IN ('CPA_HOLD', 'CPA_ACCEPT', 'CPA_REDEP')`+visitFilter+`
		GROUP BY type`, append([]any{organizationID, from, to}, visitArgs...)...)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying funnel conversion counts: %w", err)
	}
	if err := scanFunnelCounts(conversionRows, out); err != nil {
		return nil, err
	}

	return out, nil
}

// DistinctValues answers "what real values has dim actually taken" for
// org across [from, to) — backs the report builder's filter-value
// dropdown (docs/analytics-report-api.md), replacing the fixed mock
// pool apps/web/src/lib/mock/analytics.ts used to be the only source
// for. Ordered by frequency (most-common first, via count() DESC) so an
// operator picking from a long tail sees the values actually worth
// filtering on first, capped at limit.
//
// Reads click_events only, never conversion_events — reportClickColumns
// is the wider whitelist (every dimension conversion_events supports,
// click_events also supports, never the reverse), and a value that only
// ever appeared on a conversion but never on the click that produced it
// isn't a real scenario the event model allows for (every conversion
// resolves back to a click, §43's canonical funnel).
//
// Excludes the "" bucket deliberately: DimFlow/DimStreamSet's own real
// empty-string semantics (campaign fallback, blocked click) are
// meaningful to filter on, but confusing to suggest as a pickable
// dropdown value with no visible label — an operator who wants that
// filter can still type "" directly into the existing free-text value
// field, unaffected by this endpoint existing.
func (s *EventStore) DistinctValues(ctx context.Context, organizationID string, dim ReportDimension, from, to time.Time, limit int) ([]string, error) {
	col, ok := reportClickColumns[dim]
	if !ok {
		return nil, fmt.Errorf("chstore: unsupported dimension for distinct values: %s", dim)
	}

	query := fmt.Sprintf(`
		SELECT %s AS value
		FROM click_events
		WHERE organization_id = ? AND event_at >= ? AND event_at < ? AND type = 'SOURCE_CLICK' AND %s != ''
		GROUP BY value
		ORDER BY count() DESC
		LIMIT ?`,
		col, col,
	)

	rows, err := s.conn.Query(ctx, query, organizationID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying distinct values: %w", err)
	}
	defer rows.Close()

	// Initialized non-nil (not `var out []string`) so a brand-new
	// organization with zero matching clicks gets a real "[]" on the
	// wire — the frontend's own fallback-to-mock-pool logic treats an
	// empty real result as "genuinely nothing yet," not
	// indistinguishable from a JSON `null` it would otherwise have to
	// special-case.
	out := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("chstore: scanning distinct value: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func scanFunnelCounts(rows driver.Rows, out map[string]uint64) error {
	defer rows.Close()
	for rows.Next() {
		var typ string
		var count uint64
		if err := rows.Scan(&typ, &count); err != nil {
			return fmt.Errorf("chstore: scanning funnel count: %w", err)
		}
		out[typ] = count
	}
	return rows.Err()
}

// tzPattern is every character a real IANA zone name uses; anything else is
// refused rather than inlined into SQL.
var tzPattern = regexp.MustCompile(`^[A-Za-z0-9_+\-/]+$`)

// dayExpr is the `day` column: the UTC date, or the date in tz when one was
// given. tz is inlined (ClickHouse needs a constant zone in toDate), so an
// invalid name falls back to UTC instead of reaching the query.
func dayExpr(col, tz string) string {
	if tz == "" || tz == "UTC" || !tzPattern.MatchString(tz) {
		return "toDate(" + col + ") AS day"
	}
	return "toDate(" + col + ", '" + tz + "') AS day"
}

// selectAndGroupByExprs turns a resolved column list (plus whether the
// day pseudo-dimension is requested) into the two SQL fragments every
// query method above needs — the leading "col1, col2, " to prepend to
// SELECT (trailing comma included, so callers can concatenate directly
// in front of their metric expressions) and the full "GROUP BY ..."
// clause (empty string when there's nothing to group by, which ClickHouse
// requires — GROUP BY with an empty list is a syntax error, not "group
// everything into one row" the way omitting the clause already means).
func selectAndGroupByExprs(cols []string, byDay bool, tz string) (selectExpr, groupByExpr string) {
	all := make([]string, 0, len(cols)+1)
	if byDay {
		all = append(all, dayExpr("event_at", tz))
	}
	all = append(all, cols...)

	if len(all) == 0 {
		return "", ""
	}
	selectExpr = strings.Join(all, ", ") + ", "
	groupByExpr = "GROUP BY " + strings.Join(all, ", ")
	return selectExpr, groupByExpr
}

// scanDest builds the leading scan-destination pointers for a row: one
// *time.Time if byDay, then one *string per dim, in q.Dims order. Callers
// append their own metric-field pointers after calling this. Returns the
// raw pointers alongside separately so scanKey can dereference them once
// scanning succeeds.
func scanDest(dims []ReportDimension, byDay bool) (dimPtrs []*string, day *time.Time, dest []any) {
	dimPtrs = make([]*string, len(dims))
	for i := range dimPtrs {
		dimPtrs[i] = new(string)
	}
	dest = make([]any, 0, len(dims)+1)
	if byDay {
		day = new(time.Time)
		dest = append(dest, day)
	}
	for _, p := range dimPtrs {
		dest = append(dest, p)
	}
	return dimPtrs, day, dest
}

func deref(ptrs []*string) []string {
	out := make([]string, len(ptrs))
	for i, p := range ptrs {
		out[i] = *p
	}
	return out
}
