package chstore

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// Revenue nets chargebacks (§45; tech audit T4): a CPA_DECLINE or
// CPA_TRASH that reversed an accept carries the accept's payout negated
// (conversion.PostgresStore), booked on the decline's own date; any other
// decline carries 0. Conversion counts are unchanged — only revenue nets.
const (
	revenueTypes   = "('CPA_ACCEPT', 'CPA_REDEP', 'CPA_DECLINE', 'CPA_TRASH')"
	reportCPATypes = "('CPA_HOLD', 'CPA_ACCEPT', 'CPA_REDEP', 'CPA_DECLINE', 'CPA_TRASH')"
)

// ReportDimension is a whitelisted group-by dimension the report builder
// can request — never an arbitrary client string reaching a query
// verbatim; every method in this file rejects anything not in
// reportClickColumns/reportConversionColumns below before it gets near SQL.
//
// The 17 here match apps/web's lib/analytics-registry.ts DIMENSIONS.
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
	// DimPostlanding groups by the postlanding stage a click's routed path
	// went through (apps/tracker/postlanding.go). Empty is a real bucket:
	// flows without one, skipped stages (paused, behind an external
	// landing), and rows from before postlandings were served.
	DimPostlanding ReportDimension = "postlanding"
	// DimNetwork groups clicks by the network of the offer they were
	// routed to (schema/017) and conversions by the network that sent the
	// postback (schema/003). Empty is a real bucket: redirect flows,
	// fallbacks, blocked clicks.
	DimNetwork ReportDimension = "network"
	// DimTelegramChannel groups TG_JOIN by the channel joined
	// (schema/019). Everything else — clicks, every other event, joins
	// from before the channel was recorded — is the empty bucket; the
	// conversion report cannot answer it (a click may join several
	// channels).
	DimTelegramChannel ReportDimension = "telegram_channel"
)

// reportClickColumns is the whitelist for click_events and tracking_events,
// which share an identical dimension column set (both carry every field
// on the same underlying event.Event struct — see schema/002's own
// comment on why they're still two tables).
var reportClickColumns = map[ReportDimension]string{
	DimCampaign:        "campaign_id",
	DimSource:          "traffic_source_id",
	DimCountry:         "country",
	DimRegion:          "region",
	DimCity:            "city",
	DimDevice:          "device",
	DimPlatform:        "platform",
	DimOS:              "os",
	DimBrowser:         "browser",
	DimLanguage:        "language",
	DimFlow:            "flow_id",
	DimStreamSet:       "stream_set_id",
	DimLanding:         "landing_id",
	DimOffer:           "offer_id",
	DimPwa:             "pwa_id",
	DimPostlanding:     "postlanding_id",
	DimNetwork:         "network_id",
	DimTelegramChannel: "tg_channel_id",
}

// reportConversionColumns is the narrower whitelist for conversion_events
// (schema/003_conversion_events.sql) — it does not carry os/browser/
// language. Those three (reportConversionViaClickColumns) are answered by
// joining click_events onto each conversion via click_id, see
// reportConversionsJoined; every other dimension reads straight off
// conversion_events with no join.
var reportConversionColumns = map[ReportDimension]string{
	DimCampaign:    "campaign_id",
	DimSource:      "traffic_source_id",
	DimCountry:     "country",
	DimRegion:      "region",
	DimCity:        "city",
	DimDevice:      "device",
	DimPlatform:    "platform",
	DimFlow:        "flow_id",
	DimStreamSet:   "stream_set_id",
	DimLanding:     "landing_id",
	DimOffer:       "offer_id",
	DimPwa:         "pwa_id",
	DimPostlanding: "postlanding_id",
	DimNetwork:     "network_id",
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

// ValidDimension reports whether d is one of the 17 supported report
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
// apps/web/src/types/analytics.ts's client-side FilterCondition
// already used against mock data.
type ReportFilter struct {
	Dimension ReportDimension
	Value     string
}

// ReportQuery is the shared shape every method below takes. ByDay and
// Dims are independent — Timeseries-style callers set ByDay and leave
// Dims empty, report-table-style callers set Dims and leave ByDay false;
// both together also work (the report builder's "day" dimension).
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
	// DateBasis dates conversions by their own time ("" or
	// DateByConversion) or by their click's (DateByClick). Clicks, landing
	// views and installs are dated by when they happened either way.
	DateBasis DateBasis
	// Money converts revenue into the reporting currency per conversion
	// (§50-FX): nil keeps USD as stored.
	Money *MoneyFactors
}

