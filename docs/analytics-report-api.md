# Phase 35 — the real `/analytics` multi-dimension report API

Replaces the mock data `features/analytics/*` (the report builder) and
`(app)/overview` (the dashboard) ran on since their Phase 14/14.5 frontend-
first build. The original `/analytics` surface (Phase 27) was deliberately
narrow — two campaign-scoped endpoints, click volume and conversion
revenue, proving the pipeline's last two stages worked end to end. This
phase is the real multi-dimension surface that narrow start always
deferred: group-by, a daily timeseries, the §43 funnel, and the Overview
dashboard.

## Endpoints

All under `/analytics`, tenant-scoped (`tenantMiddleware`, `organization_id`
from the session — never the request body), POST + JSON body (not GET +
query string: dims/filters/sort are structured, variable-length shapes,
same reason `POST /campaigns/{id}/routing/simulate` is POST rather than a
querystring).

| Endpoint | Backs |
|---|---|
| `POST /analytics/report` | Report builder table — 0-4 dims, filter, sort, every §50 metric per group. |
| `POST /analytics/timeseries` | Report builder line chart — one row per day, every metric included so the client can switch which one it plots without a second request. |
| `POST /analytics/breakdown` | One dimension, sorted by one metric descending, top-N. Used internally by `/analytics/dashboard`'s Top-N cards; the frontend's own bar chart tab instead calls `/analytics/report` with a single dim and does its own client-side top-8 sort, so switching the bar chart's metric picker re-sorts instantly without a refetch (see `report-chart.tsx`'s `ReportBarChart`). |
| `POST /analytics/funnel` | The §43 8-stage funnel's per-stage totals for a range. |
| `POST /analytics/dashboard` | `/overview`'s endpoint — KPI totals, a daily series, and three Top-N breakdowns (campaigns/countries/flows). |

Request/response shapes: `apps/internal/analytics/handler.go`'s
`reportRequest`/`timeseriesRequest`/`breakdownRequest`/`rangeRequest` and
`reportRowResponse`/`metricValuesResponse`/`dashboardResponse`. The
frontend mirrors them in `lib/api/analytics.ts`.

## Dimension whitelist: 11 of the 16 the UI names

`chstore.ReportDimension` (11): campaign, source (proxy for campaign — see
its own doc comment), country, region, city, device, platform, os,
browser, language, flow.

`features/analytics/registry.ts`'s `DimensionKey` still lists 5 more —
landing, pwa, postlanding, offer, network — with no backing column
anywhere in the event pipeline (not on `event.Event`, not in this schema).
Adding them means threading new IDs through `apps/tracker`'s event
emission and a schema migration first: a separate, later phase, not
silently faked here. The report builder's dimension picker still offers
all 16 (unchanged — `registry.ts` is shared with the still-mocked Custom
Metrics feature, which needs the full list); requesting one of the 5
unbacked ones 400s with `apierror.Validation`.

Of the 11 backed dimensions, only 8 have a column on `conversion_events`
(`chstore.reportConversionColumns`) — it carries no `os`/`browser`/
`language`. A report grouped by any of those three gets real
clicks/land-views but `null` for every conversion-derived metric
(conversions, revenue, profit, roi, roas, cpa, epc) — `chstore.
SupportedByConversions` decides this per-request, `Service.groupedReport`
enforces it. Joining `click_events`' os/browser/language onto a conversion
via `click_id` would answer this and is real, deliberately-deferred work
(a join, not a whitelist entry).

## Cost joins only at campaign granularity or coarser

`cost_entries` (Postgres) is campaign+day grained — no country/device/os/…
breakdown exists to join against. `Service.costEligible` allows a cost
join only when `dims` is empty, `[campaign]`, or `[source]` (source is the
same underlying column as campaign). Every other dimension combination —
by-country report, by-device breakdown, etc. — returns `cost`/`profit`/
`roi`/`roas`/`cpc`/`cpa` as `null` for every row, per CLAUDE.md #6: no cost
data on file must render as "—", never a fabricated 0. `MetricValues`
(the wire shape) makes this a real JSON `null` on every nullable field,
not a sentinel value.

