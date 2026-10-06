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
| `POST /analytics/dimension-values` | The report builder's filter-value dropdown — real distinct values seen for one dimension, most-common first (below). |

Request/response shapes: `apps/internal/analytics/handler.go`'s
`reportRequest`/`timeseriesRequest`/`breakdownRequest`/`rangeRequest`/
`dimensionValuesRequest` and `reportRowResponse`/`metricValuesResponse`/
`dashboardResponse`. The frontend mirrors them in `lib/api/analytics.ts`.

## `dimension-values`: real filter-value suggestions, not a fixed mock pool

Closes a gap this doc's own "Frontend integration" section used to flag:
`dimensionValues()` (`lib/mock/analytics.ts`) was the *only* source the
report builder's filter popover had for suggesting values to filter
on — a fixed, hand-written list, never the real data. `chstore.
EventStore.DistinctValues` answers "what values has this dimension
actually taken" against `click_events` (never `conversion_events` — every
backed dimension already has a column there, the wider whitelist, and a
value that only ever appeared on a conversion but never the click that
produced it isn't a scenario the event model allows for), ordered by
`count() DESC` and capped (`analytics.maxDimensionValues`, 50) so a
high-cardinality dimension like `campaign` doesn't produce an unusably
long dropdown. The empty-string bucket (`flow`/`stream_set`'s own real
"no match" semantics) is deliberately excluded from the result — useful
to *filter* on, confusing to *suggest* as an unlabeled dropdown entry;
an operator who wants it can still type `""` into the existing free-text
value field.

Gated by the same `chstore.ValidDimension` whitelist every other
endpoint here already uses — a request for one of the 2 still-unbacked
dimensions (`postlanding`/`network`) 400s, same
as `/analytics/report` already does for one. The frontend's
`useDimensionValues` hook (`retry: false` — a 400 here is deterministic,
not transient) treats that rejection as expected, not an error to
surface: `AddFilterPopover` (`report-controls.tsx`) falls back to
`lib/mock/analytics.ts`'s pool for those 2, and also as the query's own
brief loading-state placeholder for the 15 real ones. A *successful*
response wins outright the moment it lands, even when empty (a brand-new
organization with no real events yet) — CLAUDE.md #6's "no fabricated
data" bar applies here the same way it does to cost: a genuine "nothing
yet" must render as nothing, never as a plausible-looking placeholder
value that isn't real.

## Dimension whitelist: 15 backed (14 of the original 16, plus stream_set)

`chstore.ReportDimension` (15): campaign, source (the traffic source of the click's
campaign, `traffic_source_id`, schema/011), country, region, city, device, platform, os,
browser, language, flow, stream_set, landing, offer, pwa.

`pwa` (`DimPwa`, `pwa_id`, `chstore/schema/010_pwa_id.sql`, after
`offer_id`) became possible once `apps/tracker` started serving the PWA
stage (`docs/pwa-serving.md`): set on every event of a journey whose
routed path goes through the flow's PWA stage — SOURCE_CLICK, LAND_*,
PWA_VIEW/INSTALL/OPEN — and inherited by conversions. Empty = no PWA stage
reached (none configured, skipped, or behind an external landing). Names
in the UI, "No PWA" for empty, "View statistics" from the PWA list.

`offer` (`DimOffer`, backed by `offer_id`, `chstore/schema/009_offer_id.sql`
— same three tables, appended after `landing_id`) is the offer a click was
**actually routed to**: `routing.RouteResult.OfferID`, set by the engine
only on its "Offer" destination branch — empty for a redirect flow and for
an inactive offer that fell through to a fallback URL, so a paused offer
never collects clicks it didn't receive (`internal/routing/offer_id_test.go`
covers fresh and sticky routing). `routingstore` loads each flow's
`destination_offer_id` into `routing.Destination.OfferID`. The tracker
stamps it on SOURCE_CLICK and LAND_VIEW and carries it through the signed
landing click link onto LAND_CLICK; conversions inherit it through
attribution. Empty is a real bucket ("No offer" in the UI), names resolve
like landings' do, and the offers list's "View statistics" now passes the
offer's **id** — it used to pass its name, which matched nothing once the
dimension became real.

`landing` (`DimLanding`, backed by `landing_id`) is the first of the
original five unbacked dimensions to get a real column:
`chstore/schema/008_landing_id.sql` adds `landing_id` to `click_events`,
`tracking_events` and `conversion_events` (metadata-only `ADD COLUMN IF
NOT EXISTS`, appended last — `EventStore`'s positional `batch.Append`
writes it last too, and any other raw positional insert must as well).
Populated since `apps/tracker` serves landings (`docs/landings.md`):
SOURCE_CLICK and LAND_VIEW carry the landing actually shown, LAND_CLICK
carries it from its signed click link, and conversions inherit it from
their click through attribution (`attribution.Click.LandingID`,
`chstore.ClickResolver`) exactly like `stream_set_id`/`flow_id` — so
every metric, revenue included, works by landing. Empty is a real
bucket: traffic whose flow had no landing stage, a skipped one (paused
landing, no signing key), and every row from before landings were
served. In the UI (`features/analytics/use-dimension-value-label.ts`)
landing ids display as the landing's current name everywhere a value is
shown — table (and therefore its search/CSV), bar chart, filter dropdown,
filter chip — and `""` as "No landing"; the query and URL keep the id.
The landings list's "View statistics" action deep-links here by id.