// MoneyFactors is a USD → reporting-currency multiplier per UTC day
// (fx.Conversion.Days). A conversion on a day not listed has no rate, and
// makes its group's revenue NaN — unknown, never 0; the caller turns NaN
// into a null.
type MoneyFactors struct {
	Days    []time.Time
	Factors []float64
}

// revenueValueExpr is one conversion row's revenue, converted when m is set.
// The lists are inlined from time.Time / float64 values, never user text.
func revenueValueExpr(prefix string, m *MoneyFactors) string {
	usd := prefix + "usd_value * " + prefix + "has_usd_value"
	if m == nil {
		return usd
	}
	if len(m.Days) == 0 {
		return usd + " * nan"
	}
	days := make([]string, len(m.Days))
	factors := make([]string, len(m.Factors))
	for i, d := range m.Days {
		days[i] = "toDate('" + d.UTC().Format("2006-01-02") + "')"
		factors[i] = strconv.FormatFloat(m.Factors[i], 'g', -1, 64)
	}
	return fmt.Sprintf("%s * transform(toDate(%sevent_at, 'UTC'), [%s], [toFloat64(%s)], nan)",
		usd, prefix, strings.Join(days, ", "), strings.Join(factors, "), toFloat64("))
}

// DateBasis is which moment a conversion is reported on (tech audit AN5).
type DateBasis string

const (
	// DateByConversion: the day the network reported the conversion.
	DateByConversion DateBasis = "conversion"
	// DateByClick: the day of the click it is attributed to, so revenue
	// lands on the same day as that click and its spend. A conversion with
	// no attributed click has no click time and keeps its own date.
	DateByClick DateBasis = "click"
)

// Valid reports whether b is a known basis; "" means DateByConversion.
func (b DateBasis) Valid() bool { return b == "" || b == DateByConversion || b == DateByClick }

// conversionTimeExpr is the time a conversion_events row is reported at.
// By click, an attributed conversion's click time is its own time minus
// time_to_conversion_ms (set at attribution, also on a retried one).
func conversionTimeExpr(prefix string, basis DateBasis) string {
	if basis != DateByClick {
		return prefix + "event_at"
	}
	return fmt.Sprintf("if(%[1]sattribution_outcome = 'attributed', %[1]sevent_at - toIntervalMillisecond(%[1]stime_to_conversion_ms), %[1]sevent_at)", prefix)
}

// conversionRange is the WHERE condition (with its args) keeping the
// conversion_events rows reported in [from, to). By click it still bounds
// event_at from below — a conversion is not reported before its click, a
// day of slack covers a network clock behind ours — so the scan starts at
// the right partition.
func conversionRange(prefix string, basis DateBasis, from, to time.Time) (string, []any) {
	if basis != DateByClick {
		return prefix + "event_at >= ? AND " + prefix + "event_at < ?", []any{from, to}
	}
	at := conversionTimeExpr(prefix, basis)
	return prefix + "event_at >= ? AND " + at + " >= ? AND " + at + " < ?", []any{from.Add(-24 * time.Hour), from, to}
}