## Frontend integration

- `features/analytics/analytics-view.tsx`: `useReport`/`useTimeseries`/
  `useFunnelReport` (`hooks/use-analytics-report.ts`, TanStack Query)
  replace the old `SLICES` mock array + `aggregateReport`/
  `aggregateTimeSeries`/`aggregateByDimension` client-side aggregation
  (`features/analytics/aggregate.ts` now holds only the `ReportRow`/
  `FilterCondition` types those functions used to build — `ReportTable`'s
  custom-metric formula evaluation keys off that same shape, so nothing
  downstream of the fetch needed to change). Each tab gates on its own
  query's `isPending`/`isError` (`LoadingState`/`ErrorState`), matching
  `campaign-detail-view.tsx`'s established per-section pattern rather than
  one page-level gate.
- `features/dashboard/dashboard-view.tsx`: `useDashboardReport` for the
  current and previous-period windows replaces `generateDashboardMock`.
  `lib/mock/dashboard.ts` is deleted outright (zero remaining importers);
  `PerformanceRow`/`PeriodMetrics` moved to `features/dashboard/metrics.ts`
  alongside the new `metricsFromApi`/`performanceRowFromApi` mapping
  functions. **No Top Offers card** — `offer` isn't a backed dimension
  (above), and `DashboardResult` only ever returns three breakdowns; the
  frontend drops the card rather than rendering it against fake data.
- Both views' default date range moved off the fixed mock "now"
  (`2026-08-11`) onto the real `new Date()` — real ClickHouse data has real
  timestamps. The Report Presets feature (`report-preset-bar.tsx`, itself
  still mocked — see CLAUDE.md's 7-domain gap) keeps its own fixed "now"
  anchor for period-relative presets ("Last 7 days" etc.); applying a
  preset after this phase computes a different absolute range than the
  view's own default until Report Presets gets its own integration phase.
  Documented here rather than silently reconciled, since fixing it means
  deciding what "now" means for a feature this phase didn't touch.
- `dimensionValues()` (`lib/mock/analytics.ts`) still backs the filter
  popover's value dropdown — no "distinct values seen for dimension X"
  endpoint exists (only equals-filtering). Filtering by an arbitrary value
  still round-trips to the real API correctly; a value that never occurs
  in real event data just yields a legitimate empty report, not an error.
- `TopCampaigns`/`TopFlows` display the raw `campaign_id`/`flow_id` as
  both `id` and `name` — `chstore`'s dimension whitelist resolves to those
  raw ID columns, no name join exists yet. Honest but not pretty; a real
  gap, not a silently-faked display name.

## Verified

`gofmt -l`/`go vet`/`go build`/`go test -count=1 ./...` clean repo-wide.
`apps/internal/chstore`'s `report_test.go` (ReportClicks/ReportLandViews/
ReportConversions/FunnelCounts, dimension grouping, filtering, by-day,
cross-tenant isolation) run against a real ClickHouse container
(`CLICKHOUSE_URL` set), not a mock — all pass. Full containerized dev
stack (`make up`) brought up; signed up and verified a real test
organization; `curl`'d all five endpoints directly (empty-state and, after
manually inserting real rows into `click_events`/`conversion_events`,
non-empty) — correct aggregation, correct dimension grouping, correct
null-cost cascade (cost/profit/roi/roas/cpc/cpa all `null`, not `0`, with
no `cost_entries` row on file). Real browser pass (Claude-in-Chrome)
against `/overview` and `/analytics`: KPI tiles, all four daily-metric
charts, and all three Top-N tables render correctly for both an empty org
and the seeded one; all four report-builder tabs (Table/Line/Bar/Funnel)
render real data with no console errors; a published custom metric
(`Margin per Click`) correctly evaluates to "—" against a null-cost row
instead of throwing or showing a fabricated number. `tsc --noEmit`/
`eslint`/`next typegen` clean on `apps/web`.