`stream_set` (`DimStreamSet`, backed by `stream_set_id`) is a later
addition, not one of the original 16 the report builder's dimension
picker was designed against — the column has existed and been populated
by `apps/tracker` on every click since Phase 21; the gap closed here was
a whitelist omission, not a missing column, migration, or backfill (see
`chstore.ReportDimension`'s own doc comment). Empty string is a real
group for this dimension, exactly like `flow`'s `flow_id`: a click that
matched no stream set (campaign fallback) or was blocked (`SOURCE_FILTER`)
carries `stream_set_id = ""`, as does a conversion that never resolved to
a click — grouping by `stream_set` returns that as its own `""` row, not
missing data. The stream sets list's "View statistics" action
(`features/stream-sets/stream-set-row.tsx`) deep-links into this
dimension with the set's real id, not its display name — unlike the
pre-existing network/source "View statistics" links (below), this
one filters on the same value the column actually stores, so it works.
The filter chip it produces shows that raw id, since `FilterCondition`
(`features/analytics/aggregate.ts`) has no separate display-label field;
adding one is out of scope for a query-layer/UI change.

`features/analytics/registry.ts`'s `DimensionKey` still lists 2 more from
the original 16 — postlanding, network — with no backing column anywhere
in the event pipeline. postlanding is blocked on more than a column:
`apps/tracker` doesn't serve the postlanding stage yet (no
POSTLANDING_VIEW is ever emitted), so there is nothing honest to
attribute; it follows once that stage is served, like pwa did. network has no such blocker (an offer's network is
known at routing time), it just hasn't been asked for. The report builder's dimension
picker offers all 17 named keys now (unchanged in kind — `registry.ts` is
shared with the still-mocked Custom Metrics feature, which needs the full
list); requesting one of the 2 unbacked ones 400s with
`apierror.Validation`.

Of the 11 backed dimensions, only 8 have a column on `conversion_events`
(`chstore.reportConversionColumns`) — it carries no `os`/`browser`/
`language`. Grouping or filtering by those three is answered by joining
`click_events` onto each conversion via `click_id`
(`EventStore.reportConversionsJoined`): `any(os/browser/language)` per
`click_id` (a sticky returning visitor repeats the click row), restricted to
the click_ids of conversions in the report window but NOT bounded by the
window itself — a conversion can land days after its click. The join is
tenant-scoped on both sides. A conversion with no click on record lands in
the `""` bucket (shown as an empty value, like the other empty buckets).
Queries without os/browser/language keep the plain single-table path.
So conversions, revenue, profit, roi, roas, cpa and epc are real numbers
under these dimensions and filters, not `null`.

## `day`, timezone, funnel filters

- **`day`** is a pseudo-dimension accepted anywhere in `dims` of `POST
  /analytics/report` (`analytics.DimDay`). It is not a column: `Service.Report`
  splits it out, sets `ByDay`, and the row comes back with both `day` and the
  other `dims`. It counts toward the max-4 limit, can be a sort key (`"day"`),
  and is valid in a saved preset (dimensions and group-by). Per-day cost is
  attached for no other dimension, `[campaign]` or `[source]`
  (`attachDailyCost`: `DailySpendForOrg`, `CampaignSpendByDay`,
  `SourceSpendByDay`); any other combination gets `null` cost, as before.
- **`timezone`** (IANA name, `""`/`UTC` = UTC) is accepted by `report`,
  `timeseries` and `funnel`. It moves the day boundary (`toDate(event_at,
  'tz')`) and the From/To window (local midnights converted to UTC instants),
  so a report in another zone covers the same local days. Unknown names are a
  400; the name is also re-checked in `chstore` before it is inlined.
- **`funnel` takes `filters`.** A step counts a `click_id` only when its
  SOURCE_CLICK matches every filter (`click_id IN (SELECT ... click_events
  ...)`), so every stage is answerable with every dimension, including os/
  browser/language on the conversion steps. The click may predate the window.
- The report builder no longer offers `postlanding`/`network` (unbacked,
  they 400) and its filter-value picker no longer falls back to a mock pool.

## `source` is the traffic source, not a campaign proxy

`traffic_source_id` (`click_events`/`tracking_events`/`conversion_events`,
`chstore/schema/011_traffic_source_id.sql`) is stamped by the tracker from the
campaign row at click time (`routingstore.TrackingLink.TrafficSourceID`),
carried onto LAND_CLICK/PWA_* by the signed link claims (`landinglink.Claims.ts`)
and onto conversions through attribution (`attribution.Click`). So two
campaigns on one source merge into one `source` row, and the label hook
(`use-dimension-value-label.ts`) resolves the id straight from the sources
list. The sources list's "View statistics" link now filters on the id.
Rows written before 011 have `''` (the "No source" bucket) until backfilled.