// datedByConversionExpr counts, by click, the rows that had no click time
// and kept their own date (0 by conversion).
func datedByConversionExpr(prefix string, basis DateBasis) string {
	if basis != DateByClick {
		return "toUInt64(0)"
	}
	return "countIf(" + prefix + "attribution_outcome != 'attributed')"
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
	// Blocked counts SOURCE_FILTER rows: clicks that matched no destination
	// at all (no flow, no fallback) and were therefore never redirected.
	Blocked uint64
	// Bots counts rows the classifier flagged is_bot, over both SOURCE_CLICK
	// and SOURCE_FILTER — a bot that fell through to a fallback URL is a bot
	// even though it still counts as a click.
	Bots uint64
	// EmptySubs counts SOURCE_CLICK rows whose sub1..sub10 all arrived
	// empty (§42 "unfilled FB subs"): the whole set lost on the way, not a
	// campaign that only uses sub1-3.
	EmptySubs uint64
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
	// Declines / Trash count CPA_DECLINE / CPA_TRASH events (§50
	// cpa_decline, cpa_trash) — a chargeback is a decline too, and a
	// decline later reversed still counts, as a hold later accepted still
	// counts as a registration.
	Declines uint64
	Trash    uint64
	// DatedByConversion counts, by click, the group's conversions with no
	// attributed click, reported on their own date instead.
	DatedByConversion uint64
	// Unpriced counts the group's ACCEPT/REDEP conversions that carried
	// revenue but got no USD value (no currency, or no rate on file): their
	// money is missing from RevenueUSD (audit 2026-10-15, DATA-1).
	Unpriced uint64
}

// InstallAgg is one group's stage events counted off tracking_events:
// PWA installs and the Telegram events (TG_START, TG_JOIN).
type InstallAgg struct {
	Key         ReportKey
	PwaInstalls uint64
	TgStarts    uint64
	TgJoins     uint64
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

// emptySubsCondition is true for a click_events row with none of
// sub1..sub10 set.
const emptySubsCondition = "sub1 = '' AND sub2 = '' AND sub3 = '' AND sub4 = '' AND sub5 = '' AND " +
	"sub6 = '' AND sub7 = '' AND sub8 = '' AND sub9 = '' AND sub10 = ''"

// uniqueVisitorExpr is who a click is, for unique clicks: the visitor's
// keyed pseudonym (schema/020, internal/visitorkey) per campaign. A click
// without one — from before the pseudonym, or a tracker without
// VISITOR_KEY — falls back to raw IP + user agent while they are kept
// (RETENTION_IP_DAYS), and a click with neither counts as its own visitor
// (audit 2026-10-15, GDPR-1).
const uniqueVisitorExpr = `multiIf(visitor_key != '', concat(campaign_id, '|', visitor_key),
		ip = '' AND user_agent = '', click_id,
		concat(campaign_id, '|', ip, '|', user_agent))`

// ReportClicks answers "how many SOURCE_CLICK rows, and how many distinct
// visitors among them" for q's date range/dims/filters, plus how many
// SOURCE_FILTER rows (Blocked) and is_bot rows (Bots) fall in the same slice. A visitor is the
// campaign + IP + user agent triple, counted by its keyed pseudonym
// (uniqueVisitorExpr) — the same fingerprint the tracker hashes
// to pick a flow (apps/tracker visitKey) — NOT the click_id: a click_id is
// minted per click (it is the postback attribution key) and is only reused by
// stickyFlowKeepClickId, so counting it made every click "unique" by default.
// A click with neither IP nor user agent has no fingerprint and falls back to
// its click_id. SOURCE_FILTER is not part of Clicks/UniqueClicks — matches the
// one place this codebase already computes a real "Clicks" number today,
// campaign-detail-view.tsx's own `.filter((c) => c.type === "SOURCE_CLICK")` —
// it is reported separately as Blocked so filtered traffic stays visible
// (§43). A group that only has blocked rows therefore appears with 0 clicks.
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
		SELECT %scountIf(type = 'SOURCE_CLICK') AS clicks,
		       uniqExactIf(`+uniqueVisitorExpr+`, type = 'SOURCE_CLICK') AS unique_clicks,
		       countIf(type = 'SOURCE_FILTER') AS blocked,
		       countIf(is_bot = 1) AS bots,
		       countIf(type = 'SOURCE_CLICK' AND `+emptySubsCondition+`) AS empty_subs
		FROM click_events
		WHERE organization_id = ? AND event_at >= ? AND event_at < ? AND type IN ('SOURCE_CLICK', 'SOURCE_FILTER')%s
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
		var clicks, unique, blocked, bots, emptySubs uint64
		dest = append(dest, &clicks, &unique, &blocked, &bots, &emptySubs)
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("chstore: scanning click report row: %w", err)
		}
		out = append(out, ClickAgg{Key: scanKey(q.Dims, deref(dimVals), day), Clicks: clicks, UniqueClicks: unique, Blocked: blocked, Bots: bots, EmptySubs: emptySubs})
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

