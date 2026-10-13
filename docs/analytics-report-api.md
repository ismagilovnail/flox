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
| `POST /analytics/cost-timezones` | `{from, to, timezone}` → `{"mismatched": [...]}`: the ad-account zones whose daily spend in the range is cut in other days than the report (below). |

`report`, `timeseries`, `breakdown` and `dashboard` also take `dateBasis`
(`conversion` | `click`) — see "Conversions by click date" below.

Request/response shapes: `apps/internal/analytics/handler.go`'s
`reportRequest`/`timeseriesRequest`/`breakdownRequest`/`rangeRequest`/
`dimensionValuesRequest` and `reportRowResponse`/`metricValuesResponse`/
`dashboardResponse`. The frontend mirrors them in `lib/api/analytics.ts`.

## `dimension-values`: real filter-value suggestions, not a fixed mock pool

The report builder's filter popover suggests values that really occur.
`chstore.
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
endpoint here already uses — a request for the still-unbacked
dimension (`network`) 400s, same
as `/analytics/report` already does for one. The frontend's
`useDimensionValues` hook (`retry: false` — a 400 here is deterministic,
not transient) treats that rejection as expected, not an error to
surface: `AddFilterPopover` (`report-controls.tsx`) then offers no value
and disables "Add filter" — while loading too. A *successful*
response is used as is, even when empty (a brand-new
organization with no real events yet) — CLAUDE.md #6's "no fabricated
data" bar applies here the same way it does to cost: a genuine "nothing
yet" must render as nothing, never as a plausible-looking placeholder
value that isn't real.

## Dimension whitelist: 16 backed (15 of the original 16, plus stream_set)

`chstore.ReportDimension` (16): campaign, source (the traffic source of the click's
campaign, `traffic_source_id`, schema/011), country, region, city, device, platform, os,
browser, language, flow, stream_set, landing, offer, pwa, postlanding.

`postlanding` (`DimPostlanding`, `postlanding_id`,
`chstore/schema/016_postlanding_id.sql`, after `event_id`; also on
`click_lookup`, so conversions inherit it) followed once `apps/tracker`
served the postlanding stage (`docs/postlanding.md`, "Serving visitors"):
set on every event of a journey whose routed path goes through the flow's
postlanding — SOURCE_CLICK, LAND_*, PWA_*, POSTLANDING_VIEW/CLICK — and on
its conversions. Empty = no postlanding reached (none configured, paused,
behind an external landing, or before 2026-10-13). Names in the UI, "No
postlanding" for empty, "View statistics" from the postlanding list.

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

`features/analytics/registry.ts`'s `DimensionKey` still lists one more from
the original 16 — network — with no backing column anywhere in the event
pipeline. Nothing blocks it (an offer's network is known at routing time);
it is gap 4 of `docs/spec-conformance-2026-10-11.md`. The report builder
does not offer it (`UNBACKED_DIMENSIONS`); requesting it 400s with
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
- The report builder no longer offers `network` (unbacked, it 400s; `postlanding`
  was backed on 2026-10-13) and its filter-value picker no longer falls back to a mock pool.

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

## Spend days are the ad account's days (tech audit AN4)

Facebook (`date_start` with `time_increment=1`), TikTok (`stat_time_day`)
and Google Ads (`segments.date`) report spend per calendar day **in the ad
account's own time zone**, and `cost_entries.entry_date` keeps that day. A
report cuts clicks by its own zone, so with an account in
America/Los_Angeles and a report in UTC, "Oct 10" of spend and "Oct 10" of
clicks are 7–8 hours apart. Daily totals cannot be re-cut without hourly
spend; over weeks or months the edges barely matter.

So the report says so instead. The sync learns each account's zone
(Facebook `act_{id}?fields=timezone_name`, one extra call; TikTok
`advertiser/info` `display_timezone`, falling back to `timezone`; Google
`customer.time_zone` in the same GAQL query) and keeps it on
`ad_account_connections.timezone` (migration 00067) when it is a zone Go
knows; connecting another account clears it. `POST
/analytics/cost-timezones` lists the zones of synced spend in the range
whose UTC offset differs from the report zone's at the start or end of the
range (a DST change counts; Berlin and Paris do not differ). The overview
and the report builder show a note when the list is not empty
(`CostTimezoneNotice`). Manual spend has no zone and is never named.

Not verified against the real platforms (no accounts here): the field
names follow their API documentation and are tested against fakes. A wrong
guess only means no zone is stored and no note is shown — the spend itself
does not depend on it.

## Conversions by click date (tech audit AN5)

`report`, `timeseries`, `breakdown`, `dashboard` (body) and `GET
/analytics/campaigns/{id}/daily-revenue` (query) take `dateBasis`:
`"conversion"` (default, also `""`) or `"click"`; anything else is a 422
on `dateBasis`. By conversion, a conversion counts on the day the network
reported it. By click, it counts on the day of the click it is attributed
to — the day that click's spend and the click itself are on — so ROI per
campaign day compares like with like: a Monday registration that deposits
on Wednesday is Monday's revenue.

Only conversions move. Clicks, landing views and installs are dated by
when they happened either way, spend by its day.

- **No schema change.** The click time of an attributed conversion is
  `event_at − time_to_conversion_ms` (`conversionTimeExpr` in
  `chstore/report.go`), filled in at attribution and again when a retry
  attributes an `unknown_click` later (`internal/reattribute`). The range
  filter and the day are cut from that time, in the report's timezone;
  `event_at` is still bounded from below (a day of slack) so the scan
  starts at the right partition.
- **Conversions without a click** (`unknown_click`, `no_identifier`,
  ambiguous) have no click time and keep their own date. Each row then
  carries `datedByConversion`, their count, and `report`/`timeseries`/
  `breakdown` add the total at the top level, `dashboard` for its totals;
  the UI says how many (`DateBasisNotice`). They belong to no campaign, so
  campaign rows are never affected.
- **Recent days fill in.** By click, the last days keep gaining
  conversions as their clicks convert; the UI says so.
- **Totals agree.** Over a range holding the clicks and the conversions,
  the two bases only move revenue between days.
- `daily-revenue` by conversion reads the daily rollup
  (`conversion_events_daily_campaign`); by click it reads
  `conversion_events`, since the rollup only knows the conversion's day.
- Report presets keep the basis (`report_presets.date_basis`, migration
  00068); the funnel is unchanged (per click_id, its own logic).

Tests: `chstore/date_basis_test.go` against ClickHouse (days, range, a
non-UTC zone, the os join path, campaign daily revenue, equal totals),
`analytics/date_basis_test.go` (basis reaches the query, 422 on an unknown
one), `reportpresets` (round trip).

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
- ~~The filter popover's values came from a fixed mock pool~~ — closed,
  see `dimension-values` above; the mock (`lib/mock/analytics.ts`) is
  deleted.
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