### Backfilling traffic_source_id

From Postgres (`campaigns.traffic_source_id`) into ClickHouse, repeatable
(`traffic_source_id = ''` only touches rows not yet filled):

```sh
docker exec infra-postgres-1 psql -U flox -d flox -At -F'|' -c \
  "select traffic_source_id, string_agg(quote_literal(id), ',') from campaigns group by traffic_source_id" |
while IFS='|' read -r src ids; do for t in click_events tracking_events conversion_events; do
  echo "ALTER TABLE $t UPDATE traffic_source_id = '$src' WHERE campaign_id IN ($ids) AND traffic_source_id = '' SETTINGS mutations_sync = 1;"
done; done | docker exec -i infra-clickhouse-1 clickhouse-client --user flox -d flox --multiquery
```

Events whose campaign no longer exists in Postgres stay `''`.

## Cost joins only at campaign granularity or coarser

`cost_entries` (Postgres) is campaign+day grained — no country/device/os/…
breakdown exists to join against. `Service.costEligible` allows a cost
join only when `dims` is empty, `[campaign]`, or `[source]`. For `[source]`
cost is rolled up through the campaign (`cost.SpendBySource`: each entry counts
toward `campaigns.traffic_source_id`, the same attribution clicks use), not
through the entry's optional own `traffic_source_id` tag. Every other dimension combination —
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
  functions. **No Top Offers card** — `offer` wasn't a backed dimension
  then (it is now, see above; adding the card is a separate, not-yet-made
  product call), and `DashboardResult` only ever returns three breakdowns;
  the frontend drops the card rather than rendering it against fake data.
- Both views' default date range moved off the fixed mock "now"
  (`2026-08-11`) onto the real `new Date()` — real ClickHouse data has real
  timestamps. The Report Presets feature (`report-preset-bar.tsx`, itself
  still mocked — see CLAUDE.md's 7-domain gap) keeps its own fixed "now"
  anchor for period-relative presets ("Last 7 days" etc.); applying a
  preset after this phase computes a different absolute range than the
  view's own default until Report Presets gets its own integration phase.
  Documented here rather than silently reconciled, since fixing it means
  deciding what "now" means for a feature this phase didn't touch.
- ~~`dimensionValues()` (`lib/mock/analytics.ts`) still backs the filter
  popover's value dropdown — no "distinct values seen for dimension X"
  endpoint exists~~ — closed, see `dimension-values` above.
  `lib/mock/analytics.ts` still backs the loading-state placeholder and
  the 2 still-unbacked dimensions. Filtering by an arbitrary value still
  round-trips to the real API correctly regardless of its source; a
  value that never occurs in real event data just yields a legitimate
  empty report, not an error.
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

## PWA / CPA funnel metrics

Every metrics object also carries: `pwaInstalls` (PWA_INSTALL count),
`registrations` (CPA_HOLD), `purchases` (CPA_ACCEPT), `repurchases` (CPA_REDEP),
`clickToInstall`, `installToReg`, `regToPurchase` (percent) and
`costPerInstall`, `costPerReg`, `costPerPurchase` (USD). The CPA counts are
`null` when the requested dimensions can't be answered from conversion_events;
rates and costs are `null` when the denominator is 0 or no cost is on file.
`conversions` counts CPA_ACCEPT + CPA_REDEP. `revenue` is CPA_ACCEPT +
CPA_REDEP **net of chargebacks**: a CPA_DECLINE/CPA_TRASH that reversed an
accepted FTD carries that payout negated and is summed in, on the decline's
own date (so a past day keeps the revenue it showed; the decline's day can
be negative). See `docs/conversion.md`, "Chargebacks".

## Blocked traffic and bots

Every metrics object also carries `blocked`, `bots` (counts) and `blockedRate`,
`botRate` (percent, `null` when no traffic reached the tracker).

- `blocked` = `SOURCE_FILTER` rows: clicks that matched no destination (no flow,
  no stream-set or campaign fallback), so there was no redirect. They are **not**
  part of `clicks`/`uniqueClicks`.
- `bots` = rows the classifier flagged `is_bot`, over `SOURCE_CLICK` and
  `SOURCE_FILTER` — a bot sent to a fallback URL still counts as a click *and* a bot.
- `blockedRate = blocked / (clicks + blocked)`, `botRate = bots / (clicks + blocked)`.

A group made only of blocked clicks (e.g. a country that is filtered out entirely)
now appears as a row with `clicks: 0`. These metrics follow the click dimensions
(`campaign`, `country`, `device`, … `offer`, `pwa`); the empty `flow`/`stream_set`/
`landing` bucket is where blocked clicks land, since they never reached one. The
Overview shows four tiles: Blocked, Blocked share, Bots, Bot share.