// installTypes is what counts as a PWA install: the install page's beacon
// (PWA_INSTALL) and an iOS app's first launch (IOS_INSTALL — iOS has no
// beacon; docs/pwa-serving.md). installAs folds them into PWA_INSTALL for
// the funnel and the flow summary, which count distinct visits per stage.
const (
	installTypes = `('PWA_INSTALL', 'IOS_INSTALL')`
	installAs    = `if(type = 'IOS_INSTALL', 'PWA_INSTALL', type)`
)

// ReportPwaInstalls answers "how many PWA_INSTALL + IOS_INSTALL rows" for
// q — the pwaInstalls metric — and TG_START / TG_JOIN (tgStarts, tgJoins).
// Same table and dimensions as ReportLandViews.
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
		SELECT %scountIf(type IN `+installTypes+`) AS pwa_installs,
		       countIf(type = 'TG_START') AS tg_starts, countIf(type = 'TG_JOIN') AS tg_joins
		FROM tracking_events
		WHERE organization_id = ? AND event_at >= ? AND event_at < ?
		  AND (type IN `+installTypes+` OR type IN ('TG_START', 'TG_JOIN'))%s
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
		var installs, starts, joins uint64
		dest = append(dest, &installs, &starts, &joins)
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("chstore: scanning pwa-install report row: %w", err)
		}
		out = append(out, InstallAgg{Key: scanKey(q.Dims, deref(dimVals), day), PwaInstalls: installs, TgStarts: starts, TgJoins: joins})
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
	rangeClause, rangeArgs := conversionRange("", q.DateBasis, q.From, q.To)
	args := append([]any{q.OrganizationID}, rangeArgs...)
	filterClause, args, ok := buildFilterClause(reportConversionColumns, q.Filters, args)
	if !ok {
		return nil, fmt.Errorf("chstore: unsupported filter dimension in conversion report")
	}

	selectExpr, groupByExpr := selectAndGroupByExprsAt(selectCols, q.ByDay, q.Timezone, conversionTimeExpr("", q.DateBasis))

	query := fmt.Sprintf(`
		SELECT %scountIf(type IN ('CPA_ACCEPT', 'CPA_REDEP')) AS conversions,
			sumIf(`+revenueValueExpr("", q.Money)+`, type IN `+revenueTypes+`) AS revenue_usd,
			countIf(type = 'CPA_HOLD') AS registrations,
			countIf(type = 'CPA_ACCEPT') AS purchases,
			countIf(type = 'CPA_REDEP') AS repurchases,
			countIf(type = 'CPA_DECLINE') AS declines,
			countIf(type = 'CPA_TRASH') AS trash,
			%s AS dated_by_conversion,
			countIf(type IN ('CPA_ACCEPT', 'CPA_REDEP') AND revenue > 0 AND has_usd_value = 0) AS unpriced
		FROM conversion_events
		WHERE organization_id = ? AND %s AND type IN `+reportCPATypes+`%s
		%s`,
		selectExpr, datedByConversionExpr("", q.DateBasis), rangeClause, filterClause, groupByExpr,
	)

	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying conversion report: %w", err)
	}
	defer rows.Close()

	var out []ConversionAgg
	for rows.Next() {
		dimVals, day, dest := scanDest(q.Dims, q.ByDay)
		var conversions, registrations, purchases, repurchases, declines, trash, datedByConversion, unpriced uint64
		var revenueUSD float64
		dest = append(dest, &conversions, &revenueUSD, &registrations, &purchases, &repurchases, &declines, &trash, &datedByConversion, &unpriced)
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("chstore: scanning conversion report row: %w", err)
		}
		out = append(out, ConversionAgg{
			Key: scanKey(q.Dims, deref(dimVals), day), Conversions: conversions, RevenueUSD: revenueUSD,
			Registrations: registrations, Purchases: purchases, Repurchases: repurchases,
			Declines: declines, Trash: trash,
			DatedByConversion: datedByConversion,
			Unpriced:          unpriced,
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

	const convTypes = reportCPATypes
	innerRange, innerArgs := conversionRange("", q.DateBasis, q.From, q.To)
	outerRange, outerArgs := conversionRange("c.", q.DateBasis, q.From, q.To)
	// Args in placeholder order: click subquery (org, inner org + range), then outer WHERE.
	args := []any{q.OrganizationID, q.OrganizationID}
	args = append(args, innerArgs...)
	args = append(args, q.OrganizationID)
	args = append(args, outerArgs...)
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
		all = append(all, dayExpr(conversionTimeExpr("c.", q.DateBasis), q.Timezone))
	}
	all = append(all, selectCols...)
	selectExpr, groupByExpr := "", ""
	if len(all) > 0 {
		selectExpr = strings.Join(all, ", ") + ", "
		groupByExpr = "GROUP BY " + strings.Join(all, ", ")
	}

	query := fmt.Sprintf(`
		SELECT %scountIf(c.type IN ('CPA_ACCEPT', 'CPA_REDEP')) AS conversions,
			sumIf(`+revenueValueExpr("c.", q.Money)+`, c.type IN `+revenueTypes+`) AS revenue_usd,
			countIf(c.type = 'CPA_HOLD') AS registrations,
			countIf(c.type = 'CPA_ACCEPT') AS purchases,
			countIf(c.type = 'CPA_REDEP') AS repurchases,
			countIf(c.type = 'CPA_DECLINE') AS declines,
			countIf(c.type = 'CPA_TRASH') AS trash,
			%s AS dated_by_conversion,
			countIf(c.type IN ('CPA_ACCEPT', 'CPA_REDEP') AND c.revenue > 0 AND c.has_usd_value = 0) AS unpriced
		FROM conversion_events AS c
		LEFT JOIN (
			SELECT click_id, any(os) AS os, any(browser) AS browser, any(language) AS language
			FROM click_events
			WHERE organization_id = ? AND type = 'SOURCE_CLICK' AND click_id IN (
				SELECT click_id FROM conversion_events
				WHERE organization_id = ? AND %s AND type IN %s
			)
			GROUP BY click_id
		) AS k ON c.click_id = k.click_id
		WHERE c.organization_id = ? AND %s AND c.type IN %s%s
		%s`,
		selectExpr, datedByConversionExpr("c.", q.DateBasis), innerRange, convTypes, outerRange, convTypes, filterClause.String(), groupByExpr,
	)

	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying joined conversion report: %w", err)
	}
	defer rows.Close()

	var out []ConversionAgg
	for rows.Next() {
		dimVals, day, dest := scanDest(q.Dims, q.ByDay)
		var conversions, registrations, purchases, repurchases, declines, trash, datedByConversion, unpriced uint64
		var revenueUSD float64
		dest = append(dest, &conversions, &revenueUSD, &registrations, &purchases, &repurchases, &declines, &trash, &datedByConversion, &unpriced)
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("chstore: scanning joined conversion report row: %w", err)
		}
		out = append(out, ConversionAgg{
			Key: scanKey(q.Dims, deref(dimVals), day), Conversions: conversions, RevenueUSD: revenueUSD,
			Registrations: registrations, Purchases: purchases, Repurchases: repurchases,
			Declines: declines, Trash: trash,
			DatedByConversion: datedByConversion,
			Unpriced:          unpriced,
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
		SELECT `+installAs+` AS stage, count(DISTINCT click_id)
		FROM tracking_events
		WHERE organization_id = ? AND event_at >= ? AND event_at < ?
			AND type IN ('LAND_VIEW', 'LAND_CLICK', 'PWA_VIEW', 'PWA_INSTALL', 'IOS_INSTALL')`+visitFilter+`
		GROUP BY stage`, append([]any{organizationID, from, to}, visitArgs...)...)
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
// dropdown (docs/analytics-report-api.md). Ordered by frequency (most-common first, via count() DESC) so an
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
// filter can still type "" into the free-text value field.
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
	return selectAndGroupByExprsAt(cols, byDay, tz, "event_at")
}

// selectAndGroupByExprsAt is selectAndGroupByExprs with the day cut from
// timeExpr instead of event_at.
func selectAndGroupByExprsAt(cols []string, byDay bool, tz, timeExpr string) (selectExpr, groupByExpr string) {
	all := make([]string, 0, len(cols)+1)
	if byDay {
		all = append(all, dayExpr(timeExpr, tz))
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

// flowStageTypes are the stages FlowStageCounts counts, by the table that
// carries them (§25 "analytics summary" on a flow's nodes).
var flowStageTypes = map[string][]string{
	"click_events":      {"SOURCE_CLICK"},
	"tracking_events":   {"LAND_VIEW", "LAND_CLICK", "PWA_VIEW", "PWA_INSTALL", "IOS_INSTALL", "POSTLANDING_VIEW", "POSTLANDING_CLICK"},
	"conversion_events": {"CPA_ACCEPT", "CPA_REDEP"},
}

// FlowStageCounts counts, per flow and event type, what happened in
// [from, to): distinct visits (click_id) for clicks and the landing, PWA
// and postlanding stages — a reload is not a second view — and events for
// CPA_ACCEPT/CPA_REDEP, as the report's `conversions` does (a redeposit is
// a second conversion of the same visit). A flow with nothing is absent;
// the caller zero-fills.
func (s *EventStore) FlowStageCounts(ctx context.Context, organizationID string, from, to time.Time, flowIDs []string) (map[string]map[string]uint64, error) {
	out := map[string]map[string]uint64{}
	if len(flowIDs) == 0 {
		return out, nil
	}
	for _, table := range []string{"click_events", "tracking_events", "conversion_events"} {
		count := "count(DISTINCT click_id)"
		if table == "conversion_events" {
			count = "count()"
		}
		rows, err := s.conn.Query(ctx, `
			SELECT flow_id, `+installAs+` AS stage, `+count+`
			FROM `+table+`
			WHERE organization_id = ? AND event_at >= ? AND event_at < ? AND flow_id IN ? AND type IN ?
			GROUP BY flow_id, stage`,
			organizationID, from, to, flowIDs, flowStageTypes[table])
		if err != nil {
			return nil, fmt.Errorf("chstore: querying flow stage counts from %s: %w", table, err)
		}
		for rows.Next() {
			var flowID, typ string
			var n uint64
			if err := rows.Scan(&flowID, &typ, &n); err != nil {
				rows.Close()
				return nil, fmt.Errorf("chstore: scanning flow stage counts: %w", err)
			}
			if out[flowID] == nil {
				out[flowID] = map[string]uint64{}
			}
			out[flowID][typ] = n
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, fmt.Errorf("chstore: reading flow stage counts: %w", err)
		}
	}
	return out, nil
}
