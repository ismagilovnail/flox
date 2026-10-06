# CHANGELOG

Format follows [Keep a Changelog](https://keepachangelog.com/). Entries are
per-phase, matching `CLAUDE.md`'s phase protocol. The one exception is
[Between phases], below: spec amendments and the code changes that follow from
them land between phases and would otherwise be invisible here.

## [Between phases] — blocked traffic and bots in analytics

Audit finding #3 (P0): `SOURCE_FILTER` clicks (no destination — nothing redirected) and bot
flags were stored but invisible, so clicks in the ad cabinet could not be reconciled with the
tracker (in the dev data: 1116 blocked vs 4634 counted clicks). New metrics `blocked`, `bots`,
`blockedRate`, `botRate` in every report/timeseries/breakdown row and on the Overview (four
tiles, up-is-bad, compared with the previous period); selectable columns in the report
builder with hints (EN/RU), accepted in saved presets. One ClickHouse query (`countIf`),
`Clicks`/`UniqueClicks` unchanged; rates are shares of everything that reached the tracker
(`clicks + blocked`) and `null` — "—" — with no traffic. A group with only blocked clicks now
shows up with 0 clicks. Tests: service unit tests, ClickHouse test (blocked-only bucket,
bot among redirected clicks, cross-org isolation). Verified: full-env `go test` (`eventqueue`
flakes against the live worker, passes alone), tsc + eslint, API against a throwaway org
(40 / 10 / 8 / 20% / 16% exactly as seeded; org removed afterwards). Browser pass done afterwards
(real session): Overview shows the four tiles (Blocked 0, Blocked share 0.00%, Bots 1, Bot share
5.56% for 18 clicks), the report's metric picker finds both new metrics and the Blocked column
renders in the table. Not done: bots as a report
dimension/filter, a bots-only exclusion toggle, default-column placement (audit #12).

## [Between phases] — offer daily cap is enforced

Audit finding #1 (P0): the offer's "Daily cap" was stored and shown but never read.
Now a capped offer stops receiving traffic once it has `cap` new conversions today
(organization timezone; a conversion = a click's first HOLD/ACCEPT, so HOLD→ACCEPT counts
once). Its flows leave the weighted draw like paused ones — other flows (other offers) take
the traffic, and with none left the stream-set/campaign fallback applies; a sticky visitor
of a capped offer is re-evaluated. Decision logic stays in `internal/routing`
(`RequestContext.CappedOffers`, `FlowCandidate.Capped`); `internal/offercap` rebuilds the
snapshot every 30 s off the hot path (Postgres offers + `chstore.NewConversionsByOffer`,
org-scoped); tracker and the API's routing simulator both use it. A failed refresh keeps the
previous snapshot. Tests: routing unit tests, offercap unit tests, ClickHouse test with
cross-org isolation, and `e2e/TestOfferDailyCap` against the real stack (two offers 50/50,
cap 1 on one, one postback → all traffic to the other). Full-env `go test ./...` green
(`eventqueue` flaked once against the live worker, passes alone). Not done: UI badge
"limit reached" in the offers list, `capped` marker in the simulator UI, alerts.

## [Between phases] — conversions list: offer, flow, tags, payout, status history

The Conversions list gained: **Offer** and **Flow** (names resolved client-side), a
**Status history** column (every CPA status the click_id reached, oldest first, with dates:
hold → accept → redep), **Payout** and **Currency** as separate columns, and hidden-by-default
**USD**, **Network txn ID**, `utm_*` and `sub1`–`sub10` (turned on via "Columns"; columns are
remembered and reorderable). `GET /conversions` rows carry `flowId`, `offerId`,
`networkTxnId`, the utm/sub fields and `history`. `conversion_events` has no `offer_id`, so
the offer is read from the click's `click_events` row (absent for a redirect flow or a
fallback). Both new batched ClickHouse reads (`StatusHistoryByClickIDs`, `OffersByClickIDs`)
filter on `organization_id`; tests cover cross-org isolation. USD shows "—" when no FX rate
was on file. Not done: inline row expansion (the history is shown as a chain in its column
instead; the detail page keeps the full timeline). Verified in the browser with temporary
rows (removed afterwards) and with the full-env `go test ./...`.

## [Between phases] — tracking link builder with Facebook/TikTok macros

Each tracking link on the campaign page has a "With parameters" menu: copy for Facebook
or TikTok (the campaign's traffic source type decides which is listed first), or open the
builder — editable `utm_*`, `sub1`–`sub10` and `ttclid` fields, a "fill from" preset, and a
live preview of the final URL. The presets (`lib/tracking-link-params.ts`) map the
platforms' own macros (`{{campaign.id}}`, `{{adset.id}}`, `__AID__`, `__CALLBACK_PARAM__`…)
into the keys the tracker already reads (`apps/tracker/params.go`); macros are never
percent-encoded, other values are. Presets live in code, not per traffic source. Unit tests
for the URL assembly. Verified in the browser (dev compose `web`): menu, builder prefilled
from the source type, Facebook/TikTok presets, encoding of a hand-typed value, copy toast.
That pass found that a traffic source's `type` is a free string (older rows hold
`facebook`), so the source-type match ignores case; the copy menu was also widened.
The clipboard's content itself was not read back.

## [Between phases] — organization timezone is the default for analytics and the overview

The timezone from Settings is now the default for the analytics report (instead of a
hard-coded `UTC`) and for the overview, whose days were always cut in UTC. The picker in
the report controls stays a per-report override. `POST /analytics/dashboard` and
`/analytics/breakdown` accept `timezone` (empty = UTC, unknown zone = validation error);
`Dashboard` passes it to its totals, daily series and top-N. The default period's
"today" and presets' relative periods are anchored to the zone's calendar day
(`todayInZone`). Saved presets keep their own explicit timezone, so presets saved before
this change still say `UTC`. Not verified in a browser.

## [Between phases] — marketer audit report

`docs/audit-marketer-2026-10-06.md`: a prioritized audit of the platform from a media
buyer's side (4 P0, 6 P1, 3 P2 findings, what was verified and what was not). Findings
only; nothing in the code was changed for it.

## [Between phases] — creating a network, offer or traffic source returns to the list

After "create" the new-network / new-offer / new-traffic-source pages go to the
directory list (`/networks`, `/offers`, `/traffic-sources`) instead of the new item's
detail page. A new network still shows the postback secret first (it is displayed only
once) and goes to the list when that dialog is closed. Campaigns are unchanged.

## [Between phases] — campaigns, flows, offers, networks and traffic sources remember their columns

The five directory tables (campaigns, flows, offers, networks, traffic sources) use
`DataTable`'s `persistKey` and `enableColumnReorder`, like the click log: hidden
columns, drag-and-drop order and widths survive a reload (per browser, one entry per
table under `flox.table.<name>`). `DataTable.moveColumn` also refuses to drop a column
onto the row-selection checkbox, which stays first.

## [Between phases] — click log remembers its columns and can be reordered

The click log table now uses `DataTable`'s `persistKey` ("click-log") and
`enableColumnReorder`: hidden columns, their drag-and-drop order and widths survive a
reload (per browser). Without a stored choice the table keeps its default (the
`CLICK_LOG_INITIAL_COLUMN_VISIBILITY` set of hidden columns).

## [Between phases] — filter audit: end-to-end matrices, ASN normalization, queue batch fix

Two end-to-end tests (`apps/e2e/filters_test.go`, throwaway organization, real API +
tracker + classifier + GeoIP + worker + ClickHouse):

- `TestFilterMatrix` — ~150 cases: every routable field (country, city, region, asn,
  device, platform, os, os_version, browser, browser_version, language, bot, proxy,
  connection_type, referrer, utm_*, sub1..sub10, external_click_id), every operator
  (IS, IS_NOT, IN, NOT_IN, CONTAINS, NOT_CONTAINS, STARTS_WITH, ENDS_WITH, MATCHES,
  EXISTS, NOT_EXISTS, GT, GTE, LT, LTE, BETWEEN incl. numeric-not-text comparison),
  AND / OR / nested groups, and the empty group; each with a matching and a
  non-matching request, asserting the 302 destination.
- `TestAnalyticsFilterMatrix` — four distinguishable clicks, then the report endpoint
  under a filter on each dimension (country, region, city, device, platform, os,
  browser, language, campaign, source, stream_set, flow, landing/offer/pwa empty
  buckets) and AND combinations; counts must be exact.

Findings fixed:
- **ASN**: the classifier reports "AS15169"; a filter typed as "15169" or "as15169"
  matched nobody. The stream-set API now normalizes ASN values to "AS<digits>" at save
  time for IS / IS NOT / IN / NOT IN (the routing hot path stays literal).
- **Event queue**: `EnqueueBatch` is one transaction, so one event of a deleted
  organization (foreign key) rolled back the whole batch, including other
  organizations' events buffered with it. It now retries per organization: only the
  orphan's events are lost, and their error is still returned and logged.
  Test: `TestEnqueueBatchOrphanOrganizationDoesNotDropTheRest`.

Known limits, not changed (the UI offers them, the classifier cannot produce them):
`proxy` is always "0" (no IP-reputation vendor), and `connection_type` is always
"unknown", so "proxy = 1" and "connection_type = wifi / cellular / ethernet" never
match. Brave is reported as chrome.

## [Between phases] — language dropdown, more browsers, `language` now matches

Language dropdown: the `language` condition of a stream-set filter is a searchable
list of ISO 639-1 languages ("RU — русский", names from `Intl.DisplayNames`, CIS and
major traffic languages first), single for IS / IS NOT and multiple for IN / NOT IN,
like the country list (`lib/languages.ts`).

Behaviour fix behind it: the classifier stored the whole first Accept-Language tag
("ru-RU", "en-US") while routing compares literally, so "language IS ru" matched no
real browser. `primaryLanguage` now reduces it to the lowercase language subtag
("ru-RU" -> "ru"), as the filter docs always said (RU, EN, UK). Consequences: a
filter that was written against a full tag ("en-US") stops matching, and the
`language` analytics/click-log value is "en" instead of "en-US" for new events
(old events keep what they were stored with).

More browsers: the classifier and the `browser` filter list now know Opera, Yandex
Browser, Vivaldi, UC Browser, Huawei Browser, Mi Browser, DuckDuckGo and Internet
Explorer next to Chrome, Safari, Firefox, Edge and Samsung Internet (all of the new
ones carry a `Chrome/` token, so they are matched before Chrome). Brave sends a
Chrome-identical User-Agent and cannot be told apart: it counts as chrome. Tests:
`TestClassify_LanguageIsTheLowercaseLanguageSubtag` and a user-agent case per new
browser.

## [Between phases] — `region`, `city`, `proxy` and `connection_type` removed from the stream-set filter list

The filter builder no longer offers `region`, `city`, `proxy` or `connection_type`
(nor does the filter reference sheet describe them). The last two had nothing behind
them: the classifier always reports proxy "0" and connection_type "unknown". The field types and the routing engine's support
are untouched, so a stream set that already has such a condition keeps matching as
before.

## [Between phases] — country dropdown in stream-set filters

The `country` condition of a stream set's filters used to be a free-text box of ISO
codes. For the exact-match operators it is now a searchable list of all 249 ISO-3166
countries ("RU — Россия", names from `Intl.DisplayNames` in the interface language;
CIS markets and the common CPA markets pinned first): one country for IS / IS NOT
(new `SearchableSelect`, the single-value sibling of `MultiSelect`), several for
IN / NOT IN. The pattern operators (CONTAINS, STARTS WITH, MATCHES, ...) keep the
text input. The stored value is unchanged (comma-separated codes), so existing
filters keep working.

## [Between phases] — "Уникальные клики" count visitors, not click_ids

Two visits through one link showed 2 clicks and 2 uniques. The metric was
`count(DISTINCT click_id)`, but a click_id is minted per click (it is the postback
attribution key) and is reused only when a campaign enables
`stickyFlowKeepClickId`, so by default every click was "unique". A unique click is
now a distinct visitor: campaign + IP + user agent, the same fingerprint the
tracker's `visitKey` uses to pick a flow (a click with neither IP nor user agent
falls back to its click_id). `uniqExact` over that string in
`EventStore.ReportClicks`; no schema change. Counted within the report's period
and group, so the same visitor on two different days is one unique per day-row but
one overall unique per period row. Test:
`TestReportClicksUniqueIsPerVisitorNotPerClickID`. Not a fraud control: two people
behind one IP with an identical browser count as one.

## [Between phases] — campaign page: "Настройки" tab removed

The tab held only a "danger zone" archive button, duplicating the "Архивировать"
button (and its confirmation dialog) in the page header. The tab, its own dialog
and the seven unused `detail.*` strings are gone; the page now has Обзор, Расходы
and Симулятор.

## [Between phases] — no search box on a campaign's tracking links

`DataTable` gets `enableSearch` (default true, so every other table keeps its
search); the tracking-links table on the campaign page turns it off.

## [Between phases] — "Статистика кампании" link

Campaigns get the same drill-in the other directories have: an icon button in each
campaigns-table row and a labelled button in the campaign page header open
`/analytics?dim=campaign&val=<id>` (the report pre-filtered to that campaign,
on the table tab: `viewStatisticsHref` no longer forces the line tab for any
directory; `?tab=line` is still understood). `campaign` is added to `viewStatisticsHref`'s dimensions and to the
analytics view's deep-link allowlist.

## [Between phases] — Overview tables remember their columns; header hints

`DataTable` gets an opt-in `persistKey`: the table's own column visibility, order
and widths are kept in this browser (`flox.table.<key>`, read through
`useSyncExternalStore`, so the server render and the first client render agree);
a part the owner controls (the analytics report) is left alone. The three
"Топ" tables on the Overview use `dashboard.top-<entity>`, one setting per table.
The same tables' column headers now carry a "?" hint (`meta.hint`, ru/en in the
`dashboard` namespace): what the table lists, clicks, CVR, revenue, ROI.

## [Between phases] — unsaved report edits survive a reload

Choosing a timezone (or hiding a column, reordering, changing metrics, filters,
grouping) without saving the preset was lost on reload: the remembered preset was
re-applied over it. The report builder now keeps a per-browser draft
(`flox.analytics.draft` in localStorage: timezone, dimensions, metrics, filters,
column order/widths/hidden columns, groupBy) on top of the active preset id; it is
re-applied after the preset on load and dropped as soon as the report matches the
preset again (saved, or edited back). Not part of the draft: date range, sort,
compare. A draft made on a deleted/other preset is ignored.

## [Between phases] — Russian timezones in the timezone dropdown

`lib/timezones.ts` + `components/ui/timezone-select.tsx`: one dropdown shared by
the report builder and organization settings. A "Россия" group (Калининград,
Москва, Самара, Екатеринбург, Омск, Красноярск, Новосибирск, Иркутск, Якутск,
Владивосток, Магадан, Камчатка) with city names and a right-aligned UTC offset,
then "Весь мир" (UTC, London, Berlin, Dubai, Singapore, New York, Chicago, Los
Angeles). The list opens below the trigger (`position="popper"`), with no max-height of
its own: all ~20 rows fit on a normal screen, so Radix has nothing to scroll to
the selected zone and there is no jump (an earlier scroll-reset workaround
flickered and was removed). City names are in the `analytics` i18n namespace (ru/en); a zone
outside the list stays selectable under its IANA id. The API now embeds
`time/tzdata`: the alpine runtime image ships no zone database, so
`time.LoadLocation` for any zone but UTC would have returned "unknown timezone"
there (the dev image only worked through GOROOT's zoneinfo).

## [Between phases] — PWA / registration / purchase metrics; hidden columns are saved with presets

Metrics (§50 CPA FUNNEL): `pwaInstalls` (PWA_INSTALL, from `tracking_events`),
`registrations` (CPA_HOLD), `purchases` (CPA_ACCEPT, the first purchase / FTD),
`repurchases` (CPA_REDEP); step rates `clickToInstall`, `installToReg`,
`regToPurchase` (percent) and costs `costPerInstall`, `costPerReg`,
`costPerPurchase`. Rates and costs are `null` (shown "—") when the denominator
is zero or no cost is on file — never 0 (CLAUDE.md #6). The three CPA counts are
`null` when conversion_events can't answer the requested dimensions, like
`conversions`. `conversions`/`revenue` are unchanged (accept + redep only): the
conversion queries now also read CPA_HOLD, so a group with only a registration
is a row with 0 conversions. New `EventStore.ReportPwaInstalls`, per-status
counters on `ConversionAgg`, 10 keys in `validMetrics` and the registry.
Custom Metrics' registry does not know the new metrics yet.

Hidden table columns: the report's column visibility was DataTable-internal, so
a reload forgot it. It is now part of the report state and of presets
(`report_presets.hidden_columns text[]`, migration 00047, `hiddenColumns` on the
API: omitted on update = unchanged, `[]` = show all). Like order and widths it is
restored when the preset is applied; unsaved edits still need "Сохранить
изменения". Tests: `TestReportPwaAndCpaFunnelMetrics`,
`TestReportFunnelMetricsAreNullNotZeroWhenUndefined`,
`TestReportPwaInstallsCountsAndIsTenantScoped`,
`TestReportConversionsCountsRedepsSeparately`,
`TestPresetHiddenColumnsRoundTripUpdateAndClear`.

## [Between phases] — funnel, bar, line and Overview charts no longer flicker / vanish on hover

Cause: series were filled with `oklch(...)` colours, and on hover zrender animates
a series' colour towards its emphasis state; it cannot parse oklch, so the fill
came back as `none`. Measured: sampling a pixel of the top funnel segment while
the mouse moved showed it flipping to `0,0,0,0` (16 flips in ~200 frames); the
bar chart's `fill` went `oklch → none → oklch → none` on three hovers and stayed
`none` (the bar disappeared). Fix: `CHART_RGB` / `rgba()` in `lib/chart-theme.ts`
(sRGB twin of `CHART_COLORS.primary`) are used for the series fills in the funnel,
bar and line charts, and their series have `emphasis.disabled` (the tooltip carries
the numbers). After it: funnel 0 changes in ~400 frames, bar 0 DOM mutations on
hover and the fill stays `rgb(76,153,248)`. Also: the funnel is drawn on canvas with
`silent` labels (a first, insufficient attempt, harmless to keep), and its tooltip
shows the translated step name instead of the raw `SOURCE_CLICK`. The Overview dashboard's
`LineMetricChart` (revenue / spend / profit / conversions) had the same oklch line
and gradient fills: it now takes a `CHART_RGB` key (`primary`, `success`,
`warning`, `danger` — sRGB twins computed from the oklch tokens) and builds
`rgba()` line and gradient stops, with `emphasis.disabled`; `CHART_COLORS` is no
longer used by any chart.

## [Between phases] — column widths are part of the report and of presets

Same gap as column order: dragging a column's width was internal to the table,
so "Сохранить изменения" never appeared and presets couldn't keep widths. Widths
are now part of the report state and presets: `report_presets.column_widths`
(jsonb, migration 00046, existing rows get `{}` = natural widths),
`columnWidths` on the API (px by column id; omitted on update = unchanged,
`{}` = reset; 48-2000 px, ≤ 60 entries). `DataTable` accepts a controlled
`columnSizing`/`onColumnSizingChange`. Test:
`TestPresetColumnWidthsRoundTripUpdateAndClear`.

## [Between phases] — column order is part of the report and of presets

Dragging a table column didn't count as a change (the order lived inside the
table), so "Сохранить изменения" never appeared and a preset couldn't remember
it. The order is now part of the report state and of presets:
`report_presets.column_order` (text[], migration 00045, existing rows get `{}` =
natural order), `columnOrder` on the API (omitted on update = unchanged, `[]` =
reset; ≤ 60 ids of ≤ 64 chars). `DataTable` accepts a controlled
`columnOrder`/`onColumnOrderChange`; dragging back to the natural arrangement
stores `[]` so nothing shows as modified. Applying a preset restores its order.
Column *widths* are still per-view and not saved. Test:
`TestPresetColumnOrderRoundTripUpdateAndClear`.

## [Between phases] — "Сохранить изменения" on a modified preset

When the chosen preset is your own and the report on screen differs from it
(dimensions, metrics, filters, grouping, period or timezone), a "Сохранить
изменения" button appears next to the chips and writes the current report over
that preset in one click (no dialog; the name stays), and the preset's chip
turns dashed with a dot until it is saved. The default preset can't be edited,
so there only "Сохранить как пресет" is offered. Period is now part of the
"unchanged" check. Frontend only (`report-preset-bar.tsx`), uses the existing
update endpoint.

## [Between phases] — report presets save their filters

A preset stored dimensions, metrics, grouping, period and timezone but not the
report's filters, so a preset saved with "Источник = Facebook" came back
unfiltered. Presets now carry filters: `report_presets.filters` (jsonb,
migration 00044, existing rows get `[]`), `Filter {dimension, value}` on the
API (`filters` on create/update/read; omitted on update = unchanged, `[]` =
clear), validated (known dimension, not `day`, value ≤ 200 chars, ≤ 10
filters). Applying a preset sets its filters; saving or updating one stores the
current ones; the "this preset is active" highlight also requires the same
filters. Tests: `TestPresetFiltersRoundTripUpdateAndClear`,
`TestPresetRejectsInvalidFilters`.

## [Between phases] — the chosen report preset is remembered

Picking a preset on /analytics is remembered in the browser
(`flox.analytics.activePreset`, localStorage): after a reload the preset is
re-applied (silently) and highlighted. A freshly saved or updated preset
becomes the chosen one; deleting the chosen preset forgets it; a page opened
from a "View statistics" link (`?dim=`) keeps that link's report. The highlight
goes away once dimensions, metrics, grouping or timezone are edited, since it is
then a different report. Per browser — presets themselves stay org-wide.

## [Between phases] — "День" dimension and analytics fixes

New "День" dimension in the report builder: per-day rows combined with any
other dimension (Кампания + День, Источник + День, ...), per-day cost for
campaign/source, sortable, saveable in presets. Timezone is now actually
applied (it was a select that did nothing): days and the From/To window are cut
in the chosen zone for report, timeseries and funnel. The funnel honours the
report filters (it ignored them). Fixed: the "Линия" chart drew nothing when the
window had data on a single day (no symbol for a lone point; axis is now by
day); the dimension picker and filter picker offered unbacked "Постлендинг"/
"Сеть" (400 → "Не удалось загрузить отчёт"); the networks list's "View
statistics" link led to a guaranteed 400 and is removed until a network
dimension exists; the filter-value picker offered made-up mock values while
loading. Tests: day grouping / sort / limit / timezone / funnel filters in
`analytics`, timezone day cut and funnel filters on real ClickHouse in `chstore`,
per-day spend in `cost`, `day` in presets.

## [Between phases] — `source` is the real traffic source

The `source` dimension no longer proxies campaign: `traffic_source_id`
(ClickHouse migration `011_traffic_source_id.sql`, 3 tables) is stamped by the
tracker from the campaign at click time, carried through the signed link
claims to LAND_CLICK/PWA_*, and onto conversions through attribution. Two
campaigns on one source now merge into one row. Cost for `[source]` is the sum
of the source's campaigns (`cost.SpendBySource`). The UI resolves source ids
directly (with a "No source" bucket) and the sources list's "View statistics"
link filters on the id. Pre-011 rows are `''` until backfilled — recipe in
`docs/analytics-report-api.md`, already run on the dev ClickHouse. Tests:
`TestReportBySourceMergesCampaignsOnOneSource` (real ClickHouse),
`TestSpendBySourceRollsCampaignsUpToTheirSource`,
`TestReportCostBySourceSumsTheSourcesCampaigns`,
`TestTrafficSourceIDOnEveryLandingEvent`, plus attribution/conversion carry-over.

## [Between phases] — conversion metrics under os/browser/language

Reports grouped or filtered by os, browser or language now show real
conversions, revenue, profit, ROI, EPC and CPA: `ReportConversions` joins
`click_events` onto each conversion via `click_id` (new
`reportConversionsJoined`; the click side is not bounded by the report window,
tenant-scoped on both sides, conversions without a click land in the `""`
bucket). Before this, an os/browser/language filter 500'd the report and
timeseries ("unsupported filter dimension in conversion report") and a grouping
showed "—". Tests: `TestReportConversionsGroupsByOSViaClickJoin`,
`...FiltersByBrowserViaClickJoin`, `...ViaClickJoinByDayAndTenantIsolation`
(real ClickHouse), `TestReportQueriesConversionsForClickOnlyDimension`.
Also in the analytics UI this stretch: dimension picker capped at the API's 4,
campaign/source names instead of ids, column hints + drag-reorder + width fix,
media-buyer default preset (migration 00043), preset "today" no longer pinned
to 2026-08-11.

## [Between phases] — PWA comments block; save/cancel moved to the top

Constructor form: "Отмена"/"Сохранить" now sit in the top card header next to
the status buttons. New "Комментарии" block on /pwa/new|[id]: sets by geo +
language, hand-entered comments (author, 1-5 rating, text), select/delete,
CSV import/export (client-side, `geo,lang,author,rating,text`). No generator
by design. The list is part of the form state and saved with it:
`pwa_comments` (migration 00041, ON DELETE CASCADE), `GET|PUT
/pwas/{id}/comments` (replace-all in one tx, validated, org-scoped), CORS now
allows PUT. The editor preview shows the active set and takes rating, count
and star shares from it. The tracker renders comments only on the
constructor's signed test links (`LoadPwaComments`, set picked by page
language), never to real visitors. Tests: `TestCommentsReplaceRoundTripAndValidation`,
`TestCommentsTenantIsolation`, `TestPwaPageComments`; full-env go test, tsc,
eslint clean; migrated, block checked live on /pwa/new (set add). Known:
Duplicate does not copy comments; add-comment/CSV/save flow not clicked
through in the browser.

## [Between phases] — PWA comment sets: country and language dropdowns

(Later widened from the top 20 to 134 countries — West/Central Africa incl.
Mali, Burkina Faso, Senegal, Côte d'Ivoire; Latin America; Europe; Asia;
Middle East — sorted by localized name, with a search box that matches the
localized name, the code and the English name.)

Adding a comment set now uses two dropdowns instead of free text: top-20
countries (US, IN, BR, ID, RU, MX, JP, DE, GB, FR, TR, IT, ES, PH, VN, TH,
PL, CA, EG, NG), each with its common languages; picking a country preselects
the first language. Names come from `Intl.DisplayNames` in the UI language.
Other pairs still work through CSV import (the API accepts any two-letter
codes). tsc, eslint clean; checked live on /pwa/new.

## [Between phases] — fix: screenshot slider (preview and install page)

The editor preview's Embla carousel did not react to mouse drag, and the
install page's strip had a hidden scrollbar so a mouse could not scroll it at
all. Both are now a native horizontal scroll (touch, trackpad, shift+wheel)
with click-and-drag for the mouse (pointer capture, grab cursor, native image
drag disabled). tsc, eslint, `go test ./tracker` clean; drag verified live in
the editor preview. The tracker page itself was not opened live (needs a
signed test link). `embla-carousel-react` is no longer imported (still listed
in package.json).

## [Between phases] — comments modal: CSV format hint and preview caption removed

The "CSV: geo,lang,…" line and the "Превью в реальном времени" caption are gone
from the modal (the preview itself stays). tsc, eslint clean.

## [Between phases] — live phone preview while editing comments

The comments modal gets a third column with the phone preview (the form's own
`PwaPreviewCard`, via a shared `renderPreview`), scrolled to the ratings and
reviews block (`focusReviews`) and showing the active set. The new/edit
comment dialog has the same preview with the draft included, updating as you
type; rating, count and star bars come from the comments shown. tsc, eslint
clean; checked live (typed a draft, it appeared in the phone with 5.0).

## [Between phases] — pointer cursor on buttons (app-wide)

Tailwind's preflight leaves buttons with the arrow cursor. One base-layer rule
in `globals.css` gives enabled buttons and button-like roles (combobox,
checkbox, radio, switch, tab, menuitem, option) `cursor: pointer`, disabled
ones `not-allowed`. Checked live: all 22 buttons on /pwa/new compute to
pointer. tsc, eslint clean.

## [Between phases] — PWA screenshots: Upload opens the gallery picker

In the "Скриншоты" block the "Upload" tile now opens the Content Gallery
picker (which also uploads new files) instead of uploading straight away; the
separate "Выбрать из галереи" button is gone. tsc, eslint clean; not checked in
the browser (the browser tool refused to open the page).

## [Between phases] — PWA form: bottom comments card removed

The summary "Комментарии" card at the bottom of the form is gone; the comments
modal is reachable only from the header button. tsc, eslint clean.

## [Between phases] — PWA form: manifest preview removed, comments button in the header

The "Превью манифеста" JSON block is gone from the form. "Управлять
комментариями" now sits in the top card header next to Отмена / Сохранить and
opens the comments modal (its open state lives in the form; the summary card
below stays). tsc, eslint clean; header button opens the modal live.

## [Between phases] — PWA comment form: avatar, likes, date, developer reply

"Новый комментарий" dialog (also used to edit): username, avatar from the
Content Gallery, 5-star rating, likes, comment date, text (500) and an
optional developer reply (500). `pwa_comments` gained avatar_url, likes,
comment_date, reply (migration 00042); validation in the service; the editor
preview and the tracker's test-link page show them; CSV is now
`geo,lang,author,rating,text,likes,date,reply,avatar`. Deliberately NOT
built: the reference's "actual date spread randomly over the last 5 days" and
"date with offset" modes — a comment date is a specific day the operator
types, or empty (none shown). Tests: round-trip and validation of the new
fields; full-env go test, tsc, eslint clean; dialog opened live.

## [Between phases] — PWA comments editor in a large modal

The "Комментарии" card in the PWA form now shows a summary of the sets with
counts and a "Управлять комментариями" button; sets, adding, selection,
deletion and CSV live in a large modal (two columns: sets left, the selected
set's comments right). The list stays in the form state, so edits reach the
PWA on the form's own Save. tsc, eslint clean; modal opened live on /pwa/new
(adding a comment inside the modal not clicked through).

## [Between phases] — PWA bottom bar toggle

Constructor switch "Нижняя панель меню" hides the bottom bar with the menu
buttons on the install page (and the padding reserved for it) and in the
preview. `pwas.show_bottom_nav` (migration 00040, default true) through
create/update/duplicate, `PwaPage`, form and preview card. Tests:
`TestRenderPwaPageShowBottomNav`; full-env go test, tsc, eslint clean;
toggle checked live on /pwa/new.

## [Between phases] — PWA header title and logo

With "Шапка страницы" on, the constructor shows a logo picker (Content
Gallery) and a title field (max 40); the top bar on the install page and in
the preview uses them, falling back to the default mark and "Apps".
`pwas.header_title` / `header_logo_url` (migration 00039, default '') through
create/update/duplicate with validation (`validateHeaderBrand`), `PwaPage`,
form and preview. Tests: `TestRenderPwaPageHeaderBrand`; full-env go test,
tsc, eslint clean; fields checked live on /pwa/new.

## [Between phases] — PWA header toggle

Constructor switch "Шапка страницы" hides the top bar (brand mark, "Apps",
search, help) on the install page and in the preview. `pwas.show_header`
(migration 00038, default true) through create/update/duplicate, `PwaPage`,
the form and the preview card. Tests: `TestRenderPwaPageShowHeader`;
full-env go test, tsc and eslint clean; toggle checked live on /pwa/new.

## [Between phases] — PWA auto theme

New "Авто (по устройству)" option in the constructor's Тема dropdown: the
install page follows the visitor's prefers-color-scheme. `pwas.theme_auto`
(migration 00037, default false — existing PWAs unchanged) through
create/update/duplicate, `PwaPage`, the form and the preview card.
Tests: `TestRenderPwaPageThemeModes`; full-env go test, tsc and eslint clean.
Not added, by decision: a reviews block with operator-typed reviewers.

## [Between phases] — One store-style PWA page for visitors and test links

By the operator's decision `templates/pwa.html` is now the full editor card
for everyone; `pwa_store.html` and the test-only branch are gone. A bad edit
of `pwa.html` (missing `stars`/`fill` funcs) had crashed the tracker at init.

- `pwaPageTemplate` carries the `stars`/`fill` funcs; copy and details rows
  are built once for every render.
- Tests: `TestRenderPwaPageAllFormFields` (every form value, visitor + test),
  `TestPwaStageDirect` now expects the numbers. Full-env go test green;
  tracker container back up.

## [Between phases] — Store-style PWA page for constructor test links

Signed test links now render the full editor card (`templates/pwa_store.html`);
visitors still get the stats-free page. Detail: `docs/pwa-serving.md`.

- `PwaPage`/`LoadPwaPage` also load rating, rating_count, downloads_label,
  size_mb, age_rating and the star distribution.
- `renderPwaPage` returns an error; a failing template redirects the visitor
  to the destination. Header label is "Apps", matching the editor.
- Tests: `TestRenderPwaStoreTemplateOnlyForTest` (numbers only on test links).

## [Between phases] — PWA install page laid out like the constructor card

The visitor page at `/pwa/{token}/` now follows the editor preview's layout
and install animation, minus everything numeric. Detail: `docs/pwa-serving.md`.

- Header block, accent/theme-driven colors, light/dark from the PWA's
  background color, screenshot strip, About + details sheet (`<dialog>`),
  Updated on, What's new; ring-around-the-icon install sequence.
- `routingstore.PwaPage` + `LoadPwaPage` gain whats_new, version, category,
  interactive_elements, contains_ads, in_app_purchases, updated_date.
- Not shown, by decision: rating/stars/review count/downloads/size/age
  rating/verified badges and the store logo/nav/footer chrome (see doc).
- Tests: `TestRenderPwaPageNoListingStats` (no DB), `TestPwaStageDirect`
  extended (fields present, listing numbers absent). Full go test with all
  deps green; checked live in Chrome (installing state, details sheet, dark).

## [Between phases] — Tracker serves the PWA stage (+ `pwa` report dimension, constructor test page)

A flow's PWA stage now reaches real visitors. Full detail:
`docs/pwa-serving.md`.

- `apps/tracker` `/pwa/{token}/`: installable page (manifest, pass-through
  service worker, `beforeinstallprompt` install button, iOS "Add to Home
  Screen" steps, "continue without installing"), reached straight from
  the click or after an internal landing's click-through. Events:
  PWA_VIEW, PWA_INSTALL (`appinstalled` beacon), PWA_OPEN (the installed
  app's start_url → destination). One signed token binds page, manifest,
  SW scope and start_url to the click_id; manifest/open accept tokens past
  the 30-day max age (`landinglink.VerifyAnyAge`) so installed apps keep
  working.
- In-app WebView bounce (§28/§73) when the PWA's flag is on: Android
  `intent://`, iOS `x-safari-https://`; generic UA self-identification
  only, never visitor detection.
- The page is the operator's app as itself — name, icon, developer,
  description, screenshots, colors. It does **not** imitate an app store
  and never shows the editor's store-listing stats (ratings, reviews,
  downloads) to visitors; the editor's store-styled preview card stays an
  admin-only preview. The user asked for the Google Play imitation to be
  served; declined (deceptive to the people installing), neutral page
  built instead.
- PWA constructor: **Test page** button — `POST /pwas/{id}/test-link`
  mints a signed `Test` link; the tracker serves the real page but records
  nothing. New config `TRACKER_PUBLIC_URL` (+ `LANDING_LINK_KEY` on api).
- `pwa` report dimension: ClickHouse `schema/010_pwa_id.sql`,
  `event.PwaID` through tracker/attribution/conversions, `chstore.DimPwa`
  (15 backed), names + "No PWA" in the UI, PWA list "View statistics".
- Skipped stages (302 to destination): types external/ios_app (no URL in
  schema), paused PWA, PWA behind an external landing.
- Verified: full `go test ./...` with all deps + e2e; benchmarks
  unchanged; live in Chrome (installable — Chrome fired
  beforeinstallprompt; test mode recorded zero events) and real traffic
  through the tracker (ClickHouse SOURCE_CLICK 5 / PWA_VIEW 4 /
  PWA_INSTALL 2 / PWA_OPEN 2, all with pwa_id; WebView bounce recorded no
  view). Postlanding serving not in this phase.

## [Between phases] — `offer` report dimension (pwa/postlanding deliberately not)

Asked for pwa, postlanding and offer dimensions; shipped offer. pwa and
postlanding have no honest data source yet: `apps/tracker` never serves
the PWA or postlanding funnel stages (no PWA_VIEW/POSTLANDING_VIEW is
emitted anywhere), so attributing a click to a flow's configured-but-never-
shown PWA would be fabricated data (CLAUDE.md NEVER list). They need those
stages served first.

- Routing: `routing.Destination.OfferID` (loaded from
  `flows.destination_offer_id`) and `RouteResult.OfferID`, set only on the
  engine's "Offer" branch — never for redirects or an inactive offer's
  fallback. Fresh + sticky covered (`routing/offer_id_test.go`).
- ClickHouse `schema/009_offer_id.sql`: `offer_id` on click/tracking/
  conversion events, after 008's `landing_id`.
- Tracker stamps it on SOURCE_CLICK/LAND_VIEW and carries it in the signed
  landing link to LAND_CLICK; attribution copies it onto conversions.
- `chstore.DimOffer` (14 backed dimensions; pwa/postlanding/network left).
- UI: offer names instead of ids, "" as "No offer"; the offers list's
  "View statistics" now passes the offer **id** (it passed the name, which
  would have matched nothing).
- Tests that used `offer` as their "unbacked dimension" example moved to
  `pwa` (analytics, chstore, reportpresets).
- Verified live: a flow landing→real offer and a redirect flow, real
  tracker traffic; ClickHouse rows and the browser report by Оффер+Лендинг
  matched exactly (9 / 12 "Без оффера · Без лендинга"); deep link from the
  offers list filtered correctly. Fixtures removed.

## [Between phases] — `landing` report dimension

The first of the report builder's five unbacked dimensions gets a real
column. Detail: `docs/analytics-report-api.md`, "Dimension whitelist".

- ClickHouse `schema/008_landing_id.sql`: `landing_id` on `click_events`,
  `tracking_events`, `conversion_events` (metadata-only ADD COLUMN,
  appended last to match `EventStore`'s positional inserts).
- `event.Event.LandingID`: set by `apps/tracker` on SOURCE_CLICK/LAND_VIEW
  (the landing actually shown — internal or external; empty when skipped)
  and LAND_CLICK (from the signed link); copied onto conversions via
  `attribution.Click.LandingID`. Revenue/CVR/everything works by landing.
- `chstore.DimLanding` in both report whitelists (13 backed dimensions;
  4 left: pwa/postlanding/offer/network). Report presets accept it too.
- Frontend: `useDimensionValueLabel` shows landing names instead of ULIDs
  in the report table (incl. search/CSV), bar chart, filter dropdown and
  chip, `""` as "No landing"; landings list gets a "View statistics"
  action.
- Running `go test ./...` with ClickHouse configured (phase B's run
  hadn't) surfaced two stale tests, both fixed: the e2e full-funnel
  scenario still expected a click on a flow with an enabled external
  landing to skip it (since phase B it goes to the landing first, per
  §25 — the test now asserts that, plus the click's `landing_id`), and a
  report-preset test used `landing` as its "unbacked dimension" example
  (now `offer`). `click_log_test`'s raw positional insert got the new
  column.

## [Between phases] — Tracker serves landings to visitors (ZIP landings phase B of 2)

Closed the loop phase A left open: a flow's landing stage now actually
does something for a real visitor. Full detail: `docs/landings.md`,
"Serving visitors".

- `apps/tracker` `/t/{slug}`: an enabled internal landing (active or
  archived) is served as the response itself — `landing.Render` with a
  `<base href>` to the bundle and a signed `trackUrl` — recording
  `SOURCE_CLICK` + `LAND_VIEW`. Paused → skipped; external → 302 to its
  URL; page-load failure → logged, 302 to the destination.
- New `GET /lc/{token}`: verifies the HMAC-signed click link
  (`apps/internal/landinglink`), records `LAND_CLICK` on the same click_id
  with the original utm/subs, 302 to the routed destination. No DB access.
  Bad/forged/expired tokens → 404, nothing recorded (no open redirect).
- `routingstore` loads each flow's landing stage with the flows (org-
  scoped join) and reads page source by PK only when serving.
- New config `LANDING_LINK_KEY` (unset → internal landings skipped, warned
  at startup); tracker now also gets `S3_*` endpoint/bucket for asset URLs
  (dev/test compose, `.env.example`).
- First time `LAND_VIEW`/`LAND_CLICK` are emitted by anything — the
  analytics funnel's landing stages now have real data.
- Benchmarks: serving a landing ≈0.49 ms (same as a plain redirect),
  click-through ≈0.01 ms. Also fixed `BenchmarkTrack`/`_Parallel`
  panicking on a nil `clientIP` (broken since the trusted-proxy change).

## [Between phases] — Landings become uploaded ZIP bundles (phase A of 2, reference: adset.pro "custom landings")

Brought the Landings module up to the reference product's custom-landing
feature set, management side. Full detail: `docs/landings.md`.

- **ZIP upload** (`POST /landings/upload`): `index.html`/`index.twig` at
  the root (or under one top-level folder, stripped), zip-slip/zip-bomb/
  size/file-count guards, OS junk dropped. Entry file → Postgres (what the
  editor edits); assets → object storage under a per-upload version
  prefix. Root-absolute asset paths come back as warnings.
- **Re-upload keeps the ID** (`POST /landings/{id}/archive`): new version
  uploaded first, row switched in one UPDATE, old version deleted.
- **Logo + title** fields (required for internal, optional for external;
  logo via the Content Gallery 1:1 crop picker). Status can now be set on
  create (the form already showed it; the API used to ignore it).
- **Template editor**: full-screen CodeMirror 6 (lazy-loaded) + live
  preview from `POST /landings/{id}/preview`, rendered by the same Go
  `Render` the tracker will use; sandboxed iframe without
  `allow-same-origin`.
- **Injected runtime** (`render.go`): global `trackUrl`,
  `engine_onClickButton()`, every `<a>` bound (incl. framework-rendered
  ones via delegation), `data-flox-skip` opt-out, `{{ trackUrl }}` macro,
  `<base href>` to the bundle.
- Duplicate copies the bundle server-side; delete cleans every version.
- `type` is now immutable after creation. The fake slug URL
  (`cdn.floxlink.io/lnd/…`, served by nothing) is gone; an internal
  landing's `url` is its real asset base, derived at read time.
- New dependency: `@uiw/react-codemirror` + `@codemirror/lang-html` +
  `@codemirror/theme-one-dark` (template editor only).
- Migration `00036_landing_bundles.sql` (up/down/up verified).

Not in this phase (phase B): the tracker serving landings to visitors and
emitting `LAND_VIEW`/`LAND_CLICK`. Out of scope: full Twig, and the
push-subscription/PWA-install variants of `engine_onClickButton`.

## [Between phases] — DataTable gets real column resizing and CSV export (§64 UX floor)

Closed the last of §64's own explicit table requirements that hadn't
shipped yet: `components/ui/data-table.tsx` — the one shared table every
list page in the app builds on — had sort, paginate, search, and column
visibility, but no resizing and no export.

Column resizing: registered TanStack Table's `columnSizingFeature` +
`columnResizingFeature`, with a resize handle on every sortable column's
right edge. Deliberately opt-in per column rather than switching the
whole table to fixed-width layout: a column only ever gets an explicit
pixel width once its own entry exists in `columnSizing` state (i.e.
someone actually dragged its handle) — every other column keeps
rendering at its natural, auto-layout width exactly as before this
phase, so none of the 22 existing tables across the app changed
visually by default. A resized column also gets `overflow-hidden
text-ellipsis whitespace-nowrap`, so narrowing one actually does
something instead of the browser just re-expanding it around long
content.

CSV export: a new toolbar button exports every row matching the current
search/sort — not just the rows on screen (`table.getSortedRowModel()`,
post-filter-and-sort, pre-pagination) — using each visible column's
accessor value (`row.getValue()`), the same mechanism the existing
search box already filters with. A column with no accessor (a pure
computed/joined `cell` renderer, no `accessorKey`/`accessorFn`) was
already unsearchable for that same reason and exports empty here too — a
pre-existing, consistent limitation, not a new one. Columns with no real
string header (the row-actions column's icon buttons, conventionally
`header: ""`) are skipped outright rather than exported as an unlabeled
empty column. RFC-4180-ish quoting handles embedded commas/quotes/
newlines.

Added `exportFilename` to `DataTable`'s props and set a real one at all
22 existing call sites (offers, networks, campaigns, traffic sources,
domains, landings, pwa, postlanding, pixels, conversions, click log,
custom metrics, team members, tracking links, cost entries, three
referral tables, postback logs, the dashboard's per-entity top tables,
and the analytics report table) — every existing call site keeps
working unchanged when the prop is omitted (falls back to a generic
name), matching this component's existing "every new option is opt-in"
convention.

Verified: `tsc --noEmit`/`eslint`/`vitest` clean on `apps/web` (this
phase touched no Go code). Real browser pass against a real org with
real networks: dragged a column wider (every other column shifted
naturally, no layout break), dragged one drastically narrower (content
correctly truncated with an ellipsis instead of breaking the row),
exported CSV and confirmed the downloaded file's real content matched
exactly what was on screen, re-exported after typing into the search box
and confirmed the file narrowed to just the matching row (proving the
"all filtered rows, not just the current page" contract), and confirmed
a second page (Team, whose Role column renders as an interactive
`<select>`) exported correctly too. No console errors on either page.
All test fixtures removed afterward.

## [Between phases] — report builder's filter-value dropdown suggests real distinct values, not a fixed mock pool

Closed a gap this same doc's own Phase 35 entry (and `docs/analytics-
report-api.md`) had flagged since the real `/analytics` backend first
landed: the report builder's filter popover picked its value suggestions
from `lib/mock/analytics.ts`'s hand-written `POOLS`, the one piece of
that screen still backed by mock data.

Added `chstore.EventStore.DistinctValues` — real distinct values for one
dimension from `click_events`, ordered by `count() DESC` (most-common
first) and capped at 50 (`analytics.maxDimensionValues`), excluding the
`flow`/`stream_set` empty-string bucket deliberately (useful to filter
on, confusing to suggest as an unlabeled dropdown entry). Gated by the
same `chstore.ValidDimension` whitelist every other report endpoint
already uses, so the 5 still-unbacked dimensions (`landing`/`pwa`/
`postlanding`/`offer`/`network`) 400 the same way `/analytics/report`
already does for one. New `Service.DimensionValues` and `POST /analytics/
dimension-values` follow the exact same shape as `Report`/`Breakdown`/
etc.

Frontend: `useDimensionValues` (`retry: false` — a 400 here is
deterministic) backs `AddFilterPopover` (`report-controls.tsx`), which
falls back to the mock pool both as the query's brief loading-state
placeholder and as the only source for the 5 unbacked dimensions. A
successful response — even an empty one, for a brand-new organization
with no real events yet — wins outright once it lands, matching CLAUDE.md
#6's "no fabricated data" bar. Rewrote the popover's selection state to
derive the effective value during render (`manualValue` + a fallback to
`values[0]`) rather than sync it via a `useEffect`, after React's own
lint rules flagged the original `useEffect`-calls-`setState` version as
the cascading-render anti-pattern it actually was.

Caught and fixed live, via browser verification, not just in review: the
first implementation of `Service.DimensionValues` forgot the `+24h`
adjustment `groupedReport` already applies centrally to make a caller's
inclusive "to" date match `chstore.ReportQuery.To`'s exclusive contract
— every other endpoint gets this via `groupedReport`, but
`DimensionValues` calls the repository directly. Silently excluded
"today" whenever `to` was today's date, exactly the range a fresh
`/analytics` page load defaults to; a real browser pass surfaced it
immediately (the value dropdown showed "no values yet" for a dimension
with real events from minutes earlier), fixed, and covered by a new
regression test asserting the exact boundary passed to the repository.

Verified: `gofmt`/`go vet`/`go build`/`go test -count=1 ./...` clean
across all Go packages against live Postgres/ClickHouse/Redis/MinIO,
including new tests in both `chstore` (live ClickHouse — frequency
ordering, empty-bucket exclusion, the limit cap, the unsupported-
dimension rejection) and `analytics` (pass-through, the unbacked-
dimension rejection, the `+24h` boundary regression above).
`tsc --noEmit`/`eslint`/`vitest` clean on `apps/web`. Real browser pass:
created a test org, generated real clicks through `apps/tracker` with
two distinct User-Agent strings (mobile/desktop — device/platform
classification needs no GeoIP, unlike country/city, which stayed
genuinely empty in this dev environment and correctly showed "No values
yet" with the picker disabled rather than fabricated country names),
confirmed the filter popover's value dropdown listed exactly the two
real device values, and confirmed selecting one and adding the filter
correctly narrowed the report's click count. No console errors. All
test fixtures (org, user, campaign, traffic source, domain, tracking
link, ClickHouse click events) removed afterward.

## [Between phases] — key the general rate limit by organization for API-key traffic, not client IP

The general rate limiter (300 req/min, `apps/internal/httpserver`) has
been purely IP-keyed since it was built — flagged as a known limitation
the moment API-key authentication became real (finding #4): a
high-volume legitimate integration shares one bucket with every other
visitor behind the same address (shared hosting, corporate NAT, ...),
while the limiter has no way to tell them apart, since auth had never
resolved yet at the point it runs.

Added `tenant.PeekIdentity` — the same session/API-key resolution
`tenant.NewMiddleware` already does, but never rejecting the request on
a missing or invalid credential (an absent/bad credential just leaves
the request unidentified rather than 401ing, since this runs ahead of
genuinely public routes too). Mounted once, globally, in
`apps/internal/httpserver.New`, immediately before the general limiter —
the limiter's key function (`apps/api/main.go`) reads the identity it
resolved: `"org:" + orgID` for API-key-authenticated traffic, `"ip:" +
clientIP` for everything else (unauthenticated requests, and
session-cookie browser traffic, which isn't the "many unrelated
requests behind one address" case this fixes).

To avoid paying for two Postgres/session round trips per authenticated
request, `tenant.NewMiddleware` (the real, per-route-group *enforcing*
check) now short-circuits when `PeekIdentity` already resolved identity
into context — a request it couldn't identify still falls through to
NewMiddleware's own resolution and 401 exactly as before. `httpserver.
New`'s signature changed (`clientIP *clientip.Resolver` replaced by a
pre-built `generalLimitKey`/`identify` pair, both built in `main.go`,
which already imports `tenant` — `httpserver` itself still doesn't) and
its one call site moved down in `main.go`, to after `tenantMiddleware`
exists.

Added `TestPeekIdentityResolvesAPIKey`/`ResolvesSessionCookie`/
`NeverRejects`, `TestNewMiddlewareSkipsReresolutionWhenAlreadyIdentified`/
`StillEnforcesWhenNotPreIdentified` (`apps/internal/tenant`), and two new
`apps/internal/httpserver` integration tests against live Redis
confirming the actual Redis key an API-key request produces is
org-scoped and an unauthenticated one is still IP-scoped. Updated
`docs/security.md` §3 and its "Known limitations" list (struck through,
matching the existing convention for closed items).

Verified: `gofmt`/`go vet`/`go build`/`go test -count=1 ./...` clean
across all Go packages against live Postgres/ClickHouse/Redis/MinIO,
including the new tests — `apps/api`'s own `rbac_coverage_test.go`
confirms the `main.go` reordering didn't drop any route's RBAC gate.
Live verification against the real containerized dev stack: signed up
a real test org, created a real API key, hit a real endpoint with it,
and confirmed `ratelimit:api:org:<orgID>` was the Redis key actually
created (not an IP-keyed one) — then confirmed a session-cookie request
from the same org still produced the original `ratelimit:api:ip:<addr>`
key, unaffected. A full browser pass (login, `/overview`, `/team`,
`/settings`, all RBAC-gated) showed no regression and no console
errors. All test fixtures (org, user, API key, Redis keys) removed
afterward.

## [Between phases] — encrypt facebook_ads/tiktok_ads' access_token at rest, closing the last plain-text credential gap

`ad_account_connections.access_token` (facebook_ads/tiktok_ads) was the
one credential column in the codebase still stored in plain text —
migration 00018 predated any encryption infrastructure, and Google
Ads' own developer_token/client_secret/refresh_token (migration 00034)
deliberately left this column as a flagged, real follow-up rather than
silently retrofitting it in an unrelated phase. Four AES-256-GCM
implementations already existed to copy the pattern from (TOTP,
integrations, adaccount's own Google Ads fields) by the time this
session picked it as the smallest, clearest remaining security win.

Retrofitted `access_token` onto the exact same `apps/internal/adaccount/
crypto.go`/`ADACCOUNT_ENCRYPTION_KEY` machinery Google Ads' three
secrets already use. No schema migration: the column was already
`text` and ciphertext is text too — only its *format* changed.
`repository.go`'s `selectColumns` dropped the `right(access_token, 4)`
SQL-level slice that only worked against plaintext; `scanConnection`
now decrypts whichever secret is present (`access_token` or
`refresh_token`) in Go to compute `TokenPreview`, mirroring how
`refresh_token`'s own preview already worked. `Connect` encrypts via
`encryptOptional`; `CredentialsByTrafficSourceID` decrypts via
`decryptOptional` — the same two helpers the other three fields
already used. No API/wire contract change: `Connection` never had an
`AccessToken` field (write-only from Phase A), and the frontend's
`Connect` request shape is unaffected.

Added `TestAccessTokenRoundTripEncryption`, mirroring the existing
`TestGoogleAdsCredentialsRoundTripEncryption`: connects a real
plaintext token, reads the raw `access_token` column directly via SQL
and asserts it is genuinely ciphertext, confirms
`CredentialsByTrafficSourceID` decrypts it back to the exact original,
and confirms `TokenPreview` still shows the real last-4 characters.
The pre-existing `TestConnectGetDisconnect` needed no changes — its
`TokenPreview` assertion passes unchanged, since the preview's value
is unaffected by where in the stack it gets sliced from.

Updated `docs/ad-account-connections.md` (added a "Phase D" section,
left Phase A/Google Ads' own historical sections intact with forward
pointers rather than rewritten) and `docs/settings-integrations.md`'s
comparison line, both of which explicitly documented this as a known,
flagged gap.

Verified: `gofmt`/`go vet`/`go build`/`go test -count=1 ./...` clean
across all Go packages against live Postgres/ClickHouse/Redis/MinIO,
including the new test. No frontend changes were needed, so no
`tsc`/`eslint`/`vitest`/browser pass was required for this phase.

## [Between phases] — add the stream_set analytics dimension, closing the "no stream_set_id on events" roadmap gap

A prior research pass (this same session) traced the full event lifecycle
and found the roadmap gap stale: `stream_set_id` has existed on
`click_events`/`conversion_events` and been populated by `apps/tracker`
on every click since Phase 21 (`conversion.go` copies it onto conversion
events from the resolved click). The real gap was narrower — `chstore`'s
report-dimension whitelist and the frontend's dimension registry never
picked it up — so this phase is query-layer/UI only: no schema migration,
no backfill, no tracker/conversion changes.

Added `chstore.DimStreamSet` (`"stream_set"` → `stream_set_id`) to both
`reportClickColumns` and `reportConversionColumns`, making it the 12th
backed `ReportDimension` (11 of the original 16 registry-named
dimensions, plus this one addition). Added `"stream_set"` to the
frontend's `DimensionKey` union and `DIMENSIONS` registry, its i18n
labels (en/ru), and the `VIEW_STATS_DIMENSIONS` allowlist that gates
`?dim=&val=` deep-linking. Empty string is preserved as its own real
bucket — a campaign-fallback click, a blocked (`SOURCE_FILTER`) click,
and an unattributed conversion all carry `stream_set_id = ""`, the same
"real group, not missing data" semantics `flow_id` already has.

Stream Sets rows get the same "View statistics" drill-in
`network-row-actions.tsx`/`offer-row-actions.tsx`/`source-row-actions.tsx`
already use, but with one deliberate deviation: those three pass the
entity's display `.name` (a pre-existing, already-decorative pattern —
network/offer have no backing dimension at all, and source's link
actually filters on `campaign_id`, not a name column). `stream_set` is a
genuinely working dimension, so `stream-set-row.tsx` passes the set's
real `.id` instead, matching what `stream_set_id` actually stores. The
tradeoff: since `FilterCondition` (`aggregate.ts`) has no separate
display-label field, the resulting filter chip on `/analytics` shows the
raw ULID rather than the set's name — disclosed here rather than quietly
shipped, and out of this phase's query-layer/UI-only scope to fix.

Extended `chstore/report_test.go`'s shared fixture with real
`StreamSetID` values (including an explicit empty-bucket case) and added
two new tests against a live ClickHouse container —
`TestReportClicksGroupsByStreamSet` and
`TestReportConversionsGroupsByStreamSetPreservesEmptyBucket` — proving
the grouping, including the `""` bucket, end to end. Updated
`docs/analytics-report-api.md`'s stale "11 of 16" dimension count and
added a `stream_set` section explaining the addition.

Verified: `gofmt`/`go vet`/`go build`/`go test -count=1 ./...` clean
across all Go packages against live Postgres/ClickHouse/Redis/MinIO;
`tsc --noEmit`/`eslint`/`vitest` clean on `apps/web`. Real browser pass:
signed up a test org, created a campaign with two real Stream Sets ("US
Mobile Set" filtered on `sub1`, "EU Desktop Set" as the catch-all),
generated real clicks against each through `apps/tracker` (4 vs 3), and
confirmed each set's "View statistics" link lands on `/analytics` with a
`stream_set` filter chip showing the exact, correct, distinct click
count for that set — no console errors. All test fixtures (org, user,
campaign, stream sets, network, offer, traffic source, domain, and the
ClickHouse click events) removed afterward.

## [Between phases] — bump golang.org/x/crypto to close 2 of 3 remaining advisories

Finding #6's Go 1.26.6 bump left 3 `golang.org/x/crypto` advisories in
place (`GO-2026-6355`, `GO-2026-6354`, `GO-2026-5932`), noted as
"dependency hygiene, not live" since `govulncheck` already confirmed
zero of them reachable from this code — only `internal/auth/crypto.go`'s
`bcrypt` import touches the module at all, a different subpackage from
the `ssh`/`openpgp` ones the advisories are about.

Checked each one directly rather than assuming: `GO-2026-6355` and
`GO-2026-6354` (both `x/crypto/ssh` DoS issues) are fixed in `v0.56.0`,
already published; `GO-2026-5932` (`x/crypto/openpgp` is unmaintained
and unsafe by design) has no fix — `Fixed in: N/A` — because the
upstream package itself is deprecated, not because of a missing
release. Bumped `golang.org/x/crypto` v0.55.0 -> v0.56.0
(`go get` + `go mod tidy`, a two-line go.mod/go.sum diff, no other
dependency changes) — the only actionable fix available; the third
advisory stays permanently flagged (upstream has abandoned `openpgp`),
harmless since this codebase never imports it.

Verified: `go build`/`vet`/`test -count=1 ./...` clean across all 41
packages against live Postgres/ClickHouse/Redis/MinIO; `gofmt` clean.
`govulncheck ./...` now reports 1 advisory (was 3) — `GO-2026-5932`
only, unreachable, no fix exists to apply.

## [Between phases] — MultiSelect gets real `disabled` support, closing domain-form.tsx's permission gap

The frontend permission-gating phase (finding-#1 follow-up) locked every
field in `domain-form.tsx` except one: `MultiSelect` (the Purpose field)
had no `disabled` prop at all, so it stayed fully interactively editable
— openable, toggleable — even when the rest of the form was locked for a
role without `settings.write`. The submit button staying disabled meant
no actual write was ever possible, so this was a UX gap, not a security
hole, and was called out honestly rather than silently left as-is when
the permission-gating phase shipped.

Fixed at the source: `components/ui/multi-select.tsx` (shared across 7
features — domains, offers, pixels, postlanding, stream-sets, analytics
report controls, stream-set filter conditions) gained a real `disabled`
prop, wired to the trigger `Button` (a disabled button never fires the
Popover's open) and defensively into `onOpenChange`/`toggle()` too. All
six other callers pass nothing and default to `false` — zero behavior
change for them. `domain-form.tsx` now passes `disabled={disabled}`
through; its own doc comment (which had explicitly named this exact gap)
updated to match.

Verified: `tsc`/`eslint`/`vitest` clean. Live browser pass: created a
real domain as Owner, invited and accepted a real Viewer, confirmed
clicking the Purpose field as Viewer opens nothing (previously it would
have) with zero console errors; logged back in as Owner on the same
domain and confirmed the dropdown still opens and toggles normally,
proving the six other callers' default (unchanged) behavior wasn't
affected either. Test org and fixtures deleted afterward via SQL.

## [Between phases] — offer-columns.tsx: close the accessorFn stale-cache gap the prior audit left open

The repo-wide `accessorFn` stale-cache audit (see that entry, below)
fixed six sites but explicitly left `features/offers/offer-columns.tsx`'s
"network" column alone, classifying it "near miss, safe": `offer-list.tsx`
gates its loading state on `offersQuery.isPending || networksQuery.isPending`
together, so the table never mounts with an empty `networkNameById` — the
"freezes on a raw id during initial load" failure mode the other six sites
had doesn't apply here. That audit did flag one real, narrower residual
risk and leave it unfixed: if a network is ever renamed while the Offers
page stays mounted, the cached `accessorFn` value would freeze on the
*old* name, not a raw id — accepted at the time since no co-mounted
rename flow existed.

Re-verified via a fresh audit request: the dual-loading-gate is still
present and still holds (confirmed by reading `offer-list.tsx:62` again,
not assumed), so the original "no raw-id-on-load" conclusion still
stands. Fixed anyway, for the same reason the other six sites already
were: `cell` removes the fragile invariant entirely rather than leaving
correctness dependent on that gate never changing, and it closes the
narrower rename-while-mounted gap the prior audit knowingly left open.
Same fix shape as the other six: `accessorFn` removed, `cell: ({ row })
=> networkNameById[row.original.networkId] ?? row.original.networkId`,
same doc comment cross-referencing `campaign-columns.tsx`.

New regression test (`offer-columns.test.ts`, the first test file for
this feature area): calls `offerColumns()` directly with different
`networkNameById` maps for the same row and asserts the network column
reflects whichever map was passed on the most recent call — proving the
mechanism (no accessorFn-style caching) rather than trying to reproduce
the narrow race live. Also asserts the column carries no `accessorFn` at
all, so the bug can't silently return.

Verified: `tsc --noEmit`/`eslint`/`vitest` (25/25, was 21) all clean.
Live browser pass against the real running dev stack: signed up a fresh
org, created a real network and a real offer through it, confirmed the
Offers list renders the resolved network name (not the raw id) on both
first load and a hard reload, zero console errors. Test org and its
fixtures deleted afterward via SQL.

## [Security] — real API-key authentication (audit finding #4, closed)

`apps/internal/apikey`'s `Create`/`List`/`Revoke` were real, hashed
credential bookkeeping, but nothing ever authenticated a request with one
— no `Authorization` middleware existed anywhere, and the stored
`read`/`write`/`admin` scope was read by zero handlers.

Confirmed via `AskUserQuestion`: a key authenticates against the exact
same route tree a session does (no separate "public API"), and the flat
scope maps onto the 18-permission vocabulary as coarse tiers rather than
inheriting the creator's own role.

### The fix

`tenant.NewMiddleware` — the single choke point every one of ~25
tenant-scoped route groups already shares — checks `Authorization: Bearer
flx_live_...` before falling back to the session cookie, calling a new
`apikey.Service.Authenticate` (hash lookup, `revoked_at IS NULL` check,
best-effort `last_used_at` stamp — previously a dead column). Zero
downstream handlers changed. `RequirePermission` branches on a new
`tenant.APIKeyScope`: present means skip the membership-table DB query
entirely and evaluate a pure string-suffix rule instead — `read` = every
`*.read` key; `write` = read plus every `*.write` key except
`team.write`/`settings.write`; `admin` = everything. `userID` on an
API-key request is the key's creator, used only for audit attribution —
never re-checked against `memberships`, so a key outlives its creator's
own membership by design (like a Stripe or GitHub key). CSRF's
`RequireSameOrigin` needed no change: it already treats an absent
`Origin` header as a non-browser client.

`features/settings/api-keys-panel.tsx`'s "not live yet" copy (from
finding #4's first pass) is reverted to accurate, with the create
dialog now spelling out what each tier actually grants and the
key-reveal dialog showing the `Authorization: Bearer <key>` header
format.

### Verified

`go build`/`vet`/`test -count=1 ./...` clean, 41 packages, live Postgres
— new tests cover `Authenticate` (valid/revoked/garbage keys, a real
`last_used_at` write) and `RequirePermission`'s scope-tier branch (6
cases against real, live-mounted `team.read`/`team.write` keys).
`tsc`/`eslint`/`vitest` clean on the frontend copy.

Full live pass against the real running API (rebuilt the dev Docker
image): signed up a real org, verified via mailhog, created three real
keys. `GET /campaigns`/`GET /team/members` 200 for all three scopes.
`POST /campaigns` 403 for read, a genuine 201 for write once given valid
fields. `POST /team/members/invite` and `PATCH /security` both 403 for
read AND write, 200s/passes-permission only for admin — confirming the
`team.write`/`settings.write` exclusion holds against a live request,
not just the unit test. A garbage Bearer token 401'd; a revoked key
401'd immediately after a real `DELETE /api-keys/{id}`. Test org and its
fixtures deleted afterward via SQL, FK order respected.

Known limitation, not addressed: the general rate limit stays IP-keyed,
so a high-volume legitimate integration shares one bucket with anything
else behind its IP — fixing that needs auth to resolve before the
general limiter runs, a bigger restructuring than this phase scoped.

## [Security] — permission-gate the frontend action buttons (finding #1 follow-up)

Finding #1's RBAC fix enforced `campaign.write`/`offer.write`/`source.write`/
`content.write`/`settings.write` on the API, but the frontend still showed
Create/Edit/Delete/Pause/Archive/Duplicate/Regenerate-secret buttons to every
role regardless of permission — a read-only role saw a "Create" button that
then 403'd on click.

### The fix

A new `usePermission(key)` hook (`hooks/use-auth.ts`, built on the `useMe()`
`permissions` array `/auth/me` already returns) and a new
`PermissionDeniedState` component (`components/ui/permission-denied-state.tsx`,
matching the existing `EmptyState`/`ErrorState` shape) are the two building
blocks; everything else follows the same convention `team/member-list.tsx`
already established (`!!me.data?.permissions.includes(key)`), applied
consistently for the first time across the other four write keys.

Across all five permission groups: list pages hide their "New X" button;
row-actions components hide Delete/Archive/Pause/Duplicate/Regenerate-secret
(Edit-links and read-only actions like Open/View-statistics/Verify stay
visible); shared `*-form.tsx` components (used by both create and edit) take
a `disabled` prop locking every field and the submit button; and — scoped
with the user before starting, since this doubles the file count — the ~14
route-level `new-*-view.tsx` create pages also guard against direct
navigation, rendering `PermissionDeniedState` instead of the form for a
role that can't write, closing the gap where hiding a list's button alone
doesn't stop someone from typing `/campaigns/new` directly.

`campaigns/` was built first as the reference implementation (it has the
one real subtlety: `campaign-form.tsx`/`tracking-links.tsx` already had an
`archived`-conditioned notice tied to a shared `disabled` prop, so a plain
`disabled = isArchived || !canWrite` would have shown "this campaign is
archived" to a non-archived campaign's read-only viewer — fixed by adding a
separate `archived` prop so the notice text stays accurate to the real
reason). The other four groups (offer.write, source.write, content.write,
settings.write) followed the same pattern; none of their forms had a
pre-existing `disabled`-tied notice to disambiguate, so they needed no
equivalent.

### Bug fixed along the way, not just a gap closed

`content-gallery/gallery-browser.tsx` had an existing role check —
`me?.role === "Owner" || "Admin"` — that (a) only gated the "Uploaded by"
filter dropdown, not any real mutation, and (b) used the wrong tier:
`content.write`'s actual seeded policy (migration `00035`) grants
Owner/Admin/**Manager** write. Replaced with `usePermission("content.write")`
and used to actually gate upload, archive, restore, and permanent-delete.
`gallery-folder-panel.tsx`'s create/rename/delete-folder controls had zero
check before this and now have one too.

### Tags handled once, not per host page

`content.write` also covers the tag system (`apps/internal/tags.Handler`:
create/update/delete/assignment all sit behind the same `requireWrite`).
Rather than gating the "Edit Tags" trigger button on every one of the ~10
different entity list pages that embed it, `tag-picker-popover.tsx` and
`bulk-tag-dialog.tsx` — the two shared components every host page's tag
mutations actually route through — are gated once, correctly covering
every call site with no per-page duplication.

### Verified

`tsc --noEmit`, `eslint`, and `vitest` (21/21) all clean across every
touched file. Live in a real browser against the running dev stack: signed
in as Owner (all buttons visible, e.g. campaigns' New button), invited and
accepted a real Viewer, and confirmed as that Viewer — New-campaign/offer/
network/pixel/landing buttons all hidden; a real traffic source's row
actions showed only Edit and View-statistics (Pause/Duplicate/Archive
gone); its detail form rendered with every field and the Save/Connect
buttons disabled; navigating directly to `/campaigns/new` rendered
`PermissionDeniedState` ("У вас нет прав для этого действия") instead of
the form, in the session's own Russian locale, confirming the i18n keys
resolve correctly. Test invite (`viewer-verify@example.com`) and its
fixtures deleted afterward via SQL (`audit_logs` first, FK order).

## [Security] — move shadcn's CLI to devDependencies (audit finding #7)

The audit found `shadcn` (the component-scaffolding CLI, invoked
manually via `npx shadcn add ...` when adding a new UI component) listed
in `apps/web/package.json`'s `dependencies`, pulling `express`, `qs`, and
`@modelcontextprotocol/sdk` into the production install tree for a tool
Next.js's runtime never imports.

Moved `shadcn` to `devDependencies` and regenerated `package-lock.json`
(`npm install`) so its transitive-only packages are correctly re-flagged
`dev`. Confirmed with a real `npm ci --omit=dev` in an isolated
directory: 147 packages installed (was 790 with dev deps included), and
`shadcn`, `express`, `qs`, and `@modelcontextprotocol/sdk` are absent
from `node_modules` entirely. `npx shadcn` from a project checkout is
unaffected — `npx` resolves devDependencies the same as dependencies.

Verified: `tsc --noEmit`, `eslint`, `vitest` (21/21), and `next build`
all clean.

## [Security] — bump Go to 1.26.6 (audit finding #6)

The audit's `govulncheck` pass found the pinned toolchain, Go 1.26.5,
carried 7 known stdlib vulnerabilities. `apps/go.mod`'s `go` directive is
now `1.26.6`; `GOTOOLCHAIN=auto` (the module's existing default) picked up
the new toolchain automatically on the next build, no other config
changes needed. `apps/api`, `apps/worker`, and `apps/tracker`'s
Dockerfiles already build on `golang:1.27-alpine`, ahead of either
version, so they were never affected and needed no change.

Verified: `go build`/`go vet` clean under the new toolchain; `go test
-count=1 ./...` clean across all 41 packages against live
Postgres/ClickHouse/Redis/MinIO; `govulncheck ./...` now reports zero
vulnerabilities reachable by this codebase (3 unrelated, pre-existing
`golang.org/x/crypto` advisories remain in transitively-required
dependencies that this code never calls — out of scope for this fix,
noted for later).

## [Security] — stop promising API keys authenticate calls (audit finding #4)

The audit found `apps/internal/apikey` implements real, hashed credential
bookkeeping (`Create`/`List`/`Revoke`, `api_keys.key_hash` unique-indexed)
but nothing in the tree ever authenticates a request with one — no
`Authorization`/`X-Api-Key` middleware exists anywhere, and the
`read`/`write`/`admin` scope stored per key is read by zero handlers. The
Settings > API keys panel nonetheless told users "Used for programmatic
access" and "Scope controls what this key can read or change" — a direct
hit against CLAUDE.md's own "no fake APIs that look real" rule.

Scoped with the user before touching anything: building real key
authentication is a substantial feature in its own right (a second auth
path mounted across the whole route tree, plus deciding how a flat
3-value scope maps onto the 18-permission RBAC model finding #1 just
built, plus rate limiting for key traffic) — not something to fold into
a security-audit pass. The user chose the cheap, honest fix: keep key
management working exactly as before (creation, one-time reveal,
revocation are all real and unchanged), but stop the UI from claiming it
does something it doesn't.

`apps/web/src/features/settings/api-keys-panel.tsx`: the card description,
an added inline `Alert`, the create dialog's scope description, and the
revoke confirmation now all say plainly that API authentication isn't
live yet and scope isn't enforced. No backend changes — `apikey.Service`
and its handler are untouched. `docs/security.md`'s Known limitations
gets the same explanation for anyone reading the code instead of the UI.
Real key authentication is deferred to its own future phase, scoped
properly when it's actually built.

Verified: `tsc --noEmit`, `eslint`, and `vitest` (21/21) all clean — no
existing test asserted the old copy strings.

## [Security] — trusted-proxy IP resolution & pending-login lockout (audit findings #2, #3)

Every client-IP read in the tree — `apps/internal/ratelimit`'s `ClientIP`,
`apps/tracker`'s own `clientIP` helper, and chi's `middleware.RealIP`
mounted in front of both — took `X-Forwarded-For`'s leftmost entry
unconditionally. That entry is exactly the one the client itself writes,
so all three were attacker-controlled: rotating the header defeated the
20-req/15-min auth rate limit outright, made the Settings > Security IP
allowlist decorative, and let a tracker visitor spoof the country/ASN
routing filters read and choose its own weighted-flow arm.

### The fix

`apps/internal/clientip.Resolver` (new package) only trusts
`X-Forwarded-For` from a peer address in `TRUSTED_PROXIES` — a new `.env`
variable, empty by default, which falls back to loopback + the RFC
1918/4193 private ranges. That default is a safe no-op for this
deployment (nothing sits in front of it yet): a public peer is never in
that set, so a spoofed header from the open internet is still ignored.
Once trusted, it walks the header right-to-left for the first
non-trusted entry — the header grows left to right as each proxy appends
its own upstream peer, so the rightmost untrusted entry is the closest
verifiable claim to the real client.

Every prior call site now goes through one `*clientip.Resolver`
constructed once per binary from `TRUSTED_PROXIES`: `apps/api/main.go`'s
auth and general rate limits, `apps/internal/auth`'s `LoginInput.ClientIP`
(feeds the IP allowlist) and email-verification audit trail, and
`apps/tracker`'s classifier input and `visitKey` fingerprint. `ClientIP`
is removed from `apps/internal/ratelimit` entirely rather than left
alongside the resolver as a second, insecure way to get the same value.
chi's own `middleware.RealIP` — deprecated upstream for the identical
unconditional-trust reason (GHSA-3fxj-6jh8-hvhx, GHSA-rjr7-jggh-pgcp,
GHSA-9g5q-2w5x-hmxf) — is no longer mounted in either binary.

Separately, `VerifyPendingLogin` used to leave a wrong TOTP/backup-code
attempt's pending row untouched, so its 5-minute TTL allowed unlimited
guesses against one token. A wrong code now deletes the pending row
immediately, forcing a fresh login — the same "one wrong attempt costs
the whole token" trade `VerifyEmail`/invite tokens already make
elsewhere in this codebase. `TestSelfServeTOTPEnrollmentAndLogin` is
updated to assert the new behavior instead of documenting the old one.

### Verified

`go build`/`go vet` clean; `go test -count=1 ./...` clean — 41 packages
(the new `clientip` package included), run with live Postgres/
ClickHouse/Redis/MinIO. `apps/internal/clientip` ships its own test file
(none existed before): untrusted-peer spoofing is ignored, a trusted
chain is walked right-to-left, an all-trusted chain falls back to the
leftmost entry, `TRUSTED_PROXIES=none` disables the header outright, and
IPv4-mapped IPv6 addresses unmap to the same identity as their IPv4 form.

### Known limitation, documented rather than left implied

`TRUSTED_PROXIES` defaults to private ranges only. A deployment placed
behind a real reverse proxy, load balancer, or CDN must set it to that
proxy's actual address range in `.env`/deployment config, or
`X-Forwarded-For` stays ignored and every client behind it shares one
rate-limit bucket. Not done in this phase: no such proxy exists in this
environment yet (confirmed with the user), so there was nothing real to
set it to.

## [Security] — enforce RBAC on every mutating route (audit finding #1)

A code/security audit found the role → permission policy seeded by migrations
`00001`/`00020` to be real in the database and enforced almost nowhere in the
API. `RequirePermission` guarded `/team`, `/security` and the four features
whose own later phases wired it (integrations, custom metrics, API keys,
report presets, referral). The other 23 tenant-scoped route groups had no
authorization at all beyond "is a member of this org".

A **Viewer** — a role the Team UI presents as read-only, holding only
`campaign.read` and `analytics.read` — could create, edit and delete
campaigns, offers, networks, traffic sources, domains, tracking links and
stream sets over the API. Editing a stream set rewrites where live traffic
goes. The frontend hid the buttons; the API accepted the calls.
`campaign.write`, `offer.write` and `source.write` existed as rows in
`permissions` and in no production code path — the only references to
`campaign.write` in the Go tree were inside `auth_test.go`.

### Enforcement

82 mutating routes across 17 packages now sit behind `RequirePermission`.
Each handler takes a `requireWrite func(http.Handler) http.Handler` and wraps
its own mutating routes with `r.With(h.requireWrite)` — the shape
`custommetrics`/`reportpresets`/`referral`/`domain` already used;
`apps/api/main.go` builds one middleware per key and injects it.

- `campaign.write` — `/campaigns`, stream sets, tracking links, cost entries
- `offer.write` — `/offers`, `/networks`, `/event-mappings`, postback replay
- `source.write` — `/traffic-sources`, ad-account connection + spend sync
- `content.write` — `/landings`, `/pwas`, `/postlandings`, `/pixels`, `/tags`,
  `/content-gallery`
- `settings.write` — `/domains` (its `renew`/`sync-dns` already used this key;
  plain CRUD and `verify` were open and now don't)

Reads stay open to any member of the org, matching the split those four
packages already used — so no existing account loses access to anything it
can see today, and the change carries no lockout risk. Gating reads as well
would drop Viewer from `/offers`/`/networks` and Analyst from
`/traffic-sources`; that is a deliberate product decision, not something to
fold into a security fix.

Migration `00035` adds `content.read`/`content.write` — the six route groups
they cover had no permission key that could name them, so there was nothing
to gate them with even in principle. One key pair rather than six per-entity
pairs: all six are campaign creative/tracking assets landing on identical
tiering (Owner/Admin/Manager write, everyone reads), so six pairs would have
been six identical grants. Splitting later is a cheap additive migration;
collapsing six back into one is not.

### Deliberately left open

`/analytics`'s five POST endpoints are POST only because they carry a JSON
query body — they read ClickHouse and write nothing, and are not gated on
`analytics.read` either since all six roles hold it (a check that rejects
nobody reads as a control that isn't one). `POST /routing/simulate` resolves
routing in memory and persists nothing. `POST /referral/payouts` stays open
to every role by the Referral phase's own decision. Pre-session and personal
2FA auth routes have no role to check yet.

### Keeping it closed

`apps/api/rbac_coverage_test.go` parses every handler's `Register` function
with `go/ast` and fails on any `POST`/`PATCH`/`PUT`/`DELETE` not wrapped in
`r.With(h.requireWrite)`; the exceptions above are its `deliberatelyOpen`
allowlist, each with a stated reason. Verified it actually fails by
unwrapping one route and watching it name `campaign.delete` — a unit test of
`RequirePermission` itself could never have caught the original bug, since
the middleware always worked and simply wasn't mounted.

`TestPermissionGrantsMatchTheSeededPolicy` pins the whole 6-role ×
18-permission matrix against real Postgres; these grants are load-bearing for
the first time, so a wrong one is now an access-control bug.

### Frontend vocabulary had drifted

`lib/mock/team.ts` still listed only the original ten permission keys long
after `custom_metric.*`, `report_preset.*` and `referral.*` were seeded — and
`features/team/roles-permissions-panel.tsx` renders that copy to admins as
the org's access policy, so the matrix they read to decide who to invite as
what was incomplete. It now carries all eighteen, cross-checked
programmatically against the migrations (all six roles, exact counts), and is
marked as a manual sync point.

Action buttons themselves are still not permission-gated in the UI (only
`member-list.tsx` checks `team.write`), so a read-only role now sees a
"Create" button and gets a 403 from it. That is a UX gap, not a security one,
and it is the natural follow-up.

### Verified

`go build`/`go vet` clean; `go test -count=1 ./...` clean — 40 packages, run
with live Postgres/ClickHouse/Redis/MinIO so the DB-backed tests actually
executed rather than silently skipping. `tsc --noEmit`, `eslint`
(zero warnings) and `vitest` (21/21) clean.

Live end-to-end against the running containerized API: signed up a real org,
pulled the real verification link out of mailhog, invited and accepted a real
Analyst. `/auth/me` returned exactly the eight read keys for Analyst and all
eighteen for Owner. Twelve writes (campaigns, offers, networks, traffic
sources, domains, landings, PWAs, pixels, tags, content-gallery folders,
postlandings, event mappings) all returned 403 with the correct
`missing permission: <key>` message; twelve reads all returned 200 — no
lockout. Against a real campaign: Analyst got 403 on DELETE, PATCH, pause and
stream-set create, 200 on GET and on `routing/simulate`; Owner's PATCH
returned 200 and the rename actually persisted, confirming the write path is
intact rather than merely unblocked. Test org, users and every created
fixture deleted afterwards via SQL.

## [Between phases] — repo + plan audit: stale claims corrected, ROADMAP rewritten

A full audit of the code and the planning documents, re-verifying every
"done / not done" claim against what the tree actually contains. No feature
work; three corrections fell out of the verification, plus two stale
planning documents rewritten.

Re-verified from scratch rather than trusted from earlier write-ups:
`go build`/`go vet` clean, `go test -count=1 ./...` clean, `tsc --noEmit`
clean, `eslint` clean (zero warnings), `vitest` 21/21, working tree clean at
`1d345fe`.

### The "pre-existing" TypeScript errors were a stale build artifact

Every phase since the PWA-preview work has recorded 17 `[id]/page.tsx`
route-typing errors as pre-existing and unrelated. They were never a code
problem, and they are gone.

`apps/web`'s containerized dev service mounts `web_next_cache` over
`/app/.next`, so the container's `.next` is a Docker volume and the host's is
never written by it. The host still carried
`.next/dev/types/routes.d.ts` from a host-run `npm run dev` on 2026-09-02,
predating every `[id]` route — and `next-env.d.ts` imported exactly that
file, so `tsc` resolved `AppRoutes` from a fossil. `npx next typegen`
regenerated the types and repointed `next-env.d.ts` (gitignored) at
`.next/types/`. `tsc --noEmit` now exits 0.

This comes back silently if anyone runs `next dev` on the host again; the fix
is the same one-liner, not a code change.

### Phase 34's "0.0% coverage" reading was measured with the DB switched off

Phase 34's audit recorded that the repository/handler-layer Go packages
(network, offer, pixel, campaign, streamset, cost, "and ~18 others") sat at
0.0% unit-test coverage. That run had no `DATABASE_URL` set, so every
DB-backed test silently `t.Skip`'d — and `go test ./...` passes either way,
which is what made the number look real.

Re-measured with the dev stack's own URLs exported: network 49.5%, offer
52.0%, pixel 52.8%, campaign 42.8%, cost 51.0%, auth 56.8%, and comparable
figures across the layer. The genuinely-0.0% packages are only infrastructure
plumbing: `apierror`, `config`, `httpserver`, `idgen`, `logging`, `postgres`,
`rediscache`, `storage`, `telemetry`, `routingstore`. Core computation
remains high — event/macro/metrics/attribution 100%, classifier 94.7%,
conversion 89.6%, routing 81.7%.

Worth knowing generally: a bare `go test ./...` in this repo proves
compilation and pure-Go logic only.

### Dead code and stale comments

- `lib/mock/campaigns.ts` deleted. Phase 34 kept it because
  `tag-assignments.ts` imported its fixtures; the Tags phase then deleted
  `tag-assignments.ts` and nothing rechecked the survivor. Confirmed zero real
  importers — the only two remaining mentions were inside doc comments.
- Those two comments (in `lib/api/networks.ts` and `lib/api/offers.ts`) were
  themselves doubly stale, still describing `lib/mock/{networks,offers}.ts`
  and `stores/{networks,offers}.ts` as live and calling stream-sets,
  postbacks and conversions "still fully mocked features" long after all of
  it became real. Rewritten to describe what those clients actually are.
- `.env.example`'s `FX_RATE_PROVIDER_API_KEY` removed — no Go code has ever
  read it. FX rates come from the `fx_rates` Postgres table; no provider
  client exists. Replaced with a comment saying so, the same cleanup the
  registrar/DNS phase did for its own stale placeholders.
- `apps/api/main.go`'s note that "none of this file's 17
  `r.Use(tenantMiddleware)` call sites change" — there are 29. Dropped the
  count rather than correcting a number that rots on every new route group.

### ROADMAP.md rewritten

The worst-drifted document in the repo: it still listed Phase 1 as *in
progress* and Phases 2 through 34 as *planned*, having gone untouched since
Phase 1, while this CHANGELOG carries a real entry for every one of them.

Rewritten into three tables — §9's fixed phase order (all done), the post-34
user-scoped work (18 rows, each matching a real entry here), and a **Known
open gaps** table that collects in one place the deliberately-unbuilt work
that until now was scattered across ten phases' own NEXT lists.

### Audited clean, no action needed

Zero TODO/FIXME/console.log/debugger anywhere in source; all 29 tenant-scoped
route groups behind `tenantMiddleware`; `config.go` and `.env.example` agree
exactly once the dead FX key is gone; every §9 phase has a CHANGELOG entry;
`stores/` holds only genuine UI state (command-menu, sidebar); `lib/mock/`
holds only `team.ts` (real role/permission vocabulary, misfiled under
`mock/` — flagged, not moved) and `analytics.ts` (the report builder's
filter-value suggestion pool, a real and honestly-documented gap).

### One newly-quantified gap

Seven feature areas are English-only in an otherwise EN/RU app:
`content-gallery`, `custom-metrics`, `domains`, `referral`, `settings`,
`tags`, `team`. Settings' English-only state was an explicit earlier
decision; the other six were never decided — they just never got a namespace.

## [Between phases] — PWA live preview: the header eases down on install

Last difference from the original the user spotted: the avatar and the
app name themselves shift slightly down when the install starts, where
this build kept the header pinned in place. The header row now
transitions `pt-0` → `pt-3` (padding, not margin, so it can't collapse
against the parent flex gap) over the same 500ms as the rest of the
install transition.

Verified live with a before/after pair framed on the identical region,
the same way the user's own reference frames were.

## [Between phases] — PWA live preview: the install transition is animated

The user posted two reference frames of the same region — before the
click and immediately after — and pointed out what the earlier passes
missed: in the real Play Store the change is *animated*, not an instant
swap ("логотип и верхняя часть плавно смещается"). Two smaller details
differed too: the ads/purchases line is dropped during install, and
"Please wait…" renders dark and semibold, not muted.

`pwa-preview-card.tsx` now transitions the icon's inset/radius over
500ms; the install button is **collapsed** rather than unmounted, via a
`grid-template-rows` `1fr` → `0fr` + opacity transition (the technique
that makes an auto-height element animatable at all), so everything
below slides up smoothly instead of jumping; the ring fades in over
250ms off the same next-frame flip that starts its arc. The collapsed
button carries `tabIndex={-1}` under an `aria-hidden` wrapper so a
zero-height control can't be tabbed into. The icon's shrink ratio was
also corrected (inset 9px → 6px = a 60px circle in the 72px ring,
matching the reference's ~0.83 icon:ring diameter).

Verified live: two consecutive zooms right after one click caught real
intermediate frames (icon mid-round, button half-collapsed and
semi-transparent, share row sliding up behind it), and the settled
state matches the reference frame. No console errors.

## [Between phases] — PWA live preview: install ring surrounds a circle-cropped icon

Follow-up correction: the previous pass drew the ring at `inset-0`
straight over the unchanged rounded-square icon, so it cut through the
artwork and left the square's corners poking outside it. The real Play
Store (per the user's own screenshot) circle-crops the icon *and*
insets it, leaving the ring as a clean halo with a visible gap.

`pwa-preview-card.tsx`'s icon wrapper is now conditional —
`inset-0`/`rounded-2xl` at rest, `inset-[9px]`/`rounded-full` while
installing — so the 72px ring surrounds a 54px circular icon. Verified
live against the reference frame; the icon returns to its rounded
square with the developer name/badge once the cycle completes.

## [Between phases] — PWA live preview: install ring moves to the icon, matching real Android

Follow-up to the simulated install flow below: the user pasted a real
Play Store screenshot of an in-progress install and pointed out the
real behavior differs from the first pass — the Install button
disappears entirely (not disabled-in-place), a circular progress ring
wraps the app icon itself, and the developer line reads "Please wait…"
instead of the developer name/verified badge.

Rebuilt `pwa-preview-card.tsx` to match: `InstallProgressRing` is now
generalized (`size`/`strokeWidth`/`className`) and moved from inside
the button onto a new wrapper around the 72px icon; the button is
conditionally rendered (absent while `installStatus === "installing"`)
rather than merely disabled; the developer line swaps to a new muted
"Please wait…" string during that same window. New i18n key
`preview.pleaseWait` (en+ru).

Verified: tsc/eslint/vitest clean; a real browser pass confirmed the
button vanishes, the ring wraps the icon, and the developer line reads
"Пожалуйста, под…" during the simulated install, with no console
errors.

## [Between phases] — PWA live preview: simulated install flow

`features/pwa/pwa-preview-card.tsx`'s "Install" button — in the live
Google-Play-listing mockup shown on the PWA create/edit page — was
`disabled` since it was first built, purely decorative. The user asked
for it to be clickable and to simulate an install with a Google-Play-
style preloader.

No real install happens anywhere in this card (it only ever renders
form values — there's no installable app behind it), so this is a
deliberate FLOX-specific preview convenience, the same "our own code,
not a claim about real store behavior" precedent every other
interactive preview affordance in this file already carries.

The button now cycles `installStatus`: idle ("Установить") → clicking
swaps it for a transparent, accent-bordered button showing a small
circular SVG progress ring (a new `InstallProgressRing` helper) that
fills once via a CSS `stroke-dashoffset` transition over a fixed
1800ms → installed ("Открыть"/"Open"). Clicking "Open" resets back to
idle so the simulation can be replayed while designing a listing.
New i18n key `preview.open` (en+ru).

Verified: `tsc --noEmit`/eslint/vitest clean; a real browser pass
(Claude-in-Chrome) on a fresh `/pwa/new` page confirmed the button is
clickable, the ring visibly animates mid-click (caught via a zoomed
screenshot), it auto-flips to "Open," and "Open" resets back to
"Install" — no console errors.

## [Between phases] — Registrar/DNS automation for Domains (§30)

User picked this off the Google Ads phase's own NEXT list: wire the real
Namecheap/Cloudflare credentials Settings > Integrations now stores to
Domains' own `registrar`/`dnsProvider` fields, which had stayed plain
operator-declared enums since the original Domains + Tracking Links
phase. Scoped with a multiSelect question distinguishing three
genuinely separable pieces by risk (read-only status sync vs. a real
billable renewal vs. a real live-DNS-mutating record sync) — the user
picked all three.

**Backend.** `domain.Service` gained a `CredentialSource` interface
reading real credentials through a new, trusted, Go-only
`integrations.Service.CredentialsFor` method (decrypts and returns the
stored fields, never HTTP-exposed — mirrors `adaccount.Repository.
CredentialsByTrafficSourceID`'s own "internal callers only" precedent).
New `apps/internal/domain/namecheap` (real `domains.getInfo`/`domains.
renew` XML calls) and `apps/internal/domain/cloudflare` (real zone
lookup/SSL-verification/DNS-record calls) subpackages, structurally
verified only against fake `httptest.Server`s — the same pattern every
other provider adapter in this codebase already uses, since no live
Namecheap/Cloudflare account exists here. No new migration: Domain's
existing `Status`/`SSL`/`ExpiresAt`/`VerifiedAt` columns (previously
permanently null/operator-set) are now written for real, only through a
new `Repository.ApplyAutomationResult` path the client-facing
`UpdateInput` still can't reach.

Three new endpoints: `POST /domains/{id}/verify` (read-only — checks
whichever of registrar=namecheap / dnsProvider=cloudflare a domain
actually names); `POST /domains/{id}/renew` (registrar=namecheap only,
a genuine billable action, gated behind `settings.write` plus an
explicit frontend confirm dialog stating it charges the account and
cannot be undone); `POST /domains/{id}/sync-dns` (dnsProvider=cloudflare
only, creates/updates a real proxied CNAME pointing at a new
`TRACKER_DNS_TARGET` config value — deliberately no default, refuses
with a clear error while unset rather than pointing a real customer's
domain at a fabricated address — also gated behind `settings.write`
plus its own confirm dialog).

**A real bug found via live curl verification, not caught by the
original test suite** (which only ever exercised "both providers
connected" and "neither connected"): the first `Verify` implementation
checked registrar then dnsProvider, returning immediately on the first
applicable-but-disconnected provider — so a domain with Namecheap
disconnected but Cloudflare connected never even attempted the real,
connected Cloudflare check. Fixed by tracking "not connected" as a soft
per-provider skip rather than an immediate return, only surfacing that
error if literally nothing ran; a new regression test
(`TestVerifyRunsWhicheverHalfIsConnected`) covers the exact shape that
broke, and the fix was re-verified live afterward.

Drive-by cleanup while touching the adjacent `.env.example` section:
`DOMAIN_REGISTRAR_API_KEY`/`DNS_PROVIDER_API_KEY`/`FACEBOOK_ADS_APP_ID`/
`FACEBOOK_ADS_APP_SECRET`/`TIKTOK_ADS_APP_ID`/`TIKTOK_ADS_APP_SECRET`
were all stale, confirmed-unused placeholders from the original
spec-planning stage — removed and replaced with the one genuinely new
global var, `TRACKER_DNS_TARGET`, plus explanatory comments about why
the rest are per-org database rows now, not global env vars.

**Frontend.** New `features/domains/domain-automation-card.tsx`
(Status/SSL/Expires/Last-verified badges plus Verify/Renew/Sync-DNS
buttons, the latter two disabled unless their own provider matches, each
behind its own confirm dialog) wired into the domain edit page. `lib/
api/domains.ts` gained `verify`/`renew`/`syncDomainDNS`; `hooks/use-
domains.ts` gained matching mutations.

Verified: `go build/vet/test -count=1 ./...` clean, including new
`domain` package tests (both providers connected, only one connected —
the regression case, neither connected, an unsupported-provider domain,
`Renew`'s registrar/years validation and its "a failed renewal changes
nothing" guarantee, `SyncDNS`'s "refuses without a configured target"
guarantee) and two new adapter-level suites (Namecheap's real XML shapes
for `getInfo`/`renew` incl. API-error propagation; Cloudflare's real
JSON shapes for zone lookup/SSL verification/DNS record create/update/
no-op) plus a new `integrations` test for `CredentialsFor`. `tsc`/
`eslint`/`vitest` clean.

Real, live, end-to-end verification against the actual running dev API.
With nothing connected, `verify`/`renew`/`sync-dns` all returned clean,
correctly-worded validation errors. To exercise the connected path
without a real Namecheap/Cloudflare account, a test `org_integrations`
row was inserted directly (AES-256-GCM encrypted with the real dev
`INTEGRATION_ENCRYPTION_KEY`, the same cipher `apps/internal/
integrations` itself produces) — confirmed `GET /integrations`
correctly reported it connected, then `sync-dns` made a **real live
HTTP call to `api.cloudflare.com`** and got back Cloudflare's own real
`"Invalid access token"` error (this exact sequence is what surfaced the
Verify short-circuit bug above). Real browser pass: created a real
domain with registrar=Namecheap/dnsProvider=Cloudflare, confirmed the
real automation card renders with live status fields, clicked Verify (a
real 422 network call confirmed via the network log, no console
errors), opened both the Renew and Sync DNS confirm dialogs and
confirmed their real warning copy word-for-word, confirmed Sync DNS's
real network call. All test fixtures deleted afterward via SQL.

## [Between phases] — Google Ads cost integration (§74/§27-COST)

User picked this off the Settings > Integrations phase's own NEXT list:
Google Ads as a third real `adaccount.CostProvider`, per-traffic-source
like Facebook/TikTok Ads — that phase deliberately deferred it out of
the org-level Settings panel for exactly this architectural reason.

Google Ads' own API has no long-lived access token to paste directly
(tokens expire in ~1 hour), unlike Facebook/TikTok. An operator instead
pastes a refresh token minted once via Google's OAuth 2.0 Playground
using their own developer token and OAuth client — the same "no
registered public callback in this environment" constraint the original
`adaccount` package doc already documents for Facebook/TikTok.

**Backend.** Migration 00034 widens `traffic_sources.cost_integration`
and `cost_entries.source` to add `google_ads`, and adds four nullable
columns to `ad_account_connections` (`developer_token`, `client_id`,
`client_secret`, `refresh_token`; `access_token` becomes nullable — a
google_ads row never sets it). New `apps/internal/adaccount/crypto.go`
(AES-256-GCM, its own dedicated `ADACCOUNT_ENCRYPTION_KEY`) encrypts
`developer_token`/`client_secret`/`refresh_token` at rest — a real
improvement over `access_token`'s own pre-existing plain-text storage
(that column predates any encryption infrastructure in this codebase
and is deliberately left as-is; retrofitting it is called out
explicitly as separate hardening work, not silently left inconsistent).
`client_id` stays plain text, a public OAuth identifier rather than a
secret. `Repository.scanConnection` became a method since the masked
"token preview" can no longer be sliced in pure SQL once `refresh_token`
is ciphertext — it decrypts in Go purely to compute the last-4 preview.

New `apps/internal/adaccount/googleads` (same `Adapter`/`New()`/
`CostProvider` shape as `facebookads`/`tiktokads`): a real OAuth 2.0
refresh-token exchange against `oauth2.googleapis.com`, then one GAQL
`googleAds:search` query per page against `googleads.googleapis.com` —
`customer.currency_code` is queried alongside `campaign`/`segments`/
`metrics` in the same call (GAQL allows it directly, unlike `tiktokads`'
own separate advertiser-info call); `metrics.cost_micros` divided by
1,000,000 into the same `DailyCampaignSpendRecord.Amount` shape every
adapter here already produces.

Two real bugs found and fixed while tracing the request path, not
assumed clean: `trafficsource.CostIntegration.Valid()` — a completely
separate whitelist from the database CHECK constraint, in a different
package — still only listed the original two providers, so creating a
`google_ads` traffic source 422'd even after the migration landed
(caught live via curl); and `costsync.Providers`/
`sourceByCostIntegration` needed their own third case each, or the sync
would silently find no provider for a `google_ads` source's request.

**Frontend.** `lib/api/traffic-sources.ts`'s `CostIntegration` type and
i18n map gained `google_ads`. `ad-account-connection-section.tsx`
generalized from a fixed 2-field Facebook/TikTok form to a
schema-driven one (`superRefine` enforcing the right required subset
per provider) that renders 5 fields (customer ID/developer
token/OAuth client id/OAuth client secret/refresh token) for Google
Ads instead of the plain access-token pair. Fixed two stale i18n
strings while touching this exact copy: the cost-integration hint still
named only Facebook/TikTok and claimed "no scheduled sync yet, only
on-demand" — the real scheduler (`costsync.Scheduler`, built in an
earlier phase) has existed the whole time; the copy just never caught
up.

Verified: `go build/vet/test -count=1 ./...` clean, including new
`adaccount` tests (google_ads allowed alongside tiktok_ads, rejects a
bare `accessToken` and a request missing just `refreshToken`, and a
dedicated encryption round-trip test reading the *raw* `refresh_token`
column via SQL to confirm it's genuinely not the plaintext) and a new
`googleads` package suite (a real two-fake-server harness covering the
happy path incl. `cost_micros` division and customer-id dash-stripping,
pagination, a failing token refresh, a real Google Ads API error shape,
and malformed `cost_micros`), plus a plain unit test for the
`CostIntegration.Valid()` bug. `tsc`/`eslint`/`vitest` clean.

Real, live, end-to-end verification against the actual running
containerized dev stack. Also fixed, since already touching this exact
concern: `docker-compose.test.yml`'s api/worker services were entirely
missing `TOTP_ENCRYPTION_KEY`/`INTEGRATION_ENCRYPTION_KEY` — a
pre-existing gap from those two earlier phases that would have
crash-looped that compose file's own api service on its very first boot
attempt, never caught since verification always used
`docker-compose.dev.yml` instead. Confirmed real validation errors for
a mismatched-provider connect attempt and a missing-field one; connected
real (fake-valued) Google Ads credentials and confirmed the response's
token preview matched the real last four characters of the refresh
token; confirmed via direct SQL that `refresh_token`/`client_secret`/
`developer_token` are genuinely encrypted at rest while `client_id` is
genuinely plain; triggered "Sync now" and confirmed a **real live HTTP
call reached `oauth2.googleapis.com`** and got back Google's own real
`"invalid_client: The OAuth client was not found."` error, surfaced as
a normal 500 with no panic. Real browser pass against a disposable test
account: the cost-integration dropdown lists Google Ads as a fourth
option, selecting it shows the corrected hint/badge copy, saving
reveals the real 5-field form (not the 2-field Facebook/TikTok shape),
connecting shows the real masked preview, "Sync now" fires the same
real failing request with no console errors or crash, and disconnect
returns cleanly to the pre-connection state. All test fixtures deleted
afterward.

## [Between phases] — Settings > Integrations (§30): real backend

User picked "Integrations settings panel" off the accessorFn-audit
phase's own NEXT list. Before starting: found six-plus phases of work
(Tags through the accessorFn audit) sitting uncommitted since Tags —
committed as six logical, per-phase commits first (see each commit's
own message; Organization+API-Keys/Referral/Security were bundled into
one commit since they share `apps/internal/auth`'s core files, built as
one continuous uncommitted sequence with no clean split point).

Then scoped Integrations itself with the user: all four remaining
providers (Google Ads, Namecheap, Cloudflare, Slack), using the same
manual-credential pattern `apps/internal/adaccount` already established
for Facebook/TikTok Ads (no registered OAuth app/public callback exists
in this environment).

**Architecture call, not re-asked to the user:** Facebook/TikTok/Google
Ads all belong to the same per-traffic-source cost-integration design
(`trafficsource.CostIntegration`'s own singular shape) — duplicating
that connection flow at the org level would create two parallel,
driftable mechanisms for the same three things. So this phase built
real org-level backends only for the providers that are genuinely
org-scoped (Namecheap, Cloudflare, Slack), and made the panel's
Facebook/TikTok/Google Ads cards a real **read-only aggregate** over
`adaccount`'s own existing data, with a "Manage" link to Traffic
Sources — never a second connect flow. Google Ads itself as a third
real `adaccount.CostProvider` is deliberately not built this phase (see
NEXT).

**Backend.** Migration 00033 adds `org_integrations` (id/org/provider
namecheap|cloudflare|slack, `UNIQUE(organization_id, provider)`/
encrypted_payload/last_verified_at/last_error). New `apps/internal/
integrations` (model/repository/service/handler/crypto/payload +
namecheap.go/cloudflare.go/slack.go adapters):

- Credentials are AES-256-GCM encrypted via a new, dedicated
  `INTEGRATION_ENCRYPTION_KEY` (deliberately separate from
  `TOTP_ENCRYPTION_KEY`, same "fail loudly at boot if missing" bar) — a
  real improvement over `ad_account_connections`' own plain-text
  storage (that table predates any encryption infrastructure in this
  codebase; this one uses the real AES-256-GCM machinery `apps/
  internal/totp` already established, rather than repeating that
  now-avoidable gap).
- `Connect` requires the adapter's real `Verify` call to succeed
  *before* writing anything — a failed attempt leaves no row at all. A
  separate `Verify` action re-checks an already-stored credential and
  records failure as `last_error` without deleting the row (a real
  "error" status distinct from "not_connected"), recoverable by a later
  successful re-verify.
- Namecheap's adapter calls the real `namecheap.users.getBalances` XML
  endpoint; Cloudflare's calls the real `GET /user/tokens/verify` JSON
  endpoint — both verified structurally only, no live account exists in
  this environment (same `adaccount` precedent). Slack's adapter posts
  to a real, operator-supplied incoming-webhook URL — genuinely
  live-testable infrastructure, since an incoming webhook needs no
  registered app at all — and, because that URL is operator-supplied,
  reuses `apps/internal/urlsafety` (§54)'s real SSRF defense
  (`ValidateURL` at request time plus `SafeDialContext` as the HTTP
  client's `DialContext`, closing the DNS-rebinding gap a save-time
  check alone can't), the same defense `network.Service`'s own
  `postback_url` field already uses.
- A new `adaccount.Repository.AdPlatformSummary` method (grouped by
  `traffic_sources.cost_integration`, not a fixed provider list, so a
  future platform needs no code change here) powers the read-only
  ad-platform cards.
- RBAC: read open to any member, write requires `settings.write`
  (Owner/Admin) — the same permission key Organization/API Keys/
  Security already use for this Settings panel.

**Frontend.** New `lib/integrations.ts` (real vocabulary plus a
`CREDENTIAL_PROVIDERS` metadata list driving a generic connect-dialog
form, field-for-field matching each provider's real `RequiredFields`) +
`lib/api/integrations.ts` + `hooks/use-integrations.ts`. Rebuilt
`integrations-panel.tsx`: three read-only ad-platform cards (real
aggregate + "Manage" link) and three real connect/verify/disconnect
cards. `lib/mock/settings.ts` + `stores/settings.ts` deleted outright —
Integrations was their last remaining content.

Verified: `go build/vet/test -count=1 ./...` clean, including 12 new
integration tests against real Postgres (`Connect` requires successful
`Verify` and writes nothing on failure, required-field validation, the
full connect→verify-success→verify-failure-records-error→verify-
recovery→disconnect lifecycle, `Verify`/`Disconnect` 404 with nothing
connected, an ad-platform name is rejected as a credential provider,
the real `AdPlatformSummary` aggregate passes through unchanged,
cross-org isolation) plus 7 adapter-level tests against fake
`httptest.Server`s per provider. `tsc`/`eslint`/`vitest` clean.

Real, live, end-to-end verification over curl against the actual
running dev API: a **real live HTTP call reached the real Cloudflare
API** (`api.cloudflare.com`) with a deliberately invalid token and got
back Cloudflare's own real "Invalid request headers" error, proving the
adapter's real request shape against the genuine endpoint, not a fake
transport; confirmed required-field validation, `google_ads` correctly
rejected as a credential provider, a Slack connect against a real cloud
metadata address (`169.254.169.254`) rejected by `urlsafety` before any
request was sent; connected a real Facebook Ads ad account on a real
traffic source via the pre-existing endpoint and confirmed `GET
/integrations` immediately reflected the real aggregate
(`connectedCount: 1, totalCount: 1`); invited a real Buyer and confirmed
read succeeds, write gets a real 403. Real browser pass against a
disposable, self-controlled test account left over from an earlier
session's own browser-verification work: the Integrations tab rendered
all six real cards including the genuine "Connected on 0 of 1 traffic
source" aggregate; opened the Cloudflare connect dialog, submitted a
fake token, and confirmed a real toast rendered the actual live
Cloudflare error — the failure path works end to end through the real
UI. No console errors; no test artifacts needed cleanup (the failed
connect wrote nothing).

## [Between phases] — Repo-wide `accessorFn` stale-cache audit

Direct follow-up to the combined browser verification pass's own real
finding: a live bug in Referral's Earnings History "By" column, caused
by TanStack Table caching a row's `accessorFn`-computed value per (row,
columnId) and never invalidating it when only the accessorFn's
closed-over external state changes — only when the `data` array's own
reference changes. That phase's own follow-up list flagged this exact
class as "confirmed no other exact match exists today, but this is a
real, easy-to-repeat mistake worth a deliberate sweep" — this phase is
that sweep.

Grepped the whole `apps/web/src` tree for every `accessorFn` use (9
files) and read each one's full surrounding component, including
reading `@tanstack/table-core`'s actual installed source (not
documentation or assumption) to confirm the exact caching mechanism:
`row_getValue` caches on `row._valuesCache[columnId]` on first read, and
`Row` instances are rebuilt only when `table.options.data` changes
reference — column array identity, accessorFn closure identity, and
`useMemo` dependencies on the columns definition are all irrelevant to
whether the cache gets busted. Also confirmed from the same source read
that `column_getCanSort()` explicitly requires `!!column.accessorFn` —
so a column with only a `cell` (no accessorFn/accessorKey) loses its
sort control entirely, rather than silently sorting against a
frozen/undefined value. This isn't a regression: sorting by a value
frozen at whatever loaded first was never reliable to begin with, so
dropping the sort affordance on these specific resolved-name columns is
the correct outcome.

Found five more real, live instances of the exact same bug shape
(`xxxNameById[row.xxxId] ?? row.xxxId` inside `accessorFn`, where
`xxxNameById` comes from a separate query than the one supplying the
table's own `data`, and the page only gates its loading state on the
data query, not the lookup query) — six fix sites across five files:

- `features/campaigns/campaign-columns.tsx` ("source" column,
  `sourceNameById` from `useTrafficSources()`)
- `features/campaigns/campaign-cost-entries.tsx` ("source" column, the
  identical shape behind an extra ternary branch)
- `features/clicklogs/click-log-columns.tsx` ("campaign" column,
  `campaignNameById` from `useCampaigns()`)
- `features/conversions/conversion-columns.tsx` (two columns:
  "campaign" via `useCampaigns()`, "network" via `useNetworks()`)
- `features/postbacks/postback-log-columns.tsx` ("network" column,
  `networkNameById` from `useNetworks()`)

One near-miss confirmed safe, not just assumed: `features/offers/
offer-columns.tsx`'s own "network" column has the identical shape, but
`offer-list.tsx` gates its loading state on both the offers and
networks queries together, so there's no empty-map first-read to freeze
on. (A narrower, non-blocking residual risk noted, not fixed: if a
network is ever renamed via a mutation while this page stays mounted,
the column would freeze on the old name rather than a raw id — no such
co-mounted rename flow exists today.) `features/analytics/
report-table.tsx` and `features/campaigns/tracking-links.tsx` were also
confirmed safe on the same read (pure row-local/static reads, no
external lookup closed over at all).

All six sites fixed identically to the referral fix: `accessorFn`
removed, its exact resolution logic moved into `cell` (which runs fresh
every render) reading from `row.original` instead of `row`. The same
doc comment (cross-referencing `campaign-columns.tsx` as the canonical
explanation) added at every site.

Verified: `tsc --noEmit` clean (pre-existing, unrelated `[id]/page.tsx`
errors remain); `eslint` clean (zero warnings) on all five touched
files; `vitest` clean (21/21, unrelated). Real browser pass on a
still-connected test account: Campaigns, Conversions, Postbacks, and
Click Log pages all load with zero console errors post-fix (all four
were empty on the throwaway test org used, so this confirms no runtime
crash from the edit rather than a renewed visual "resolves correctly"
check — that visual mechanism was already proven once, live, on the
referral fix, and is identical here by construction and by the exact
same library source just read). No further browser-side
re-verification was attempted against fixture data across five
unrelated domains purely to re-demonstrate an already source-verified
library behavior a sixth time.

## [Between phases] — Combined real browser verification pass

The browser tool reconnected mid-session after being unreachable for
five straight phases (Custom Metrics, Settings > Organization/API
Keys, Report Presets, Referral, Settings > Security) that had only ever
been curl-verified. This was the queued combined pass — every one of
those five phases' real frontend UI, exercised live, for the first
time.

One real, unplanned complication at the very start: the browser's own
already-open tab carried a real, live session for the actual reporting
user's own account, not a throwaway test account. After verifying
Organization/API Keys settings on it, the Security tab's own live
verification led to enrolling real TOTP 2FA on that real account, then
logging it out to test the login-time code step — at which point it
became clear this session never had that account's password to log
back in with. Surfaced to the user immediately and transparently, with
the real secret and backup codes handed over in the same message. The
rest of the pass moved to a brand-new, fully-self-controlled test
account created through the real signup form, so the remaining
verification could proceed without depending on credentials this
session doesn't hold. Standing lesson: acting inside an
already-authenticated session means never assuming a way back in
exists.

Verified live, all via actual clicks/typing against the real backend:
Settings > Organization and API Keys on the original account; Settings
> Security defaults and the full personal 2FA enroll → confirm →
"Enabled" cycle on both accounts; the complete real login-time 2FA flow
end to end on the fresh account; 2FA disable with password
confirmation; the full real Referral lifecycle (add adjustment, request
payout, approve, mark paid — balance math checked out against the real
backend's own computation); real Custom Metrics create → delete; real
Report Presets save → delete, with the default preset correctly
un-deletable in the UI. No console errors surfaced on any page visited.

**One real, non-obvious frontend bug found and fixed live** — not a
wiring gap, a genuine defect curl-only verification could never have
caught: `features/referral/earnings-history-table.tsx`'s "By" column
resolved every row to the raw creator user id instead of the real team
member's name, every time, even though the real `/team/members` data
was correct and the exact same lookup logic run manually in the
browser's own JS console resolved correctly. Root cause, confirmed by
direct experiment: the column computed its value inside `accessorFn`,
which TanStack Table caches per (row, columnId) on first read and does
not invalidate when only an accessorFn's closed-over external state
changes (`memberNameByUserId`, which only becomes non-empty once the
team-members query resolves asynchronously after the first render) —
only a change to the `data` array's own reference clears that cache,
and `transactions` never changes reference here. The column's very
first read (before the team query loaded) permanently cached the
raw-id fallback. Fixed by moving the resolution into a `cell` renderer
instead of `accessorFn` (cell functions run fresh on every render) —
confirmed fixed live via the exact same flow that first surfaced it. No
other call site in the repo used this same pattern at the time
(confirmed, not assumed) — a later dedicated audit phase found five
more instances of the same shape elsewhere.

All test artifacts cleaned up: the original account's 2FA disabled and
returned to its exact prior Security settings; the fresh test account's
2FA disabled; the test API key revoked; the test Custom Metric and
Report Preset deleted; the Referral test transactions/payout left as
real, harmless ledger history on a throwaway test org.

## [Between phases] — Settings → Security (§30): real backend, real 2FA

Phase 34's own audit list is fully closed by this point (Tags/Custom
Metrics/Report Presets done this session; Organization + API Keys done
earlier this session too). Security was Settings' own last deferred
sub-panel (Integrations remains deferred). Scoped with the user to all
three pieces in one phase: session timeout, IP allowlist, and 2FA —
despite 2FA alone being a real, separate feature (full TOTP enrollment,
backup codes, a login-flow change), not a settings toggle.

Confirmed via grep before writing code: the spec has zero mentions of
2FA/session-timeout/IP-allowlist/TOTP/MFA anywhere. The mock UI's shape
(`lib/mock/settings.ts`'s `SecuritySettings`) was the only existing
design and had zero real enrollment UX. Real session mechanics already
existed (a hardcoded 30-day TTL, never configurable per-org); zero
MFA/TOTP columns anywhere; no TOTP library in `go.mod` (added
`github.com/pquerna/otp`, a real RFC 6238 implementation).

**Backend.** Migration 00032 adds three `organizations` columns
(`session_timeout_minutes` default 43200 — preserves every existing
org's current behavior; `ip_allowlist text[]`; `two_factor_required`)
and two `users` columns (`totp_secret` — the one symmetrically
*encrypted*, not hashed, secret in this codebase, since verifying a
live code needs it back in plaintext; `totp_enabled_at`), plus two new
tables (`user_totp_backup_codes`, hashed like every other one-way
credential; `pending_totp_logins`, deliberately a separate table from
`sessions` so a half-authenticated row can never be mistaken for a real
session).

New `apps/internal/totp` (`totp.go`): AES-256-GCM secret
encryption/decryption (key from `TOTP_ENCRYPTION_KEY`, no default — a
missing key must fail startup loudly, never silently run 2FA with a
well-known key); `GenerateEnrollment` (real secret + a server-rendered
QR PNG as a `data:` URI — the secret never leaves this process);
`ValidateCode` (RFC 6238, ±1 step skew); backup-code generation/hashing.

`apps/internal/auth` additions (the largest single-package change):
- Login/AcceptInvite/VerifyEmail now funnel through one `completeLogin`
  chokepoint enforcing IP allowlist then 2FA in one place. All three
  return a new `LoginOutcome` (`Session` | `RequiresTOTP`+pending token
  | `RequiresTOTPEnrollment`+pending token) instead of a bare session —
  backward compatible for every org that hasn't touched 2FA.
- IP allowlist is enforced only at login/accept-invite/verify-email time
  (granting a *new* session), never on existing sessions' later
  requests — deliberately, to avoid kicking an already-logged-in
  operator mid-work over an IP/list change.
- `UpdateSecuritySettings` rejects a new IP allowlist that would exclude
  the acting request's own current IP — the most common self-inflicted
  lockout — but cannot and does not guarantee no *other* admin gets
  excluded; that's a real, visible consequence of a policy an Owner
  chose to set.
- 2FA is real, per-user, and dual-context by design: the same
  `POST /auth/2fa/enroll`/`confirm` endpoints serve both a self-serve
  opt-in (identified by session cookie) and a forced enrollment
  mid-login for a brand-new invitee whose org already requires 2FA
  (identified by the pending token, before any session exists) — both
  mounted under `RegisterPublic` for the no-session case. Confirming a
  forced enrollment also completes the login that triggered it.
- `DisableTOTP` requires re-entering the account's own current password
  and refuses outright while the org's policy requires 2FA.
- RBAC: `GET`/`PATCH /security` reuses the existing `settings.write`
  permission; the five `/auth/2fa/*` and `/auth/login/totp` endpoints
  carry no `settings.write` gate at all — 2FA is a personal-account
  action gated only by "are you this account."

**Frontend.** New `lib/security.ts` + `lib/api/security.ts` +
`hooks/use-security.ts`. `lib/api/auth.ts` gained `LoginOutcome`
(mirroring the Go JSON) and five TOTP functions/hooks; `login()`/
`acceptInvite()` return types changed accordingly, and `useLogin`/
`useAcceptInvite` only invalidate the session query when a real session
was actually granted. New `features/auth/totp-step.tsx`
(`TOTPCodeStep`/`TOTPEnrollStep`, shared by login and accept-invite,
which both gained a `pending` state swapping the whole card for one of
these steps). `features/settings/security-panel.tsx` rebuilt onto the
real hooks plus a new "Your two-factor authentication" personal card.

Verified: `go build/vet/test -count=1 ./...` clean, including a new
`totp` package test file and 8 new `auth` integration tests against
real Postgres (real defaults + persistence + the lockout safeguard,
login respects a real IP allowlist with a real 403, full enroll-confirm-
login-with-pending-token cycle, a real backup code consumes exactly
once, org-required 2FA forces enrollment on an invited member's first
login, `DisableTOTP` requires the real password and refuses while the
org requires 2FA). One incidental bug this phase's own tests caught:
`reflect.DeepEqual` was needed instead of `==` to compare
`SecuritySettings` in a test, since `IPAllowlist` (`[]string`) makes the
struct non-comparable — caught by `go vet`. `tsc`/`eslint`/`vitest`
clean.

Real, live, end-to-end verification over curl (the extension stayed
disconnected). Confirmed real defaults; enrolled real TOTP via the
self-serve endpoint; confirmed a wrong confirm-code 422s and the real
code (computed independently in Python) confirms; confirmed a
subsequent login returns `requiresTotp` with no session cookie; wrong
login-time code 401s, correct code exchanges for a real session;
confirmed a backup code works exactly once; confirmed `PATCH /security`
rejects an allowlist excluding the caller's own IP and accepts one that
includes it; confirmed login is blocked with a real `ip_not_allowed`
403 when spoofing `X-Forwarded-For` to an excluded address; invited a
real second member into an org with `two_factor_required` on, walked
the dual-context enroll+confirm flow using only the pending token, and
confirmed their second login falls through to the ordinary path, not
enrollment again; confirmed `DisableTOTP` rejects/succeeds correctly
against the org's policy and a wrong password; confirmed a Buyer's read
succeeds and write gets a real 403. One environmental snag: this
testing exhausted the real auth rate limiter for the test IP, which
then made the repo's own e2e test 429 — not a code bug, confirmed by
clearing the specific exhausted Redis keys and re-running clean. All
test security settings/2FA state reset afterward.

## [Between phases] — Referral Program (§30.7, Phase 14.8): real backend

The last remaining item on Phase 34's audit list at the time (Tags,
Custom Metrics, Settings > Organization/API Keys, Report Presets done
earlier this session). Research before writing code: the spec text
(§30.7) is ambiguous about the referral model, but the frontend mock's
own doc comment already committed to a specific design — org-refers-org
(a tenant refers *other advertiser orgs* to sign up for FLOX itself),
one referral account per workspace, flat USD amounts explicitly exempt
from §50-FX normalization, and a balance derived from an append-only
ledger, never stored. Built to that design exactly, since it already
matched the spec's actual wording once resolved.

No commission formula is specified anywhere — resolved the same way
§27-COST's cost-entry gap was: a referred signup is attributed
automatically via a real referral code used at real signup time;
crediting a dollar amount is a deliberate, audited Owner/Admin action
(an `accrual` transaction), never an invented formula. No payment
processor exists anywhere in this repo — payouts are a real state
machine with a real audit trail, but fulfillment stays a manual,
off-platform action, matching the mock's own design.

**Backend.** Migration 00031 adds `referral_codes` (one row per org,
lazily created), `referral_signups` (referrer/referred org, unique per
referred org, snapshotting the signing-up user), `referral_payouts`
(pending→approved→paid|rejected), and `referral_transactions`
(append-only — no `UPDATE`/`DELETE` path exists on purpose). Plus
`referral.read` (all roles) / `referral.write` (Owner/Admin only —
narrower than every other Phase-34 feature's tiering this session,
matching the mock's own gating and because money movement warrants it).

New `apps/internal/referral`: no owner-based "manage only what I
created" restriction at all — any `referral.write` holder can act on
any payout/transaction for the org, simpler than Custom Metrics'
tiering on purpose. `Repository.ComputeBalance` mirrors the mock's own
`computeBalances` formula exactly and is never stored.
`MarkPayoutPaid` updates the payout and inserts the `payout_paid`
ledger row in one DB transaction. `RequestPayout` recomputes the
available balance server-side and rejects an amount exceeding it —
never trusts a client-supplied figure.

Cross-package signup attribution without `auth` importing `referral`:
a small `ReferralRecorder` interface lives in `auth` itself (mirroring
the existing `Mailer` interface pattern), wired in `main.go` after
constructing both services. `auth.Service.Signup` calls it best-effort,
swallowing any error — the account is already real regardless of
whether referral bookkeeping succeeds, and an unknown/garbage code is
already a silent no-op, not an error, so a signing-up person's own typo
never surfaces as a signup failure.

**Frontend.** The real public `/signup` page now reads an optional
`?ref=` query param and threads it through silently — no visible field,
no client-side validation of the code's shape. New `lib/referral.ts` +
`lib/api/referral.ts` + `hooks/use-referral.ts` replace
`stores/referral.ts` and `lib/mock/referral.ts` outright (including
their client-side balance-computation helpers, now redundant since the
real response already carries a computed balance). All six
`features/referral/*` files refactored onto the new hooks;
`currentMemberId` prop-threading is gone throughout.

Side effect: Referral was the last remaining consumer of the mock
`useCurrentMember()` hook and of `stores/team.ts` — both deleted
outright once confirmed to have zero other consumers.
`referral-dashboard.tsx` now reads the real session via `useMe()`;
`earnings-history-table.tsx` resolves a transaction's real
`createdByUserId` via the real `useTeamMembers()` hook. `lib/mock/
team.ts` trimmed (Role/Permission vocabulary stays; the fake member
roster/activity fixtures go).

Verified: `go build/vet/test -count=1 ./...` clean, including 8 new
integration tests against real Postgres (stable/unique code generation,
signup attribution and silent no-op on an unknown code, the full payout
state machine, cross-org isolation). `tsc`/`eslint`/`vitest` clean.

Real, live, end-to-end verification over curl. Signed up a real Org A,
signed up a brand-new Org B using Org A's real code through the real
signup endpoint, confirmed Org A's dashboard showed the real
attribution; signed up Org C with a garbage code and confirmed the
signup still succeeded with no attribution; added a real accrual
transaction, requested a payout exceeding balance (real 422), requested
a valid payout, confirmed mark-paid-before-approval 409s, approved and
marked it paid, confirmed the final balance math and the
auto-generated ledger row; invited a real Buyer, confirmed their read
and payout-request succeed while adding a transaction gets a real 403;
confirmed Org C's dashboard is fully independent. All test payouts
resolved; throwaway test orgs/users left, matching precedent.

## [Between phases] — Settings (§30): Organization + API Keys, real backend

User picked Settings next off Phase 34's audit list. Settings turned
out to be four panels with very different readiness: Organization
(name/timezone/currency) and API Keys both had real, already-existing
schema (organizations.timezone/currency since migration 00001; the
whole `api_keys` table, unwired, since migration 00010) — zero new
migrations needed. Security (2FA/session timeout/IP allowlist) and
Integrations (ad platforms, registrar, DNS, Slack) were deferred as
their own, larger, separately-scoped asks.

**Backend.** Organization settings added to the existing
`apps/internal/auth` package (a new `organization_handler.go` plus
model/repository/service additions) rather than a new package — `GET`
is open to any authenticated member, `PATCH` requires `settings.write`
(Owner/Admin). Validation is deliberately broader than the frontend's
own curated 8-timezone/8-currency dropdown lists: timezone via Go's
real IANA tzdata, currency as a plausible 3-letter code — the FX
converter already degrades gracefully for an untradeable currency, so
there was never a narrower list to validate against. Every update is
logged to the real `audit_logs` table.

New `apps/internal/apikey` (model/repository/service/handler/crypto):
key generation mirrors the frontend mock's exact format byte-for-byte
(`flx_live_` + secret; a `flx_live_XXXX...` prefix, since the panel
already renders `key.prefix` verbatim). Same one-way-hash-at-rest
precedent as sessions/invite tokens — only the SHA-256 hash is
persisted, the full key exists only in the create response. Revoke sets
`revoked_at` rather than deleting the row, matching the mock's own
revoke-removes-from-list behavior. Gated the same way as Organization
settings.

**Frontend.** New `lib/organization.ts` + `lib/api-keys.ts` hold the
real vocabulary moved out of `lib/mock/settings.ts`. New `lib/api/
{organization,api-keys}.ts` + `hooks/use-{organization,api-keys}.ts`
replace the corresponding slices of `stores/settings.ts` — that store
and the mock file were trimmed, not deleted (Integrations and Security
stayed mock at this point). `organization-panel.tsx`'s form schema was
deliberately loosened from a `z.enum` against the curated dropdown
lists to a plain non-empty string, since a stricter frontend enum would
reject loading/re-saving a real org already set to a value outside the
UI's own shortlist even though the backend accepts it.

Found one real cross-feature dependency while trimming the store, not
by inspection: `features/referral/referral-dashboard.tsx` read the org
name from the mock settings store for its referral-link preview — a
hook this phase was about to make permanently undefined. Referral
itself stayed mock (out of scope), but since the org's name was now
genuinely real, pointed that one read at the new `useOrganization()`
hook instead of leaving it broken.

Verified: `go build/vet/test -count=1 ./...` clean, including 3 new
`auth` integration tests (real defaults after signup, update persists
and is independently re-readable, logged to audit_logs, invalid
timezone/currency/name rejected) and 4 new `apikey` integration tests
(create/list/revoke round-trip incl. the exact prefix shape, revoking
twice rejected, invalid input rejected, cross-org isolation). `tsc`/
`eslint`/`vitest` clean.

Real, live, end-to-end verification over curl. Confirmed `GET
/organization` returns real defaults, a real `PATCH` renames the org
and normalizes currency casing, an invalid timezone 422s, a Buyer can
read but not write (real 403); confirmed `POST /api-keys` returns a key
matching the exact prefix shape, a Buyer can list but not revoke, Owner
revoke succeeds and the key disappears from a later `GET`. All test
mutations reverted afterward.

## [Between phases] — Report Presets (§27.5, Phase 14.7): real backend

Continuing down Phase 34's audit list (Tags and Custom Metrics done
this session; Settings and Referral remained). §27.5/Phase 14.7 also
bundles a "View statistics" directory drill-in, confirmed already real
and frontend-only (a navigation + pre-filter into the report builder,
no backend involved) for 3 of the spec's 4 named entities — Flows has
no standalone list view to hang the action off, a structural gap, not a
bug. This phase scoped to Report Presets itself.

The frontend already carried the preset shape and every interaction
(`features/analytics/report-preset-bar.tsx`: apply/save/rename/delete)
against a zustand mock store. No backend existed anywhere.

**Backend.** Migration 00030 adds `report_presets` (id/org/name/
dimensions text[]/metrics text[]/group_by/period fields (flat columns,
matching the frontend's own tagged union)/timezone/is_default/
created_by_user_id) plus `report_preset.read`/`report_preset.write`
permission keys. New `apps/internal/reportpresets`:

- Presets are genuinely team-scoped — unlike Custom Metrics there's no
  owner-based "manage only what I created" restriction; anyone holding
  `report_preset.write` can edit or delete any non-default preset.
- Dimension validation reuses the real `chstore.ValidDimension`
  whitelist directly — a genuine shared source of truth, not a mirrored
  copy. Metric validation has no Go-side equivalent to import, so
  `reportpresets.validMetrics` mirrors the frontend's `MetricKey` union,
  same tradeoff `custommetrics/registry.go` already accepted.
- `is_default`: a system default preset can't be renamed or deleted,
  enforced both in the service layer and defensively in the repository's
  own `WHERE is_default = false` clause. `Repository.EnsureDefault`
  lazily seeds an org's default preset the first time `List` runs for an
  org with none yet, race-safe via `ON CONFLICT DO NOTHING` against a
  partial unique index (`report_presets_one_default_per_org`).
- Timezone validated via Go's real IANA tzdata, same precedent
  Organization settings established.

**Backend bug caught mid-build, not by review:** the migration's own
`CHECK` bound (copied from the frontend mock's 14-day dropdown) rejected
the same migration's `DEFAULT 43200`-equivalent default period — the
migration failed outright against live Postgres on first apply. Fixed
by widening the check to the real ceiling, not the frontend's curated
range — the same "backend validates broadly, frontend curates a
shortlist" precedent Organization settings' timezone/currency already
established. The transactional migration rolled back cleanly on the
first failed attempt.

**Frontend.** New `lib/report-presets.ts` holds the real vocabulary
(`ReportPeriod`/`ReportPreset`/`ReportPresetInput`, `RELATIVE_PERIODS`,
period↔date-range conversions) moved out of `lib/mock/`. New `lib/api/
report-presets.ts` + `hooks/use-report-presets.ts` (TanStack Query)
replace `stores/report-presets.ts` and the mock's fake array (both
deleted). `report-preset-bar.tsx` refactored onto the new hooks with
real loading (skeleton chips) and error (inline retry) states the mock
version never needed.

Verified: `go build/vet/test -count=1 ./...` clean, including 6 new
integration tests against real Postgres (lazy default-seeding is
exactly-once, create/update/delete round-trip, the default preset
rejects rename and delete, 10 invalid-input cases incl. an unbacked
dimension, custom period round-trips, cross-org isolation). `tsc`/
`eslint`/`vitest` clean.

Real, live, end-to-end verification over curl again this round — the
extension stayed disconnected. Signed up two separate test orgs:
confirmed a real `GET` seeds exactly one default and a repeat `GET`
doesn't duplicate it; created a non-default preset with a non-UTC
timezone and multi-dimension grouping; confirmed real 422s for an
unbacked dimension and an invalid timezone; renamed the preset and
confirmed persistence; confirmed real 409s renaming/deleting the
default; deleted the non-default preset and confirmed it's gone while
the default remains; invited a real Buyer-role member, confirmed their
`GET` succeeds and `POST` gets a real 403; confirmed Org B's `GET`
returns its own independently-seeded default, not Org A's, and a direct
cross-org `GET` 404s. This proves the real backend end-to-end; it did
not prove the preset bar itself renders/behaves correctly in a live
browser — that check happened in the later combined browser
verification pass.

## [Between phases] — Custom Metrics (§30.5, §50): real backend

Next off Phase 34's own audit list of frontend-mock features (Settings,
Custom Metrics, Report Presets, Referral remained; Tags just finished
this session). Needed far less new work than expected: the frontend
already carried a genuinely complete, real formula engine
(`lib/formula-engine.ts` — a real tokenizer/recursive-descent
parser/evaluator, not an `eval()`/`Function()` shortcut) and a full
builder UI (`features/custom-metrics/*`) built against a zustand mock
store, and `features/analytics/report-table.tsx` already evaluated a
published custom metric's formula live against real `/analytics/report`
row data from an earlier phase — the "does it actually compute"
requirement was already real and shipped.

**Backend.** Migration 00029 adds `custom_metrics` (id/org/name/
group_name/formula/format/status draft-published/active/targets
text[]/created_by_user_id) plus `custom_metric.read`/`custom_metric.
write` permission keys. New `apps/internal/custommetrics` (model/
repository/service/handler/registry/formula):

- `formula.go` is a faithful Go port of the TS engine's tokenizer,
  recursive-descent parser, and validator (same grammar, same functions
  DIV/EMPTYIF/IF/ROUND/ABS/MIN/MAX) — parse-and-validate only, since
  computation stays client-side.
- `registry.go` mirrors the frontend's `METRIC_CATALOG` and
  `SURFACE_AVAILABLE_METRIC_IDS` exactly, with a doc comment flagging
  the two copies must stay in sync — the same tradeoff already accepted
  for the report dimension whitelist between `registry.ts` and
  `chstore.ReportDimension`.
- RBAC maps §30.5's stale v2-era role names (Owner/Tech/Lead/Buyer) onto
  the real six roles using the exact tiering the mock UI already
  implied: Owner/Admin manage any metric; Manager creates and manages
  only metrics it created; Buyer/Analyst/Viewer see published+active
  metrics only. Enforced two ways — coarse read/write via
  `auth.Service.RequirePermission`, plus a new `Repository.
  CallerRoleKey`/`Service.callerTier`/`canManage` pair for the row-level
  "own metric only" restriction the coarse system can't express.
- List/Get filter server-side by the same tiers, not just client-side.
- A draft is force-cleared of its Show-in targets on every
  publish→draft transition, and a formula's requested targets are
  silently narrowed to whichever surfaces can actually compute it — both
  rules the mock only ever enforced in the frontend.

**Frontend.** New `lib/custom-metrics.ts` + `lib/custom-metrics-
registry.ts` hold the real domain vocabulary moved out of `lib/mock/`.
New `lib/api/custom-metrics.ts` + `hooks/use-custom-metrics.ts`
(TanStack Query) replace `stores/custom-metrics.ts` and `lib/mock/
custom-metrics.ts`'s fake data (both deleted). All six
`features/custom-metrics/*` files plus `features/analytics/
{analytics-view,report-table}.tsx` refactored onto the new hooks.
`custom-metrics-list.tsx` now reads the real session directly via the
already-real `useMe()` rather than the mock "current member" hook.

Real bug caught by the integration tests, not by inspection: a Go `nil`
slice for `targets` on a fresh draft encoded as SQL `NULL`, not `{}`,
and the column's `NOT NULL` constraint rejected every draft-status
`Create` outright — `DEFAULT '{}'` only applies when a column is
omitted from an `INSERT`, not when `NULL` is passed explicitly. Fixed
at both call sites that build a deliberately-empty targets list, and
defensively in `Repository.Create` itself.

Verified: `go build/vet/test -count=1 ./...` clean, including 11 new
integration tests against real Postgres (CRUD, 8 invalid-formula cases
incl. LTV-forbidden/mixed-data-source/wrong-arity, publish-time target
filtering, unpublish clears targets, delete blocked-then-allowed across
a real publish/unpublish transition, ownership tiering, cross-org
isolation). `tsc`/`eslint`/`vitest` clean.

Real, live, end-to-end verification over curl (not a browser pass — the
Claude-in-Chrome extension was disconnected for the whole session):
signed up a real test org, verified a real signup email via the live
mailhog container, invited and accepted a real second Buyer-role
membership, then round-tripped the real HTTP API as both roles —
created and published a real metric as Owner, confirmed real 422s for
an LTV-referencing formula and a mixed-data-source formula, confirmed
unpublishing actually cleared targets, confirmed delete is blocked
while published/exposed and succeeds once cleared, confirmed the
Buyer's real `GET` saw nothing until Owner published something, and
confirmed the Buyer's real `POST` got a real 403 with the correct
permission-key message. Test data cleaned up afterward; the throwaway
test org/users were left, matching precedent (no delete-org endpoint
exists). This proves the real backend end-to-end; it did not prove the
Settings > Custom Metrics tab renders/behaves correctly in a live
browser — that check happened in the later combined browser
verification pass.

## [Between phases] — Tags (§30.6): real cross-entity backend

Continuing down Phase 34's own audit list of frontend-mock features
with no real backend (Domains was resolved in its own earlier phase,
Content Gallery just finished this session — Settings, Tags, Custom
Metrics, Report Presets, and Referral remained). User picked Tags to
go first.

Confirmed via grep before starting: no `tags`/`taggables` table, no
`apps/internal/tags` package — the color-label system spanning
Campaigns, CPA Networks, Offers, Flows, Traffic Sources, PWA apps, and
Landing Pages (§30.6) ran entirely against a zustand mock store
(`stores/tags.ts`) seeded from `lib/mock/{tags,tag-assignments}.ts`,
already wired into all seven taggable entity lists.

**Backend.** Migration 00028 adds `tags` (id/org/name/color) +
`taggables` (id/org/tag_id ON DELETE CASCADE/entity_type CHECK/
entity_id, unique per tag+entity) — the generic-table-plus-polymorphic-
join shape the spec itself names. `entity_id` is deliberately not a
foreign key (it's polymorphic across seven different tables) —
referential integrity is enforced in the service layer instead, the
same pattern the Domains/Tracking-Links phase already established for
`domain_id`/`campaign_id`. New `apps/internal/tags` (model/repository/
service/handler): tag CRUD; `GET /tags/assignments?entityType=X`
(hydrates a whole list page's badge/filter state in one call, the same
shape the old mock store held in memory); `POST /tags/assignments/set`
(replace one entity's full tag set — named `/set` rather than a `PUT`,
since the shared frontend API client's method union doesn't include
`PUT`); `POST /tags/assignments/bulk` (§30.6's exact bulk semantics —
`toAdd` applied to every entity, no-op where already present; `toRemove`
removed from every entity, no-op where absent; untouched tags left
exactly as they were per entity, not globally).

Two ownership checks the service layer now enforces that the mock
never could: the `entity_id` must belong to a real, same-org row of
that entity type (checked via a small internal entity-type → table-name
map plus an `EXISTS` query — the interpolated table name is always an
internal constant, never user input, so there's no injection risk); and
every referenced `tag_id` must be a real tag owned by the caller's own
org — without this, nothing would stop a request naming another org's
tag_id. No data would actually leak (a foreign tag_id never resolves
against the caller's own tag list), but that org's later deletion of
its own tag would cascade-delete this org's assignment row too, a real
cross-org side effect caught by reasoning through the schema rather
than by observing it happen.

**Frontend.** New `lib/tags.ts` holds the real domain vocabulary
(`TaggableEntityType`, `TAG_COLORS`, `Tag`, `TagAssignment`,
`tagColorHex`, `randomTagColor`) moved out of `lib/mock/tags.ts` — it's
permanent vocabulary, not disposable mock data, so it no longer belongs
under `mock/`. New `lib/api/tags.ts` + `hooks/use-tags.ts` (TanStack
Query: `useTags`/`useTagAssignments`/`useCreateTag`/`useUpdateTag`/
`useDeleteTag`/`useSetEntityTags`/`useBulkEditTags`) replace
`stores/tags.ts` (deleted outright), along with `lib/mock/tags.ts`'s
fake `TAGS` array and `lib/mock/tag-assignments.ts` (deleted — seed
data, not vocabulary). All six `features/tags/*` components were
refactored to consume the new hooks instead of the store, keeping
identical external props/behavior, so the seven integration points
across the app needed only an import-path-plus-one-line-hook swap each,
not a rewrite.

Also added two stable exported empty-array constants
(`EMPTY_TAGS`/`EMPTY_TAG_ASSIGNMENTS` in `hooks/use-tags.ts`) replacing
every `?? []` fallback on a query result — a fresh `[]` literal there
gets a new identity on every render while the query is still loading,
which silently breaks any `useMemo` built from it (exactly the
snapshot-stability trap `use-entity-tags.ts`'s own pre-existing doc
comment already warned about from its zustand days). Caught by eslint's
`exhaustive-deps` warning across seven files, not by inspection.

Verified: `go build/vet/test -count=1 ./...` clean repo-wide, including
8 new integration tests against real Postgres (tag CRUD, cross-org
isolation on update/delete, `SetEntityTags` rejects a nonexistent
entity and rejects another org's `tag_id`, correctly replaces the full
tag set on repeated calls, `BulkEdit`'s exact §30.6 semantics verified
across two entities in one call, tag deletion cascades every
assignment). `tsc`/`eslint` (zero warnings, not just zero errors)/
`vitest` clean.

Real browser pass against the actual reporting user's own live account,
using the network-request log and direct authenticated fetches as
ground truth rather than screenshots alone: quick-created a real tag
from the PWA list's picker (one early attempt looked like a silent
failure — root-caused via the network log to a stale/mismatched entry
in the browser tool's own log, not a real server error; the actual
server access log showed every request succeeding), assigned it to a
PWA and confirmed the real badge rendered; used the tag filter to
narrow a list to exactly the tagged row; bulk-assigned the same tag
across three PWAs via multi-select and confirmed all three real
`taggables` rows via a direct `GET`; navigated to Campaigns and
confirmed the identical tag (same `tag_id`) was pickable there too,
assigned it to a real campaign, and confirmed the cross-entity
assignment persisted correctly scoped by `entity_type`; renamed and
recolored the tag via a direct `PATCH` and confirmed the change
propagated on both list pages; deleted the tag via the UI and confirmed
via `GET` that every assignment row across both entity types was
cascade-removed, with every list page's badge reverting cleanly to "Add
tags." Stray tags created by earlier misclicks during testing were
deleted via the UI itself before the final check — no test artifacts
left behind, no console errors on any page touched.

## [Between phases] — PWA form: status dropdown replaced by card-header buttons

User asked to remove the "Статус" dropdown from the PWA form and move
its buttons into the card instead. This exact pattern already shipped
for Campaigns: `campaign-form.tsx` carries no visible status field at
all (only in schema/defaultValues, submitted transparently on every
save), with `CampaignRowActions` rendered separately as Pause/
Duplicate/Archive buttons in the page header next to the Save button,
via an `"icons" | "inline"` variant split — icon buttons for the table
row, labeled buttons for the detail page. Mirrored exactly for PWA.

`pwa-form.tsx`: removed the Статус `Label`/`Controller`/`Select` block
— `status` stays in the schema and `defaultValues` only, same approach
the previous entry's `startUrl` removal used (no hidden input needed
here, since `campaign-form.tsx`'s own status field already proves RHF
submits a field that's seeded via `defaultValues` but never registered
in the DOM). Removed the now-dead `STATUS_OPTIONS` constant and
`statusLabel` i18n key (en+ru; confirmed by grep that every other
`form.statusLabel` hit in the repo belongs to a different feature's own
i18n namespace, not this one).

`pwa-row-actions.tsx` gained the same `variant` prop/split as
`CampaignRowActions`: `"icons"` (the existing table-row buttons,
unchanged) and a new `"inline"` branch — labeled Pause-or-Resume/
Duplicate/Archive buttons, "Edit" dropped since you're already on the
edit page. Duplicate now navigates to the newly created PWA's own edit
page on success (previously it just toasted and stayed put) — matching
`CampaignRowActions`' own duplicate behavior exactly, not a new UX
decision invented here. `PwaForm` gained an optional `headerActions?:
ReactNode` prop, rendered via a new `<CardAction>` in its own
`CardHeader` (omitted on the new-PWA page, since there's no existing
record yet to act on); `pwa-detail-view.tsx` passes `<PwaRowActions
pwa={pwa} variant="inline" />`.

**Real bug caught live, not in review.** The first browser pass showed
clicking the header's Pause button do nothing visible; a second click
produced a toast that read like a full form-submit ("PWA обновлено")
instead of the expected pause toast. Root cause: unlike
`CampaignRowActions`, which renders as a page-header sibling entirely
outside `CampaignForm`'s own `<form>`, `PwaRowActions` here renders
*inside* `PwaForm`'s `CardHeader`, itself inside the form element — and
shadcn's `Button`/`IconButton` pass `type` straight through to a native
`<button>` with no default, so every action button lacking an explicit
`type="button"` fell back to the HTML default of `type="submit"` and
silently submitted the surrounding form on click. Fixed by adding
`type="button"` to all eight buttons in `pwa-row-actions.tsx` — both
variants, plus the archive-confirm dialog's own two buttons (which are
Radix-portalled outside the form's DOM subtree and so weren't actually
broken, but got the same fix defensively rather than relying on portal
semantics for correctness).

Verified: `tsc --noEmit`/`eslint` clean; `vitest run` clean (21/21,
unrelated); both locale files confirmed to stay valid JSON. Real
browser pass against a real duplicated test PWA, using the browser
tool's own network-request log (not just the toast text) to confirm
the fix actually rewired click → `POST /pwas/{id}/pause`, not a form
submit: clicked Pause (200, a direct `GET` confirmed status
`"paused"`, button flipped to "Возобновить" with no form-submit side
effect), clicked Resume (`GET` confirmed back to `"active"`), opened
and confirmed the Archive dialog (`GET` confirmed `"archived"`).
Separately confirmed Duplicate had already created a real new PWA and
navigated to its edit page. Confirmed the list page's `"icons"` variant
is unaffected (edit/pause/duplicate/archive for an active row,
edit/duplicate only for an archived one), and that the new-PWA page
shows neither a Статус field nor any header buttons. No console errors
on any of the three pages touched. Both test PWAs left archived, not
deleted or reactivated.

## [Between phases] — PWA form: remove the operator-facing start_url field

The user asked what "URL запуска" (`start_url`, the Web App Manifest
field for what a launched-from-home-screen install opens) does, then
asked to remove it for the operator: "это PWA привязывается к кампании
и в потоки" (this PWA gets attached to a campaign and into flows) —
real navigation after install/open is resolved through campaign/
stream-set/flow attachment, not a manually-typed manifest path
disconnected from that routing.

Confirmed via a repo-wide grep that `start_url` was never actually
consumed anywhere in `apps/tracker` or by any manifest-serving
endpoint — it's stored and shown only in the client-side manifest-JSON
preview, so removing the operator input changes nothing about real
request handling; there was no hidden dependency to work around.

Kept `startUrl` in the Zod schema, the manifest-JSON preview, and the
backend (it's still a real, valid manifest field), but replaced its
`<Label>`/`<Input>`/error-display block with a plain `<input
type="hidden" {...register("startUrl")} />` — kept registered rather
than dropped outright, so form submission doesn't depend on subtle
"defaultValues without registration" behavior. The new-PWA path keeps
its existing `"/install"` default, now permanently fixed instead of
operator-editable; edit mode is unaffected, since a record's real saved
value passes straight through untouched via the form's own
`...defaultValues` spread — the same mechanism the previous entry's
palette removal already relied on. Removed the two now-dead i18n keys
(`startUrlLabel`, `startUrlPlaceholder`, en+ru); the validation message
key stays, since the schema still references it as a safety net even
though the UI can no longer trigger it.

Verified: `tsc --noEmit`/`eslint` clean; `vitest run` clean (21/21,
unrelated); both locale files confirmed to stay valid JSON. Real
browser pass: confirmed "URL запуска" no longer appears on either the
create or edit form; created a real new PWA end to end (gallery-picked
icon, cropped, name/short name filled in, submitted) and confirmed via
a direct authenticated fetch against the live API that the persisted
record's `startUrl` came back exactly `"/install"` despite no visible
input ever setting it — proof the hidden-field wiring actually works,
not just that it compiles. No console errors on either page; the test
PWA was archived afterward rather than left active.

## [Between phases] — PWA color fields: drop the raw palette, dropdown-only; green/light defaults

Two follow-ups to the color-scheme dropdowns from the previous two
entries.

**Palette removal.** The user pointed at a screenshot of both
dropdowns showing "Своя" (Custom) with their raw color-swatch +
hex-input rows underneath and said "Убери эти палитры они теперь не
нужны" (remove these palettes, they're not needed anymore). Removed
the raw-picker escape hatch entirely — not just hidden — the same
"pick-only, no manual entry path" call already made for the icon field
earlier in this phase, now applied to these two fields on the user's
explicit instruction. This reverses an earlier round's own "never
remove the ability to set an arbitrary color" reasoning for these two
fields specifically, but that reasoning was this feature's own design
assumption at the time, not a user directive — and the user has now
directed otherwise.

Removed: the `custom` `SelectItem` from both dropdowns, both `<input
type="color">` + hex `Input` blocks and their error displays, the
`forceCustomAccent`/`forceCustomTheme` state, and the "custom" branch
in both change handlers. `matchPreset` now returns `""` instead of
`"custom"` when a stored color matches neither preset list — the
dropdown simply renders with no selection until the operator picks a
preset; the field's real underlying value is never touched by this.
Also removed four now-unused i18n keys (`themeColorLabel`,
`themeColorPickerAria`, `backgroundColorLabel`,
`backgroundColorPickerAria`, en+ru) — these four were already dead
before this round, a leftover from the dropdown refactor a few entries
back, confirmed via a repo-wide grep before deleting.

**Default colors.** Same message, a second ask: "по умолчанию оставь
цветовая схема зеленая и тема светлая" (leave color scheme green and
theme light by default). The form's new-PWA `defaultValues` already
defaulted `themeColor` to green (`#16a34a`, already matching the
accent preset) but `backgroundColor` defaulted to `#0a0a0a` (dark).
Changed to `#ffffff` so a brand-new PWA now opens on
Зелёная/Светлая, matching the ask exactly. Edit mode is unaffected —
a real record's saved values always override these defaults via the
form's own `...defaultValues` spread.

Verified: `tsc --noEmit`/`eslint` clean; `vitest run` clean (21/21,
unrelated); confirmed both locale JSON files stayed valid after the
key removals. Real browser pass: opened a live PWA whose stored colors
(`#27914e`/`#c39797`) match neither preset list and confirmed both
dropdowns now render with no visible selection and no palette rows
beneath them, that opening the accent dropdown shows exactly the four
presets with no "Своя" entry, and that the PWA's own real color values
were left untouched (the preview's install button still showed the
original green, not silently reset to a default). Opened a fresh
`/pwa/new` and confirmed via zoomed screenshot that "Цветовая схема"
shows "Зелёная" and "Тема" shows "Светлая" by default, with the
listing mockup itself rendering light/green from first load. No
console errors on either page.

## [Between phases] — PWA listing preview: background follows the selected theme (light/dark)

Same ask as the previous entry, for the other dropdown: "тоже самое
сделай с темой при выборе темная тема фон должен смениться на темную и
наоборот" — selecting "Тёмная"/"Светлая" in the theme dropdown should
flip the listing mockup's own background, the same way the color-scheme
dropdown now flips its accent. Same tension as the accent fix (a real
Google Play listing is always light, never driven by an app's declared
background) — resolved the same way: an explicit, documented
FLOX-specific preview convenience, not a claim about real store
behavior.

`PwaPreviewCard` gained an optional `backgroundColor` prop (fed
`manifest.backgroundColor`). A new `isDarkColor` helper (perceived
brightness via ITU-R BT.601 weights) decides light vs. dark from the
color's own value rather than an exact match against the two dropdown
presets, so a custom dark hex picked through the dropdown's own "Своя"
fallback also flips the mockup correctly. Every place in the component
that used to be a hardcoded Tailwind slate/white class — card
background, all body/muted/subtle text, borders, the rating-bar track,
the empty-star fill, hover backgrounds — now reads one of seven
`--pv-*` CSS custom properties set once on the card's root element.
This was chosen over threading a `dark` boolean prop through every
sub-component (`PartialStar`, `StarBar`, the "About this app" sheet)
because CSS custom properties already cascade to every descendant for
free, the About sheet overlay included, with no extra prop plumbing.

Found and fixed one real bug from the previous entry along the way:
the active-nav-pill's translucent highlight background was still the
literal hardcoded class `bg-[#00A173]/10` — that round's own accent
work only ever set the pill's *text* color dynamically, never touched
this background class, so the highlight tint stayed the old fixed
green under every other color scheme the entire time since. Fixed with
a new `withAlpha` helper (hex → `rgba(...)` at a given opacity) so the
tint now reads the live `accent` value like everything else that round
touched.

Verified: `tsc --noEmit`/`eslint` clean on both touched files; `vitest
run` clean (21/21, unrelated). Real browser pass against a live PWA:
selected "Тёмная" and confirmed via screenshot that the entire mockup
— header bar, icon/name block, stat row, About section, Data safety,
Reviews, What's new, footer, bottom nav — switched to a dark background
with light text, while the install button and other accent elements
correctly stayed on their own independent accent color; opened the
"About this app" sheet and confirmed it rendered dark too, with no
extra wiring needed; switched back to "Светлая" and confirmed a full,
exact revert; zoomed on the bottom nav's active pill and confirmed its
tint now matches the live accent color rather than the old fixed green.
Reloaded the page afterward and confirmed the PWA's real saved state
(light, untouched — no test edits were submitted) came back correctly,
with no console errors on load.

## [Between phases] — PWA listing preview: accent color follows the selected color scheme

The previous entry's `PwaLaunchPreview` addition (a simulated status
bar + splash screen reacting to `theme_color`/`background_color`) was a
plausible reading of "picking a color scheme does nothing in the live
preview," but the user rejected it outright — "Убери ее я ни это имел
ввиду чтобы ты сделал" (remove it, that's not what I meant). Removed
the component, its `isLightColor`/`HEX_RE` helpers, its i18n keys, and
its usage entirely — confirmed clean via grep.

The actual ask, stated directly in the next message: "Мне надо чтобы
при выборе допустим цветовой схемы Синяя чтобы все зеленые цвета
сменились на синию и так далее" — selecting a color scheme should
recolor the Google Play *listing mockup itself* (install button,
developer name, rating star, per-star bars, share/wishlist icons,
active-nav pill), not a separate widget next to it. This does cut
against the previous entry's own reasoning (a real Play Store listing
always renders Google's fixed brand green, never an app's declared
color) — resolved by treating the mockup's accent as an explicit,
documented FLOX-specific preview convenience rather than silently
reversing that earlier call: `PwaPreviewCard` gained an optional
`accentColor` prop (fed `manifest.themeColor`), with a doc comment
stating plainly it's not a claim about real Google Play behavior. Every
hardcoded `REF_PRIMARY` (`#00A173`) render-site use now reads a
computed `accent` — `accentColor` validated against a hex pattern,
falling back to `REF_PRIMARY` when unset or invalid, so a PWA with no
theme color set renders exactly as before. `PartialStar`/`StarBar` (the
rating-breakdown helpers) gained their own `accent` prop threaded from
the same value.

Verified: `tsc --noEmit`/`eslint` clean on both touched files; `vitest
run` clean (21/21, unrelated to this change). Real browser pass against
a live PWA: filled in real rating/rating-count/star-breakdown values
(the fields held only placeholder text before), selected "Синяя
современная" and confirmed via zoomed screenshot that the install
button, developer name, rating star, and all five per-star bars turned
blue; switched to "Фиолетовая" and confirmed the same elements turned
purple with no reload — genuinely reactive, not a one-off. Reloaded the
page afterward and confirmed the PWA's real saved state (green,
untouched — none of the test edits were submitted) came back correctly,
with no console errors on load.

## [Between phases] — PWA theme/background color: a curated scheme dropdown

The user pointed at the raw theme-color/background-color picker pair and
a reference tracker's landing-page color pickers (a "Цветовая схема"
dropdown — Auto/Green/Blue/Blue-modern/Purple — and a separate "Тема"
light/dark/auto dropdown), asking for dropdowns here too, wired up for
real.

PWA only has two raw manifest fields (`theme_color`/`background_color`),
not a whole design system with many derived colors the way a landing
page's "scheme" implies, so the two reference dropdowns were combined
into one: a new `PWA_COLOR_SCHEMES` list of six named (theme,
background) pairs — Зелёная/Синяя/Синяя современная/Фиолетовая/Тёмная/
Светлая — each rendered as a `SelectItem` with two small colored dots
matching the pair, replacing the raw color-picker section entirely.
Picking a named scheme sets both fields at once via `form.setValue`. A
seventh option, "Своя" (Custom), isn't a preset — it's the fallback that
reveals the *original* raw color-picker pair, unchanged, so any exact
hex combination stays reachable; this only adds a faster path for
common choices, it never removes the ability to set an arbitrary color.

The dropdown's own selected value is derived, not independent state:
`matchColorScheme(themeColor, backgroundColor)` compares the current
field values against the six presets and falls back to "custom" if
nothing matches — so a saved PWA whose colors don't happen to be one of
the six presets correctly opens showing "Custom" with its real colors
intact, never silently overwritten. A small `forceCustomColors` boolean
handles the one edge derived state can't: picking "Custom" when the
current colors already exactly match a preset (e.g. a fresh PWA's own
green defaults) — without it, the dropdown would immediately snap back
to the matching preset the instant Custom was chosen, since nothing
about the underlying values would have changed.

Verified: `tsc`/`eslint` clean. Real browser pass against a live PWA:
opened the dropdown, confirmed all six presets render with correct
swatch-pair colors, selected "Фиолетовая," and confirmed via the raw
manifest JSON preview that `theme_color` became `#7c3aed` and
`background_color` `#0a0a0a` — not a cosmetic label change, the real
manifest fields moved. Selected "Своя" afterward and confirmed the
original raw picker + hex inputs reappear correctly. No console errors.

**Follow-up, same session**: the user clarified they wanted the
reference's own *two separate* dropdowns, not one combined picker —
"Цветовая схема" (accent color) driving `theme_color` alone, and a
second "Тема" (light/dark) driving `background_color` alone. Split
`PWA_COLOR_SCHEMES` into `PWA_ACCENT_COLORS` (Зелёная/Синяя/Синяя
современная/Фиолетовая — four accent colors, matching the reference
exactly) and `PWA_BACKGROUND_THEMES` (Светлая/Тёмная — two background
values; the reference's own third "Авто" option was left out
deliberately, since a required manifest color field has no real "auto"
value to map it to and inventing one would be fabricating a capability).
Each dropdown now has its own independent "Custom" fallback revealing
just *that* field's raw picker — not both at once — via two separate
`matchPreset`-derived values and two separate `forceCustom*` booleans
(the same derived-with-an-override-escape-hatch pattern as the round
above, just duplicated per field instead of shared).

Verified: `tsc`/`eslint` clean. Real browser pass against a live PWA:
confirmed the "Тема" dropdown lists Светлая/Тёмная/Своя, selecting
"Светлая" changed the real manifest's `background_color` to `#ffffff`
while the "Цветовая схема" dropdown's own selection (`theme_color`)
stayed untouched at `#16a34a` — confirming the two are now genuinely
independent, not still coupled under the hood. No console errors.

**Second follow-up, same session**: the user reported picking a color
scheme (or theme) did nothing visible in "Живое превью." Root cause
wasn't a bug — `PwaPreviewCard` (the Google Play *listing* mockup) never
accepted `themeColor`/`backgroundColor` as props at all, because the
real Play Store listing genuinely never uses an app's own declared
colors (it always renders its own green branding, regardless of what
any individual app's manifest says) — so wiring them into that card
would have been inaccurate, not a fix. The real gap: nothing anywhere
in the form showed these two fields' actual, spec-defined effect, so
changing them looked like it did nothing.

Added `PwaLaunchPreview` (`pwa-preview-card.tsx`), rendered above the
listing mockup: a small simulated status bar tinted with `theme_color`
and a splash-screen block tinted with `background_color` (the app icon
centered, name below) — exactly what the Web App Manifest spec actually
uses these two fields for (OS chrome tint and pre-paint launch
background), not a fabricated extra field. Text color inside picks
readable black/white via a small perceived-brightness check
(`isLightColor`) against whatever background the operator chose. A
caption states plainly that neither field affects the listing preview
below it, so the split isn't left implicit.

Verified: `tsc`/`eslint` clean. Real browser pass: the launch preview's
status bar visibly and immediately turned blue after selecting
"Синяя" in the color-scheme dropdown — the concrete, live feedback
that was missing. No console errors.

## [Between phases] — Screenshot carousel rebuilt on Embla, matching the reference's actual technique

The user pasted the reference tracker's own rendered HTML for its
screenshot gallery: `.embla`/`.embla__viewport`/`.embla__container`/
`.embla__slide`, sliding via `transform: translate3d(...)` — the exact
DOM shape of **Embla Carousel**, a real, widely-used open-source library
(MIT, zero dependencies — also what shadcn/ui's own carousel component
wraps), not a native `overflow-x: scroll`.

`pwa-preview-card.tsx`'s screenshot row was built on native scroll
(`overflow-x-auto` + CSS scroll-snap) in the round before this one — it
worked, but wasn't how the reference actually does it. Added
`embla-carousel-react` (same library, matching the reference's real
technique rather than reimplementing drag/momentum by hand) and rebuilt
the row as its own `ScreenshotsCarousel` component: `useEmblaCarousel({
dragFree: true, containScroll: "trimSnaps" })`, an Embla-ref'd viewport
div wrapping a flex container of slide divs, each holding one screenshot
`<img>`. `dragFree: true` matches how the reference's own mixed-width
(portrait + landscape) screenshots actually feel to drag — the slide
travels however far you drag it, rather than hard-snapping to exactly
one slide at a time.

Restructuring surfaced a real sizing bug in the process: the previous
round's squish-guard (`h-[204px]` alongside `pb-5` on the *same*
element) rendered images at 184px, not 204 — Tailwind's border-box
sizing means padding on an element with an explicit height eats into
that height. Fixed by moving the `px-4 pb-5` inset onto a plain, un-
padded outer wrapper div, keeping the Embla-ref'd viewport (the element
that actually needs the squish-guard, since it's the one with non-
visible overflow) unpadded so its own `h-[204px]` stays exact. Also
added `loading="lazy"` to each screenshot `<img>`, matching the
reference's own markup.

Verified: `tsc`/`eslint` clean. Real browser pass against the same live
PWA record: confirmed via `getComputedStyle` that all three real
screenshots now measure exactly 204px tall (not 184), the Embla
viewport clips with `overflow: hidden`, and — critically — a real
synthetic mouse drag (not a JS-dispatched event, which Embla's gesture
recognition didn't register in an earlier attempt) actually moved the
container's `transform` from `translateX(0)` to `translateX(-118px)`,
confirmed both via `getComputedStyle` and visually in a follow-up
screenshot showing the next slide partially scrolled into view. No
console errors.

## [Between phases] — Fix: screenshot row crushed to a sliver in the live preview

The user reported the just-fixed screenshot carousel rendering "криво"
(crooked/squished) — a real regression from the round above, not the
same reference-mismatch the previous fix addressed.

Root cause: `pwa-preview-card.tsx`'s whole card is a `flex flex-col`
capped at `max-h-[70vh]` with `overflow-y-auto` (§65's "everything
scrolls" pattern). The screenshot row, one flex item in that column, had
`overflow-x-auto` set (needed for the horizontal swipe) but no explicit
height of its own — relying on its `h-[204px]` *images* to establish
the row's natural height. That works for a normal block, but not inside
a flex column under real memory pressure: per the CSS flexbox/overflow
interaction, a flex item's *automatic minimum size* collapses to `0`
once it has non-`visible` overflow on any axis (`overflow-x: auto`
forces `overflow-y` to compute to `auto` too, per the CSS Overflow
spec's "if either axis is non-visible, so is the other" rule). Once
this particular PWA's card content exceeded `70vh` — true for any
record with a filled-out detail sheet, ratings breakdown, etc. — the
flex layout algorithm had a `0`-height floor to shrink this row toward
instead of leaving it alone and letting the *outer* scroll container
absorb the overflow, exactly as intended. Every other section of the
card was safe from this because none of them set `overflow-x`/`-y`
themselves; this row was the only one that did.

Fix: `h-[204px] shrink-0` added directly to the row `div` itself (not
just its images) — an explicit height a flex item can't be shrunk below
by this mechanism, and `shrink-0` as a second, independent guard against
the same class of squish regardless of cause. Comment left in place
explaining why both are load-bearing, not decorative, so a future edit
doesn't strip them as apparently-redundant styling.

Verified: `tsc`/`eslint` clean. Real browser pass against the same
account/PWA record that surfaced the bug: confirmed via
`getBoundingClientRect()` that the row and all three images now measure
a genuine 204px tall (previously crushed to a thin sliver), screenshot
matches the reference's own card proportions. No console errors.

**Follow-up, same session**: the user asked to look up the real Google
Play screenshot height and use exactly that, with no vertical scroll
inside the row. Live-inspected `play.google.com`'s own desktop app
listing (`Emulation`/viewport resize isn't available in this sandboxed
browser, so a true native-app measurement wasn't reachable — the
desktop web listing was the closest real, inspectable source) and found
something more useful than a single number: Google's own gallery keeps
one **fixed height** (`296px` there) and lets each screenshot's **width
follow its own natural aspect ratio** — portrait shots measured 177–187px
wide, landscape ones 469–494px wide, all sharing the identical 296px
height, confirmed across every image in the gallery via
`getBoundingClientRect()`.

That's not what the round above shipped: fixing the squish bug had also
added `aspect-[9/16]` + `object-cover`, forcing every screenshot into a
uniform cropped box — a change nobody asked for, made while chasing the
squish fix. `git diff` against the pre-session baseline showed the
original code was already `h-[204px] w-auto ... object-contain` (natural
width, no forced box) — a value from an earlier, already-reference-
validated phase (see this file's own "screenshot slider... 204px"
history). Reverted to that shape (dropped `aspect-[9/16]`), keeping only
the genuinely new pieces from this round (`snap-start`, the row-level
`h-[204px] shrink-0` squish fix). Result: the existing 204px height
turns out to already match Google Play's own real technique — fixed
height, natural per-image width — it just needed the erroneous forced-
aspect-box undone. Added explicit `overflow-y-hidden` on the row
(previously only implicit via the CSS spec's overflow-x/-y coupling) so
it can never grow a vertical scrollbar regardless of content, per the
user's explicit "no vertical scroll" ask.

Verified: `tsc`/`eslint` clean. Real browser pass against the same live
record: the three real screenshots (one landscape, two portrait) now
render at three *different* widths sharing one 204px height — no
cropping, no distortion — confirmed `overflow-y: hidden` is active on
the row and `overflow-x: auto` still drives the swipe. No console
errors.

## [Between phases] — PWA screenshots: a real upload grid, no count cap, swipeable preview

Two more asks, same session: replace the PWA screenshots field's URL-list
UI with a real upload area (matching a reference screenshot the user
described — "Upload screenshots (1080×1920px recommended) / Supports:
PNG, JPEG, WebP / No limit on quantity"), and make the live preview's
screenshot row actually look and behave like the real Google Play
listing it's modeling — swipeable with a finger.

**Screenshots field rebuilt** (`pwa-form.tsx`): the old list of URL
`Input` rows is gone, replaced with a thumbnail grid (`aspect-[9/16]`
cards, each with a hover-revealed × to remove) plus a `GalleryUploadCell`
tile at the end for direct upload — no picker dialog needed for the
common case, matching the reference's own big dropzone. A "Choose from
gallery" button stays for picking an already-uploaded file.
`GalleryUploadCell` gained an optional `className` prop (merged via
`cn`/tailwind-merge over its default `aspect-square`) so this portrait
grid and the square Content Gallery grid can share the one component.
The video formats the reference lists (MP4/WebM/MOV) are deliberately
left out of the shown copy — Content Gallery doesn't support video
uploads yet (a real, separately-scoped gap, not this round's to close),
and claiming support that doesn't exist would be exactly the kind of
fabricated capability CLAUDE.md's own rules rule out; the copy only
promises the formats that actually work (PNG/JPEG/WebP).

**The count cap is gone**, both sides: the reference's own copy says "no
limit on quantity," and the previous 12-screenshot cap was never a real
constraint (screenshots is a plain `text[]` column) — removed
`apps/internal/pwa/service.go`'s `screenshotsMax` check and the
matching frontend `zod` `.max(12, ...)`, per-item URL validation kept.

**Live preview screenshot row now matches the reference and is genuinely
swipeable** (`pwa-preview-card.tsx`): previously `object-contain` +
`w-auto`, so each card's width varied with its own image's aspect ratio
— loose and inconsistent, not the reference's uniform card row. Now
`aspect-[9/16]` + `object-cover` for a uniform portrait card regardless
of the source image's own shape, `snap-x snap-mandatory` +
`snap-start` so it settles one card at a time, and the scrollbar hidden
(`scrollbar-width: none` + the `::-webkit-scrollbar` pseudo-element) to
match Google Play's own chrome. The swipe itself needed no new code —
the row already had `overflow-x-auto`, which is what makes a horizontal
strip finger-scrollable on any touchscreen; the fix was purely the
visual mismatch, not missing interactivity.

Verified: `go build/vet/test -count=1 ./...` clean (`internal/pwa`'s own
suite included — no test asserted the removed cap, so nothing broke).
`tsc`/`eslint`/`vitest` clean. Real browser pass against the actual
reporting user's own live PWA record (already carrying 3 real
screenshots from their own earlier testing): confirmed via the DOM that
all three loaded with real non-zero natural dimensions (not broken),
`overflow-x: auto` and `scroll-snap-type: x mandatory` were both active,
the row's content genuinely overflows its container, and setting
`scrollLeft` programmatically actually moved the scroll position — the
same underlying mechanism a real finger swipe drives. Uploaded a new
screenshot through the new dropzone tile in a separate session and
confirmed it appended as a real thumbnail and appeared in the live
preview immediately. No console errors.

## [Between phases] — Crop moved to attach-time; a real "select region" cropper

Two corrections to the crop work above, same session, both from the user
watching the reference tracker's actual UI and comparing it live against
this build.

**Crop belongs at attach-time, not upload-time.** The user described the
reference's real flow: a client uploads images into the gallery exactly
as they are — no crop step there — then when attaching one to a field
that needs a specific shape (e.g. a PWA icon), *that's* when cropping
happens, on whichever file was picked or just uploaded. The previous
round had cropping backwards: every gallery upload went through a crop
dialog first, with a free choice of aspect presets, regardless of what
it would ever be used for.

Reworked: `gallery-upload-cell.tsx` uploads straight through again, no
crop step, matching every use of it (the main Content Gallery page
included) — a gallery upload keeps its original dimensions. Cropping now
lives in `gallery-browser.tsx`'s pick flow: a new `requiredAspect` prop
(only meaningful in `mode="pick"`) — confirming a selection (existing
file or one just uploaded through the same dialog) opens
`ImageCropDialog` locked to that ratio before `onPick` fires; skipping
uploads the file's own untouched URL. A freshly uploaded file becomes
the pick immediately (via a new `onUploaded` callback on
`GalleryUploadCell`) since it isn't in the list query's results yet to
select from. Applying a crop uploads the *cropped* result as its own new
Content Gallery file — the original, unfiled, is never mutated or
overwritten. `pwa-form.tsx`'s icon field passes `requiredAspect={1}`
(square, the real Web App Manifest convention); screenshots don't pass
one — Play Store screenshots genuinely vary in aspect, so requiring one
there would be fabricated, not real.

Cropping a file already in the gallery (not just a fresh upload) draws
its remote URL onto a `<canvas>`, which needs CORS response headers or
the canvas is "tainted" (`toBlob` throws `SecurityError`). Tried the
S3-compatible bucket-CORS API first (`minio-go`'s `SetBucketCors`) —
self-hosted MinIO doesn't implement it (`NotImplemented`, confirmed
live). Turned out unnecessary: MinIO serves CORS off its own server-
level `api.cors_allow_origin` admin config, which already defaults to
`"*"` — confirmed with a raw `curl` carrying an `Origin` header, matching
`Access-Control-Allow-Origin` came back with zero extra setup. Removed
the bucket-CORS call from `apps/internal/storage.Client.EnsureBucket`
entirely rather than keep a call the server rejects to duplicate a
default it already applies; documented in that function's own comment
that a real S3-compatible provider in production would need its bucket
CORS set through its own console/CLI if this behavior is wanted there.

**The crop interaction itself was backwards.** Built first on
`react-easy-crop` (pan/zoom the *image* behind a fixed frame — a
"profile-picture cropper" pattern). The user pointed at the reference's
actual crop screenshot: the image stays completely static, and the user
drags/resizes a selection *rectangle* over it, locked to the required
aspect — no zoom, no pan. Different interaction model entirely. Swapped
libraries: `react-easy-crop` out, `react-image-crop` in (`ReactCrop`
component + its own `cropToCanvas` helper, which handles the natural-
vs-displayed-size scaling and `devicePixelRatio` this package doesn't
have to). `image-crop-dialog.tsx` rewritten around it: a static `<img>`
inside `<ReactCrop aspect={...}>`, an initial centered 90% selection
(`centerCrop`/`makeAspectCrop`), a required-ratio caption matching the
reference's own "Required aspect ratio: 1.00" wording. `mediaProps`/
zoom-slider/aspect-preset code from the old library is gone — there's
only ever one fixed ratio now, matching the reference exactly, not a
chooser.

Also added, per a follow-up ask: a small "×" button on the PWA icon
field's thumbnail (visible only once an icon is set) that clears
`iconUrl` back to empty — a plain sibling button positioned over the
picker button, not nested inside it (native `<button>` can't nest).

Verified: `go build/vet/test -count=1 ./...` clean (including the
existing `contentgallery`/`storage` suites — no behavior change there
beyond the removed CORS call). `tsc`/`eslint`/`vitest` clean. Real
browser pass against the actual reporting user's own live account:
opened the icon picker, selected an existing (non-square) gallery file,
confirmed the crop dialog shows it static with a centered draggable/
resizable square selection and "Required aspect ratio: 1.00", dragged a
corner handle to resize the selection, clicked Apply, and confirmed via
a fresh `Image()` probe that the resulting uploaded file is a genuine
376×376 square — not the original image's own dimensions, and not a
placeholder — with a brand-new object key distinct from the source
file's. Confirmed the source file itself was untouched (still present,
unmodified, in the gallery). No console errors. Test-derivative file
archived afterward; the account's own real gallery content untouched.

## [Between phases] — PWA icon field: picker thumbnail replaces the raw URL input

Follow-up to the crop/picker wiring above, same session: the user
pointed at the icon field specifically and asked to drop the raw URL
text box entirely — just a "pick from gallery or upload" surface.

`pwa-form.tsx`'s icon field is now a single 96px square button:
empty state shows an upload glyph and "Choose image"; once set, it
shows the actual icon thumbnail with a "Change" overlay on hover.
Clicking it (in either state) opens `GalleryFilePickerDialog` — the
same upload+crop flow from the round above. The manual URL `Input` and
its adjacent browse-icon button are gone; there's no longer any way to
paste an arbitrary external URL into this field, which was the user's
explicit ask, not an oversight. `register("iconUrl")` is gone too —
the field is now written only via `form.setValue` from the picker,
which `react-hook-form` tracks the same way even without a bound DOM
input (the resolver validates against the full internal values tree,
not just registered fields).

i18n: `form.iconUrlLabel` shortened from "Icon URL"/"URL иконки" to
"Icon"/"Иконка" (there's no URL displayed anymore); new
`form.iconUrlChoose`/`form.iconUrlChange`; removed the now-unused
`form.iconUrlPlaceholder`/`form.iconUrlBrowseAria`.
`form.validation.iconUrlInvalid`'s message changed from "Enter a valid
icon URL" to "Choose an icon" — the old wording assumed a user could
type a malformed URL, which is no longer possible.

Verified: `tsc`/`eslint`/`vitest` clean. Real browser pass against the
same live account: opened the picker from the new thumbnail button,
selected an existing gallery file, confirmed the button now renders
that file as a real thumbnail (not a placeholder) and the live preview
panel's app icon updated to match. No console errors.

## [Between phases] — PWA icon/screenshots wired to the Content Gallery picker, with a crop step

The user asked for PWA image fields (icon, screenshots) to go through
the Content Gallery — upload or pick an existing file — with the
ability to crop.

**Crop**: new `apps/web/src/features/content-gallery/image-crop-
dialog.tsx`, built on `react-easy-crop` (new dependency). Every image
`GalleryUploadCell` accepts (jpeg/png/webp — an animated GIF skips the
dialog entirely and uploads as-is, since canvas has no concept of
frames and would silently flatten the animation) now opens a crop step
before uploading: drag to reposition, zoom via slider, three aspect
presets (1:1/4:3/9:16), "Skip crop" uploads the original untouched. No
fixed target dimension is enforced anywhere — no field in this app
declares one, and inventing one would be a fabricated requirement — the
presets are just common starting shapes. The output filename's
extension is derived from the actually-produced `Blob`'s `type`, not
the *requested* one: `canvas.toBlob` silently falls back to PNG if a
browser can't encode the requested type, and apps/internal/
contentgallery's own upload validation cross-checks extension against
sniffed content, so a mismatched extension would get a correctly-
cropped image rejected by the server for no visible reason.

Building this surfaced two real bugs, both caught by an actual browser
session rather than review:

1. **The upload cell never rendered inside the picker dialog.** It was
   gated to `mode === "manage"` — meaning `GalleryFilePickerDialog`
   (used everywhere images get picked from a form field) could only
   ever select an *existing* file, never upload a new one, directly
   contradicting the reference's own "upload directly within file
   selection dialogs" behavior this package's own doc comment already
   promised. Fixed: the upload cell (and the empty state) now key off
   `status === "active"` regardless of mode; the Active/Inactive tabs
   and "uploaded by" filter — both meaningless while picking, since a
   picker only ever shows active files — are hidden in `pick` mode
   instead of rendering as dead controls.
2. **Blob URL revoked before the cropper ever displayed the image.**
   The first version created the object URL in a `useState` lazy
   initializer and revoked it in a separate cleanup-only effect.  Those
   look equivalent to pairing creation/destruction in one place, but
   aren't: React 19's dev-mode Strict Mode double-invokes every effect
   (mount → cleanup → mount again) specifically to catch this class of
   bug, and the cleanup-only effect's first simulated pass revoked the
   URL the state initializer had already committed — the "recreate"
   half of the pair never happened, since a `useState` initializer only
   ever runs once. The Cropper displayed a dead blob (`naturalWidth`/
   `naturalHeight` stuck at 0, a blank gray box). Fixed by computing the
   URL via `useMemo` (recomputed correctly if Strict Mode's double-
   render reruns it) and revoking it explicitly from the two real exit
   paths (apply, skip) rather than from an automatic effect cleanup —
   abandoning the dialog via Escape/backdrop/X leaks one blob URL,
   accepted as a minor trade (garbage-collected on tab close) for a
   dialog opened occasionally, not a hot path.

**PWA form**: the screenshots `useFieldArray` gained an inline
"browse gallery" icon button per row (replace that row's URL) and a
new "Add from gallery" button (append a new row from a picked file) —
the manual "Add screenshot" blank-row button stays, for pasting an
external URL. New i18n keys `form.browseScreenshotAria`/
`form.addScreenshotFromGalleryButton` (en+ru).

Verified in a real, live browser session against the actual reporting
user's own account: opened the icon picker, uploaded a real image
through it, cropped it at 4:3, confirmed the crop actually applied (the
uploaded thumbnail was visibly cropped, not the original), selected it
back into the icon field, and confirmed both the live preview panel's
app-icon render and the raw manifest JSON preview showed the real
public URL — not a placeholder. Repeated for screenshots via "Add from
gallery," confirming the live preview's screenshot slider picked it up.
No console errors on a fresh page load. Test upload archived afterward
(the account's own pre-existing real gallery content — uploaded
independently by the user testing the feature live while it was being
built — left untouched). `tsc`/`eslint`/`vitest` clean across the whole
`apps/web` tree.

## [Between phases] — Content Gallery: real media library (images), replacing the template-gallery mock

The user pointed at a reference tracker's real Content Gallery page
(adset.pro/guide_ru/galereya-kontenta) and asked for the same. FLOX's
own `/content-gallery` was a frontend-only mock of something else
entirely — a "template gallery" of pre-built landing/PWA/postlanding
starter content with no object storage anywhere in the repo. Scoped
with the user first: replace it outright with a real media library
(images only this phase; video needs an ffmpeg/worker transcoding
pipeline this repo doesn't have, deliberately deferred).

**Backend**: migration 00027 adds `content_gallery_folders` (flat, no
nesting — matching the reference) and `content_gallery_files`
(`folder_id` `ON DELETE SET NULL` — deleting a folder moves its files
to the root, never deletes them, matching the reference's own rule;
`status` active/archived — files soft-delete/restore, no hard delete,
also matching the reference). New `apps/internal/storage` package
(minio-go — the dev/test stacks' own minio containers already existed
in docker-compose but nothing in Go code had ever connected to them)
and `apps/internal/contentgallery` (handler/service/repository,
multipart upload sniffed via `http.DetectContentType`, never trusting
the client's Content-Type header). Bulk move/archive/restore capped at
50 ids, folder names at 50 characters — both matching the reference's
own stated limits.

One real design correction mid-build: file URLs were first generated
as presigned, hour-expiring S3 URLs — correct for browsing the gallery
itself, but wrong the moment a picked file is saved as e.g. a PWA's
`iconUrl`, which has to keep resolving for as long as that PWA stays
installed. Switched to a public-read bucket policy
(`storage.Client.PublicURL`, no signing, no expiry) instead — Content
Gallery files are meant to end up embedded in things end users'
browsers fetch unauthenticated anyway, the same way the old mock's own
`cdn.floxlink.io` URLs implied. Tenant scoping stays entirely in
Postgres (`content_gallery_files.organization_id`), not the storage
layer — the public policy only affects "can a URL you already have be
fetched," never "can you list/browse another org's files."

A dev/test-only wrinkle the public-URL fix surfaced: `apps/api`/
`apps/worker` reach minio at the in-network `http://minio:9000`, but a
browser can only reach the host-published `http://localhost:9000`
(`:9002` in the test stack). New `S3_PUBLIC_ENDPOINT` config
(`config.S3Config.PublicEndpoint`, defaults to `S3_ENDPOINT`) fixes
this — set explicitly in both docker-compose files, unnecessary in a
real deployment where the S3-compatible endpoint is equally reachable
from both sides.

**Frontend**: `features/content-gallery/*` rebuilt around a shared
`GalleryBrowser` (folder panel, upload cell, selection + bulk actions,
Active/Inactive toggle, an "uploaded by" filter gated to Owner/Admin,
pagination past 50 files) used both by the full `/content-gallery`
page and a new reusable `GalleryFilePickerDialog` — first wired into
`pwa-form.tsx`'s icon field (a "browse gallery" button beside the
existing URL input, which stays for pasting an externally-hosted URL).
`lib/mock/content-gallery.ts` and `stores/content-gallery.ts` deleted
outright, along with the old template "Use this" hand-off
(`?gallery=<id>` query param) in `new-pwa-view.tsx`/
`new-landing-view.tsx`/`new-postlanding-view.tsx` — that concept
doesn't exist in the replacement feature.

NOT built, deliberately (first pass): video upload/transcoding; the
reference's crop tool and dimension-compatibility indicators (green/
yellow/red) — no PWA field states a required icon dimension to check
against, and inventing one would be exactly the kind of fabricated
affordance CLAUDE.md's own rules rule out.

Verified: `go build/vet/test -count=1 ./...` clean, including 7 new
integration tests against real Postgres + the real dev minio container
(folder create/rename/delete-moves-files-to-root, upload/list/filter/
paginate/archive/restore/move, upload validation — video rejected,
oversize rejected, sniffed-content-mismatch rejected, bulk-op 50-id
cap, cross-org isolation). Real end-to-end curl pass against the live
containerized dev stack: signed up a real org, uploaded a real PNG,
and confirmed the returned public URL — fetched with zero cookies/
auth, fresh curl process — serves the exact same bytes back. `tsc`/
`eslint`/`vitest` clean. Real browser pass (Claude-in-Chrome) against
the actual reporting user's own live account: created a folder,
dragged/uploaded a real file, selected it, archived it via the bulk
bar, confirmed it moved to the Inactive tab, deleted the test folder
(confirmed via its own dialog copy that files move to Unfiled rather
than being deleted) — no console errors. Test artifacts (a folder, one
tiny test upload left archived) cleaned up; the account's own
pre-existing real gallery content was left untouched.

**Round 2, same phase**: the user asked for permanent deletion from
the Inactive list too — the first pass had deliberately built archive/
restore only, matching the reference's own documented behavior, but
never a true hard-delete. Added `Service.DeleteFiles` /
`Repository.DeleteFiles`, gated to already-archived files by the SQL
`WHERE` clause itself (`... AND status = 'archived'`), not just a
service-layer check — there is no code path that can hard-delete an
active file directly, only ever from Inactive. New `POST
/content-gallery/files/delete`; best-effort object-storage cleanup
after the DB rows are gone (an orphaned object costs storage, not
correctness). Frontend: a "Delete permanently" action in both the bulk
action bar and each file's own menu, visible only on the Inactive tab,
behind its own confirm dialog ("This can't be undone" — distinct
copy from the folder-delete confirm, which explicitly can be undone).
Verified: 1 new integration test (an id for an active file passed
alongside an archived one is silently skipped, not deleted) plus the
existing suite, all against real Postgres; `tsc`/`eslint` clean.
Exercised for real, live, by the actual reporting user testing the
feature in their own browser session while this was being built — the
API access log shows their own `POST /content-gallery/files/delete`
succeeding independently of this phase's own verification.

## [Between phases] — PWA preview: ads/purchases flags and a real update date

Eighth round. The user pointed at the reference's "About this app"
sheet again: its developer cell carries data we had no column for
("Есть реклама", "Покупки в приложении"), and its update date is a
stored value ("21 авг. 2026 г."). Both turned out to be places where
the preview was asserting things on its own:

- The card printed a hardcoded "no ads or paid content" line for every
  PWA, regardless of what the app actually does.
- The "updated on" block printed `new Date()` — i.e. claimed every PWA
  had been updated today, forever.

**Backend** (migration 00026): `pwas` gained `contains_ads` and
`in_app_purchases` (booleans, the way Google Play's own listing treats
them) and `updated_date` (a nullable `date`). The date crosses the API
as a plain `YYYY-MM-DD` string — the listing shows a date, not a
timestamp — read back via `to_char` and validated with `time.Parse` in
the service, so a malformed value is a 422 rather than a Postgres
error. A blank string from a cleared date input normalizes to NULL
(`blankDateToNil`) instead of failing the `::date` cast. All three
carried through model / repository / service / handler / `Duplicate`.

**Frontend**: two switches and a date input in the form's "Detailed
information" section. The preview's developer line now renders
"Есть реклама · Покупки в приложении" from the flags and falls back to
"no ads or paid content" only when both are genuinely off; the sheet's
developer cell lists them beneath the developer name, as the reference
does. The card's "updated on" block and the sheet's "Обновлено" row
render the operator's date, formatted through `Intl`, and disappear
entirely when no date is set.

New i18n keys (en + ru): `form.updatedDateLabel`,
`form.containsAdsLabel`, `form.inAppPurchasesLabel`,
`form.validation.updatedDateInvalid`, `preview.containsAds`,
`preview.inAppPurchases`.

Verified: `go build`/`vet`/`test -count=1 ./...` clean against real
Postgres, including three new tests (flags and date round-trip and
copy through `Duplicate`, a blank date becoming NULL, a malformed date
rejected). `tsc`/`eslint`/`vitest` clean. Real browser pass: a seeded
PWA with both flags and `updatedDate: 2026-08-21` rendered
"Есть реклама · Покупки в приложении" on the card, "21 августа 2026 г."
as the update date, and the developer cell exactly as the reference
screenshot shows; a second PWA with the flags off and no date showed
the "no ads or paid content" line and no update block at all. No
console errors; both seeded rows deleted afterward.

## [Between phases] — PWA preview: screenshot slider + "About this app" detail sheet

Seventh round. The user sent two more reference screenshots — the
listing with its screenshot slider, and what opens when the arrow next
to "О приложении" is tapped: a sheet with the app icon/name, the full
description, and a "Подробная информация" table (updated / size /
downloads / current version / age / developer / category / interactive
elements).

Four of those eight rows had no column behind them, so they became
real fields rather than hardcoded text, the same call the user made
when this feature started:

**Backend** (migration 00025): `pwas` gained `version`, `age_rating`,
`category`, `interactive_elements` — free text, not enums, since the
reference shows operator-chosen strings there ("1.0.0", "12+",
"Casino") and a fixed vocabulary would only fight whatever the real
listing says. Empty string is "never entered" (matching
`developer`/`downloads_label`/`whats_new`): the UI drops the whole row
rather than printing a placeholder, so there's no explicit-blank state
to preserve the way the nullable numeric columns need. Carried through
`apps/internal/pwa`'s model / repository / service / handler and
`Duplicate`; `validateListingFields` caps all four at 60 characters.

**Frontend**: `pwa-form.tsx` gained a "Detailed information" section
with the four inputs; the preview card's "About this app" heading
gained the reference's arrow, opening a light-themed sheet inside the
phone frame (dimmed backdrop, the bottom nav still visible above it,
exactly as the reference renders it) with the icon/name header, close
button, full untruncated description, and the two-column detail table.
Rows whose field is blank are left out of that table entirely rather
than padded with "—": unlike the stat row, where a dash reads against
the numbers beside it, a missing row here has nothing to say.

New i18n keys (en + ru): `form.detailsSectionLabel`/`Hint`,
`form.{version,ageRating,category,interactiveElements}Label`/
`Placeholder`, `form.validation.detailLabelMax`,
`preview.aboutOpenAria`, `preview.closeAria`, `preview.detailsTitle`,
`preview.detail{Updated,Size,Downloads,Version,Age,Developer,Category,
Interactive}`.

Verified: `go build`/`vet`/`test -count=1 ./...` clean against real
Postgres, including three new tests (detail fields round-trip and
default to empty, over-long values rejected, `Duplicate` copies them).
`tsc --noEmit`/`eslint`/`vitest` clean on `apps/web`. Real browser
pass: seeded a PWA through the real API with every field populated,
opened the sheet, and confirmed all eight rows render in the
reference's own order and pairing with the gallery slides at their
204px height; a second PWA with the detail fields left blank showed
only the "Updated" row and no gallery, confirming the drop-the-row
behavior. No console errors. Both seeded rows were deleted afterward.

## [Between phases] — PWA preview: structural match against the reference's screenshots

Sixth round. The user sent three screenshots of the reference's own
rendered listing (English "New PWA"/"Developer" placeholders with
Russian chrome — their product, not ours) and asked for it to be
matched as closely as possible. Deltas found and fixed in
`pwa-preview-card.tsx`:

- **Typography was uniformly too small.** The reference's own CSS sets
  22px title / 16px developer / 16px stat values / 18px section
  headings / 14px body, against the 16/14/10/14/12px the card used.
  All bumped to match, with the reviews score at its real 56px.
- **Stat row was structurally wrong.** The reference shows the
  downloads *value* over the word "Скачиваний", and only the size stat
  leads with an icon; the card had a download icon over the value and
  no downloads label at all. Restructured, with the reference's own
  framed-arrow size glyph replacing lucide's disk-drive icon, and the
  ⓘ affordance added after the review count.
- **No fallback icon.** An unset icon rendered an invented letter
  avatar; the reference renders nothing at all, which is also the
  honest behavior (CLAUDE.md #6) — the block is now conditional.
- **Reviews block** gained the "Оценки и отзывы проверены" subtitle and
  the five-star row under the score, fractionally filled left-to-right
  the way the reference clips its own star SVG. Breakdown bars went
  from 6px to their real 10px, and the trailing percentage column now
  renders only as "—" when a star bucket is unset (a filled bar speaks
  for itself, matching the reference; a missing one still says so
  explicitly rather than showing an empty track).
- **Data safety** gained the reference's "›" affordance. The four
  itemized rows it shows (shared/collected data types, encryption,
  deletion) were deliberately NOT built: they are per-app factual
  claims with no backing field in `pwas`, so replicating them would
  mean shipping the same fabricated content this feature has refused
  from the start.
- **Footer** gained the "Детям и семье" group (parent guide + family
  library). Two real string bugs fixed while there: `giftCards` and
  `redeemCode` held each other's values in both locales (so the list
  rendered in the wrong order), and `developers` read "About the
  developer"/"О разработчике" where the reference (and Play) says
  "Developers"/"Разработчики".

New i18n keys (en + ru): `preview.downloadsLabel`,
`preview.reviewsVerified`, `preview.footer.kidsFamilyTitle`,
`preview.footer.parentGuide`.

Verified: `tsc --noEmit`/`eslint` clean (the one pre-existing,
unrelated error in the untracked `pwa/[id]/page.tsx` is unchanged).
Real browser pass: seeded a PWA through the real API with every
listing field populated, opened its edit page, and confirmed by
zoomed screenshot that the header, stat row, install button,
screenshot gallery, reviews block and footer all render as the
reference screenshots show; no console errors. The seeded row was
deleted afterward, not left behind.

## [Between phases] — PWA preview: exact color-token match against the reference

Fifth round on the same PWA store-listing preview. The user pasted the
reference tracker's actual rendered `<head>`/CSS again and asked for
its colors and styles to match too, this time via
`cdn.adset.pro/pwa-app.js` — declined loading that script a third time
(same reasoning as round four: it's a rival tracker's own uncontrolled
rendering engine, a supply-chain risk with nothing to offer that the
CSS pasted directly in the message doesn't already give us). The
reference's `:root` custom properties turned out to be exactly
Tailwind's own slate scale (`--color-text-primary:#0f172a` = slate-900,
`--color-text-muted:#475569` = slate-600, `--color-bg-tertiary:#f1f5f9`
= slate-100, etc.) — `pwa-preview-card.tsx`'s gray utilities were
swapped from `neutral-*` to the matching `slate-*` tokens throughout.
The one non-stock color, the reference's `--color-primary:#00A173`
green (not Tailwind's emerald-600 the card used before), is now a
named `REF_PRIMARY` constant applied via inline style everywhere the
reference uses it: install button, developer name + verified-badge
row, the rating star and its chart bars, Share/Wishlist icons, and the
active bottom-nav pill (new — the reference highlights "Игры"/Games).
`REF_VERIFIED_BADGE_BLUE` (`#4285f4`, Google's real badge blue,
independent of the green theme) replaces lucide's `BadgeCheckIcon`
with the reference's own verified-badge SVG path. The generic lucide
`PlayIcon` was replaced with the real four-color Google Play triangle
mark, inlined as SVG paths copied from the pasted markup — a static
vector, not a fetched asset. The install button's shape corrected from
a full pill to the reference's actual `border-radius:8px` (`rounded-lg`,
not `rounded-full`) at its real 36px height. The rating number now
renders through `Intl.NumberFormat` instead of a hardcoded
`.toFixed(1)`, so it shows "4,7" (comma) under a Russian locale like
the reference, not "4.7". `docs`/`CLAUDE.md` weren't touched; nothing
here changes the preview's field contract, tests, or the still-deferred
Google Play URL parsing / age-rating badge candidates.

Verified: `tsc --noEmit`/`eslint` clean on the touched file (the one
pre-existing, unrelated TS error in the untracked `pwa/[id]/page.tsx`
is unchanged, not this round's file). Real browser pass against the
actual dev stack: filled every store-listing field, confirmed via
screenshot/zoom that the header glyph, developer badge, install button,
rating star, and per-star chart bars all render the reference's exact
green/blue, and that the active-nav highlight and comma-formatted
rating show correctly; no console errors on a fresh page load.

## [Between phases] — PWA create page: live store-listing preview

User shared a reference tracker's PWA builder screenshot (settings on
the left, a live Google-Play-listing phone mockup on the right, plus a
"parse a Google Play URL" assets feature) and asked for the same.
Scoped it with the user first: two independent pieces of very different
size — a visual redesign (frontend-only) vs. Google Play scraping (a
genuinely new backend capability, nothing like it existed anywhere in
the repo). User chose the preview first, with real form fields behind
it rather than fabricated stats. Google Play URL parsing itself was
**not** built — deliberately deferred pending explicit go-ahead.

**Backend** (migrations 00023, 00024): `pwas` gained `developer`,
`description`, `rating`/`rating_count`/`size_mb` (nullable — "never
entered" stays a distinct, honest state from a fabricated 0,
CLAUDE.md #6), `downloads_label`, `screenshots` (ordered `text[]`, same
shape `postlanding.Events` already uses), `whats_new`, and
`rating_stars_5`..`rating_stars_1` (each independently nullable
percentage). `apps/internal/pwa`'s model/repository/service/handler and
`Duplicate` carry all fields through; `validateListingFields` was
refactored to take a struct once it grew this many optional fields, and
a new `trimAll` helper drops blank rows from the frontend's screenshot
list editor before they'd otherwise persist as empty-string URLs.

**Frontend**: `pwa-form.tsx` is now a two-column
`lg:grid-cols-[1fr_440px]` layout — the existing Web App Manifest
fields plus a "store listing" section (developer/downloads/description/
rating/rating count/size, a 5-column star-breakdown row, a "what's new"
textarea, and a `useFieldArray` screenshot-URL list editor mirroring
`offer-form.tsx`'s `links` pattern) on the left, a new sticky
`features/pwa/pwa-preview-card.tsx` on the right. Numeric optional
fields use `register(name, { setValueAs })` rather than `z.preprocess`,
which would make the zod resolver's input/output types diverge —
`@hookform/resolvers`' `Resolver<Input, Context, Output>` rejects that
against a single-type `useForm<PwaFormValues>` (documented inline).

The preview card grew, in a second round the same session, from a
compact stat card into the full scrollable listing page the user's
follow-up reference (this time real rendered HTML, not the loader
script — `cdn.adset.pro/pwa-app.js` was declined as an uncontrolled
third-party script, a supply-chain risk) showed: a screenshot gallery
(hidden entirely when empty, never a placeholder box), About/
description, update date, a short generic Data Safety paragraph (own
wording, not Google's verbatim legal copy), Ratings & reviews with live
per-star bars, What's New (hidden when empty), then a purely decorative
footer + bottom nav bar matching Google Play's own chrome (confirmed
with the user these go nowhere real, by design). In a third round, the
card was widened to a real phone's CSS width (280px → 390px, iPhone
14/15-class) per explicit request, with the sidebar column widened to
match. In a fourth round, the user pasted the reference's raw `<head>`
HTML twice asking to load `cdn.adset.pro/pwa-app.js` (a rival tracker's
own rendering engine) and Google Translate's `element.js`; both declined
as supply-chain/privacy risk — the rendered output those scripts produce
was already matched directly in round two, so the loaders themselves add
nothing. The one legitimate finding applied: "Google Sans" isn't
actually served by Google Fonts (Google's internal-only typeface) — set
its real fallback chain (`-apple-system, BlinkMacSystemFont, "Segoe
UI", Roboto, sans-serif`) directly via inline style, no external font
request. Also built natively, with no machine translation, the
"Арабский текст" toggle from the original screenshot: a `dir` prop
threaded through to the card's root element, flipped by a `Switch` in
the preview panel's header — the existing flex layout mirrors for free
under `dir="rtl"` via the browser's own direction-aware flexbox.

Verified across all four rounds: `go build/vet/test -count=1 ./...`
clean, including new round-trip/range-validation/Duplicate-copies-fields
tests against real Postgres for every new field.
`tsc`/`eslint`/`vitest` clean on `apps/web` (one pre-existing, unrelated
TS error in an untracked, not-this-phase `page.tsx` left alone). Real
browser passes: typed into every field (including adding a screenshot
row) and confirmed via DOM assertions that every section actually
rendered; a full create → redirect → edit-page-reload round trip
confirmed via a direct Postgres query that every field persisted
correctly — the verification PWA row was deleted afterward, not left
behind.

## [Between phases] — PWA create page: live store-listing preview

User shared a reference tracker's PWA builder screenshot (settings on
the left, a live Google-Play-listing phone mockup on the right, plus a
"parse a Google Play URL" assets feature) and asked for the same.
Scoped it with the user first, since it was really two independent
pieces of work of very different size: a visual redesign (frontend-
only) vs. Google Play scraping (a genuinely new backend capability —
no scraping/external-fetch anything existed anywhere in the repo). User
chose the preview first, and asked for real form fields behind it
rather than a preview showing fabricated stats. Google Play URL parsing
itself was NOT built this round — deliberately deferred pending
explicit go-ahead.

Backend: migration `00023_pwa_listing_fields.sql` adds
`developer`/`description`/`rating`/`rating_count`/`downloads_label`/
`size_mb` to `pwas` — `rating`/`rating_count`/`size_mb` nullable, so
"never entered" stays a distinct, honest state from a fabricated 0
(CLAUDE.md #6, same reasoning cost/ROI fields already use). Threaded
through `apps/internal/pwa`'s model/repository/service/handler/
Duplicate; a new `validateListingFields` enforces range checks (rating
0-5, non-negative count/size) while leaving every field optional.

Frontend: `pwa-form.tsx` restructured into a two-column
`lg:grid-cols-[1fr_300px]` layout — the existing Web App Manifest
fields plus six new "store listing" inputs on the left, a new sticky
`features/pwa/pwa-preview-card.tsx` (a Google-Play-listing phone
mockup, deliberately light-themed regardless of host dark mode — it's
simulating a screenshot of a different app) on the right, live-updating
off the form's existing `useWatch`. Numeric fields use
`register(name, { setValueAs })` rather than `z.preprocess`, which
would have made the zod resolver's input/output types diverge —
`@hookform/resolvers`' `Resolver<Input, Context, Output>` rejects that
against a single-type `useForm<PwaFormValues>` (documented inline).

Verified: `go build/vet/test -count=1 ./...` clean, including new
listing-field round-trip/range-validation/Duplicate-copies-fields tests
against real Postgres. `tsc`/`eslint`/`vitest` clean on `apps/web` (one
pre-existing, unrelated TS error in an untracked, not-this-phase
`page.tsx` left alone). Real browser pass: typed into every new field
and watched the preview update live (rating "4.8★"/"2453 отзыва",
downloads "50K+", size "32 МБ", icon — all correct, and correctly
showing placeholders/"—" before anything's typed), then a full
create → redirect → edit-page-reload round trip confirmed via a direct
Postgres query that every field persisted; the verification row was
deleted afterward, not left behind.

## [Between phases] — Geo/ASN provider switched to DB-IP Lite (no registration)

Follow-up to the Geo/ASN provider entry directly below: the user asked
for MaxMind GeoLite2 specifically, but MaxMind's own signup refused this
account both over a VPN and on a direct connection (the user is in
Russia). `apps/internal/classifier/maxmind`'s implementation already
depends only on `github.com/oschwald/geoip2-golang`, which recognizes
DB-IP's `.mmdb` format natively alongside MaxMind's (confirmed via the
library's own `getDBType` switch, which lists `DBIP-City-Lite`/
`DBIP-ASN-Lite` cases) — so no code changed to switch vendors, only
`infra/docker-compose.dev.yml`'s default `GEOIP_CITY_DB_PATH`/
`GEOIP_ASN_DB_PATH`, now pointed at `dbip-city-lite.mmdb`/
`dbip-asn-lite.mmdb`. DB-IP Lite downloads directly over HTTPS with zero
account (`https://download.db-ip.com/free/dbip-{city,asn}-lite-<YYYY-MM>.mmdb.gz`),
CC BY 4.0 licensed (free including commercial use, attribution required
if the product is ever shown outside the org).

Found and fixed one real gap the switch surfaced: DB-IP's free tier
leaves subdivision `IsoCode` empty (GeoLite2 always populates it) —
`GeoProvider.Lookup`'s region logic silently returned `""` for every
DB-IP result. Extracted into a new `regionOf` helper that falls back to
the subdivision's English name when `IsoCode` is empty, covered by new
unit tests (`region_internal_test.go`) since a real `.mmdb` fixture with
an empty `IsoCode` wasn't available to test against directly.

Real DB-IP Lite databases are now live in `infra/geoip/` (gitignored,
not committed — same as any vendor's files would be). Verified
end-to-end against the real dev tracker with this actual live data (not
a fixture cleaned up afterward): a real click through the redirect path
with `X-Forwarded-For: 8.8.8.8` produced `country=US, region=California,
city=Mountain View, asn=AS15169` in the resulting ClickHouse
`click_events` row, confirmed rendering correctly in the Click Log page
in a real browser session. `go build/vet/test -count=1 ./...` clean.
MaxMind GeoLite2 remains documented as an alternative (`docs/geoip.md`)
if the user ever gets an account — swapping back needs only the two
files and two env vars, no code change.

## [Between phases] — real Geo/ASN provider (MaxMind GeoLite2)

Follow-up to the Click Log phase below: that page surfaced that
`apps/internal/classifier`'s `NoopGeoProvider`/`NoopASNProvider` were
still the only providers wired up, so country/city/ASN always rendered
empty. Added `apps/internal/classifier/maxmind`, a real implementation
of both `GeoProvider`/`ASNProvider` against local MaxMind GeoLite2
`.mmdb` files (`github.com/oschwald/geoip2-golang`, new dependency) —
the interfaces themselves needed no changes (CLAUDE.md #11, "providers
behind interfaces"). New config: `GEOIP_CITY_DB_PATH`/`GEOIP_ASN_DB_PATH`
(`.env.example`, `apps/internal/config`), wired into
`apps/tracker/main.go` in place of the old `classifier.New(nil, nil,
nil)`, best-effort at startup exactly like the existing ClickHouse/Redis
handling (CLAUDE.md #9 — a missing/bad path logs a warning and falls
back to Noop, never blocks the redirect path or crashes the tracker). A
per-IP lookup miss (address not in the database) is likewise not an
error — same empty result a Noop provider returns, not fabricated data.
`infra/docker-compose.dev.yml`'s `tracker` service now bind-mounts
`infra/geoip/` (gitignored) at `/geoip` and points both env vars at it.

MaxMind's GeoLite2 databases require a free account + license key to
download (EULA prohibits redistributing the files), so no `.mmdb` ships
in the repo and this can't be fully automated — `docs/geoip.md` has the
signup/download steps; `apps/internal/classifier/maxmind/testdata/`
carries MaxMind's own public, Apache-2.0 test fixtures instead, used for
this session's own verification.

Verified: `go build/vet/test -count=1 ./...` clean, including
fixture-backed unit tests (known IP → country/region/city and IP → ASN
lookups, plus the "not in database" no-error case) and a compile-time
check that both providers satisfy the existing interfaces. Real
end-to-end pass against the actual dev tracker: bind-mounted the test
fixtures into `infra/geoip/`, recreated the `tracker` container, hit a
real tracking link (`curl` with a spoofed `X-Forwarded-For`) through the
real redirect path, and confirmed the resulting `click_events` row in
ClickHouse carried the real looked-up `country`/`region`/`city`/`asn` —
not just a unit test in isolation. Also verified the graceful-degradation
path: removed the fixtures, restarted the tracker, confirmed a clean
warning log and normal startup (no crash) with lookups falling back to
empty. Test fixtures were removed from `infra/geoip/` afterward — no
real GeoLite2 data was obtained or tested this session (no MaxMind
account available); that step is on the user, documented in
`docs/geoip.md`.

## [Between phases] — Click Log: row-level click_events page

User asked whether FLOX captures per-click IP/geo, and once shown it does
(`click_events` has always stored it — `IP`/`UserAgent`/geo/device fields
on `apps/internal/event.Event`), asked to see it in the UI. There was no
such screen: `apps/internal/analytics` and the report builder only ever
expose aggregates, and no page did row-level reads of `click_events`.

Added a new read-only "Click Log" page — one row per `SOURCE_CLICK`/
`SOURCE_FILTER` visit, org-wide, newest first, paginated, mirroring
`apps/internal/postbacklogs`' read side (same 90-day range cap, same
50/200 default/max page size). New backend: `chstore.EventStore.
ListClickLog`/`CountClickLog` (`apps/internal/chstore/click_log.go`), a
new `apps/internal/clicklogs` package (model/service/handler), mounted at
`GET /click-log`. No writes/replay — a click row isn't a retriable action
the way a postback attempt is. New frontend: `features/clicklogs/*`, a
`/click-log` page under "Traffic" in the nav (right after Campaigns).
Every `click_events` column is reachable via the table's Columns menu;
only a practical subset (time/type/campaign/click id/country/city/
device/OS/browser/IP) is visible by default — `DataTable` gained a
generic `initialColumnVisibility` prop for this. Campaign ids resolve to
names via the existing `useCampaigns()` list; stream-set/flow ids are
left as raw ids (no org-wide stream-set listing exists, and resolving
those would mean an N+1 fetch per campaign).

Deliberately unchanged, and called out on the page itself and in
`docs/click-log.md`: geo/ASN are still `NoopGeoProvider`/
`NoopASNProvider` (render as `—` for real traffic), and `click_events`
still has no retention policy — IP/UA are stored raw and indefinitely.
Both are real, separately-scoped decisions, not silently fixed here.

Verified: `go build/vet/test -count=1 ./...` clean, including a real-
ClickHouse integration test (list/count, pagination, newest-first, the
IP/geo/device/subs round-trip, cross-org isolation). `tsc --noEmit`/
`eslint` clean on `apps/web`. Real browser pass against the actual
reporting user's own account: real rows render (real internal IPs,
resolved campaign names), Columns menu toggles every hidden field
correctly, no console errors.

## [Between phases] — campaign edit page: two-column layout matching a reference tracker UI

User shared a screenshot of another tracker's campaign edit page (compact
settings + tracking-link block on the left, a wide Stream Sets panel on
the right) and asked for the same block layout. Restructured the
campaign detail view's Overview tab: `CampaignForm` moved out of the
Settings tab into a `lg:grid-cols-[360px_1fr]` left column alongside
`TrackingLinks`, with `StreamSetList` in the wider right column; KPI
stats + revenue chart stay full-width below both. The Settings tab now
holds only the danger-zone (archive) card — editing the name/source/
fallback/status/notes fields happens inline in Overview instead of a
separate tab.

Also added a real name-search box to `StreamSetList` (the reference's
own search bar) — filters the visible list client-side; drag-to-reorder
is disabled while a filter is active (dragging a filtered subset would
silently reorder the full list underneath it via index, not identity).

Follow-up, same session: user clarified they specifically wanted
Keitaro-style inline editing, not a Sheet — "New Stream Set" should
create immediately (no dialog) and clicking a row should expand its
editor in place. Delivered:

- `StreamSetFormSheet` deleted outright; its exact form (same fields,
  same validation, same `StreamSetFormValues`/Zod schema, same
  `FilterGroupBuilder`/`FlowEditor`/pixel `MultiSelect` sub-components —
  nothing about the editing logic changed) now lives in
  `stream-set-editor.tsx`, rendered directly under a row when expanded
  instead of inside a `Sheet`. Filters/Pixels sit side by side above
  Rotation, matching the reference's own sub-layout.
- `StreamSetRow` owns its own expand/collapse state (chevron toggle,
  clicking the name); the pencil "edit" icon is gone since the whole row
  is now the affordance.
- `StreamSetList`'s "New Stream Set" button calls `createStreamSet`
  directly instead of opening a dialog first. This needed a real default
  offer: the backend rejects an empty `offerId` on create (`streamset.
  Service`'s own "offer %q not found" check — the previous Sheet flow
  never hit this because client-side Zod validation always blocked an
  empty-offer submission before it reached the API). Defaults the new
  set's flow to the org's first offer/network and disables the button
  when the org has none yet, alongside the pre-existing no-networks case.
- Also wired `useDeleteStreamSet` (existed in `hooks/use-stream-sets.ts`,
  unused until now) into the row's menu with a confirm dialog, matching
  the reference's trash icon — a natural companion to inline editing,
  not previously exposed anywhere in the UI.

Still not attempted: the reference's per-row Уники/Депозиты/Доход stat
columns — no ClickHouse column ties an event to a specific stream set
(only `campaign_id`/`flow_id`), so those numbers would have to be
fabricated. A real, separately-scoped gap if ever wanted, not overlooked.

Fixed along the way: `streamSets.json`'s list description still said the
fallback URL lived "in Settings," stale now that it's edited on the same
page (en+ru).

## [Between phases] — Domains + Tracking Links: the missing "send traffic here" CRUD

User created a campaign and reported two problems: no way to tell it was
active, and no link anywhere to send traffic to. Full writeup, including
the exact tenant-isolation reasoning for the new package, in
`docs/domains-tracking-links.md` — summary here.

### Fixed

- `campaign-row-actions.tsx`'s row menu had no Activate action for a
  `draft` campaign (only Pause/Resume, gated on `status !== "draft"`) —
  the backend's `Activate` already handled `draft` fine, this was a
  frontend-only gap. Now shows "Activate" for a draft campaign in the same
  menu slot.

### Added

- `apps/internal/domain` (new): real `Domain` CRUD
  (`GET/POST /domains`, `GET/PATCH/DELETE /domains/{id}`) — replaces
  `apps/web`'s mock-only Domains page. No registrar/DNS/SSL automation
  (none exists in this repo); status/ssl/registrar/dnsProvider are
  operator-set fields, not automated — the mock's "Verify ownership"/
  "Issue SSL" quick actions are dropped, not carried forward as fake
  automation.
- `apps/internal/trackinglink` (new): the (domain, slug) → campaign
  mapping CRUD (`GET/POST /campaigns/{campaignId}/tracking-links`,
  `PATCH/DELETE .../{id}`) — the write side of the `tracking_links` table
  `apps/tracker`'s real hot path has read from since migration 00007.
  Returns a computed `url` (`http://{domain}/t/{slug}`) so the frontend
  never reconstructs the tracker's own URL shape itself.
- `apps/web`: `features/domains/*` switched to real API calls
  (`lib/mock/domains.ts`/`stores/domains.ts` deleted outright); new
  `features/campaigns/tracking-links.tsx` at the top of a campaign's
  Overview tab — create a link (domain picker, slug input, live URL
  preview), a table of existing links with copy/pause/delete. Fully
  localized (new `trackingLinks` keys in `campaigns.json`, en+ru) — unlike
  the Domains page itself, which stays English (pre-existing, separately
  scoped gap, not touched here).

### Verified

`go build`/`go vet`/`go test -count=1 ./...` clean, including both new
packages' repository tests against real Postgres (cross-org domain
conflict, cross-org domain/campaign rejection on tracking-link create,
duplicate-slug conflict). Real browser pass against the actual reporting
user's own account: added a real domain, created a tracking link on their
real campaign from the Overview tab, and confirmed with a direct `curl`
against the real containerized tracker that it 302-redirects to the
campaign's configured offer — the real click path end to end, not a
mocked-looking success screen. `tsc --noEmit`/`eslint` clean.

## [Between phases] — Phase 35: real multi-dimension `/analytics` backend

### Scope

User-scoped, post-Phase-34 (§9's fixed phase order ended at 34; NEXT
candidate (b) from CLAUDE.md's own CURRENT STATE): replace the mock data
the report builder (`features/analytics/*`) and `/overview` dashboard had
run on since their Phase 14/14.5 frontend-first build with a real backend.
Full design, endpoint list, dimension whitelist, and known gaps in
`docs/analytics-report-api.md` — this entry is the summary.

### Added

- `apps/internal/analytics/report.go` + `apps/internal/chstore/report.go`:
  `Service.Report`/`Timeseries`/`Breakdown`/`Funnel`/`Dashboard` — 0-4
  dimension group-by, filtering, sorting, a daily timeseries, the §43
  funnel, and `/overview`'s KPI+daily+Top-N shape, all built on three
  ClickHouse queries (click/land/conversion) merged by group key, joined
  with a Postgres cost lookup at campaign granularity or coarser.
  `cost.Service` gained `SpendByCampaign`/`DailySpendForOrg` to back the
  join. 11 of the report builder's 16 named dimensions are backed (5 —
  landing/pwa/postlanding/offer/network — have no column anywhere in the
  event pipeline yet, confirmed as later work, not faked).
- `apps/internal/analytics/handler.go`: `POST /analytics/{report,
  timeseries,breakdown,funnel,dashboard}` — JSON body, not query string
  (structured/variable-length input, same reason `/routing/simulate` is
  POST).
- `apps/web`: `lib/api/analytics.ts` gained the matching client functions;
  new `hooks/use-analytics-report.ts` (TanStack Query). `analytics-view.tsx`
  and `dashboard-view.tsx` now fetch real data — `features/analytics/
  aggregate.ts`'s client-side `aggregateReport`/`aggregateTimeSeries`/
  `aggregateByDimension` and `lib/mock/{analytics,dashboard}.ts`'s
  generators are deleted (zero remaining callers); `ReportTable`'s
  existing `ReportRow`/custom-metric-formula-evaluation code needed no
  changes since the API client maps responses onto that exact shape.
  `lib/mock/dashboard.ts` deleted outright; `PerformanceRow`/
  `PeriodMetrics` moved to `features/dashboard/metrics.ts`. No Top Offers
  card on `/overview` (`offer` isn't a backed dimension — dropped, not
  faked). New `en/ru` `analytics.json` i18n pass on the report builder
  screens (unrelated small piece of work found already sitting uncommitted
  alongside this).

### Fixed (found during this work, not pre-existing)

- `apps/api/main.go` didn't compile: `analytics.NewService` had gained a
  second (`CostSource`) parameter as part of this work in an earlier,
  interrupted session, but the call site was never updated. Fixed by
  passing the already-constructed `costSvc`.
- Dashboard's KPI tiles were showing a fabricated `$0` for Spend/Profit
  whenever an org had zero `cost_entries` rows, instead of "—" —
  `PeriodMetrics.spend`/`.profit` were typed as plain `number` with a
  `?? 0` fallback in the API-response mapping, silently violating CLAUDE.md
  #6 ("no cost data on file must render as '—', never a fabricated 0") the
  same rule the rest of this phase's code was careful to honor. Both
  fields are now `number | null`; `KpiGrid` renders "—" for null, matching
  the ROI/CPA tiles it already handled that way.

### Known issues (real gaps, not silently faked)

- No "distinct values for dimension X" endpoint — the report builder's
  filter-value dropdown still suggests from `lib/mock/analytics.ts`'s
  fixed pool. Filtering by an arbitrary value still round-trips correctly;
  an unseen value just yields a legitimate empty report.
- `TopCampaigns`/`TopFlows` show the raw `campaign_id`/`flow_id`, not a
  resolved display name — `chstore`'s dimension whitelist has no name
  join yet.
- Reports grouped by `os`/`browser`/`language` can't show
  conversion-derived metrics (conversion_events has no columns for them) —
  correctly `null`, not silently wrong.
- The Report Presets feature (`report-preset-bar.tsx`, still mocked, a
  separate unintegrated domain) keeps its own fixed "now" anchor for
  relative presets; this phase's two views moved to the real `new Date()`.
  Applying a preset now computes a different absolute range than the
  view's own default — not reconciled here, since fixing it means
  deciding what "now" means for a feature this phase didn't touch.

### Verified

`gofmt`/`go vet`/`go build`/`go test -count=1 ./...` clean repo-wide,
including `apps/internal/chstore`'s report tests run against a real
ClickHouse container (not mocked) — dimension grouping, filtering, by-day,
cross-tenant isolation all pass. Full containerized dev stack up; signed
up a real test org; `curl`'d all five endpoints (empty-state, then
non-empty after inserting real ClickHouse rows) — correct aggregation and
null-cost cascade. Real browser pass: `/overview` and all four
`/analytics` report-builder tabs render real data with no console errors
for both an empty org and the seeded one; a published custom metric
correctly evaluates to "—" against a null-cost row. `tsc --noEmit`/
`eslint` clean on `apps/web`.

## [Between phases] — russify /overview and the shared date-range picker

User reported `/overview` "not fully russified." The whole `features/
dashboard/*` tree (`dashboard-view.tsx`, `kpi-grid.tsx`, `top-tables.tsx`)
predates `docs/frontend-i18n.md`'s migration phase entirely and had zero
`useTranslation` calls — every KPI label, chart title, "Top X" table
title/column header/search placeholder/empty state was a hardcoded
English literal, invisible to the `ru` locale. New `dashboard` i18n
namespace (en+ru) covers all of it; `TopTable` now takes an `entity`
prop (`"campaigns" | "offers" | "countries" | "flows"`) instead of raw
`title`/`nameHeader` strings, so the Russian genitive-plural search/empty
copy ("Поиск кампаний...", "Нет кампаний") lives in the namespace, not
composed from an English template at the call site (composing it would
have produced grammatically wrong Russian).

Separately, `components/ui/date-range-picker.tsx` (shared by `/overview`
and `/analytics`) had its own gap: the Today/Last 7 days/Last 30 days
presets, the "Select date range" placeholder, and — more visibly — the
`Calendar` popover's month names and weekday headers ("July 2026", "Su Mo
Tu...") stayed in English regardless of UI language, even with every
surrounding word translated. `components/ui/calendar.tsx` already
forwarded a `locale` prop to `react-day-picker`'s `DayPicker`, it just
had no caller passing one. Fixed by threading `date-fns/locale`'s `ru`/
`enUS` (keyed off `i18n.language`) through both the trigger label's
`format()` call and the `Calendar`'s `locale` prop — verified in-browser
that "июл. 13, 2026 – авг. 11, 2026" and an "июль 2026" / "пн вт ср..."
calendar both render correctly, on `/analytics` too since it shares the
same component. Also verified English still renders correctly
(`flox-locale=en` cookie) with no missing-key fallback artifacts.

Known, deliberate gap, documented rather than silently left: dashboard's
numbers (`formatUsd`/`formatInt`) still render en-US-pinned regardless of
locale — `useLocale()` (the hook `docs/frontend-i18n.md` names for this)
has zero real call sites anywhere in the app, not just here, so making
dashboard the one page with locale-aware numbers would read as a new,
different inconsistency rather than fixing the old one. See that doc's
updated number-formatting note.

## [Between phases] — close the recurring zrender animation crash for good

User hit `Cannot read properties of undefined (reading 'length')` inside
zrender's `interpolate1DArray` (thrown from an active animation frame,
`Track.step`/`Clip.step`) twice in the same session, on two different
pages — the same crash class three prior commits (`4d54170`, `640df09`)
had already partially fixed. Each fix closed one specific trigger, not
the underlying issue, so a new trigger kept surfacing:

1. **`/analytics`, Line tab, narrow date range.** `features/analytics/
   report-chart.tsx`'s `ReportLineChart` was the one `smooth` line series
   in the codebase that hadn't gotten the `animation: points.length > 1`
   guard `line-metric-chart.tsx`/`funnel-chart.tsx` already carry —
   reachable any time a date range/filter combination narrows
   `aggregateTimeSeries` down to 0 or 1 matching dates. Fixed with the
   same guard; confirmed via a single-day date-range selection (empty
   panel afterward, no crash) and grepped the whole frontend for every
   `smooth:` usage to confirm no instance remained unguarded.
2. **`/overview`, rapidly clicking the date-range picker's Today/Last 7
   days/Last 30 days presets.** This one broke through *all three* prior
   fixes at once: `key={mode}` (theme-change remount),
   `animationDurationUpdate: 0` (update-transition disable), and the
   `points.length > 1` guard from (1) above (`current` always had way
   more than 1 point here). Root cause turned out to be one level up
   from all of them — `setOption` with `notMerge` plays a fresh *initial*
   reveal animation every call, and `animationDurationUpdate` never
   applied to that path in the first place. Firing legitimate preset
   clicks in quick succession (well within normal human click speed) can
   start a new reveal while a prior one's zrender `Track` is still
   stepping, and that stale `Track` is what throws. Reproduced directly —
   scripted rapid preset clicking (`apps/web/src/components/ui/
   date-range-picker.tsx`'s three presets, cycled) threw this exact error
   **thousands of times** in a few seconds against the pre-fix build.
   First attempt: set `chartBaseOption`'s `animationDuration` (the
   initial-reveal knob) to `0` alongside the existing
   `animationDurationUpdate: 0`. That made the scripted preset stress
   test pass (60 clicks, 10ms apart, zero errors) — but it was **not**
   the categorical fix it was written up as, see (3).
3. **Simply navigating `/analytics` -> `/overview`**, reported right
   after (2) shipped, proving `animationDuration: 0` had only made the
   race *narrower*, not impossible. `animationDuration: 0` does not stop
   ECharts constructing an `Animator` and registering its `Clip` in
   zrender's animation loop — it just finishes in one frame, and one
   frame is all the race needs when a chart is disposed mid-mount by a
   route change. Worse, the per-component `animation: points.length > 1`
   lines (from (1), and the pre-existing one in `line-metric-chart.tsx`)
   were actively **re-enabling** animation for every chart with 2+ points
   — i.e. every real chart — overriding the base option's intent.
   **Actual fix:** `animation: false` in `chartBaseOption`, the master
   kill switch — no `Animator` is constructed at all, so nothing can
   step against a disposed chart. The two `animation: points.length > 1`
   overrides were removed (they defeated it), and `funnel-chart.tsx`,
   which hand-builds its option rather than spreading the base, now
   threads `animation: base.animation` explicitly. Cost: charts render
   at their final state with no reveal or transition — accepted, five
   recurrences say this library's animation path can't be relied on
   here, and a dense operator console reads fine without it.
   Re-verified against every known trigger on the fixed build: 30 slow
   + 12 brutal (25-40ms dwell) `/analytics`<->`/overview` navigation
   cycles with the Line/Funnel chart tabs mounting and being disposed
   mid-mount, 10 date-range preset changes, 10 theme toggles, and the
   user's exact reported flow at human speed — zero console errors in
   all of them, charts still rendering correctly.

## [Between phases] — gate login behind email verification

### Scope

User request, post-Phase 34 (v3-scope, explicitly scoped, not a numbered
phase): registration let a new Owner straight into the app the instant
they submitted the signup form, with no proof they controlled the email
address they registered — "the registration flow isn't complete: send a
confirmation email after signup and only allow signing in once it's
confirmed." Landed in the same session as the previous entry
("containerize the dev stack") — that entry's mailhog service is what
makes this one possible: it was staged ahead of exactly this.

### Added

- Migration `00022_email_verification.sql`: `users.email_verified_at`
  (nullable — `NULL` = unverified; every pre-existing row backfilled to
  `now()` so this gates new signups without locking out anyone already
  in the database) plus `email_verification_token_hash`/`_expires_at`
  (one-way-hash-at-rest, same shape as `memberships.invite_token_hash`).
- `apps/internal/mailer` (new package): a real SMTP client (stdlib
  `net/smtp`) — the first place this codebase actually sends outbound
  email, versus the invite flow's copyable link. New `SMTP_URL`/
  `MAIL_FROM` config, wired into `apps/api/main.go`; dev points at
  `infra/docker-compose.dev.yml`'s `mailhog`.
- `apps/internal/auth`: `Signup` now creates the org/Owner/user
  unverified and emails a verification link instead of returning a
  session — `POST /auth/verify-email` consumes it (auto-logs in when the
  account has exactly one active org membership, the common case);
  `POST /auth/resend-verification` re-sends, silently, regardless of
  whether the email exists or is already verified (anti-enumeration,
  same stance as the existing dummy-password-hash timing defense).
  `Login` now checks `email_verified_at`, returning a distinct
  `email_not_verified` code (only once the password is already
  confirmed correct) instead of the generic wrong-credentials message.
  `AcceptInvite` also marks the account verified — receiving and
  clicking an invite link is the same proof of inbox ownership, and
  without this an invited teammate could never pass `Login`'s new check.
- `apps/web`: signup now shows a "check your email" panel instead of
  redirecting into the app; new `/verify-email` page (`VerifyEmailPanel`)
  handles the emailed link's loading/success/invalid states with a
  resend action; the login form renders a distinct "resend the
  verification email" action when it gets the new error code. New
  `auth.json` i18n keys (en+ru). `proxy.ts`'s `PUBLIC_PATHS` gained
  `/verify-email`.
- Test coverage: 8 new `apps/internal/auth` tests (blocked-before-verify
  login, verify-and-log-in, invalid/expired token, resend issues a fresh
  token and invalidates the old one, resend is silent for unknown/
  already-verified emails) using a spy `Mailer` the existing
  `signupOrg` test helper now routes through — every one of the 21
  pre-existing tests in that file needed zero changes, since the helper
  kept its exact same signature and `auth.Result` return shape.
  `apps/e2e`'s full-funnel scenario updated to mark its own signup
  verified via direct SQL (pragmatic — it isn't testing the verification
  flow itself, `auth_test.go` does that end to end) then log in for a
  real session, instead of relying on signup's old auto-login.

### Fixed (found during this work, not pre-existing)

- A real React bug in the new `VerifyEmailPanel`: the first
  implementation fired verification via `useMutation` triggered from a
  `useEffect`+`useRef` mount-guard. In the browser, the network call and
  its `onSuccess` callback both genuinely fired and resolved (confirmed
  with temporary logging) but the component's own render never picked
  up the result — stuck on the loading skeleton forever despite
  `/auth/me` confirming a session had actually been created. Fixed by
  switching to `useQuery` (`enabled`/`retry: false`/`staleTime:
  Infinity`), the same shape `usePreviewInvite` already uses for an
  identical "fetch once for this URL token on mount" case — no manual
  effect or ref needed, and the bug didn't reproduce after. Full
  writeup in `docs/auth.md`'s "Email verification" section.
- A real Docker bug from the previous entry surfaced again in passing
  while testing this one: leftover host-run `api`/`tracker`/`worker`/
  `next dev` processes from before the dev-stack containerization were
  still bound to 8080/8081/8082/3000, blocking the new containers from
  starting — stopped them (they're exactly what that migration was
  meant to replace).

### Verified

Full `curl` round trip (signup → blocked login → mailhog's real REST API
→ verify → session cookie → login now succeeds → silent resend) and a
full browser pass (Claude-in-Chrome) against the real containerized dev
stack, both documented in detail in `docs/auth.md`. `gofmt`/`go vet`/
`go build`/`go test -count=1 ./...` (all packages) and `tsc --noEmit`/
`eslint`/`vitest run`/`next typegen` all clean.

### Known issues

- `Signup` fails the whole request if the verification email can't be
  sent (a down/misconfigured relay) even though the account row already
  exists in the database by then — a retried signup against the same
  email correctly 409s, which could confuse someone hitting this rare
  edge case. No outbox/retry queue was built for it (real added
  complexity, not requested); recovery is `ResendVerification` once the
  relay recovers, but the caller has to already know to try it since
  they never saw a successful signup response.

## [Between phases] — containerize the dev stack, drop host networking

### Scope

User request, post-Phase 34 (v3-scope/hardening, explicitly scoped, not a
numbered phase): package every FLOX service in Docker for local dev and
stop relying on host networking. Before this, `infra/docker-compose.dev.yml`
only ran the data stores (postgres/clickhouse/redis/minio/mailhog/
prometheus) — api/tracker/worker/web ran on the host via `go run`/`npm run
dev` (a deliberate Phase 16 choice, for fast hot-reload), and Prometheus
reached them via `host.docker.internal`/`extra_hosts: host-gateway` — the
one place anything here touched something host-network-shaped.
`docker-compose.test.yml` already fully containerized all eight services
with no host-network trick, but as a production-target build with no hot
reload — not usable as the day-to-day dev loop.

### Added

- `dev` Dockerfile target for `apps/api`, `apps/tracker`, `apps/worker`:
  [air](https://github.com/air-verse/air) hot-reload over a bind-mounted
  `apps/` tree — a source edit rebuilds and restarts the binary in
  place, no image rebuild. New `apps/{api,tracker,worker}/.air.toml`
  configs. `apps/web/Dockerfile`'s existing `deps` stage is reused the
  same way (`npm run dev` against a bind-mounted `apps/web`).
- `infra/docker-compose.dev.yml`: `migrate` (one-shot goose runner, same
  Dockerfile target `docker-compose.test.yml` uses), `api`, `tracker`,
  `worker`, `web` services, all on the file's one default bridge network,
  addressed by compose service name. Go module/build caches and web's
  node_modules/.next live in per-service named volumes (a first attempt
  sharing one `go_mod_cache`/`go_build_cache` volume across api/tracker/
  worker hit a real Docker race — three containers populating the same
  fresh named volume from their own image layer concurrently — fixed by
  giving each service its own volume instead).
- `prometheus`'s `extra_hosts: host-gateway` entry removed; `infra/
  prometheus.yml`'s scrape targets now point at `api:8080`/`tracker:8081`/
  `worker:8082` (compose service names) instead of
  `host.docker.internal:PORT`.
- Root `Makefile` rewritten around `docker compose -f
  infra/docker-compose.dev.yml` (`make up`/`down`/`restart`/`status`/
  `<svc>-logs`/`<svc>-restart`) instead of host `go run`/`npm run dev` +
  PID files — `.pids/`'s now-stale `.gitignore` entry removed.
  `migrate-up`/`-down`/`-status` and `fmt`/`vet`/`test`/`lint`/`check`
  are unchanged (still host toolchain — validation, not a service).
  `test-stack-*` targets (`docker-compose.test.yml`) unchanged.

### Fixed

- api/tracker/worker's `dev` `ENTRYPOINT` now runs `mkdir -p tmp/<svc>`
  before `air`: air's configured `tmp_dir` has to exist before its
  watcher will create files under it, and since `apps/` is bind-mounted
  over `/src` at container start (replacing whatever `COPY` put there at
  build time), that directory can only be created at container start, not
  image-build time.

### Verified

Built every image (`docker compose -f infra/docker-compose.dev.yml
build`); brought the full stack up — all nine containers healthy,
`api`/`tracker`/`worker`/`web` all reachable on their host-published
ports, `docker inspect`'s `NetworkMode` is `infra_default` (the compose
bridge network) for all four, not `host`; Prometheus's own `/api/v1/
targets` shows all three Go services `up` via their service-name scrape
URL. Confirmed hot reload live: edited `apps/api/main.go` while the stack
was running, air detected it, rebuilt, and restarted the server in
~2s (reverted after). `gofmt -l`/`go vet`/`go build`/`go test -count=1
./...` clean repo-wide (no Go/TS source changed by this work, only
Dockerfiles/compose/Makefile/docs).

### Known issues

- Each app container's own Go module/build cache volume starts cold on
  first `make up` (a few seconds to `go mod download` inside the `dev`
  image build) — same cost the old host workflow paid once per machine,
  now paid once per volume instead.

## [Final Audit] — Phase 34: repo-wide audit, one real product-surface gap surfaced

### Scope

§62's full checklist: UX/UI/accessibility/security/tenant isolation/
performance/database/API/routing/attribution/analytics/LTV/testing/
observability/documentation audit, plus a repo-wide search for
TODO/FIXME/mock/dummy/temporary/hardcoded/console.log/debugger leftovers.

### Findings

- **Major, not fixed this phase (real product-surface work):** seven
  whole frontend feature domains — Domains, Settings (organization/
  API-keys/integrations panels), Tags, Custom Metrics, Report Presets,
  Referral, Content Gallery — were built frontend-first in Phases 14/
  14.5–14.9 against `apps/web/src/lib/mock/*.ts` and were never taken
  through a backend-integration phase, unlike every other domain (Stream
  Sets, Networks/Offers, Traffic Sources, Conversions, Postback Logs,
  etc., each of which got its own "wired to a real…API" phase). Confirmed
  by absence: no `apps/internal/{tags,custommetrics,referral,
  contentgallery,domains,settings}` Go package, no matching
  `apps/web/src/lib/api/*.ts` client, no route registered in
  `apps/api/main.go`; their zustand stores (`apps/web/src/stores/{tags,
  custom-metrics,domains,referral,content-gallery,settings,report-
  presets}.ts`) import directly from `lib/mock/`. Concretely: CLAUDE.md
  non-negotiable #12 (custom-metric formula safety — division-by-zero →
  empty, registry-stable ids, no LTV mixing, team-private scope) is
  currently unenforceable because no formula-evaluation engine exists
  anywhere, frontend or backend — `apps/internal/analytics/analytics.go`
  only mentions "custom metrics" in a comment. CLAUDE.md's CURRENT STATE
  previously flagged only the narrower "missing tracking-URL generation
  API" version of this (the Domains slice); the true gap is six more
  domains wide. Left open deliberately — building six backends is not an
  audit-phase action — but now accurately tracked below and in CLAUDE.md.
- **Related, same root cause:** the standalone Analytics report builder
  (`features/analytics/*`) and the Overview dashboard (`(app)/overview`)
  are also still 100% mock-data-driven. Unlike the seven domains above,
  this isn't "never integrated" — the real `/analytics` backend exists
  and is correctly wired elsewhere (`hooks/use-campaign-analytics.ts`
  calls it for the campaign-detail chart) — it just never grew the
  multi-dimension/funnel query capability this page's UI needs.
  Legitimately deferred, not a wiring bug.
- **Three real bugs found and fixed, not just documented:**
  1. `campaign-cost-entries.tsx` deleted a cost entry with zero
     confirmation, violating CLAUDE.md's destructive-action rule — every
     sibling row-actions file (network, traffic source, postlanding)
     gates delete behind a `Dialog`; this one didn't. Fixed, with new
     `cost.json` i18n keys (en+ru).
  2. `apps/internal/routing/weighted.go`'s doc comment claimed the
     frontend had "a temporary simulator mock" mirroring the Go pick
     logic — that file hasn't existed for a while; the simulator has
     genuinely been a thin `/routing/simulate` client all along
     (invariant #1 was never actually violated, the comment was just
     stale). Corrected.
  3. `use-current-member.ts`'s comment claimed "no auth session yet
     (that's Phase 28)" — false, real `/auth/me` has existed since Phase
     28. The actual reason it still reads the mock team roster (Custom
     Metrics/Referral/Content Gallery have no backend of their own yet,
     and switching identity source ahead of their integration phase
     would break their mock "is this mine" checks) is now stated
     accurately.
- **Dead code removed** (zero importers, confirmed by grep before
  deletion): `apps/web/src/stores/{campaigns,offers,event-mappings}.ts`
  and `apps/web/src/lib/mock/{offers,event-mappings,conversions}.ts` —
  all six pre-date the real API clients that replaced them.
  `lib/mock/campaigns.ts` was kept — `tag-assignments.ts` (the live,
  still-deferred Tags mock) still imports it.
- Everything else audited clean: routing single-source-of-truth,
  deterministic weighted pick and sticky-cookie-is-truth (invariants
  #4/§38), tenant isolation (every one of 19 route groups in
  `apps/api/main.go` sits behind `tenantMiddleware`; repository layer
  spot-checked across 13+ domains, all filter on `organization_id` — the
  only two queries without an explicit filter are `auth/repository.go`'s
  `AcceptInvite`, correctly scoped upstream by a verified invite token
  instead), CSRF/secrets/postback-dedup (invariant #3 — `event_ref`
  enforced in code, not convention), no hardcoded secrets, `.env`
  correctly gitignored, tracker hot path (invariant #9), DB indexes on
  tenant-scoped hot tables, ClickHouse sort keys lead with
  `organization_id`, LTV/cohort currency handling (invariant #7 — FX
  rate looked up at the event's `OccurredAt`, never "now"), OpenTelemetry
  tracing genuinely wired (real OTLP exporter + `otelhttp` instrumentation
  on all 3 binaries), no sensitive data in any log call site, Prometheus
  alert rules in `infra/alerts.yml` cross-checked against actual metric
  names (all resolve), `go build`/`vet`/`test -count=1 ./...` and
  `tsc`/`eslint` clean, zero TODO/FIXME/console.log/debugger left in
  source.

### Fixed

- `apps/web/src/app/globals.css`: added the one global
  `@media (prefers-reduced-motion: reduce)` rule (§66 accessibility) —
  previously absent entirely; no component respected the OS setting.
- `docs/deployment.md`: documented that none of the four Go binaries set
  explicit Postgres/Redis pool limits (`pgxpool.New`/`redis.Options` both
  use library defaults) — not a bug (Phase 31's real load test measured
  p95 1.0–1.5ms against the 50ms budget with defaults), but an
  undocumented production knob; added the `DATABASE_URL?pool_max_conns=`
  tuning path.
- `campaign-cost-entries.tsx` delete confirmation dialog, the two stale
  comments, and the six dead files above — see Findings.

### Known issues

- The seven-domain frontend-integration gap above — tracked in CLAUDE.md
  CURRENT STATE, not blocking, needs its own phase(s).
- The Analytics report builder / Overview dashboard's missing
  multi-dimension backend, above.
- `components/ui/data-table.tsx` (the shared table every list screen
  uses) never got column resizing or CSV/export wired, despite both
  being in CLAUDE.md's own table rules (§64) — sorting, pagination,
  column visibility, and search/filter are all real and consistently
  used; resizing and export are structurally absent repo-wide.
- Repository/handler-layer Go packages (network, offer, pixel, campaign,
  streamset, cost, and ~18 others) sit at 0.0% unit-test coverage —
  DB-backed logic is exercised only via the Phase 32 e2e scenario and
  manual dev-stack verification. Core computation packages are well
  covered (routing 81.7%, ltv 67.0%, attribution/event/macro/metrics
  100%).

### Verified

`go build`/`go vet`/`gofmt -l`/`go test -count=1 ./...` clean repo-wide;
`tsc --noEmit`/`eslint` clean on `apps/web`.

## [Production] — Phase 33: Dockerfiles, deployment, monitoring, alerts

### Scope

§61's full checklist for all eight services (web/api/tracker/worker/
postgres/clickhouse/redis/object-storage): Dockerfiles,
docker-compose.dev.yml, docker-compose.test.yml, production deployment
docs, environment documentation, backup strategy, migration strategy,
monitoring, alerts. Full detail in `docs/deployment.md`,
`docs/environment.md`, `docs/backup.md`, `docs/migrations.md`.

### Added

- `apps/api/Dockerfile` (also builds a `migrate` target — a one-shot
  goose runner for `apps/api/migrations`), `apps/tracker/Dockerfile`,
  `apps/worker/Dockerfile`, `apps/web/Dockerfile` — all four built and
  smoke-tested standalone before being wired into compose.
- `apps/web/next.config.ts`: `output: "standalone"`, needed for a
  minimal production image. `apps/web/src/app/api/health/route.ts`: new
  container health-check route.
- `infra/docker-compose.test.yml`: the full containerized stack, all 8
  services, built from this repo's own Dockerfiles — brought up for
  real, verified with a real signup through the containerized `api` and
  real `/metrics` scrapes on all three Go services, then torn down.
- `infra/alerts.yml`: 8 Prometheus alerting rules over Phase 29's nine
  tracked metrics (tracking latency vs CLAUDE.md non-negotiable #9's own
  50ms p95 budget, event loss, queue backlog, postback dead-letter rate,
  analytics latency, service-down) — `promtool`-validated and confirmed
  loaded into a real Prometheus. `infra/prometheus.yml` now loads it via
  `rule_files:`.
- `docs/deployment.md`, `docs/environment.md`, `docs/backup.md`,
  `docs/migrations.md` — new.

### Fixed

- `apps/web/src/proxy.ts`'s session-guard middleware was redirecting the
  new `/api/health` route to `/login` (its matcher didn't exclude it,
  unlike `_next/static`/`favicon.ico`) — found by actually curling the
  built container, not assumed working. Matcher now excludes
  `api/health` alongside the existing static-asset exclusions.

### Known issues

- `docker-compose.test.yml`, run once without an explicit Compose
  project name, shared the "infra" default project name with
  `docker-compose.dev.yml` and briefly recreated the dev stack's live
  data-store containers under the test config. No data was lost (named
  volumes survive `down` without `-v`; the dev stack reattached its
  original volumes intact on `up -d`), but this is exactly the kind of
  mistake worth naming: fixed permanently with an explicit `name:
  flox-test` in `docker-compose.test.yml`, documented there and in
  `docs/deployment.md`.
- The missing tracking-URL generation API (flagged in Phase 32) remains
  open — not touched this phase, real product-surface work for its own
  phase or a slice of a nearby one.

### Verified

`gofmt`/`go vet`/`go build`/`go test -count=1 ./...` green repo-wide;
`tsc`/`eslint`/`vitest` clean on `apps/web`. `docker compose -f
infra/docker-compose.test.yml up --build` brought all 8 containers to
healthy; a real `POST /auth/signup` through the containerized `api`
succeeded (real CSRF/Origin check, real Postgres write, real session
cookie), `apps/web`'s `/login` served 200 through the containerized
frontend, and `/metrics` on `api`/`tracker`/`worker` returned 43/47/43
`flox_` series respectively. `docs/backup.md`'s Postgres/ClickHouse
commands were run for real against the dev stack before being written
down. All fixtures and the test stack itself torn down afterward
(`down -v`); dev stack confirmed unaffected (`organizations` count back
to 1, original dev org intact).

## [E2E Testing] — Phase 32: full-funnel scenario test

### Scope

§57's brief: one continuous scenario — Create organization -> Create
source -> Create network -> Create offer -> Create landing -> Create
Stream Set (nested flow + filter) -> Create campaign -> Enter cost ->
Generate tracking URL -> Click -> Route -> Record event -> Receive
conversion (HOLD -> ACCEPT -> REDEP) -> Attribute conversion -> Send
postback -> Analytics + LTV — verifying every step. Two scope decisions
confirmed with the user first: an automated Go test (not a manual
walkthrough or browser-driven UI pass), against already-running
`apps/api`/`apps/tracker`/`apps/worker` (not spawned by the test itself).
Full detail, gotchas, and the tracking-URL product gap in
`docs/e2e-testing.md`.

### Added

- `apps/e2e/scenario_test.go` (`TestFullFunnel`): drives the entire
  scenario over real HTTP against real running services and real
  Postgres/ClickHouse/Redis — no mocks. `t.Skip`s with a clear message
  when `DATABASE_URL`/`CLICKHOUSE_URL` are unset or the services aren't
  reachable, so `go test ./...` stays green without them running.
  Fixtures clean up in `t.Cleanup` on both pass and failure.
- `docs/e2e-testing.md`: how to run it, the missing tracking-URL API
  gap, and the two real bugs this phase's own test-writing surfaced
  (below).

### Fixed

- `internal/ltv/handler.go`'s `parseParams` parsed a bare date-only `to`
  as that date's midnight; `internal/chstore`'s `LTVFilter` query is a
  half-open `event_at >= from AND event_at < to` range, so `?to=<today>`
  silently excluded every same-day LTV cohort anchor. Now adds
  `23:59:59.999` to a same-day `to`, matching `internal/conversions`'
  handler, which already had the identical fix for the identical reason.
  Found by the E2E scenario's own LTV assertions failing against a real
  `to=today` query.

### Known issues

- No HTTP API exists for "generate a tracking URL" (no `/domains` or
  `/tracking-links` endpoints anywhere in `apps/api` — only
  `apps/cmd/loadtestseed`'s raw SQL has ever written those tables). The
  E2E test seeds the same two rows the same way, directly over its own
  Postgres connection, rather than expanding this phase into building the
  missing endpoint. Real backend work for a future phase.

### Verified

`gofmt`/`go vet`/`go build ./...`/`go test -count=1 ./...` green
repo-wide (including the new `apps/e2e` package and the `ltv` fix),
against real `apps/api`/`apps/tracker`/`apps/worker` started per the
documented local dev workflow. `TestFullFunnel` run 3x consecutively with
`-count=1` after fixing an event-queue polling race in the test itself
(see `docs/e2e-testing.md`) — 3/3 pass. No frontend touched this phase.
All fixtures deleted afterward, confirmed zero leftover rows.

## [Performance] — Phase 31: benchmarks + load test, no code changes

### Scope

§56's brief: benchmark tracking/routing/classifier/postback/analytics, hit
tracking p50 < 20ms / p95 < 50ms, load-test for zero event loss, optimize
only after measurement. Measurement showed every number already clears its
target by 1-2 orders of magnitude on the real code path against the real
local dev stack — no optimization work was warranted or done. Full detail,
numbers, and reproduction commands in `docs/performance.md`.

### Added (measurement tooling only — no production code changed)

- `internal/routing/bench_test.go`, `internal/classifier/bench_test.go`:
  pure in-memory benchmarks (`BenchmarkResolve`/`_Sticky`,
  `BenchmarkClassify`/`ParseUserAgent`) — 0.27-2.5 µs/op, three orders of
  magnitude under budget.
- `internal/routingstore/bench_test.go`: `BenchmarkLoadRoutingConfig`
  (0.40 ms) and `BenchmarkResolveTrackingLink` (0.086 ms) against a real
  Postgres — the two DB round trips that make up nearly all of §41's
  actual cost.
- `apps/tracker/bench_test.go`, `apps/tracker/postback_bench_test.go`:
  `BenchmarkTrack` (0.51 ms serial, 0.084 ms 16-way parallel — the whole
  §41 hot path through a real `net/http` mux), `BenchmarkPostback`
  (1.7 ms, no SLA — explicitly off the §41 budget per `tracker/postback.go`'s
  own doc comment).
- `internal/analytics/bench_test.go`: `BenchmarkCampaignDaily`/
  `CampaignDailyRevenue` (~2 ms) against a real ClickHouse seeded with
  ~6,000 rows.
- `apps/cmd/loadtestseed`: a small manual tool (`seed`/`cleanup <orgId>`)
  seeding a real campaign for the load test below — not wired into any
  service.

### Verified

`gofmt`/`go build ./...`/`go vet ./...`/`go test -count=1 ./...` green
repo-wide, including all five new `bench_test.go` files.

A real `apps/tracker` binary, load-tested with `vegeta` at 300/1,000/
3,000/6,000 req/s sustained (6,000-47,997 requests per run): p50 stayed at
0.64-0.99 ms and p95 at 1.0-1.5 ms across every rate, with **zero event
loss** confirmed three independent ways after each run —
`flox_events_enqueued_total` == `flox_events_queue_written_total`
(Prometheus), `flox_events_buffer_dropped_total` /
`_queue_write_failed_total` stayed at 0, and a direct
`SELECT count(*) FROM event_queue` matched the cumulative enqueued count
(104,999) exactly. All load-test fixtures (organization, cascade-deleted
children, `event_queue` rows) deleted afterward via
`loadtestseed cleanup`, confirmed zero rows remaining.

## [Security Hardening] — Phase 30: postback secrets, CSRF, rate limiting, SSRF

### Scope

Four independent §54 defenses closing gaps that had zero
authentication/throttling/host-validation before this phase: incoming
postback authentication, CSRF (cross-site fetch with credentials), abuse
rate limiting, and SSRF on outbound postback delivery. See
`docs/security.md` for full detail on each.

### Changed

- `networks.postback_secret_hash` (migration `00021`, hashed at rest,
  one-way, `DEFAULT ''` sentinel): `apps/internal/network` gains
  `crypto.go` (secret generation/hashing), `Service.RegeneratePostbackSecret`,
  and a `POST /networks/{id}/regenerate-postback-secret` route. The raw
  secret is returned exactly once — in `POST /networks`'s response
  (`CreateResult.PostbackSecret`) and in the regenerate response — never
  from any other `Network` read.
- `apps/tracker/postback.go`: `?secret=...` is now required and checked
  with `subtle.ConstantTimeCompare` against the stored hash before any
  conversion is recorded.
- New `apps/internal/tenant.RequireSameOrigin`: rejects a mutating
  (`POST`/`PATCH`/`PUT`/`DELETE`) request whose `Origin` header is present
  and doesn't match `APP_URL`; a request with no `Origin` (non-browser
  client) is allowed through. Mounted before session resolution in
  `apps/api/main.go`, including standalone on the pre-session
  signup/login/accept-invite routes.
- New `apps/internal/ratelimit` package: a fixed-window Redis counter,
  fails open on a Redis error. A general 300 req/min-per-IP ceiling across
  all of `apps/api` (`apps/internal/httpserver`, exempting
  `/health`/`/ready`/`/metrics`), and a stricter 20 req/15min-per-IP
  ceiling on `/auth/login`/`/auth/signup`. Deliberately not applied to
  `apps/tracker`'s redirect hot path (§41 latency budget).
- New `apps/internal/urlsafety` package: `ValidateURL` (save-time
  scheme/literal-IP check, wired into `network.Service` create/update) and
  `SafeDialContext` (the authoritative defense — resolves and validates the
  IP at connection time, closing the DNS-rebinding gap a save-time-only
  check can't catch), wired as `apps/worker`'s postback-delivery
  `http.Client`'s `Transport.DialContext`.
- `apps/web`: a `PostbackSecretDialog` (reveal-once, copy button) shown
  after network creation and after "Regenerate incoming secret" in the row
  actions menu.

### Fixed

- `apps/internal/httpserver.New` originally called `r.Use(limiter...)`
  *after* `/health`/`/ready`/`/metrics` were already registered on the same
  chi mux — chi panics on that ordering ("all middlewares must be defined
  before routes"), so `apps/api` failed to start at all. Fixed by moving
  the `Use` call before route registration and adding an `exemptPaths`
  wrapper so those three routes still bypass the general rate limit
  without depending on registration order. Caught during this phase's
  manual verification pass (the binary panicked on first startup attempt).

### Verified

`gofmt`/`go build ./...`/`go vet ./...`/`go test -count=1 ./...` green
repo-wide, including four new test files (`internal/ratelimit`,
`internal/urlsafety`, `internal/tenant/tenant_test.go`,
`tracker/postback_secret_test.go`) plus new cases in
`internal/network/network_test.go`. `tsc --noEmit`, `eslint`, and
`next build` all clean on `apps/web`.

Full manual pass against real running binaries (after fixing the startup
panic above): signed up a real user, created a real network, confirmed the
returned `postbackSecret` gates `apps/tracker`'s postback endpoint (missing
secret → 401, wrong secret → 401, correct secret → passes through to real
business logic), confirmed `RegeneratePostbackSecret` immediately
invalidates the old secret and the new one works. Confirmed
`RequireSameOrigin`: cross-origin `Origin` → 403, matching-origin and
no-`Origin` → pass. Confirmed `urlsafety.ValidateURL` rejects both a
cloud-metadata URL (`169.254.169.254`) and a loopback URL at network-create
time. Confirmed the auth rate limit trips (`429`) after the configured
threshold. All fixtures (user, org, network, sessions) deleted afterward.

## [Observability] — Phase 29: Prometheus metrics, tracker request IDs, traced postback delivery

### Scope

Structured logging and OpenTelemetry tracing already existed (Phase 16+);
this phase's real gap was metrics — no client library, no `/metrics`
endpoint, none of §53's nine tracked metrics existed. Confirmed via
`AskUserQuestion`: `prometheus/client_golang` (not the OTel metrics SDK),
plus a real Prometheus server added to `infra/docker-compose.dev.yml` for
genuine scrape verification. See `docs/observability.md` for full detail,
including why `event_loss` is modeled as two counters (not one — the two
sides live in different binaries) and why `postback_deliveries_total` has
three outcome values, not §53's literal two.

### Changed

- New `apps/internal/metrics` package: all nine `flox_`-namespaced
  collectors, `promauto`-registered, plus `Handler()` for `/metrics`.
- `apps/tracker`: `middleware.RequestID` + an `X-Request-Id` echo (not the
  full per-request logging middleware `apps/api` uses — a context value
  and one header write, not a synchronous log line per click, so §41's
  latency budget is untouched); `tracking_requests_total`/
  `tracking_latency_seconds` around the whole redirect handler.
- `internal/routing.Engine.Resolve`, `apps/internal/postback.Deliverer`,
  `apps/internal/analytics.Service`, and `eventqueue.Flusher` each gained
  their own metric at the natural instrumentation point; new
  `eventqueue.PostgresQueue.Depth`/`PollDepth` for the queue-depth gauge;
  new `metrics.RegisterEventBufStats` exposes `eventbuf.Writer`'s
  existing atomic counters as `CounterFunc`s (its internals untouched).
- `apps/worker`: a fifth background goroutine (queue-depth polling), and
  its postback delivery `http.Client` now wraps `otelhttp.NewTransport`
  — each delivery attempt becomes its own trace.
- `/metrics` added to `apps/api`, `apps/tracker`, `apps/worker`.
- New `prometheus` service + `infra/prometheus.yml`, scraping all three
  binaries on the host via `host.docker.internal` (`extra_hosts` for
  Linux Docker Engine compatibility).

### Verified

`gofmt`/`go build ./...`/`go vet ./...`/`go test ./...` green repo-wide,
including two new test files (`internal/metrics`, and a `Depth()` test in
`internal/eventqueue`). Full manual pass: started all three binaries plus
a real Prometheus container, confirmed all three scrape targets `up` via
`/api/v1/targets`, then generated genuine traffic for every one of the
nine metrics (a real tracking-link click end to end through
tracker→Postgres queue→worker→ClickHouse, a real failing postback
delivery, a real authenticated analytics query) and confirmed each value
via Prometheus's own query API — not just a local curl. No unexpected
errors in any binary's logs. All test fixtures deleted afterward.

## [Auth / Organizations / RBAC] — Phase 28B: frontend integration

### Scope

Wires `apps/web` to Phase 28A's real auth API. New: login/signup/
accept-invite pages (none existed before, not even as a mock), route
protection (`proxy.ts` + a client-side `AuthGuard`), and the pre-existing
mock Team page switched onto the real `/team/*` endpoints.
`NEXT_PUBLIC_DEV_ORG_ID` is gone — every request is scoped by session
cookie now. See `docs/auth.md`'s "Phase 28B" section for full detail.

### Changed

- `lib/api/client.ts`: `credentials: "include"` replaces the
  `X-Organization-Id` header; `apps/internal/httpserver`'s CORS gained
  `AllowCredentials: true`.
- New `(auth)` route group (`/login`, `/signup`, `/accept-invite`) and a
  new `auth` i18n namespace — the pre-existing Team feature files were
  left on their original hardcoded English rather than partially
  retrofitting i18n into only the parts this phase touched.
- New `src/proxy.ts` (this Next.js version renamed `middleware.js` to
  `proxy.js`) redirects based on session-cookie *presence* only (a UX
  convenience, not the enforcement boundary); `components/shell/
  auth-guard.tsx` is the defense-in-depth layer that actually calls
  `GET /auth/me` and catches a present-but-expired/revoked cookie.
- Team page: new `lib/api/team.ts`/`hooks/use-team.ts` replace
  `stores/team.ts` for the Team page only — `hooks/use-current-member.ts`
  and `stores/team.ts` itself were deliberately left untouched, since
  three other still-mock features (custom-metrics, content-gallery,
  referral) key their own role-gating against that mock roster and
  aren't wired to a real backend yet.
- New `lib/nav.ts` `requiredPermission`/`visibleNavGroups`: the sidebar
  and ⌘K palette now hide "Team" for a role lacking `team.read`, and the
  Team page itself hides/disables write controls for a role lacking
  `team.write` — found via manual testing (a Viewer could click through
  to a bare permission-denied error page before this fix).

### Fixed

- A real bug caught in manual testing: `AcceptInviteForm`'s
  `useForm({ values: {...} })` re-applied a fresh object literal on every
  render, silently resetting the password field on every keystroke — the
  submit button appeared to do nothing. Fixed with `defaultValues` + a
  one-time effect-based prefill instead.
- A real bug caught while cleaning up test fixtures: `internal/auth`'s Go
  test suite created real rows in the shared dev Postgres but never
  deleted them (every other package's test suite does) — three earlier
  runs had left 51 organizations and dozens of orphaned `users` rows
  behind. Added `t.Cleanup` to `signupOrg`/`uniqueEmail`; purged the
  pre-existing backlog by hand.

### Verified

`gofmt`/`go build ./...`/`go vet ./...`/`go test ./...` unchanged and
green (no Go code changed this phase, aside from the test-cleanup fix
above). Frontend: `next typegen`, `tsc --noEmit`, `eslint .`,
`vitest run` (21 tests), `next build` all clean. Full manual browser
pass (Claude-in-Chrome) against the real running dev stack on matching
ports (`8080`/`3000`, required for CORS): signup → real org/session →
app shell with real org/user displayed; invite → real shareable link →
accepted in a second tab → invitee's restricted permissions and hidden
nav confirmed; suspend → remove via the real UI, member count updating
live; real 4-entry activity log with human-readable labels; logout
clearing the cookie and re-redirecting on the next protected-route hit;
spot-checked the untouched mock features (content-gallery) still work.
No console errors. All test fixtures (this pass's and the pre-existing
Phase 28A backlog found during cleanup) deleted afterward.

## [Auth / Organizations / RBAC] — Phase 28A: sessions, invites, and role-based access control

### Scope

Confirmed via `AskUserQuestion`: real authentication and RBAC on the Go
side (§52), split from the frontend integration that wires it into
`apps/web` (Phase 28B, not started). Also confirmed: server-side sessions
in an HTTP-only cookie (not JWTs), invites as a shareable link (not a real
sent email — no SMTP integration exists anywhere in this project), and
RBAC enforcement wired onto this phase's own team/membership endpoints
only (sweeping permission checks across every pre-existing domain route
is an explicit, separate follow-up). See `docs/auth.md` for full detail.

### Changed

- New migration `00020_auth_sessions_rbac.sql`: `users.password_hash`
  (empty-string sentinel for a not-yet-accepted invite's shell user), a
  `sessions` table (token stored only as a SHA-256 hash, same precedent as
  `api_keys.key_hash`), `memberships.invite_token_hash`/
  `invite_token_expires_at`, and `role_permissions` seeded to match
  `apps/web`'s `ROLE_PERMISSIONS` mock exactly.
- New `apps/internal/auth` package: signup (creates org + Owner in one
  transaction), login (resolves which org a multi-membership user meant,
  constant-time-ish against email enumeration via a dummy bcrypt
  comparison), logout, `GET /auth/me` (role + permission list, for
  frontend UX gating only — enforcement stays server-side), invite/
  preview-invite/accept-invite/resend-invite, and full membership CRUD
  (`/team/members`, `/team/activity`) — every write behind a new
  `RequirePermission` middleware backed by `role_permissions`.
- `apps/internal/tenant`: `Middleware` (a fixed function reading
  `X-Organization-Id`) became `NewMiddleware(SessionResolver, logger)`,
  constructed once in `apps/api/main.go` with `*auth.Service`. Every other
  domain package's own handlers needed zero changes — they still only
  ever read `tenant.OrgID(ctx)` (plus a new `tenant.UserID(ctx)`).
- `apps/internal/httpserver`: CORS `AllowCredentials: true` (required for
  the session cookie to travel on `apps/web`'s cross-origin `fetch`
  calls), `X-Organization-Id` dropped from `AllowedHeaders` (no route
  accepts it anymore).

### Verified

`gofmt`/`go build ./...`/`go vet ./...`/`go test ./...` green across the
**entire** repo, including 20 new `internal/auth` tests against real
Postgres (signup/login/logout, session resolution, invite→accept→login,
resend invalidates the old token, role/status updates, Owner-protection,
member removal revokes access, cross-tenant isolation across
update/remove/list/activity, and the `RequirePermission` middleware).
Full manual `curl` pass against the real running `apps/api` dev stack:
confirmed the old `X-Organization-Id`-only path now 401s, then walked the
whole flow with a real cookie jar — signup, `GET /campaigns` (an
untouched pre-existing domain) succeeding through the same session
cookie, invite → public preview → accept-invite auto-login, a Viewer's
restricted `/auth/me` permissions and a correctly-403'd write attempt, an
Owner suspending the Viewer whose *existing* session immediately 401'd on
its next request with no re-login needed to observe it, a blocked
self-removal attempt by the Owner, a real five-entry activity log, and
logout clearing the cookie. No frontend changes this phase (Phase 28B's
job) — `apps/web` still runs on `NEXT_PUBLIC_DEV_ORG_ID` + the mock Team
store.

## [Ad Spend Sync Scheduler] — Phase C of FB/TikTok ad-spend import: automated recurring sync

### Scope

Confirmed via `AskUserQuestion` after Phase B shipped with no open issues:
automate the previously manual-only "Sync now" (`POST
.../connection/sync`) with a recurring background job. New scope, not
part of either of the two confirmed FB/TikTok phases before it — the
manual endpoint stays, unchanged, as a way to force a sync between
scheduled runs. See `docs/ad-account-connections.md`'s "Phase C" section
for full detail.

### Changed

- New `adaccount.Repository.ListAllConnections` — the one deliberately
  org-unscoped query in that package (every other method takes an
  `orgID`): lists every connected ad account across every org for a
  scheduler with no single tenant's request to scope from. Lives only on
  `*Repository`, never `Service`; never HTTP-reachable — same trust
  boundary as Phase B's `CredentialsByTrafficSourceID`.
- New `costsync.Scheduler` (`RunOnce`/`RunLoop`): re-runs `Service.Sync`
  for every connection on a `time.Ticker`, not `apps/worker`'s existing
  `PollLoop` claim-a-batch shape — there's no per-connection "due" state,
  just "sync everyone again every N hours." One connection's `Sync`
  failing (expired token, transient API error) is logged and skipped,
  never aborting the rest of the batch; a `RunOnce` error is logged and
  the loop waits for its next tick rather than exiting.
- `apps/worker/main.go`: wires the scheduler alongside the three existing
  poll loops, `costSyncInterval = 6 * time.Hour` (a plain const, matching
  the existing loops' non-env-configurable batch/idle constants).

### Verified

`gofmt`/`go build ./...`/`go vet ./...`/`go test ./...` all green,
including new tests: `adaccount.TestListAllConnections`,
`costsync.TestSchedulerRunOnceSyncsEveryConnection`,
`TestSchedulerRunOnceContinuesPastAFailingConnection`, and
`TestSchedulerRunLoopStopsOnContextCancel` — all against real Postgres
except the last, a pure unit test. Full manual pass: started
`apps/worker` against the real dev stack with an empty database (its
first tick logged `connections_attempted:0`), then seeded one real
Postgres row (org + `facebook_ads` traffic source + a connection with an
intentionally-invalid token) and restarted — the scheduler's first tick
picked it up and made a **real network call to the live Facebook Graph
API**, getting back the same genuine `OAuthException` (code 190) Phase
B's own manual pass got, logged with full detail, followed by the run
finishing normally and the worker continuing to serve. Test fixtures
deleted afterward.

## [Ad Spend Sync] — Phase B of FB/TikTok ad-spend import: real API adapters + sync

### Scope

Second half of §74/§27-COST's ad-spend import, the phase Ad Account
Connections (below) deliberately deferred. Confirmed via `AskUserQuestion`:
build the real Facebook Ads/TikTok Ads API adapters + a sync now, still
only structurally verifiable (no live Meta/TikTok app credentials exist
in this environment — though this environment does have outbound
internet access, so both adapters were also exercised against the real
Graph/Business API with intentionally-invalid tokens during manual
verification, see below). Mid-phase, found and fixed a real gap Phase A's
`CostProvider` interface didn't account for: `cost_entries.campaign_id`
is `NOT NULL` but a platform's spend API reports by its *own* campaign
id, and one traffic source can fund more than one FLOX campaign — nothing
mapped a synced record to a specific FLOX campaign. Confirmed via a
second `AskUserQuestion`: add an optional `externalCampaignId` field to
Campaign for this matching. See `docs/ad-account-connections.md`'s
"Phase B" section for full detail.

### Changed

- New migration `00019_campaign_external_id.sql`: `campaigns.external_campaign_id`
  (nullable-by-convention `text NOT NULL DEFAULT ''`, no uniqueness
  constraint — two campaigns can deliberately share one ad-platform
  campaign id). `campaign.Repository.ListByExternalID` matches scoped to
  `(orgId, trafficSourceId, externalCampaignId)`, returns a slice.
- `adaccount.CostProvider` revised before anything implemented it:
  `DailySpendByCampaign` (was an account-level `DailySpend`) returning
  `DailyCampaignSpendRecord{Date, ExternalCampaignID, Amount, Currency}`.
- New `apps/internal/adaccount/facebookads` and `.../tiktokads`: real
  HTTP adapters against Facebook Graph API Marketing Insights and TikTok
  Business API's integrated report endpoint respectively, each with an
  injectable `BaseURL`/`HTTPClient` for `httptest.Server`-backed tests.
- `cost.Source` (`manual`/`facebook_ads`/`tiktok_ads`) finally written —
  `cost_entries.source`'s `CHECK` constraint has allowed these values
  since migration 00009 but nothing wrote anything but `'manual'` until
  now. New `cost.Service.UpsertFromSync` (Go-only, never HTTP-reachable)
  takes `source` as an explicit parameter, structurally separate from the
  HTTP-reachable `Service.Upsert` (always `SourceManual`) — not a shared
  struct field, so an HTTP client can never spoof its provenance.
- New `apps/internal/costsync`: orchestrates connection credentials →
  provider call → campaign matching → `cost.Service.UpsertFromSync`
  writes. `POST /traffic-sources/{id}/connection/sync`, defaulting to a
  30-day lookback, mounted alongside `adaccount`'s own routes in the same
  `chi` `Route()` block (two separate `Route()` calls on the identical
  pattern panics).
- `apps/web`: `Campaign.externalCampaignId` on the campaign form (create
  + edit/Settings tab) and API types; `AdAccountConnectionSection` gained
  a "Sync now" button + inline result summary (records fetched, entries
  written, capped unmatched-external-id list).

### Verified

Backend: `gofmt`/`go build ./...`/`go vet ./...`/`go test ./...` all
green — new tests in `internal/cost`, `internal/adaccount/facebookads`,
`internal/adaccount/tiktokads` (httptest.Server-backed), and
`internal/costsync` (real-Postgres-backed, matched/unmatched/shared-
external-id/not-connected/no-provider cases). Frontend: `tsc --noEmit`/
`eslint`/`vitest run` (21 tests)/`next build` all clean. Full manual pass
against the real running `api`+`web` dev servers, including real network
calls to the actual Facebook Graph API and TikTok Business API (real
`OAuthException`/`40105` errors back from intentionally-invalid tokens,
logged server-side, generic 500 to the client — no token leaked); a
campaign created through the UI with `externalCampaignId` round-tripped
correctly through the real Postgres-backed API. Test fixtures cleaned up
afterward. See `docs/ad-account-connections.md`.

## [Ad Account Connections] — Phase A of FB/TikTok ad-spend import: credential storage

### Scope

First slice of §74/§27-COST's ad-spend import, split into two phases
(confirmed via `AskUserQuestion`): this phase stores ad-account
credentials so a later phase can pull real spend; that later phase's
Facebook/TikTok API clients and sync job aren't built yet. Also
confirmed: no live Meta/TikTok OAuth app exists in this environment (no
public HTTPS callback URL to register), so connecting an account is a
manual "paste an access token + ad account id" form rather than an OAuth
consent flow — fully buildable and verifiable end-to-end here, unlike a
real OAuth redirect/callback pair would be. `traffic_sources.cost_integration`
already recorded *intent* (`facebook_ads`/`tiktok_ads`, from an earlier
phase); this phase is what plugs a real credential into that intent. See
`docs/ad-account-connections.md`.

### Changed

- New `apps/internal/adaccount`: `model.go`/`handler.go`/`service.go`/
  `repository.go`, wired at `/traffic-sources/{id}/connection`
  (`GET`/`PATCH`/`DELETE`, no `{connectionId}` — one connection per
  source). New migration `00018_ad_account_connections.sql`. Declares
  the §74 `CostProvider` interface + `Credentials`/`DailySpendRecord`
  types a later phase's real adapters will implement — called from
  nowhere yet, declared now so this phase's storage shape is provably
  sufficient for that future caller.
- `access_token` stored in plain text (no KMS infra exists anywhere in
  this codebase yet) but never leaves the Go API as such: `Connection`
  (the JSON-response type) has no `AccessToken` field at all, only a
  SQL-computed `TokenPreview` (last 4 characters).
- `apps/web`: `lib/api/ad-account-connections.ts` + `hooks/use-ad-
  account-connection.ts`; new `AdAccountConnectionSection`, rendered
  inside the existing `SourceFormSheet` (Traffic Source edit) whenever
  the live `costIntegration` value is `facebook_ads`/`tiktok_ads`.

### Fixed

A real bug caught during this phase's own manual verification: the new
section's connect form used a nested `<form>` inside `SourceFormSheet`'s
own outer `<form>` — invalid HTML. Chrome's actual behavior wasn't "the
inner form is ignored": clicking "Connect" fired a native GET submission
that put every field, **including the raw access token**, into the URL
as a query string — exactly the "never place sensitive data in URL
parameters" case. Fixed by removing the inner `<form>` (a plain `<div>`
instead, with the button changed to `type="button"` calling
`form.handleSubmit(submit)` directly, plus a manual Enter-to-submit
handler). See `docs/ad-account-connections.md`'s "A real bug, caught and
fixed" section for the full reproduction/fix/re-verification trail.

### Verified

Backend: `go build/vet/gofmt/test ./...` all green, incl. new
`adaccount_test.go` (connect/get/reconnect-replaces-in-place/disconnect;
rejecting a source with `cost_integration` `none`/`manual`; invalid-shape
validation; cross-tenant isolation). Frontend: `tsc --noEmit`/`eslint`/
`vitest run` (21 tests)/`next build` all clean. Full manual browser pass
against the real running `api`+`web` dev servers — this is where the
nested-`<form>` bug above was caught, fixed, and re-verified (`PATCH`
now returns 200 with no URL leak, was previously a silent token-leaking
GET navigation). See `docs/ad-account-connections.md`.

## [Stream Set ↔ Pixel attachment] — wires stream_set_pixels, fixes a silent Update-blocking bug

### Scope

Closes out the Landing/PWA/Postlanding/Pixels CRUD sequence: wires
`stream_set_pixels` (migration 00008) so a Stream Set can fire zero or
more of the now-real Pixel entities. A Stream-Set-level many-to-many,
deliberately not a per-Flow concern — CLAUDE.md's own past "per-flow
Pixels" phrasing was imprecise; the schema has always scoped pixels one
level up. A pixel's own `events` selection still decides *when* it
fires; `pixelIds` only decides *whether it's eligible to* for traffic
matching that set. See `docs/stream-sets.md`.

### Changed

- `apps/internal/streamset`: `StreamSet.PixelIDs`/`CreateInput.PixelIDs`/
  `UpdateInput.PixelIDs`; `Service.checkPixelIDsBelongToOrg` (same
  never-trust-a-foreign-id reasoning as `checkFlowStagesBelongToOrg`,
  CLAUDE.md #5); `repository.go`'s `insertPixelIDs`/`loadPixelIDs`/
  `loadPixelIDsTx` (replace-wholesale, mirroring `insertFlows`'s shape).
  `Duplicate` copies `PixelIDs` verbatim.
- `apps/web`: `stream-set-schema.ts`'s `pixelIds` (no `.min(1)` — zero
  pixels is normal); `stream-set-form-sheet.tsx` renders a `MultiSelect`
  fed by a new `usePixels()` fetch in `stream-set-list.tsx`.

### Fixed

A real, pre-existing bug from the Flow CRUD (funnel-stages) phase,
caught only because this phase's manual testing exercised an Update on
a flow with a *disabled* stage (the common case, previously untested):
`FlowLanding`/`FlowPwa`/`FlowPostlanding`'s id/type fields had
`json:"...,omitempty"`, so a disabled stage's `""` value dropped the
key from the wire entirely instead of sending `""`. `apps/web`'s
corresponding types declare those fields as required, so the missing
key deserialized to `undefined` — which `z.string()`/`z.enum([...,""])`
both reject. Result: **every Stream Set Update silently failed**
whenever any flow had a disabled stage (near-always) — react-hook-form's
default no-`onInvalid` behavior is a silent no-op, so nothing was ever
visibly wrong; Create was unaffected (its default values never omit the
keys). Fixed by dropping `omitempty` from all four fields. See
`docs/stream-sets.md`'s "A real bug, caught only by testing Update with
a disabled stage present" section for the full diagnostic trail.

### Verified

Backend: `go build/vet/gofmt/test ./...` all green, incl. 2 new
`streamset` tests (full pixel-attachment round-trip through Create/Get/
Update/Duplicate; unknown and cross-org pixel id rejection). Frontend:
`tsc --noEmit`/`eslint`/`vitest run` (21 tests)/`next build` all clean.
Full manual browser pass: created/edited/duplicated a stream set with
pixel attachments against the real API (raw `curl` cross-checks of the
persisted `pixelIds` at each step) — this is where the `omitempty` bug
was caught, fixed, and re-verified (PATCH now returns 200, confirmed
previously silent). See `docs/stream-sets.md`.

## [Pixels] — wired to a real Postgres-backed API

### Scope

Fourth slice of the Landing/PWA/Postlanding/Pixels CRUD candidate list.
`pixels` was already migrated (migration 00008, alongside `postbacks`
and the `stream_set_pixels` join table). New `apps/internal/pixel`
package mirrors `apps/internal/postlanding` almost exactly (flat entity,
curated §43 event subset, pause/activate/duplicate/archive-via-Update) —
differs only in having no URL (a pixel's `provider` + `pixelId` identify
where a conversion is reported, not a page) and no defensive delete-FK
catch (`stream_set_pixels.pixel_id` `ON DELETE CASCADE`s, unlike
Landing/PWA/Postlanding's `RESTRICT`-guarded `flows.*_id` columns).
Explicitly out of scope: attaching pixels to a Stream Set
(`stream_set_pixels` has no CRUD wiring it to `flows`/`stream_sets` yet)
— CLAUDE.md's own "per-flow Pixels" phrasing was imprecise; the schema
has always scoped pixels to the Stream Set, not the Flow. See
`docs/pixels.md`.

### Changed

- `apps/internal/pixel`: new `model.go`/`handler.go`/`service.go`/
  `repository.go`, wired at `/pixels` in `apps/api/main.go`.
- `apps/web`: `lib/api/pixels.ts` + `hooks/use-pixels.ts` (real API
  layer); `features/pixels/*` rewired off the `stores/pixels.ts` Zustand
  mock onto the real hooks, with `LoadingState`/`ErrorState` added.
  `PIXEL_PROVIDER_I18N_KEY` co-located with `PixelProvider`, matching
  `traffic-sources.ts`'s `SOURCE_TYPE_I18N_KEY` pattern. New `pixels`
  i18n namespace (`en`+`ru`) — the pixel form was entirely untranslated
  under the mock. `stores/pixels.ts`/`lib/mock/pixels.ts` deleted
  outright.

### Verified

Backend: `go build/vet/gofmt/test ./...` all green, incl. new
`pixel_test.go` mirroring `postlanding_test.go`'s full test set (plus an
explicit test that an empty `pixelId` is allowed, matching the frontend's
pre-existing schema). Frontend: `tsc --noEmit`/`eslint`/`vitest run` (21
tests)/`next build` all clean. Full manual browser pass against the real
running `api`+`web` dev servers: created, edited, paused, duplicated,
and archived a pixel, confirming every action round-tripped through the
real API on a fresh reload; spot-checked the Russian locale on the same
page. See `docs/pixels.md`.

## [Flow CRUD] — Landing/PWA/Postlanding funnel stages restored on Flow

### Scope

Confirmed via `AskUserQuestion`: the base Flow entity (name/weight/
active/destination) already has real CRUD, nested under Stream Sets
(`docs/stream-sets.md`). What the Stream Sets phase deliberately dropped
was the Landing → PWA → Postlanding funnel stages ahead of a Flow's
Destination — the `flows` table (migration 00006) has always had the
columns (`landing_id`/`landing_as_pwa`, `pwa_id`/`pwa_type`,
`postlanding_id`), but no `internal/landing`/`pwa`/`postlanding` package
existed at the time to pick from. They all exist for real now, so this
phase wires the stages back in. Per-flow Pixels stays out: no
`internal/pixel` package exists yet, and pixels actually attach to the
Stream Set (`stream_set_pixels`, migration 00008), not the Flow.

### Changed

- `apps/internal/streamset`: `Flow`/`FlowInput` gained `Landing`/`Pwa`/
  `Postlanding` fields (`FlowLanding`/`FlowPwa`/`FlowPostlanding`, each
  an always-present `{enabled, ...}` struct so a disabled stage keeps its
  last pick rather than losing it). `Service.checkFlowStagesBelongToOrg`
  confirms every non-empty id belongs to the caller's org before it's
  persisted (CLAUDE.md #5); `validateFlows` requires an id (and, for
  PWA, a valid `pwaType`) whenever a stage is enabled.
  `repository.go`'s `insertFlows`/`loadFlows`/`loadFlowsTx` read/write
  the 7 new columns; `nullIfEmpty` converts the wire's `""` to `NULL` for
  `pwa_type`'s CHECK constraint.
- `apps/web`: new `features/stream-sets/flow-funnel.tsx` renders the
  full Landing → PWA → Postlanding → Destination → Fallback chain,
  delegating the last two nodes to the unchanged
  `flow-destination-editor.tsx`. Fetches real `useLandings()`/
  `usePwas()`/`usePostlandings()` alongside the existing networks/offers
  queries. `stream-set-schema.ts` validates each stage the same way the
  destination union already did. `lib/mock/flow-entities.ts` (the
  `PwaType`/`PWA_TYPES` enum, never actually mock data) moved into
  `lib/api/stream-sets.ts`.
- `docs/landings.md`/`docs/pwa.md`/`docs/postlanding.md`: their Delete
  sections' "no Flow CRUD exists yet to populate that column" notes
  updated — the RESTRICT FK is real and reachable now.

### Verified

Backend: `go build/vet/gofmt/test ./...` all green, incl. 2 new
`streamset` tests (full-funnel round-trip through Create+Get with real
seeded landing/pwa/postlanding ids; validation rejecting an enabled
stage with no id, an invalid `pwaType`, and a disabled stage referencing
another org's postlanding id). Frontend: `tsc --noEmit`/`eslint`/
`vitest run` (21 tests)/`next build` all clean. Full manual browser pass
against the real `api`+`web` dev servers: enabled all three stages on a
real stream set's flow, saved, reloaded fresh, and confirmed every
selection (incl. `asPwa`/`pwaType`) round-tripped through the real
Postgres-backed API. See `docs/stream-sets.md`.

## [i18n hydration fix] — server-side locale resolution closes the race deterministically

### Scope

Closes the known, non-blocking hydration-race issue the Postlanding phase
documented (mitigated, not eliminated, by a `requestIdleCallback`-delayed
`changeLanguage()` call). An `AskUserQuestion` confirmed the trade-off
before implementing: the only fully deterministic fix requires the
server to read a locale cookie before rendering, which opts every route
out of Next.js static prerendering into per-request dynamic rendering —
accepted, since every route already fetches its real data client-side.
See `docs/i18n-hydration-fix.md`.

### Changed

- `app/layout.tsx` is now `async`, reads `cookies()`/`headers()`, and
  renders `<html lang>` + `<I18nProvider initialLocale>` with a
  server-resolved locale (persisted cookie → Accept-Language → default)
  instead of always English.
- New `lib/i18n/locale.ts`: the pure locale-resolution logic
  (`resolveLocale`, `LOCALE_COOKIE`, `isSupportedLocale`,
  `SUPPORTED_LOCALES`, `DEFAULT_LOCALE`), with zero `i18next`/
  `react-i18next` imports so a Server Component can import it directly
  without pulling react-i18next's context creation into the
  `react-server` build (which doesn't export `createContext` — the
  literal failure mode hit and fixed mid-phase). `lib/i18n/config.ts`
  re-exports the same names for existing "use client" call sites.
- `lib/i18n/config.ts`'s `createI18nInstance(locale)` replaces the old
  shared module-level `i18next` singleton for the app's actual runtime
  path — a fresh, independent instance per call, since `I18nProvider`'s
  render now runs once per server request (concurrent requests, in
  principle different locales, on the same Node process) in addition to
  once per browser tab.
- `components/i18n-provider.tsx`: `I18nProvider` now takes
  `initialLocale` as a prop instead of self-detecting via
  `localStorage`/`navigator.language` in a post-mount effect; its
  `languageChanged` listener now writes `document.cookie` instead of
  `localStorage` (plain client-writable cookie, no Server Action/Route
  Handler round-trip needed for the persisted choice to reach the
  server on a future request).

### Verified

- `tsc --noEmit`/`eslint`/`vitest run` (21 tests, up from 15) all clean;
  `next build` confirms all 26 routes moved from `○ (Static)` to
  `ƒ (Dynamic)` as expected.
- `curl` (no browser JS involved) confirmed the raw server HTML itself
  is correct: a `flox-locale=ru` cookie, or an `Accept-Language: ru-RU`
  header with no cookie, both produced `<html lang="ru">` with Russian
  text already in the initial response body.
- Full manual pass against `next start` (production server) + real
  `api`: 10 full fresh browser navigations across `/postlanding`,
  `/landings`, `/pwa` (4 with `?gallery=<id>`, the exact condition the
  original race depended on) — zero hydration errors, read directly from
  the console each time. Confirmed the live language switcher still
  works instantly and persists correctly to a fresh navigation.

## [Incoming Postback Replay] — apps/api now builds the full conversion engine

### Scope

The architecture decision `docs/postback-logs.md` deferred at the end of
the outgoing-replay phase: how should an operator-triggered replay of a
past **incoming** postback call `apps/internal/conversion.Service.Record`,
the exact engine `apps/tracker`'s `/postback/{networkId}` calls for a real
network hit. An `AskUserQuestion` decided `apps/api` should build the same
dependency graph `apps/tracker/main.go` already builds (Redis-cached dedup
store, ClickHouse-backed attribution, async event writer, outgoing-
delivery enqueuer, attempt logger) rather than making an internal HTTP
call to `apps/tracker` — that endpoint has no operator/admin distinction
from a real network hit, so a replay through it would be indistinguishable
from a forged network request in the audit trail. See `docs/postback-
logs.md`.

### Added

- `apps/api/main.go` now constructs a full `conversion.Service`, gated
  behind the same `ch != nil` startup check the rest of the Postback Logs
  feature already uses.
- **`apps/internal/conversion/replay.go`**: `ReplayNetworkLookup`/
  `ReplayRecorder`, two adapters satisfying `postbacklogs`'
  `IncomingNetworkLookup`/`IncomingRecorder` interfaces — `postbacklogs`
  still never imports `conversion` (same one-directional adapter pattern
  `apps/internal/postback`'s `ReplayEnqueuer` already established for
  outgoing replay).
- `postbacklogs.Service.ReplayIncoming` + `POST /postback-logs/replay-
  incoming`: takes the exact fields a `PostbackLog` row already carries
  (`networkId`, `clickId`, `rawStatus`, `eventRef`, `revenue`, `currency`),
  explicitly checks the resolved network's `OrganizationID` against the
  caller's own (a check a real network hit doesn't need, since its
  `{networkId}` URL param **is** the tenant scope) before ever calling
  `Record`.
- Frontend: `replayIncomingPostback` + `useReplayIncomingPostback`;
  `PostbackReplayButton` now branches on `direction`, surfacing the
  replay's actual result (success/duplicate/ignored/error) in the toast
  instead of a fixed message.

### Verified

- Backend: `go build/vet/gofmt/test ./...` all green — new
  `postbacklogs.Service.ReplayIncoming` unit tests against fakes (happy
  path, network-not-found, cross-tenant not-found, required-field
  validation, not-configured) and new `conversion` adapter tests,
  including one that runs an actual replay-then-duplicate-replay sequence
  through a real `*conversion.Service`.
- Frontend: `tsc --noEmit`/`eslint`/`vitest run`/`next build` (production)
  all clean.
- Full manual browser pass against the real running `api`+`tracker`+
  `worker`+`web` dev servers: sent a real incoming postback with no event
  mapping configured (a genuine `error` row), added the mapping, replayed
  it from the Logs tab. Confirmed directly in ClickHouse: the replay
  recorded a new `success` row and triggered a real outgoing delivery
  attempt; replaying the identical attempt again correctly came back
  `duplicate` with no second event or delivery — the CLAUDE.md #3
  dedup/money-correctness guarantee holding under a manual re-trigger, not
  just a network's own retry. Test network and all its rows (Postgres and
  ClickHouse) removed afterward.

## [Postlanding] — wired to a real Postgres-backed API

### Scope

Third slice of the Landing/PWA/Postlanding/Pixels CRUD candidate, right
after [PWA](#pwa--wired-to-a-real-postgres-backed-api-fixes-an-i18n-hydration-race-shared-with-landings).
`postlandings` (migration 00004) was already flat, no children, real
schema — same shape as Landings/PWA. Per-flow Pixels stays mocked; this
phase is Postlanding only. See `docs/postlanding.md`.

### Added

- **`apps/internal/postlanding`**: `model.go`/`handler.go`/`service.go`/
  `repository.go`, mirroring `apps/internal/landing`/`apps/internal/pwa`'s
  shape almost exactly, wired at `/postlandings` in `apps/api/main.go`.
- `events`: at least one required, each value checked against a curated
  6-entry subset of the §43 event model (`ValidEventTypes`) a postlanding
  can plausibly fire on — not a duplicate of the canonical event enum,
  the frontend references the same string values with checked parity.
- Frontend: `lib/api/postlanding.ts` + `hooks/use-postlandings.ts` (real
  API layer, mirrors PWA's exactly); `postlanding-list`/`-form-sheet`/
  `-columns`/`-row-actions.tsx` rewired off the Zustand mock onto the
  real hooks, with loading/error states added.
- A `postlanding` i18n namespace (`en`+`ru`, key-set parity checked
  directly).

### Removed

- `stores/postlandings.ts` and `lib/mock/postlandings.ts` — deleted
  outright once a grep confirmed zero remaining importers.

### Known issue

- The `requestIdleCallback`-based i18n hydration fix from the PWA phase
  mitigates but does not fully eliminate the hydration race on pages
  using `useSearchParams()` inside `<Suspense>` (Landings/PWA/
  Postlanding). Manual testing this phase reproduced the same
  "Hydration failed" error intermittently (~2 of 6 fresh `/postlanding`
  navigations) and confirmed it also reproduces on `/landings` when
  specifically retested — not a new regression or Postlanding-specific,
  React auto-recovers, left as a known non-blocking issue rather than
  expanding scope into a deeper fix. See `docs/postlanding.md`.

### Verified

- Backend: `go build/vet/gofmt/test ./...` all green — `postlanding_test.go`
  mirrors `landing_test.go`'s/`pwa_test.go`'s full test set.
- Frontend: `tsc --noEmit`/`eslint`/`vitest run`/`next build` (production)
  all clean.
- Full manual browser pass against the real running `api`+`web` dev
  servers, in Russian locale: created/edited/paused/resumed/duplicated/
  archived a postlanding (events multi-select and URL validation both
  exercised, copy kept paused status/URL/events verbatim, archived row's
  action menu correctly dropped to Edit/Duplicate only); confirmed full
  Russian rendering throughout. Test postlanding rows deleted directly
  from Postgres afterward (no hard-delete in the UI for this entity).

## [PWA] — wired to a real Postgres-backed API; fixes an i18n hydration race shared with Landings

### Scope

`AskUserQuestion` picked this as the second slice of the Landing/PWA/
Postlanding/Pixels CRUD candidate, right after Landings landed: `pwas`
(migration 00004) was already flat, no children, real schema — same
shape as Landings. Postlanding and per-flow Pixels stay mocked; this
phase is PWA only. See `docs/pwa.md`.

### Added

- **`apps/internal/pwa`**: `model.go`/`handler.go`/`service.go`/
  `repository.go`, mirroring `apps/internal/landing`'s shape almost
  exactly, wired at `/pwas` in `apps/api/main.go`.
- The one real validation difference from Landing: `startUrl` is a
  relative install path (`/install/sweeps`), not an absolute URL, so
  it's deliberately not run through `isValidURL` — covered by its own
  test, `TestUpdateAcceptsRelativeStartURL`.
- Frontend: `lib/api/pwa.ts` + `hooks/use-pwas.ts` (real API layer,
  mirrors `landings`' exactly); `pwa-list`/`-form-sheet`/`-columns`/
  `-row-actions.tsx` rewired off the Zustand mock onto the real hooks,
  with loading/error states added.
- A `pwa` i18n namespace (`en`+`ru`, key-set parity checked directly).
- **Cross-cutting fix**: `components/i18n-provider.tsx` had a genuine
  React hydration race — its post-mount `i18n.changeLanguage()` call
  could fire before a `useSearchParams()`-driven `<Suspense>`
  boundary's own deferred hydration commit (needed for the Content
  Gallery `?gallery=<id>` integration on Landings/PWA/Postlanding),
  producing a real "Hydration failed" error. This pre-dated this phase
  (introduced with `I18nProvider` itself, already live on `/landings`,
  just not caught during that phase's manual testing — absent on pages
  without `useSearchParams()`, e.g. `/networks`/`/offers`).
  `React.startTransition` and a fixed short `setTimeout` both still
  raced; fixed with `requestIdleCallback` (`setTimeout(fn, 0)` Safari
  fallback), which waits for the main thread to actually go idle
  instead of guessing a constant. Retroactively fixes the same latent
  defect on the already-shipped Landings phase with no Landings code
  touched.

### Removed

- `stores/pwas.ts` and `lib/mock/pwas.ts` — deleted outright once a
  grep confirmed zero remaining importers.

### Verified

- Backend: `go build/vet/gofmt/test ./...` all green — `pwa_test.go`
  mirrors `landing_test.go`'s full test set plus
  `TestUpdateAcceptsRelativeStartURL`.
- Frontend: `tsc --noEmit`/`eslint`/`vitest run`/`next build`
  (production) all clean.
- Full manual browser pass against the real running `api`+`web` dev
  servers, in both locales: created/edited/paused/resumed/duplicated/
  archived a PWA (manifest preview matched form values live, copy kept
  non-active status when applicable, archived row's action menu
  correctly dropped to Edit/Duplicate only); confirmed full Russian
  rendering throughout. Also confirmed the hydration fix via repeated
  `/landings` ↔ `/pwa` navigation with the browser console read
  directly: zero hydration errors on either page. Test PWA row deleted
  directly from Postgres afterward (no hard-delete in the UI for this
  entity).

## [Landings] — wired to a real Postgres-backed API

### Scope

`AskUserQuestion` picked this as the smallest slice of the Landing/PWA/
Postlanding/Pixels CRUD candidate: `landings` (migration 00004) was
already flat, no children, real schema — the exact shape
`apps/internal/network` already establishes a real precedent for. PWA,
Postlanding, and per-flow Pixels stay mocked; this phase is Landings
only. See `docs/landings.md`.

### Added

- **`apps/internal/landing`**: `model.go`/`handler.go`/`service.go`/
  `repository.go`, mirroring `apps/internal/network`'s shape almost
  exactly, wired at `/landings` in `apps/api/main.go`.
- The one real business-logic addition beyond the template: an internal
  landing's `url` (`https://cdn.floxlink.io/lnd/{slug}`) moves from
  client-computed (the old mock's form-submit handler) to
  server-computed in `Service.Create`/`Update` from `Name` — a
  client-supplied `url` for `type: internal` is accepted in the request
  shape but always ignored. `Update` only recomputes `url` when `name`
  or `type` actually changed in that call; `Duplicate` goes through
  `Service.Create` (not `Repository.Create`) so a `(Copy)`'s `url` is
  recomputed for its new name, never copied verbatim. Go's `slugify`
  matches `lib/utils.ts`'s exactly, so the form's client-side preview
  and the server-persisted value never disagree.
- Frontend: `lib/api/landings.ts` + `hooks/use-landings.ts` (real API
  layer, mirrors `networks`' exactly); `landing-list`/`-form-sheet`/
  `-columns`/`-row-actions.tsx` rewired off the Zustand mock onto the
  real hooks, with loading/empty/error states added (the old mock was
  synchronous and had none — DoD requires them regardless).
- A `landings` i18n namespace (`en`+`ru`, registered in
  `lib/i18n/config.ts`) — Landings was correctly out of the dedicated
  i18n phase's scope (still mocked then), but leaving it the one
  hardcoded-English holdout next to every other now-real domain page
  the moment it goes real would be a visible regression.

### Removed

- `stores/landings.ts` and `lib/mock/landings.ts` — deleted outright
  once a grep confirmed zero remaining importers, the same "drop it,
  don't fake it" precedent every prior real-backend phase this session
  has followed.

### Verified

- Backend: `go build/vet/gofmt/test ./...` all green — `landing_test.go`
  mirrors `network_test.go`'s full test set (create/get/update/delete,
  invalid-shape validation, pause/activate transitions, duplicate keeps
  status, cross-tenant isolation) plus a dedicated server-computed-URL
  test (client value ignored on create, URL follows a rename, a
  status-only update leaves URL/content untouched).
- Frontend: `tsc --noEmit`/`eslint`/`next build` (production) all clean.
- Full manual browser pass against the real running `api`+`web` dev
  servers, in both locales: created an internal landing (server URL
  matched the client preview exactly) and an external one (client URL
  persisted untouched; an empty URL is rejected client-side); renamed
  the internal one (URL followed via a real round-trip, not just client
  state); duplicated it (copy's URL recomputed for its new name);
  archived one (the archived row's action menu correctly drops
  Pause/Archive, keeping only Edit/Duplicate — matches Networks'
  identical pattern); confirmed full Russian rendering throughout (list,
  form, row actions, archive confirmation, toasts). Test landings
  deleted afterward (no hard-delete exists in the UI for this entity).

## [Postback Replay] — outgoing deliveries can be re-enqueued from the Logs UI

### Scope

`AskUserQuestion` after inspection: incoming and outgoing replay (both
named as real, buildable, no-new-schema actions when Postback Logs
deferred them) turned out to be very different sizes. Outgoing needed
nothing `apps/api` didn't already have. Incoming needs the same
dependency graph (mapper, dedup store, FX, attribution, async event
writer, delivery enqueuer) only `apps/tracker` has ever had — wiring it
into `apps/api` means either duplicating that graph or `apps/api` calling
`apps/tracker`'s `/postback/{networkId}` internally, a real architecture
choice. User picked outgoing-only this phase; incoming deferred again
pending that choice. See `docs/postback-logs.md`.

### Added

- **`apps/internal/conversion.PostgresStore.FindSuccessID`**: resolves a
  ClickHouse `postback_events` row back to the Postgres `postbacks.id`
  `apps/internal/postback`'s delivery queue needs as its `NOT NULL
  source_postback_id` FK (migration 00014), by the exact dedup key
  (org, network, click_id, status, event_ref) — `event_ref` matters
  because a CPA_REDEP click can have more than one successful row, one
  per redeposit.
- **`apps/internal/postback.ReplayEnqueuer`**: adapts
  `postbacklogs.ReplayInput` → `postback.EnqueueInput`, the same
  decoupled-interface-per-consumer pattern `Enqueuer` (→
  `conversion.DeliveryEnqueuer`) already uses. Unlike `Enqueuer`'s
  best-effort, error-swallowing contract (correct for a call inside
  `Record`, where a queuing hiccup must never be reported as a
  conversion failure), `ReplayEnqueuer`'s error goes straight back to the
  HTTP caller — replaying is the entire point of that request.
- **`POST /postback-logs/replay-outgoing`** on `apps/internal
  /postbacklogs` (new `Service.ReplayOutgoing`): takes the exact fields a
  `PostbackLog` row already carries client-side, no second fetch, and
  reuses the exact URL already macro-resolved and dispatched rather than
  re-resolving against current network config.
- Frontend: `postback-log-columns.tsx`'s `actions` column and
  `RotateCcwIcon` button — dropped entirely in the read-only phase — are
  back, outgoing rows only; `useReplayOutgoingPostback` mutation hook;
  `lib/api/postback-logs.ts`'s `PostbackLog` keeps `eventRef` in its JSON
  now (previously dropped — "the UI never renders it" — needed to
  round-trip back for the replay lookup); new `postbacks.json`
  `logs.replayAria`/`logs.toast.*` keys, en+ru.

### Verified

- Backend: `go build/vet/gofmt/test ./...` all green — a new
  `FindSuccessID` integration test against real Postgres (event_ref
  disambiguation between two REDEP rows, tenant isolation, no-match), a
  new `ReplayEnqueuer` unit test, five new `Service.ReplayOutgoing` unit
  tests against fakes.
- Frontend: `tsc --noEmit`/`eslint`/`next build` (production) all clean.
- Full manual browser pass against the real running `api` + `tracker` +
  `worker` + `web` dev servers — the first phase in this arc needing all
  four, since a genuine outgoing delivery only exists once a real
  incoming postback creates one: created a real network whose postback
  URL points at an unreachable local port, a real event mapping, sent
  two real incoming postbacks through `apps/tracker`'s actual endpoint,
  confirmed `apps/worker`'s `Deliverer` picked both up and logged
  `retrying` attempts. Clicked Replay; confirmed the toast and, directly
  in Postgres, that a **new** `postback_deliveries` row was created
  (fresh id, `attempt_count` reset to 1, correct `source_postback_id`)
  and that the worker picked it up on its own next poll. Confirmed
  Replay is absent on incoming rows. Confirmed error paths directly:
  malformed org header → 422, missing required field → 422 with field
  detail, a status never successfully recorded → 404. Test network and
  every row it produced (Postgres `networks`/`event_mappings`/
  `postbacks`/`postback_deliveries`, ClickHouse `postback_events`) were
  deleted outright afterward — psql/curl-seeded rows with no UI path at
  all, not entities the app itself only lets you archive.

## [Frontend i18n] — client-side EN/RU internationalization foundation

### Scope

A cross-cutting frontend infra phase, added ahead of the next domain per
direct instruction: `react-i18next` foundation (`lib/i18n/config.ts`,
`I18nProvider`, `LanguageSwitcher`, `useLocale()`), then migrated every
currently-real (backend-wired) screen to it — Campaigns, Traffic
Sources, Networks, Offers, Stream Sets/Filters/Flows, Routing Simulator,
Cost, Conversions, Event Mappings, and Postback Logs, plus the shared
app shell (sidebar/topbar/breadcrumbs/command menu) and `DataTable`/
`ErrorState`/`LoadingState`. See `docs/frontend-i18n.md` for the full
namespace map, key conventions, and formatting/pluralization rules.

Session picked up mid-phase: the `en`/`ru` locale JSON (all 11
namespaces, both languages, fully translated) and the shell/`common`/
Campaigns/Traffic Sources/Networks component migrations already
existed; this pass wired the remaining six domains — Offers, Stream
Sets, Routing Simulator, Cost, Conversions, and Postbacks (Event
Mapping/Incoming/Outgoing/Logs panels) — to the pre-written keys.

### Added

- `useTranslation` wired into every remaining component in the six
  domains above: list/detail views, row actions, form sheets (Zod
  schemas converted to `buildXFormSchema(t)` factories, matching the
  existing `buildSourceFormSchema`/`buildNetworkFormSchema` pattern —
  validation messages need the live translator), and column-def
  factories (`t` threaded through as the first argument, matching
  `networkColumns`).
- `lib/filters.ts`: `FILTER_OPERATOR_I18N_KEY` (mirrors the existing
  `FIELD_GROUP_I18N_KEY`); `checkRE2Compatible`/`validateCountryValue`
  now take a `TFunction` and return translated messages.
- `lib/api/conversions.ts`: `CPA_STATUS_I18N_KEY`, the same
  `Record<Value, string>`-of-i18n-keys pattern as
  `SOURCE_TYPE_I18N_KEY`/`COST_INTEGRATION_I18N_KEY`, reused by
  Conversions, Event Mapping, and Postback Logs (all three render CPA
  status badges).

### Fixed (found during manual verification, not pre-existing scope)

- **`campaign-detail-view.tsx`** (Campaign detail page: Overview/Cost/
  Simulator/Settings tabs, stat cards, danger zone) was entirely
  untranslated — it wraps the newly-migrated Cost/Simulator/Stream Sets
  content in English chrome, so a Russian-locale user would see a
  jarring English-tabs-around-Russian-content page. Pre-written
  `campaigns.json` already had a complete `detail`/`overview` namespace
  sitting unused; wired it in.
- **`components/ui/multi-select.tsx`** (shared component, used by
  Offers' Countries picker and the Stream Sets filter builder's Values
  picker): its search placeholder was a hardcoded `Search
  ${label.toLowerCase()}...` template — once callers started passing a
  translated label (e.g. "ГЕО"), this produced a broken mixed-language
  string ("Search гео..."). Added `common.multiSelect.{none,
  searchPlaceholder}` keys and switched the component to
  `useTranslation("common")` internally (the same self-contained
  pattern `data-table.tsx` already uses), reusing
  `common.dataTable.selected`/`emptyTitle` for the count summary and
  "no results" text rather than duplicating them.

### Verified

- `tsc --noEmit`, `eslint`, `next build` (production build, all 26
  routes) all clean.
- `vitest run` — 15/15 passing (`lib/i18n/config.test.ts`, pre-existing,
  unchanged by this pass).
- Full manual browser pass against the real running `api` + `web` dev
  servers, in both locales: created a real network, offer, campaign,
  stream set (with a filter condition and a device-vocab value), and an
  event mapping. Confirmed correct `ru` rendering for all six domains —
  field-group labels, operator words, vocab values, pluralized counts
  (including a genuine `count=0` → Russian "many" form via
  `mappedCount_many`), the routing simulator's full pipeline trace, and
  the multi-select fix — then switched to `en` and confirmed the
  fallback locale still renders correctly. Confirmed backend-generated
  strings (`SimulateResult.destination.label`, `.stickyNote`) correctly
  stay in English per `docs/frontend-i18n.md`'s stated backend
  boundary. Test network/offer/campaign archived afterward (no
  hard-delete exists for these entities in the UI — archiving is this
  app's own non-destructive removal path).

## [Postback Logs] — Wired to real ClickHouse-backed API (read-only, replay deferred)

### Scope

Two rounds of `AskUserQuestion`. First confirmed Postback Logs as the
last remaining piece of the Conversions/Postbacks domain. Second split
scope further once inspection found the old mock's "Replay" button was a
genuine write action — re-invoking `apps/internal/conversion.Service
.Record` for an incoming row, or re-enqueuing a `apps/internal/postback`
delivery for an outgoing one. Both are real and buildable with no schema
changes, but are a second capability beyond a pure read view; user chose
logs-list-only this phase, replay deferred as its own follow-on. See
`docs/postback-logs.md`.

### Added

- **`apps/internal/postbacklogs`** (new, plural — distinct from the
  existing singular `apps/internal/postbacklog`, Phase 24's write-side
  queue/producer that feeds `postback_events`, untouched this phase): a
  thin read layer over ClickHouse's `postback_events`, mirroring
  `apps/internal/analytics`/`apps/internal/conversions`'s own shape. `GET
  /postback-logs` (org-wide, both directions mixed in one list,
  date-ranged, paginated).
- **`chstore` additions**: `ListPostbackAttempts`/`CountPostbackAttempts`
  — the first read methods against `postback_events` (previously
  write-only, fed by `InsertPostbackAttempts`).
- Frontend: `lib/api/postback-logs.ts` + `hooks/use-postback-logs.ts`
  (new, TanStack Query). `PostbackLogsPanel`/`postback-log-columns.tsx`
  rewritten against the real hook, with a wider, real `PostbackResult`
  vocabulary (incoming: success/duplicate/ignored/error; outgoing:
  queued/processing/success/failed/retrying/dead) replacing the old
  mock's three-value fiction.

### Fixed (closing a gap the Event Mappings phase documented)

`IncomingPostbacksPanel` was still reading the mock `useNetworksStore`/
`useEventMappingsStore` even after Event Mappings CRUD landed real —
flagged as a documented inconsistency in that phase's own changelog
entry. Switched to the real `useNetworks()`/`useEventMappings()` hooks
(both already existed), closing the gap.

### Removed

- `postback-log-columns.tsx`'s Replay action column and its
  `RotateCcwIcon` button — dropped entirely, not disabled or faked,
  matching this phase's scope decision above.
- **`stores/postback-logs.ts`**, **`lib/mock/postback-logs.ts`**,
  **`stores/networks.ts`**, **`lib/mock/networks.ts`** — once
  `PostbackLogsPanel` and `IncomingPostbacksPanel` both moved to real
  hooks, a repo-wide grep found all four had zero remaining importers
  (each only imported the next one in the chain). Deleted outright, the
  same "drop it, don't fake it" precedent as the Routing Simulator
  phase's `stream-sets` mock/store pair.

### Verified

- Backend: `go build/vet/gofmt/test ./...` all green — 2 new `chstore`
  integration tests against real ClickHouse, 2 new
  `postbacklogs.Service` unit tests against a fake repository.
- Frontend: `tsc --noEmit`/`eslint` clean.
- Full manual browser pass against the real running `api` + `web` dev
  servers: created a real network and a real event mapping, seeded 4
  real `postback_events` rows (an incoming/outgoing success pair, an
  incoming error, an outgoing retrying) via a throwaway, never-committed
  `chstore.EventStore.InsertPostbackAttempts` call. Confirmed the Logs
  tab renders all four correctly — raw-to-mapped status display for
  incoming, mapped-only for outgoing, correct result badges across the
  wider real vocabulary, no Replay column — and the Incoming tab now
  shows the real network id and a correct real mapped-count badge. Test
  network and its `postback_events` rows removed afterward.

### Domain complete

Conversions, Event Mappings, and Postback Logs (read-only) are all real
now — the "Conversions/Postbacks" domain is fully wired except for the
deliberately-deferred Replay action.

## [Event Mappings] — CRUD wired to real Postgres-backed API

### Scope

Confirmed via `AskUserQuestion` (the recommended option) as the smallest
remaining piece of the Conversions/Postbacks domain — a small,
self-contained CRUD slice with no ClickHouse involved, shaped like
Networks/Offers. See `docs/event-mappings.md`.

### Added

- **`apps/internal/eventmapping`** — writes rows for `event_mappings`
  (migration 00012), the table `apps/internal/conversion.PostgresMapper
  .MapStatus` already read at postback-ingest time (Phase 23). Never
  duplicates that lookup — the same relationship `streamset` has to
  `routingstore`. `FloxStatus` reuses `event.Type` directly (validated
  via the already-existing `event.Type.IsCPA()`), not a redeclared enum.
- `GET`/`POST /event-mappings` (org-wide, not per-network — the panel
  groups by `networkId` client-side, matching the old mock's org-wide
  array) and `DELETE /event-mappings/{id}`. No `PATCH`: the UI only ever
  adds or removes a mapping, never edits one in place.
- Duplicate detection relies on the database's existing unique index
  (`network_id, lower(network_status)`) rather than a race-prone
  check-then-insert — `Create` catches Postgres `23505` into a real
  `apierror.Conflict`, this codebase's first `23505` catch (every prior
  one was `23503` FK violations).
- Frontend: `lib/api/event-mappings.ts` + `hooks/use-event-mappings.ts`
  (new, TanStack Query). `EventMappingPanel` rewired to the real hooks.

### Fixed (a pre-existing gap, not new to this phase)

`EventMappingPanel` was still reading the mock `useNetworksStore` (stale
fabricated network ids like `net_afftrust`) even though it already
imported the real `CPA_STATUSES` from the Conversions phase — switched
to the real `useNetworks()` hook, since managing mappings for networks
that couldn't match any real `network_id` would have been pointless.

### Not changed, documented instead

`stores/event-mappings.ts` and `lib/mock/event-mappings.ts` were **not**
deleted, unlike the Routing Simulator phase's stream-sets mock/store
pair — both are still read by `IncomingPostbacksPanel` (a "mapped count"
badge) and the deferred Postback Logs mock, both out of this phase's
scope. Net result: the Postbacks page's "Event Mapping" tab now manages
real data while its "Incoming" tab still shows stale mock networks with
a mapped-count badge sourced from the mock store — confirmed live, not
just inferred, and documented in `docs/event-mappings.md` rather than
papered over.

### Verified

- Backend: `go build/vet/gofmt/test ./...` all green — 5 new
  `eventmapping` tests (CRUD round-trip, invalid `FloxStatus` rejected,
  case-insensitive duplicate rejected with a real conflict apierror,
  unknown network id rejected, full cross-tenant isolation).
- Frontend: `tsc --noEmit`/`eslint` clean.
- Full manual browser pass against the real running `api` + `web` dev
  servers: created two real networks, added a mapping through the real
  form (confirmed landed in Postgres via a direct API call), attempted a
  case-insensitive duplicate and confirmed the real `409` by inspecting
  the network request directly, removed the mapping and confirmed it was
  gone both in the UI and via the API, confirmed the Incoming tab's
  stale-but-non-crashing inconsistency live. Test networks removed via
  the real `DELETE` endpoint afterward.

## [Conversions] — List + detail/timeline wired to real ClickHouse-backed API

### Scope

Two rounds of `AskUserQuestion`. First picked "Conversions/Postbacks" as
the domain from three candidates. Inspection then found the hard backend
(dedup, delivery, ClickHouse logging) already fully built and wired into
`apps/tracker`/`apps/worker` — only a browser-facing read API was
missing, and the domain was really three separable pieces. Second
question narrowed to this slice: Conversions list + detail/timeline,
deferring Postback Logs and Event Mappings CRUD. See `docs/conversions.md`.

### Added

- **`apps/internal/conversions`** (new, plural — distinct from the
  existing singular `apps/internal/conversion`, Phase 23's write-side
  dedup/status-progression engine, untouched this phase): a thin read
  layer over ClickHouse's `conversion_events`/`click_events`/
  `tracking_events`, mirroring `apps/internal/analytics`'s own shape. `GET
  /conversions` (org-wide, date-ranged, paginated) lists CPA_* rows; `GET
  /conversions/{clickId}` returns a merged, chronological timeline of
  every funnel + conversion event for that click_id.
- **`chstore` additions**: `ListConversions`/`CountConversions`/
  `ConversionsByClickID`/`FunnelByClickID` — the first read methods on
  `EventStore` for `click_events`/`tracking_events`/`conversion_events`
  (previously write-only, fed by `InsertBatch`).
- A click_id can carry more than one `conversion_events` row (HOLD, then
  ACCEPT, then REDEP, ...) — real status history, not duplicate rows to
  dedupe. The old mock's fixed six-stage funnel (Click/Landing/PWA/Offer/
  Conversion/Postback, four stages fabricated with synthetic sentences)
  is replaced by a real, variable-length chronological list of whatever
  §43 events actually happened for that click_id.
- Frontend: `lib/api/conversions.ts` + `hooks/use-conversions.ts` (new,
  TanStack Query). `conversion-list.tsx`/`conversion-detail-view.tsx`/
  `conversion-timeline.tsx`/`conversion-columns.tsx` all rewritten
  against the real hooks. `CpaStatus`/`CPA_STATUSES` moved from
  `lib/mock/conversions.ts` to `lib/api/conversions.ts` (a real domain
  enum, not mock-specific) — every consumer, including the still-mocked
  Postback Logs/Event Mappings features, now imports it from there.

### Removed

- **Offer column**: `conversion_events` carries no `offer_id` (only
  `flow_id`, which would need a separate Postgres join to resolve — out
  of scope). Campaign and Network (both directly on the ClickHouse row)
  took its place.
- **Postback delivery status / "Resend postback"**: the deferred
  Postback Logs domain — the old mock's `postbackStatus` column, its
  StatCard, and the resend button are gone entirely, not shown as "—" or
  left disabled.
- **`stores/conversions.ts`** — no remaining importers once
  `ConversionList`/`ConversionDetailView` switched to the real hooks;
  deleted outright. `lib/mock/conversions.ts` stays, trimmed to just
  what the still-mocked Postback Logs feature cross-references.

### Fixed

`GET /conversions?to=YYYY-MM-DD` parsed `to` as midnight UTC of that
date. `internal/analytics`'s daily endpoints do the same thing safely,
because they query pre-aggregated day-granularity materialized views
where a date-only comparison is exactly right. This package queries raw
`event_at` timestamps — every event later that same day was silently
excluded (`event_at <= to` false for anything after 00:00:00). Caught
live seeding test data "today" and passing an explicit `?to=<today>`.
Fixed by pushing an explicit date-only `to` to end-of-day (`+24h-1ns`) in
the handler.

### Verified

- Backend: `go build/vet/gofmt/test ./...` all green — `chstore`
  integration tests against real ClickHouse for all four new query
  methods, `conversions.Service` unit tests against a fake repository
  (range/limit validation, chronological funnel+conversion merge, the
  no-events-found 404).
- Frontend: `tsc --noEmit`/`eslint` clean.
- Full manual browser pass against the real running `api` + `web` dev
  servers: seeded two real click_ids' worth of ClickHouse events (one
  full funnel, one bare click-then-decline) via a throwaway, never-
  committed `chstore.EventStore.InsertBatch` call against a real test
  campaign/network created through the real API. Confirmed the list's
  multi-row status history with resolved campaign/network names, both
  detail pages' real and genuinely different-length timelines, a clean
  404 error state for an unknown click_id, and that the still-mocked
  Postback Logs/Event Mapping panels (both touched by the `CpaStatus`
  move) kept rendering correctly. Test campaign, network, and their
  ClickHouse rows removed afterward.

## [Routing Simulator] — Wired to a real /routing/simulate endpoint

### Scope

Direct instruction, no `AskUserQuestion` scope negotiation needed — the
user named this exact candidate from the previous phase's own "NEXT"
list. See `docs/routing-simulate.md`.

### Added

- **`apps/internal/routingsimulate`** (new package) — `Service.Simulate`
  loads a campaign's real routing config via
  `routingstore.LoadRoutingConfig` and calls `routing.Engine.Explain`
  (never `Resolve` — the simulator's whole point is the trace `Resolve`
  doesn't return), reshaping the result into the frontend's wire
  contract. `Handler` mounts `POST /campaigns/{campaignId}/routing/
  simulate`, tenant-scoped like every other handler. No new routing
  decision logic anywhere (CLAUDE.md #1) — this is a thin reshape of an
  already-existing pure evaluation into JSON.
- **`routing.Explanation.DestinationLabel`**: the same human label
  (`"Offer"`, `"Redirect"`, `"Stream Set fallback"`, `"Campaign
  fallback"`, `"No destination configured"`) `resolveDestination`
  already computed for `RouteResult.Reason` internally, now also stored
  structurally instead of discarded.
- **`routing.Trace`/`StreamSetEvaluation`/`FlowCandidate` JSON tags**
  matching the frontend's field names exactly — reused directly in the
  simulate response with no separate DTO layer, honoring `trace.go`'s
  own doc comment written for exactly this moment.
- Sticky is never simulated (`Sticky: nil` always — a fabricated cookie
  would look identical to a real returning visitor's, which is
  misleading for a debugging tool). The response's `stickyNote` is
  generated from the campaign's real `sticky_flow` flag instead of the
  old mock's static, increasingly-stale string.
- `deriveVisitKey` reimplemented byte-for-byte in Go, matching the old
  frontend mock's algorithm. Unlike the mock, an all-empty request
  against a matched stream set with more than one tied weighted flow now
  surfaces a real `422` (`routing.ErrNoVisitKey`) instead of silently
  guessing.
- **Frontend**: `lib/routing-simulate.ts` (the mock) deleted outright —
  its type definitions moved into `lib/api/routing.ts`, the seam file
  built in an earlier phase specifically for this swap. `routing-
  simulator-view.tsx` dropped its `useStreamSetsStore` read (the server
  now loads stream sets itself) and calls `simulateRoute(campaignId,
  request)` instead of `(streamSets, fallbackUrl, request)`.
  `simulator-form.tsx`/`stream-set-trace.tsx` needed only an import-path
  change. `simulator-result.tsx` dropped the `selectedFlow` boolean
  check (now `flowCandidates.some(c => c.selected)`) and the `kind`-keyed
  `DESTINATION_LABEL` lookup (now renders `destination.label` directly).

### Removed

- `lib/mock/stream-sets.ts` / `stores/stream-sets.ts` — kept in the
  Stream Sets phase specifically because the Routing Simulator was their
  one remaining reader. Once the Simulator switched to the real API, a
  repo-wide grep found zero remaining importers of either; deleted
  outright rather than left as an orphaned pair.

### Fixed

Two real bugs caught during manual browser verification, both the same
root mistake hit twice: Go's `encoding/json` `omitempty` omits *any*
zero-length slice, nil or not — not just nil ones.

- `routingsimulate.Response.FlowCandidates` was `nil` (no weighted draw
  ever ran) when no stream set matched, encoding as JSON `null` and
  crashing `SimulatorResult`'s unconditional `.some()` call. Fixed by
  normalizing to `[]` in `Service.Simulate`.
- `routing.Trace.Children` and `streamset.FilterNode.Children` both had
  `omitempty`. An empty top-level filter group ("no filters — matches
  all traffic," a normal, UI-documented configuration) produces a real,
  non-nil, zero-length `Children` that `omitempty` hid anyway — crashing
  not just the Simulator but the Stream Sets card on *any* campaign with
  an empty-filter stream set, since `hydrateFilterNode`/`FilterTraceView`
  call `.map()`/`.length` on `children` unconditionally. Fixed by
  dropping `omitempty` from both types' `Children` tags. Neither bug was
  reachable in any prior phase's verification pass, because no earlier
  test fixture had exercised an empty root filter group through a real
  HTTP round-trip.

### Verified

- Backend: `go build/vet/gofmt/test ./...` all green — 5 new
  `routingsimulate` tests (match + weighted pick, no-match campaign
  fallback with a non-nil-`FlowCandidates` regression assertion,
  ambiguous-tie `422`, sticky-note reflecting the campaign's real
  `sticky_flow` flag, cross-tenant isolation) and 2 new regression tests
  for the `omitempty` bug (`streamset`, `routing`). The pre-existing
  18-case `routing` conformance fixture passes unchanged.
- Frontend: `tsc --noEmit`/`eslint` clean.
- Full manual browser pass against the real running `api` + `web` dev
  servers: matched and non-matched filter simulation against a real
  stream set; a weighted 50/50 tie-break with a deterministic pick;
  sticky-enabled note toggling live against a real Postgres update. Both
  bugs above were caught and fixed during this same pass. Test campaign
  (cascading to its stream set), offer, and network removed via real
  `DELETE` afterward.

## [Stream Sets, Filters & Flows CRUD] — Frontend/Backend Integration, next domain slice

### Scope

Confirmed via `AskUserQuestion` (the recommended option): Stream Sets +
Filters (recursive AND/OR tree) + Flows CRUD, dropping the Landing/PWA/
Postlanding stage pickers and per-flow Pixels from the writable form —
none of those four have a real backend yet, and their DB columns/tables
are nullable/optional specifically so a Flow can exist without them.
Wiring the Routing Simulator to a real `/routing/simulate` endpoint was
scoped out as its own smaller follow-on phase. See `docs/stream-sets.md`.

### Added

- **`apps/internal/streamset`** writes rows for the read path that
  already existed (`apps/internal/routingstore` + `apps/internal/routing`,
  used by the tracker's hot-path redirect) — never duplicates matching or
  weighted-selection logic (CLAUDE.md #1), and reuses
  `routing.FilterField`/`Operator`/`Joiner`/`DestinationKind`/
  `StreamSetStatus` directly rather than redefining the same enums twice.
- **`FilterNode`**: a flattened discriminated-union struct (mirrors how
  `routing/trace.go`'s own `Trace` type already solves the same
  frontend-union-on-the-wire problem), with no `id` — `routing.FilterNode`
  doesn't need one either. The frontend hydrates fresh client-side ids on
  load (`hydrateFilterNode`/`hydrateRootFilter`) for
  `filter-group-builder.tsx`'s id-addressed tree-mutation helpers, and
  strips them back out before saving (`dehydrateFilterNode`).
- **Server-side filter validation**: `MATCHES` conditions compiled with
  Go's real `regexp.Compile` (which *is* RE2, CLAUDE.md #8 — no separate
  heuristic needed, unlike the frontend's own client-side-only
  `checkRE2Compatible`); country codes checked against ISO-3166 alpha-2
  (rejects `"UK"`, same as the frontend's `validateCountryValue`, now
  enforced server-side too — "never trust the client as the source of
  truth" per `lib/filters.ts`'s own comment about its own heuristic).
- **Offer-destination network derivation**: a flow's `networkId` is
  looked up from the offer's own `network_id`, never trusted from the
  client — the flows table denormalizes both ids (its own CHECK
  constraint requires it), but there's exactly one correct network for a
  given offer. `TestOfferDestinationDerivesNetworkFromOffer` sends a
  deliberately wrong network id alongside a real offer id and asserts the
  real one wins.
- **Priority**: never client-supplied (no field in the form at all) —
  `Create` always appends after the campaign's existing stream sets,
  `POST .../stream-sets/reorder` takes a full dragged-list order and
  rewrites every priority in one transaction, matching
  `stores/stream-sets.ts`'s own `addStreamSet`/`reorder` exactly.
- **Frontend**: `lib/api/stream-sets.ts` + `hooks/use-stream-sets.ts` (new,
  parallel to the untouched mock) wire the existing Stream Sets card to
  the real API. `FlowFunnel` (which rendered the now-dropped Landing/PWA/
  Postlanding/Offer/Redirect/Fallback chain) is deleted outright, replaced
  by a new, leaner `FlowDestinationEditor` (Offer-or-Redirect + a
  read-only Fallback preview) reusing the generic `FlowNode` component
  unchanged. `filter-group-builder.tsx`/`filter-condition-editor.tsx`
  needed zero changes — both were already pure, store-free components
  operating only on the tree shape passed through props.
- **`docs/stream-sets.md`**; `docs/frontend-integration.md` and
  `ARCHITECTURE.md`'s §6-SHARED note updated — Stream Sets/Filters/Flows
  are off the "still mocked" list, and the Routing Simulator's own note
  now names what's actually left (a thin `/routing/simulate` handler over
  already-built primitives, not a new engine).

### Fixed

- **A real render-loop bug, caught during manual verification, not
  shipped**: the new offer picker would select correctly for an instant,
  then silently reset to empty. Root cause: React Hook Form's
  `useFieldArray().update()` is documented (by RHF itself) to unregister
  and re-register the field row on every call, remounting that row's
  whole subtree — including its Radix `<Select>`s — on every keystroke or
  selection; the remounting Select fired a stray `onValueChange("")` that
  raced the real selection and won. Confirmed live: a temporary
  mount/unmount effect log fired on every field edit, and a temporary
  handler log showed three `onValueChange` calls per click (the real
  offer id, then `""` twice more). Fixed by switching every per-flow edit
  from `flowArray.update(index, {...flow, ...patch})` to
  `setValue(`flows.${index}`, {...flow, ...patch})`, which patches the
  field in place with no remount. Hardened defensively alongside: the
  offer `<Select>`'s controlled value is never handed a raw empty string
  (Radix's own docs warn against an empty `SelectItem` value for the same
  underlying fragility) — a `NO_OFFER` sentinel stands in for "nothing
  chosen yet" on the wire instead.

### Notes

- **A pre-existing schema quirk inherited, not introduced**: `filter_
  conditions.position`/`filter_groups.position` are two independent
  ordering sequences (conditions among themselves, sub-groups among
  themselves), not one interleaved order — the read path
  (`routingstore.loadFilterTrees`) has always reconstructed a group's
  children as "conditions first, then nested groups" regardless of
  original interleaving, since AND/OR evaluation is commutative and never
  cared. This phase's `insertFilterGroup` matches that exact reconstruction
  rather than inventing a different write-side order the reader would
  just re-order anyway.
- 6 new integration tests in `apps/internal/streamset` (CRUD round-trip
  with a nested two-level filter tree, rejecting empty flows / a
  condition-as-root / an invalid country code / an invalid RE2 pattern /
  an incomplete BETWEEN, the offer-network derivation proof, reorder
  rewriting priority correctly, duplicate keeping status and copying the
  tree+flows with fresh ids, full cross-tenant isolation including
  create-against-another-org's-campaign) — all pass against real
  Postgres.
- Verified end-to-end against the real running `api` + `web` dev servers:
  created a network, offer, and campaign; built a stream set through the
  complete form (name, one filter condition, one offer-destination flow);
  confirmed via the real API response that the filter tree and resolved
  network id were exactly right; edited it and confirmed the hydrated
  filter tree and offer selection both pre-filled correctly; duplicated
  it (kept the tree, flow, and `active` status); toggled status to
  `paused` live; reordered two stream sets via the real endpoint and
  confirmed the new priorities survived a page reload. Test campaign
  (cascading to its stream sets), offer, and network removed via the real
  `DELETE` endpoints afterward — all net-new for this phase, no
  pre-existing seed data at risk.
- Backend: `go build/vet/gofmt/test ./...` all green. Frontend:
  `tsc --noEmit` and `eslint` both clean.

## [Networks & Offers CRUD] — Frontend/Backend Integration, next domain slice

### Scope

"Offers" was picked as the next slice on the assumption its network
reference could stay free-text until Networks existed separately — wrong:
`offers.network_id` is `NOT NULL REFERENCES networks (id)` (00003). This
was discovered before any code was written and surfaced via
`AskUserQuestion` rather than silently absorbing the extra scope or
silently weakening the schema to route around it. The user chose to build
Networks + Offers + nested `offer_links` together, completing §27's own
stated hierarchy (Network → Offer → Offer Link) in one phase. See
`docs/networks-offers.md`.

### Added

- **`apps/internal/network`** — flat entity CRUD, mirrors
  `internal/trafficsource` closely (same handler/service/repository
  split, same Duplicate-keeps-status reasoning). `Delete` is the mirror
  image of `trafficsource`'s own delete story: `offers.network_id`
  `CASCADE`s (deleting a network deletes its offers, by original schema
  design, not a choice this phase makes), while
  `flows.destination_network_id` (no Flow CRUD exists yet) would
  `RESTRICT` — caught defensively with the same `23503`-to-409 pattern.
- **`apps/internal/offer`** — the first non-flat domain in this session.
  `NetworkID` is validated against the owning org via
  `NetworkBelongsToOrg` before insert/update (§36-TENANCY, same pattern as
  `campaign`→`traffic_source`). `offer_links` use whole-array replace
  (delete-all/insert-all in one transaction) on every write where `Links`
  is present — matching the frontend form's `useFieldArray`, which
  submits every link on every save rather than a diff. No standalone link
  endpoint; a link is never addressed independently of its offer.
- **`OptionalCap`**: a small custom `json.Unmarshaler` distinguishing
  PATCH's three real states for `cap` — not sent (leave unchanged), sent
  as `null` (clear to uncapped), sent as a number (set that cap). A plain
  `*int` can't tell "key absent" apart from "key present with `null`"; the
  frontend side needed no equivalent trick — `UpdateOfferInput.cap` is
  typed `number | null | undefined` and relies on `JSON.stringify` already
  dropping `undefined` keys.
- **Frontend**: `lib/api/networks.ts` + `lib/api/offers.ts` (new, parallel
  to the untouched mocks) wire the existing mock CRUD UIs
  (`network-*`/`offer-*` feature components) to the real API, unchanged in
  shape. `lib/mock/{networks,offers}.ts` and `stores/{networks,offers}.ts`
  are left exactly as they were — stream-sets/postbacks/conversions
  (still fully mocked) import them transitively, same situation
  `lib/mock/campaigns.ts` was left in after Phase 27.
- **`docs/networks-offers.md`**; `docs/frontend-integration.md` updated —
  Networks and Offers are off the "still mocked" list now.

### Fixed

- **A real render-loop bug, caught during manual verification, not
  shipped**: `offer-form-sheet.tsx` crashed on open with React's "Maximum
  update depth exceeded." Every other rewritten form this session uses
  React Hook Form's `values` option so it re-syncs without a full remount;
  combined with this form's `useFieldArray` (the links list) and a
  `MultiSelect` (countries), a fresh `values` object literal on every
  render caused RHF to keep re-syncing the field array, which triggered a
  Radix Popper ref-callback state update, which caused another render,
  forever. Fixed by reverting to plain `defaultValues` (read once per
  mount) and restoring `key={target?.id ?? "new"}` on all three list
  components' form-dialog wrappers — the remount-on-target-change pattern
  the original mock-backed components already had, which this session's
  earlier rewrites (Traffic Sources, then this phase's own first pass)
  had quietly dropped when extracting the dialog into its own component.
  `values` stays correct for the two simple forms with no array fields.

### Notes

- 6 new integration tests in `apps/internal/network` (CRUD round-trip,
  invalid-URL rejection, pause/activate transitions, duplicate-keeps-
  status, a direct cascade-delete-to-offers proof, cross-tenant
  isolation) and 6 in `apps/internal/offer` (CRUD + country/currency
  normalization, no-links/no-countries/non-positive-payout/cross-org-
  network all rejected, whole-array link replace, the three-state Cap
  PATCH, duplicate keeps status and copies links with fresh ids,
  cross-tenant isolation) — all pass against real Postgres.
- Verified end-to-end against the real running `api` + `web` dev servers:
  created a network, then a full offer against it (network picker, GEO
  multi-select, payout, currency, daily cap, one tracking link) through
  the UI; edited it and confirmed every field pre-filled correctly
  including the link URL, with the Status field present (edit-only);
  paused it (status flipped live); duplicated it (copy kept every field
  including GEOs/cap, and correctly stayed `paused` rather than
  resetting). Test offers and the test network removed via the real
  `DELETE` endpoints afterward — both net-new for this phase, so no
  pre-existing seed data was at risk (unlike Traffic Sources' incident).
- Backend: `go build/vet/gofmt/test ./...` all green. Frontend:
  `tsc --noEmit` and `eslint` both clean.

## [Traffic Sources CRUD] — Frontend/Backend Integration, next domain slice

### Added

- **`apps/internal/trafficsource`** grew from Phase 27's deliberately
  read-only `List` into full CRUD (`Create`/`Get`/`Update`/`Delete`/
  `Duplicate`/`Pause`/`Activate`), mirroring `internal/campaign`'s
  handler→service→repository split exactly. Chosen over Offers/Networks/
  Stream Sets (confirmed via `AskUserQuestion`) as the smallest next
  slice: the read endpoint and a complete mock CRUD UI already existed,
  and the entity has no nested children.
- **`Service.Duplicate` keeps the source's status as-is** — unlike
  `campaign.Service.Duplicate`, which resets a copy to `"draft"`.
  `TrafficSource` has no draft-equivalent status, and the mock store this
  replaced never reset it either.
- **`Repository.Delete` turns a Postgres FK violation into a clean 409**:
  `campaigns.traffic_source_id` has no `ON DELETE` clause (defaults to
  `RESTRICT`, deliberately — a source with campaigns still pointing at it
  shouldn't silently vanish). Catches error code `23503` and returns
  `apierror.Conflict` instead of letting a raw 500 through.
- **Frontend**: `lib/api/traffic-sources.ts` + `hooks/use-traffic-sources.ts`
  fully rewritten off the mock store onto the real API; `source-list.tsx`/
  `source-form-sheet.tsx`/`source-row-actions.tsx`/`source-columns.tsx`
  wired to real queries/mutations with their existing UI shape unchanged
  (create/edit sheet, pause/resume/duplicate/archive row actions, tags
  column still local-mock same as campaigns).
- **`docs/traffic-sources.md`**; `docs/frontend-integration.md` updated —
  Traffic Sources is off the "still mocked" list now.

### Removed

- **`lib/mock/traffic-sources.ts` and `stores/traffic-sources.ts` deleted
  outright** — unlike campaigns' equivalents (Phase 27 kept
  `lib/mock/campaigns.ts`/`stores/campaigns.ts` because other still-mocked
  features — conversions, tag assignments — import them transitively),
  nothing outside the traffic-sources feature ever referenced these, so
  there was nothing to preserve.

### Notes

- 7 new integration tests in `apps/internal/trafficsource` (real Postgres,
  `DATABASE_URL`-gated): create/get/update/delete round-trip, invalid
  tracking-template URL rejected, pause/activate transitions (incl.
  idempotency from the target state and rejection from archived),
  duplicate keeps status, delete conflicts when a campaign references the
  source, and full cross-tenant isolation across get/update/delete/list.
- Verified end-to-end against the real running `api` + `web` dev servers:
  created a source through the UI, paused it (status flipped live),
  duplicated it (copy correctly kept `paused`, not reset), opened Edit
  and confirmed real pre-filled data with the Status field present
  (edit-only). The FK-conflict path was verified directly via `curl`
  against a real referencing campaign — a clean `409`, not a raw
  Postgres error.
- **A real mistake made and corrected during verification, not shipped
  silently**: one of the dev org's two seeded traffic sources ("Facebook
  Ads") was deleted while testing the delete flow, before the
  FK-referencing test case that would have caught it (no campaign existed
  at that moment to trigger the conflict path). Noticed immediately,
  recreated with the same name/type to restore dev state — see
  `docs/traffic-sources.md` for the full account.
- Backend: `go build/vet/gofmt/test ./...` all green. Frontend:
  `tsc --noEmit` and `eslint` both clean.

## [Phase 27-COST] — Cost Ingestion

### Added

- **`apps/internal/cost`** (repository/service/handler, mirrors
  `campaign`'s split) — manual cost entry MVP, closing the Spend/Profit/
  ROI/CPA gap Phase 27 documented and deliberately left open.
  `Repository.Upsert` overwrites-in-place on re-submitting the same
  (campaign, source, day), matching `cost_entries`' own two partial
  unique indexes (00009) exactly — two `ON CONFLICT` statements chosen by
  whether a traffic source is set, since Postgres can't target both
  partial indexes from one clause.
- **FX conversion reused, not reimplemented**: `cost.FXConverter` is the
  same shape as `internal/conversion.FXConverter`, satisfied structurally
  by the existing `conversion.PostgresFX` — one `fx_rates` lookup
  implementation for both packages. USD is already 1:1 special-cased
  there, so the common case (manual entries default to USD) always
  converts cleanly with no rate seeding needed.
- **Migration 00017**: `cost_entries.amount_usd` (nullable, §50-FX/
  CLAUDE.md #7 — a missing FX rate is NULL, never a silent 0, same
  pattern as `conversion_events.usd_value`). `created_by_user_id` DROP
  NOT NULL — there's no auth yet (Phase 28), so there's no real user id
  to attribute a dev-created entry to; a fabricated placeholder user
  would be a fake fact recorded as real data, which is worse than an
  honest NULL.
- **`DailyCampaignSpend`** sums `amount_usd` per day and flags days where
  `bool_and(amount_usd IS NOT NULL)` is false — a day with an unconverted
  entry is visibly incomplete, not silently understated by `SUM()`
  skipping the NULL.
- **`GET /campaigns/{id}/cost-entries/daily`** lives on `cost.Handler`
  itself, not under `/analytics`: unlike click/revenue analytics, spend
  never depends on ClickHouse, so its availability shouldn't share
  `apps/api`'s `if ch != nil` mount guard for the rest of `/analytics`.
- **Frontend**: a new "Cost" tab on the campaign detail page (add/edit/
  delete spend by day + optional source — the form doubles as the edit
  flow via the same upsert semantics as the backend). Overview's Spend/
  Profit/ROI/CPA stat cards are real: Spend is a direct sum (`$0.00` when
  genuinely empty, an honest number); Profit/ROI/CPA render `"—"`
  whenever no cost is entered for the range (CLAUDE.md #6 — never a
  false-positive ratio against an implicit zero); CPA additionally
  renders `"—"` whenever conversions are zero regardless of cost, since
  cost-per-acquisition with zero acquisitions is a division by zero, not
  a $0.00 acquisition cost.
- **`docs/cost-ingestion.md`**; `ARCHITECTURE.md` and
  `docs/frontend-integration.md` updated to reflect the closed gap.

### Notes

- **Deliberate architecture call, not a shortcut**: `cost_events`
  (ClickHouse, schema-only since Phase 26, with a comment promising this
  phase would build its sync pipeline) stays schema-only. Daily spend is
  answered directly from Postgres `cost_entries` — at manual-entry
  volume a `GROUP BY entry_date` is simpler and correct, and a
  write-through sync into a table with zero readers would be exactly the
  kind of abstraction CLAUDE.md's "don't add features beyond what the
  task requires" rules out. Revisit once FB/TikTok ad-spend import (§74,
  still unbuilt, explicitly "later" in §27-COST's own text) produces
  ClickHouse-scale volume.
- **A real bug caught during manual verification, not shipped**: the
  first CPA implementation returned `0` (not `null`) when conversions
  were zero, which rendered as `$0.00` — a plausible-looking but false
  acquisition cost. Caught live in the browser (a campaign with $150
  logged spend and zero conversions showed CPA `$0.00` instead of `"—"`),
  fixed before closing the phase, re-verified.
- 4 new integration tests in `apps/internal/cost` (real Postgres,
  `DATABASE_URL`-gated same as `campaign`/`trafficsource`): upsert
  updates in place rather than stacking, a missing FX rate stores `nil`
  not `0`, a mixed converted/unconverted day is correctly flagged
  incomplete, and full cross-tenant isolation (create/list/delete/daily
  spend all refuse another org's campaign).
- Verified end-to-end against the real running `api` + `web` dev
  servers: logged a $150 entry with zero conversions through the Cost
  tab UI, confirmed Spend/Profit/ROI populated correctly and CPA showed
  `"—"`, deleted the entry through the UI, confirmed the empty state and
  all four cards reverted. Backend also verified independently: `go
  build/vet/gofmt/test ./...` all green. Frontend: `tsc --noEmit` and
  `eslint` both clean. Test campaign removed via the real `DELETE`
  endpoint after.

## [Phase 27] — Frontend/Backend Integration

### Scope

§51's literal phase order ("auth, campaigns, sources, offers, networks,
flows, stream sets, filters, tracking, conversions, analytics, ltv/cohorts,
postbacks") assumes backend APIs that were never built — `ROADMAP.md` has
exactly one dedicated backend-API phase before this one (18, "Campaign
API"), and auth doesn't exist until Phase 28. Negotiated down to a single
concrete slice with the user via two `AskUserQuestion` rounds rather than
silently absorbing eleven phases' worth of missing backend work or silently
picking a scope: **Campaigns CRUD + real analytics on the campaign detail
page**. See `docs/frontend-integration.md`.

### Added

- **`apps/web/src/lib/api`** (`client.ts`, `campaigns.ts`,
  `traffic-sources.ts`, `analytics.ts`) — the first real fetch layer
  against the Go API, replacing `apps/web/src/lib/mock/*` for campaigns.
  `apiFetch<T>()` sends `X-Organization-Id` from
  `NEXT_PUBLIC_DEV_ORG_ID`, the frontend's temporary stand-in for auth
  (mirrors `internal/tenant`'s header — both go away in Phase 28), and
  throws a distinct `MissingDevOrgError` when unset so the failure is a
  clear message, not a silent 400.
- **`hooks/use-{campaigns,traffic-sources,campaign-analytics}.ts`** —
  TanStack Query hooks (first real use of the library since it was
  installed); `campaign-list.tsx`, `campaign-form.tsx`,
  `campaign-row-actions.tsx`, `new-campaign-view.tsx`,
  `campaign-detail-view.tsx` rewritten against them, each with real
  loading/error states.
- **`apps/internal/trafficsource`** (new) — `GET /traffic-sources`,
  tenant-scoped, backed by the `traffic_sources` table that already
  existed but had no read endpoint. The one backend addition this slice
  needed; campaign CRUD and analytics endpoints already existed from
  earlier phases.
- **CORS** (`github.com/go-chi/cors`) on `httpserver.New`, origin locked to
  the new `config.AppURL` (`APP_URL` env var, default
  `http://localhost:3000`) — required now that the browser calls the API
  cross-origin.
- **`apps/web/.env.example`** (+ `.env.local`, not committed) —
  `NEXT_PUBLIC_API_URL`, `NEXT_PUBLIC_DEV_ORG_ID`. Next.js loads env files
  from `apps/web/` itself, a separate surface from the root
  `.env.example` (Go services).
- **`CampaignDetailView`'s Overview tab** — real Revenue/Clicks/
  Conversions/CVR stat cards and a real daily revenue chart, computed from
  the Phase 25/26 analytics endpoints. `Spend`/`Profit`/`ROI`/`CPA` removed
  entirely rather than shown as `"—"`: invariant #6's `"—"` means "no cost
  entered yet," not "no cost pipeline exists yet" (that's Phase 27-COST).
- **`docs/frontend-integration.md`**; `ARCHITECTURE.md` §6-SHARED note
  corrected — the Routing Simulator was not switched to a real endpoint
  this phase (no stream-set/flow backend exists yet) and still runs on its
  mock, unchanged.

### Removed

- Mock-only `Campaign` fields with no backend equivalent:
  `trackingDomain`, `trackingId`, `clicks`, `conversions`, `revenue`,
  `spend`. The campaign list's Clicks/Revenue columns and the "Copy
  tracking URL" row action are gone, not faked with placeholder data.

### Fixed

- `apps/web/.gitignore`'s blanket `.env*` pattern was silently swallowing
  `.env.example` too (no `!.env.example` exception, unlike the root
  `.gitignore`) — added.

### Notes

- **Deliberately still mocked, not silently absorbed**: the standalone
  `/analytics` report builder (a full ad-hoc report UI with no matching
  backend contract at any granularity close to what it renders);
  `/ltv-cohorts` (a bare `<PageStub>` — no UI was ever built against the
  Phase 26.5 endpoints); `StreamSetList`/`RoutingSimulatorView` on the
  campaign detail page; every sources/offers/networks/flows/stream-sets/
  filters/routing-simulate/conversions/postbacks list-management page.
  Each needs its own dedicated backend phase before its existing mock can
  be wired up the same way campaigns were here.
- Verified end-to-end against the real running `api` + `web` dev servers,
  not just automated checks: created a campaign through the UI with the
  Source dropdown populated from real seeded `GET /traffic-sources` data,
  confirmed it lands via `POST /campaigns` and navigates to a real detail
  page whose Overview tab renders a correct zero-value empty state (not an
  error) for a campaign with no conversion events, confirmed the Settings
  tab pre-fills from the real record with its Archive control present, and
  confirmed the campaign list resolves and displays the real source name
  after creation. Backend also verified independently: `go build/vet/
  gofmt/test ./...` all green (incl. 2 new `trafficsource` tests), plus
  `curl` confirming traffic-sources list, campaigns list, and a CORS
  preflight all work against real seeded data. Frontend: `tsc --noEmit`
  and `eslint` both clean. Test campaign and its two seed traffic sources
  removed via the real `DELETE` endpoint after verification.

## [Phase 26.5] — LTV & Cohort Engine

### Added

- **`internal/ltv`**: FTD (first-deposit) and Reg (registration) cohort
  tables with §26.5's LTV windows (`d0`/`d1_7`/`d8_30`/`d31_90`). Pure Go
  over a narrow ClickHouse fetch — the same architecture `internal/routing`
  has over its fixture — deliberately, so §26.5's "numbers reconcile
  against fixtures" acceptance criterion is provable directly against this
  package's own tests, not trusted to a `dateDiff` expression buried in a
  materialized view. 11 fixture tests cover exact window bucketing
  (including the day-90/day-91 boundary), the "incomplete windows show
  partial revenue, never a clean zero" rule, conservative completeness
  across a multi-day cohort's members (never complete until the *youngest*
  member's window has elapsed), both §26.5 rate formulas exactly, and the
  Phase 23 FX-missing-contributes-zero invariant carrying through
  unchanged.
- **`ltv_events`** (schema/007): a materialized view over
  `conversion_events`, narrowed to `CPA_HOLD`/`CPA_ACCEPT`/`CPA_REDEP` —
  the only statuses any §26.5 formula references.
- **Anchor uniqueness needs no `MIN()`/`GROUP BY`**: `ClicksByFTDAnchor`/
  `ClicksByRegAnchor` rely on CLAUDE.md #3's dedup key already guaranteeing
  at most one `CPA_ACCEPT` (or `CPA_HOLD`) row per click_id — a direct,
  previously-undocumented-in-this-context consequence of Phase 23's own
  invariant, not a new rule invented here. One row per click IS the first
  occurrence, always.
- **`FTDCohort`/`RegCohort` are distinct types**, not one generic struct
  with optional fields: `ftd_to_redep_rate`/`dep_to_redep` denominate by
  `cpa_accept`, `reg_to_ftd_rate` by `cpa_hold` — attaching both rate
  families to a single shared type risked a silently-wrong denominator on
  whichever one didn't match that cohort's own anchor count. Both types
  still share deposit/window/lifetime accumulation through an embedded
  `base`, computed identically either way.
- **`LifetimeDaysAvg`/`HasLifetimeData`**: averaged only over clicks that
  actually redeposited — §26.5 defines `lifetime_days` as "days from FTD to
  last redeposit," which is undefined, not zero, for a click that never
  did. A cohort where nothing redeposited reports `HasLifetimeData: false`,
  never a misleading `0`.
- **REST**: `GET /analytics/ltv/ftd-cohorts` and `.../reg-cohorts`,
  `?period=day|week|month&from=&to=&campaignId=&networkId=&country=`,
  mounted on `apps/api` behind the existing `tenant.Middleware`. `from`/`to`
  default to the last 90 days when omitted — §26.5 itself names 90 days as
  the range a report needs for fully closed windows.
- **`docs/ltv-cohorts.md`**, `ARCHITECTURE.md` updated (§76).

### Notes

- **Deliberately out of scope, no new confirmation needed**: §26.5 names
  `source`/`offer` as filter/breakdown dimensions alongside
  campaign/network/country, but `event.Event` carries neither
  `traffic_source_id` nor `offer_id` — the same pre-existing gap
  `internal/macro`'s `{source}` token (Phase 24) and `click_events`' sort
  key (Phase 26) already documented, reached independently a fourth time.
  No frontend work either — no LTV/Cohorts UI mock exists to integrate
  with, consistent with every Go-backend phase since 23.
- End-to-end smoke test against the real compiled `api` binary and real
  seeded `conversion_events` (via `EventStore.InsertBatch`, the same path
  production traffic uses): an old two-click FTD cohort's windows summed to
  exactly the seeded revenue with every window `Complete: true`; a
  five-day-old cohort showed `d0` complete-and-populated with the later
  three windows correctly `Complete: false` and `0` revenue; the matching
  Reg cohort's `regToFtdRate` came out exactly `2/3`; a second organization
  querying the identical range got `{"cohorts":[]}` — tenant isolation held
  through the full stack, not just the repository layer.
- 20 new tests: `internal/ltv` 16 (11 pure-computation fixtures + 5
  service-layer validation/pass-through), `internal/chstore` +4 (the
  `ClicksByFTDAnchor`/`ClicksByRegAnchor` query layer, incl. dimension
  filtering and a no-match case). A from-scratch ClickHouse schema
  application (dropping `ltv_events`/`ltv_events_mv` and re-running
  `Migrate`) verified alongside the existing idempotency check.

## [Phase 26] — ClickHouse

### Added

- **The real §48 five-table schema** — `click_events`, `tracking_events`,
  `conversion_events`, `cost_events`, `postback_events` — replaces Phase
  25's deliberately disposable single `events` table.
  `schema/000_drop_phase25_schema.sql` drops it and its aggregate first;
  every table gets its own sort key reasoning documented in its own
  `schema/*.sql` file rather than one generic comment repeated five times.
  No TTL on any table — confirmed via user question before starting: no
  data retention policy exists anywhere in this project's docs
  (PRODUCT.md, ARCHITECTURE.md), and defaulting one in silently would be a
  real, possibly GDPR-relevant product decision made by omission.
- **`event.Type.IsClick()`** (new, alongside the existing `IsCPA()`):
  decides `click_events` vs `conversion_events` vs `tracking_events`
  routing. `TestEventClassificationIsExhaustiveAndDisjoint` guards that
  every type in the model lands in exactly one bucket — a future event
  type added to `event.go` without updating this classification fails a
  test instead of silently landing nowhere or in two tables at once.
  `chstore.EventStore.InsertBatch` now buckets one mixed batch into up to
  three ClickHouse batch inserts, never one insert per row.
- **`cost_events`**: schema only, confirmed out of scope for this phase.
  Mirrors Postgres `cost_entries` for cross-database-free JOINs against
  click/conversion volume, but the sync pipeline that populates it is
  Phase 27-COST's job ("Cost ingestion," ROADMAP.md) — the same pattern
  `cost_entries` itself followed, starting manual-entry-only in Phase 17
  before any FB/TikTok import existed. `ReplacingMergeTree(updated_at)`,
  not a plain `MergeTree` — the only append-only-by-default table here that
  anticipates needing to overwrite an edited entry once that sync exists.
- **`postback_events`: real ingestion, both directions** — confirmed via
  user question before starting, closing a gap migration 00008 (Phase 17)
  explicitly earmarked for once ClickHouse existed. Every exit point of
  `internal/conversion.Service.Record` (success, duplicate, ignored, error)
  and every dispatch outcome in `internal/postback.Deliverer` (success,
  retrying, dead) now reports through a new `AttemptLogger` interface —
  same decoupled, no-error-return contract as `EventSink`/
  `DeliveryEnqueuer` — to **`internal/postbacklog`**, a near-duplicate of
  `internal/eventqueue` (own Postgres `FOR UPDATE SKIP LOCKED` queue,
  `postback_attempt_queue`/migration 00016, own `Flusher`) rather than a
  shared/generic implementation: the two payload shapes
  (`chstore.PostbackAttempt` vs `event.Event`) have nothing else in common,
  and duplicating ~150 lines of already-proven code was judged lower risk
  than a generics refactor touching Phase 25's shipped `eventqueue`
  mid-project. `postback_events` is explicitly **not** the dedup/delivery
  source of truth — Postgres's `postbacks`/`postback_deliveries` still are
  — it's the read-side replay/audit log §45 requires, fed asynchronously,
  never on either direction's critical path.
- **Three materialized views**: `click_events_daily_campaign` and
  `click_events_daily_geo` (§48's two named patterns), plus
  `conversion_events_daily_campaign` (counts **and USD revenue** per
  campaign/day/status) — not named by §48 directly, but the same pattern
  applied to money instead of volume, added because CLAUDE.md #6 ("cost or
  it doesn't exist") and §27-COST's eventual ROI queries will need a
  revenue aggregate and the marginal cost of a third view was near zero.
  All three `SummingMergeTree`, verified firing *synchronously* on
  `INSERT` (no polling needed in tests). Revenue sums
  `usd_value * has_usd_value` — a conversion with no FX rate on file
  contributes exactly zero, not an approximation.
- **`internal/analytics` gains `GET /analytics/campaigns/{id}/daily-revenue`**,
  reading the new revenue aggregate — the query method existed since this
  phase's schema work but the REST endpoint was initially missed and added
  once the smoke test went looking for it.
- **Attribution's `MemoryResolver` replaced by `chstore.ClickResolver`** —
  not originally scoped for this phase's user-confirmed plan, but closes a
  promise `docs/attribution.md` made across Phases 22/23/25 ("arriving with
  the worker (Phase 24) and the analytical schema (Phase 26)") using
  infrastructure this phase already built. Found while verifying the
  schema end-to-end: `apps/tracker` never actually called
  `MemoryResolver.Record` anywhere — every conversion has been permanently
  `unknown_click` since Phase 23 shipped, silently. `ByClickID` resolves to
  the **earliest** occurrence of a click_id (handles `stickyFlowKeepClickId`
  reusing one click_id across a returning visitor's journey, §39-STICKY)
  and excludes `SOURCE_FILTER` rows (a filtered click never reached a
  destination, so a network could never legitimately reference one) — a
  lookup-eligibility decision, not new attribution policy; the matching
  logic itself is entirely `internal/attribution`'s own, unchanged.
  Best-effort at tracker startup (falls back to `MemoryResolver` — same
  degraded-but-honest stance as Redis elsewhere) since the redirect path
  must never depend on ClickHouse being up.
- **`internal/chstore/schema/*.sql`**, **`docs/analytics-pipeline.md`**
  (rewritten for the real schema), **`docs/attribution.md`** (the "Later"
  promise marked landed), **`ARCHITECTURE.md`** updated (§76).

### Fixed

- Nothing broken, but one real functional gap closed in passing: see
  "Attribution's `MemoryResolver` replaced" above — every conversion was
  silently unattributed in any environment that actually ran the compiled
  tracker, not just in tests.

### Security

- Tenant isolation re-verified for the new `ClickResolver`
  (`TestClickResolverTenantIsolation`): org B resolving org A's click_id
  gets `ErrClickNotFound`, the same outcome as a genuinely nonexistent
  click — indistinguishable from outside, so the failure mode can't be used
  to confirm another tenant's click_id exists (mirrors
  `internal/attribution`'s own stated reasoning for `OutcomeUnknownClick`).

### Notes

- End-to-end smoke test against compiled `tracker`/`worker`/`api` binaries
  and real Postgres/ClickHouse: a real click through `/t/{trackingID}` →
  flushed to `click_events` → a real incoming postback for that exact
  click_id → `attribution_outcome = "attributed"`, `method = "click_id"` in
  the resulting `conversion_events` row — the first smoke test all session
  to NOT read "matched no click of this organization." Also verified: the
  postback's own audit trail in `postback_events` (both `incoming` and the
  `outgoing` delivery it triggered, including a real HTTP hit against a
  local receiver), and a from-scratch ClickHouse schema application
  (`DROP`s issued manually, `Migrate` re-run, all 5 tables + 3 views
  recreated cleanly).
- 69 tests total across the touched/new packages (up from 39 last phase):
  `chconn` 1, `chstore` 12 (incl. 7 new `ClickResolver` tests), `eventqueue`
  5 (unchanged), `postbacklog` 5 (new), `conversion` 24 (+1: every `Record`
  outcome logs an attempt), `postback` 12 (+1: every dispatch outcome logs
  an attempt), `event` 4 (+1: classification exhaustiveness), `analytics` 6
  (+2: the revenue endpoint).
- Full Postgres migration round-trip (`up`/`down`/`up`) verified through
  00016; a from-scratch ClickHouse schema application verified separately
  from the idempotency check `chstore_test.go` already had.

## [Phase 25] — Analytics Pipeline

### Added

- **Scope boundary with Phase 26 confirmed via user question before
  starting**: §47's own pipeline diagram names ClickHouse as one of its
  stages, but the actual table design (five tables, dimension-specific sort
  keys, per-campaign+day/per-GEO+day materialized views) is explicitly §48's
  content, in a phase titled "ClickHouse" on its own. Building both at once
  would blur a boundary the spec draws deliberately. Chosen: a minimal,
  disposable single-table schema now, proving the pipeline end to end;
  Phase 26 replaces it wholesale. Recorded in full in
  `docs/analytics-pipeline.md` so the coming rework reads as planned, not
  as backtracking.
- **`internal/eventqueue`**: the durable link in §43's "Tracker -> Event
  Queue -> Worker -> ClickHouse" pipeline. `event_queue` (migration 00015)
  is Postgres, the same `FOR UPDATE SKIP LOCKED` job-queue pattern
  `postback_deliveries` (Phase 24) already established — STACK has no
  message broker. One difference: a row here is **deleted**, not kept
  terminal, once its batch lands in ClickHouse — this queue is disposable
  transit, not an audit ledger. `payload` is the whole `event.Event`
  JSON-encoded (it already carries `json` tags for exactly this) rather than
  a wide explicit-column table that would just be Phase 26's real schema
  designed a second time.
- **`apps/tracker` drops `eventbuf.LogSink`** for `eventqueue.Sink` — the
  one-line swap Phase 21's design always promised, since everything
  upstream only ever saw the `eventbuf.Sink` interface.
- **`apps/worker` gains a second poll loop**, `internal/eventqueue.Flusher`:
  claims up to 500 due events, ONE ClickHouse batch insert per batch,
  deletes on success or requeues the whole batch on failure (fixed 10s
  delay). No dead-letter state, unlike outgoing postbacks — a delayed
  analytics batch has no per-item deadline the way a lost conversion does,
  so it retries indefinitely rather than giving up. A deliberately different
  tradeoff from `internal/postback`'s `MaxAttempts`, not an inconsistency.
- **`internal/chconn`**: the ClickHouse connection, always over HTTP —
  `infra/docker-compose.dev.yml` exposes only ClickHouse's HTTP interface
  (port 8123) to the host, unlike some reference implementations that
  default to the native protocol (port 9000, container-internal here to
  avoid colliding with MinIO).
- **`internal/chstore`**: the minimal schema (`schema/*.sql`, embedded,
  applied idempotently via `CREATE ... IF NOT EXISTS` — no migration
  framework, no version table, a deliberate simplification matched to one
  disposable table) plus `EventStore.InsertBatch` and the one aggregate
  query. `events`' `type` column covers the full ~20-value event model even
  in this disposable table (CLAUDE.md #2 isn't phase-scoped), and
  `organization_id` leads the sort key (CLAUDE.md #5), same as it will in
  Phase 26's real schema.
- **`events_daily_campaign`**: one `SummingMergeTree` aggregate fed by a
  `MATERIALIZED VIEW`, proving the "materialized/aggregate tables" pipeline
  stage with a single rollup rather than §48's full per-campaign+day/
  per-GEO+day set. Verified firing *synchronously* on `INSERT INTO events`
  — no polling needed in the test that checks it — though reading it still
  requires `SUM(event_count)`, not a plain read: `SummingMergeTree` merges
  same-key rows only in the background.
- **`internal/analytics`**: one query, one endpoint —
  `GET /analytics/campaigns/{campaignId}/daily?from=&to=` on `apps/api`,
  mounted behind the existing `tenant.Middleware`. Deliberately narrow:
  proves the pipeline's last two stages (analytics service -> REST API)
  without delivering the eventual rich analytics surface (per-GEO/source/
  offer breakdowns, the metrics registry) that later phases own. Date range
  capped at 366 days as a guard against an unbounded ClickHouse scan, not a
  spec requirement.
- **`httpserver.readyHandler` now checks ClickHouse too** (`ch Pinger`,
  nil-able) — the doc comment left in Phase 18 said this would happen
  "whenever a later phase actually wires it in, not before." `apps/api`'s
  ClickHouse connection is best-effort at startup: `/campaigns` doesn't need
  it, only `/analytics` does, so a down ClickHouse degrades one route group
  (and shows up on `/ready`) rather than the whole control-plane API.
- **`apps/worker/README.md`**, **`apps/tracker/README.md`**,
  **`docs/analytics-pipeline.md`** updated/added (§76).

### Fixed

- **`ARCHITECTURE.md`'s ClickHouse bullet** read as if the five-table design
  already existed; corrected to note the Phase 25 minimal table and point at
  `docs/analytics-pipeline.md`.

### Security

- Tenant isolation verified end-to-end through the real `/analytics` REST
  endpoint (not just at the repository layer): org B querying org A's
  campaign id via `X-Organization-Id` gets `{"counts":[]}`, not an error and
  not another tenant's data — `organization_id` scopes the ClickHouse query
  the same way it scopes every Postgres one.

### Notes

- 13 new tests across four new packages: `chconn` (1), `chstore` (3),
  `eventqueue` (5, including pure-logic `Flusher` tests against fakes —
  success deletes, failure requeues the whole batch, an empty claim never
  calls ClickHouse), `analytics` (4). Every Postgres/ClickHouse-gated test
  ran against real local instances, not just the fakes.
- Full end-to-end smoke test against compiled `tracker`/`worker`/`api`
  binaries and real Postgres/ClickHouse: events enqueued, drained by the
  worker within its poll interval, visible in ClickHouse's `events` table
  and the `events_daily_campaign` aggregate, and returned correctly by the
  real (not mocked) `/analytics` HTTP response — plus the cross-tenant check
  above.
- `apps/web` untouched. Wiring the frontend to real backend APIs is Phase
  27's explicit job ("Frontend/backend integration," ROADMAP.md), consistent
  with this project's frontend-first-on-mocks build order — `apps/web`
  keeps reading `apps/web/src/lib/mock/*` until then.

## [Phase 24] — Postback Engine (outgoing)

### Added

- **`apps/worker`** is a real binary for the first time — a health endpoint
  plus `internal/postback.Deliverer.PollLoop`. Its *other* eventual role,
  consuming the tracker's event queue into ClickHouse, is explicitly not
  this phase's job (§47/§48, Phases 25-26); `apps/worker/README.md` now says
  so instead of describing the full future scope as if it already existed.
- **`internal/postback`** (§46): the outgoing delivery engine.
  `postback_deliveries` (migration 00014) is the durable queue — §46's exact
  status vocabulary (`queued`/`processing`/`success`/`failed`/`retrying`/
  `dead`), claimed via the standard Postgres `FOR UPDATE SKIP LOCKED` job-
  queue pattern so concurrent worker replicas can never double-claim a row.
  Deliberately a **separate table** from `postbacks`, not more
  `direction='outgoing'` rows there as an earlier comment on that table once
  planned — `result`'s one-shot dedup vocabulary and a multi-attempt
  delivery's own state don't fit one column without forcing the dedup index
  to grow a predicate it has no other need for. `source_postback_id` keeps
  the two connected. Full reasoning: `docs/postback-delivery.md`.
- **Exponential backoff, 8 attempts, ~21 hour span, dead-letter after** —
  none of these numbers are in §46; chosen so a network's multi-hour outage
  still gets delivered without retrying forever. A dead-lettered delivery
  isn't lost — Phase 13's "Resend postback" UI action already exists as the
  recovery path.
- **`internal/conversion.Service` gets a `DeliveryEnqueuer` hook**, the same
  decoupled-narrow-interface pattern `EventSink` already established: every
  `ResultSuccess` with a configured `networks.postback_url` queues a
  delivery. `Enqueue` has no error return by contract — a conversion
  `internal/conversion` already durably recorded must never be reported back
  to the network as failed just because queuing its outgoing notification
  hit a database blip.
- **Trigger scope decided via user confirmation**: all five CPA statuses
  (not just HOLD/ACCEPT/REDEP) queue a delivery. §46 doesn't specify this;
  the Phase 13 frontend mock hints at "payable only" but that reads as
  mock-data flavor, and the URL template's `{status}` token exists
  specifically so a network can branch on it — restricting by status would
  have been inventing policy the spec never states.
- **`apps/internal/macro`**: the Go half of §27's shared macro resolver,
  porting `apps/web/src/lib/macros.ts`'s exact token contract rather than
  inventing a second one. Resolves `{click_id}`/`{status}`/`{revenue}`/
  `{currency}`/`{campaign_id}`/`{country}`/`{device}`/`{sub1..10}` — every
  field `internal/conversion.Service.Record` actually has in hand.
  `{payout}`/`{offer_id}`/`{source}` are part of the shared vocabulary but
  pass through literally: no Flow→Offer or click→TrafficSource lookup exists
  anywhere in the Go codebase yet, and wiring one in was out of scope for a
  queue/worker/retry phase — documented in `docs/postback-delivery.md`
  rather than silently narrowing the macro contract.
- **`apps/worker/README.md`**, **`docs/postback-delivery.md`** (§76).

### Fixed

- **Three docs still stated the pre-A1/A2 two-part dedup key**
  (`docs/event-model.md`, `docs/domain-model.md`,
  `apps/api/migrations/README.md`'s table list was also missing
  `event_mappings`/`postback_deliveries`) — found while touching this area
  for Phase 24. `ARCHITECTURE.md` and `CLAUDE.md` were corrected in Phase 23;
  these three were missed then. Corrected now, including
  `event-model.md`'s literal repetition of the exact "else a monotonic
  sequence" wording the A1 amendment identified as actively dangerous
  (disables deduplication entirely when a network sends no txn id).

### Notes

- End-to-end smoke test against the compiled `tracker` and `worker`
  binaries, a real Postgres/Redis, and a real local HTTP receiver: verified
  both the immediate-success path and the fail→`retrying`→backoff→succeed
  path with real wall-clock timing (a 500 response, `next_attempt_at` ~30s
  out, not due until then, due and delivered on the next poll).
- 39 new/updated tests: `internal/macro` (5), `internal/postback` (11, incl.
  Postgres-gated `ClaimDue`/backoff-timing/tenant-data-shape checks), plus 3
  new delivery-enqueue tests added to `internal/conversion`'s existing suite
  (now 23 total there).
- Migration 00014 round-tripped (`goose up` → `down` → `up`) against a real
  Postgres alongside 00001-00013.

## [Phase 23] — Conversion Engine

### Added

- **`internal/conversion`** (§45): turns an inbound postback into a recorded,
  deduplicated, correctly-attributed CPA event, or an honestly-logged reason
  it wasn't. Pure orchestration — no HTTP, no database driver — over `Store`,
  `Mapper`, `NetworkLookup`, `FXConverter`, and `internal/attribution`'s
  `AttributionService`, the same shape `internal/attribution` itself has over
  `ClickResolver`.
- **Dedup key is `(click_id, status, event_ref)`** (A1, §45), computed by
  `eventRefFor`: the network's transaction id for `CPA_REDEP` only, empty
  string for every other status even when one was sent. A network sending no
  txn id on a redeposit records exactly one — a missed redeposit (support
  ticket) is preferred over a double-counted one (bad invoice).
- **Status progression** (A2, §45): the only refused transition is back to
  `CPA_HOLD`. Independent of dedup — it exists specifically because
  `acceptDuplicates` networks bypass the dedup constraint but must not bypass
  this rule (nightly partner replays re-sending an already-approved `HOLD`
  would otherwise pull revenue out of a closed report).
- **`GET/POST /postback/{networkId}`** on `apps/tracker` (§45's endpoint,
  ARCHITECTURE.md explains why tracker and not worker). `{networkId}` — not a
  header, not the body — is where `OrganizationID` comes from
  (`PostgresNetworkLookup`), matching how attribution derives tenant scope.
- **Event Mapping is now real**: `event_mappings` table (migration 00012) plus
  `PostgresMapper`, replacing the Phase 13 frontend mock's documented "what
  the real Conversion Engine runs at ingest time." Matched case-insensitively
  — networks are inconsistent about casing across retries.
- **FX normalization at event time** (§50-FX, CLAUDE.md #7): `PostgresFX`
  reads `fx_rates` by `(currency, event date)`, never the current rate. No
  rate on file is `ok=false`, not an error — the conversion is stored with its
  original currency/amount and a nil USD value rather than inventing a rate or
  dropping the conversion.
- **Redis wired for real** (`github.com/redis/go-redis/v9`,
  `internal/rediscache`): `RedisStore` caches only the progression check
  (`LastStatus`), never the dedup insert itself. The cache is written only
  *after* Postgres confirms a success, so staleness can only cause an extra
  Postgres read, never a false "already seen" — see docs/conversion.md for why
  a Redis-first dedup pre-check was deliberately rejected. Best-effort at
  tracker startup: `PostgresStore` alone is already correct, so a
  down/unconfigured Redis logs a warning and falls back rather than failing
  the process.
- **`event.Event` grows its §45 fields** (`NetworkID`, `Revenue`, `Currency`,
  `USDValue`/`HasUSDValue`, `EventRef`, `NetworkTxnID`,
  `AttributionOutcome`/`AttributionMethod`, `TimeToConversionMS`) — extended,
  not redefined, per the package doc's own stated pattern.
- **`docs/conversion.md`** (§76); resolves docs/attribution.md's open
  question (no attribution window, decided this phase — see Notes).

### Fixed

- **`postbacks` migration 00008 predated the A1/A2 amendment**: its unique
  index was `(organization_id, click_id, status)`, which would have dropped
  every redeposit after the first. Migration 00013 replaces it with
  `(organization_id, click_id, status, event_ref) WHERE NOT
  network_accepts_duplicates AND result = 'success'` — the added `result =
  'success'` clause is what lets a duplicate/ignored/error attempt log its own
  row without colliding with an already-accepted one, making "log every
  postback... with replay ability" and "have we already processed this"
  answerable from one table.
- **`postbacks_status_check` rejected `status = ''`**: found via a full
  end-to-end smoke test against a running tracker binary — a postback missing
  `click_id`/`status` entirely, or with no Event Mapping configured for its
  raw status, has no canonical `CpaStatus` to record, and the original CHECK
  (inherited unmodified from 00008) required one on every row including
  `result = 'error'` ones. Migration 00013 exempts `status = ''` rows scoped
  to `result = 'error'` only — every other result still requires a real
  `CpaStatus`.
- **`ARCHITECTURE.md`'s non-negotiable #3** still stated the pre-amendment
  `(click_id, status)` key; corrected alongside the code per CLAUDE.md's own
  rule that a doc and its code must not read differently.

### Security

- `OrganizationID` never comes from the postback body — see "Added" above.
  `PostgresStore`'s dedup/progression queries filter on it directly (mirrors
  `internal/attribution`'s repository-layer enforcement), and
  `TestPostgresStoreTenantIsolation` confirms org B's `LastStatus` lookup
  cannot see org A's click, matching CLAUDE.md #5's DoD requirement for
  data-model phases.

### Notes

- **No attribution window**, decided in this phase rather than left as Phase
  22's open question: §45's own "never lose the conversion" stance (its
  Redis-unavailable fallback would rather accept a wrong report than drop
  revenue) argues against a policy that silently discards a late conversion.
  `Attribution.TimeToConversion` remains the observable an operator can alert
  on instead.
- **Known spec gap, not a bug**: a non-REDEP status recurring for the same
  click (e.g. `CPA_ACCEPT` reinstated after a `CPA_DECLINE` reversal) computes
  the same dedup key as its first occurrence and is recorded as a duplicate —
  §45 designates only `CPA_REDEP` as repeatable and explicitly forbids
  inventing a synthetic `event_ref` to work around this. Documented in
  docs/conversion.md and exercised (not "fixed") by
  `TestProgressionAllowsEverythingElse`. Left for a future spec amendment if a
  real network's reinstatement flow needs it.
- 23 tests: 13 pure-logic (fakes for `Store`/`Mapper`/`FXConverter`/
  `AttributionService`, mirroring `internal/attribution`'s test style) plus 10
  gated on `DATABASE_URL`/`REDIS_URL` proving the actual schema/cache
  implement the contract the fakes assert. Full migration round-trip (`goose
  up` → `down` → `up`) verified against a real Postgres, plus an end-to-end
  smoke test of the compiled tracker binary exercising every `ResultKind`
  through the real HTTP endpoint (this is what caught both Fixed items above).
- Click storage behind attribution is still `MemoryResolver` — unchanged by
  this phase, still arriving with the worker (Phase 24) and ClickHouse (Phase
  26).

## [Phase 22] — Attribution

### Added

- **`internal/attribution`** (§44): decides which click a conversion belongs
  to, implementing §44's `AttributionService` interface exactly. Pure — no
  HTTP, no database driver, no clock of its own — reading clicks through a
  `ClickResolver`, the same shape `internal/routing` has with `routingstore`.
- **Evidence is tried strongest-first**: `click_id` (which FLOX minted and
  handed to the network) before `external_click_id` (which the network
  supplied and which is not reliably unique). Two refusals carry the weight of
  §44's "do not invent attribution when there is insufficient evidence":
  - a `external_click_id` matching **several** clicks is `ambiguous`, not a
    tiebreak. The same `fbclid` recurs across a redirect chain, a prefetch and
    a genuine second visit; preferring "the most recent" would look sensible
    and credit the wrong click a fraction of the time, invisibly.
  - a present-but-unmatched `click_id` does **not** fall back to the external
    id. If the network echoed back an identifier we minted and it resolves to
    nothing, that claim is suspect, and re-matching it on a weaker field would
    hide exactly the case worth investigating.
- **Unattributed is a first-class answer, not an error.** Four closed outcomes
  (`attributed`, `no_identifier`, `unknown_click`,
  `ambiguous_external_click_id`), each carrying a human-readable reason for the
  postback log — a disputed payout gets re-argued from that record (§72's
  spirit). `no_identifier` is deliberately distinct from `unknown_click`: the
  first is a misconfigured postback template, the second is a real lookup miss,
  and the fixes differ.
- **`TimeToConversion`**, including negative values. A conversion timestamped
  before its click means clock skew or a replayed postback; clamping it to zero
  would erase the one signal that says so. It is a diagnostic, never grounds
  for refusing a click that matched.
- **`MemoryResolver`** — the honest stand-in, per-process and gone on restart,
  labelled as such. The tracker still writes to `eventbuf.LogSink`, which is
  explicitly not durable storage, so there is nothing to query yet; the real
  ClickHouse-backed resolver arrives with the worker (Phase 24) and the
  analytical schema (Phase 26). Adding a clicks table to Postgres instead would
  have contradicted §7 and §47.
- **`docs/attribution.md`** (§76).

### Security

- **Tenant isolation is enforced in the repository layer** (CLAUDE.md #5):
  every `ClickResolver` method takes `organizationID` and filters on it, rather
  than the service comparing ids after the fact — a filter that is part of the
  query cannot be forgotten at one call site. `Conversion.OrganizationID` comes
  from the authenticated credential, never the request body, and a missing one
  returns `ErrNoOrganization` rather than searching globally.
- A click belonging to a **different** organization is reported as
  `unknown_click`, indistinguishable from a nonexistent one, so the outcome
  cannot be used to confirm another tenant's click id exists.
- Four isolation tests, including one asserting that the same
  `external_click_id` present in two organizations resolves cleanly for each
  rather than turning ambiguous — the failure mode a dropped org filter would
  produce, and one that would otherwise read as a data-quality problem rather
  than a breach.

### Notes

- A resolver failure surfaces as an error, never as `unattributed`: recording a
  database blip as "no click found" would permanently write off real revenue.
- **No attribution window** is implemented. §44 specifies none, and a policy
  that silently discards late revenue is not this phase's decision to make
  alone; §45 already notes partners re-send deposits with hours-to-days delay.
  Recorded as an open question for Phase 23, where the postback timing rules
  live.
- Nothing is wired into a binary yet — Phase 23's postback handler is the
  consumer, exactly as `internal/routing` (Phase 19) waited for the tracker
  (Phase 21).

## [Between phases] — spec amendments (§38, §45, §58, §59)

Three amendments drafted in `docs/spec-amendments-phase22.md` after reviewing
a third-party tracker, then applied. All three are now spec **and** code: A3
landed with routing (Phase 22); A1 and A2 landed with the conversion engine
([Phase 23], above).

### Changed — weighted flow selection is deterministic (A3, §38)

- **`internal/routing.pickWeighted` now hashes a visit key instead of rolling
  a die.** §38 previously said routing must be deterministic "where
  configuration requires deterministic behavior", which reads as "when sticky
  is on" and permitted the injected RNG the engine actually used. The
  consequences were real: a replayed request landed in a different flow than
  the original before a sticky cookie existed, two replicas behind one load
  balancer disagreed about the same visit, and a restart re-bucketed everyone.
  The draw is now an unseeded FNV-1a/64 of `RequestContext.VisitKey` —
  uniform, so shares still converge to the configured weights, and pure, so a
  replay resolves identically. Explicitly **not** `hash/maphash`, whose
  per-process random seed would reintroduce exactly the cross-replica
  disagreement being fixed. Sticky is untouched: the cookie is still the
  source of truth and short-circuits before the draw.
- **`routing.Engine` lost its `Rand01` field and now has no fields at all** —
  no state, no entropy. A fresh `Engine{}` per call is indistinguishable from
  another replica or a restart, which is what makes the new determinism test
  expressible.
- **`RequestContext.VisitKey` (new, required for a real split).** The caller
  derives it, because "the same visit" is an HTTP-layer question the routing
  package is deliberately blind to: `apps/tracker` fingerprints
  `campaignID|clientIP|userAgent`, and the simulator mock derives it from its
  form. The campaign id is in the tracker's key so one visitor is bucketed
  independently per campaign rather than landing in the same relative
  position in every split on the platform.
- **A missing key is refused, not guessed** (`ErrNoVisitKey`). Hashing the
  empty string would route 100% of traffic to one arm of a split while every
  dashboard still reported the configured percentages — silent, and only
  discoverable after the experiment is over. A single eligible flow is not a
  draw and still needs no key, so the most common configuration is unaffected.
- **Eligibility is now decided before the draw, and zero/negative weights are
  skipped rather than clamped.** The previous implementation could select a
  zero-weight flow through its "float rounding fallback" branch, which meant a
  paused arm could still take traffic.
- **`lib/routing-simulate.ts` mirrors all of the above**, including FNV-1a in
  `BigInt` (hashing UTF-8 bytes, matching Go's byte-indexed strings — UTF-16
  code units would have diverged the two on exactly the non-ASCII user agents
  that are hardest to debug). Verified: Go and TS agree on `""`, `"a"`,
  `"hello"`, an ASCII key and a non-ASCII key, with the values checked against
  an independent third implementation rather than captured from either side.

### Changed — postback correctness (A1/A2, §45, §59 — spec only)

- **Dedup key is `(click_id, status, event_ref)`.** The old normative line said
  `(click_id, status)` while the rationale beneath it already described a txn
  id; the two-part form is what would have been coded, dropping every redeposit
  after the first. `event_ref` is scoped to `CPA_REDEP` and empty for every
  other status even when a network sends a txn id, since networks retry with a
  fresh one per attempt. Also drops "else a monotonic sequence", which would
  have disabled deduplication outright.
- **Status never moves back to `CPA_HOLD`.** No ordering rule existed anywhere
  in the spec, so a nightly partner replay re-sending the original hold after
  approval would have been recorded — it is not a duplicate under any dedup key
  — and would have taken revenue out of an already-published report. Only that
  one transition is refused; chargebacks and their reversals stay allowed.
- **§59** gained the cases that make both testable, and the `NEVER` lists in
  §80 and `CLAUDE.md` were widened: they banned only dedup on `click_id` alone,
  which after A1 left the newly-wrong two-part key permitted.

### Fixed

- `docs/routing.md` pointed at `apps/api/internal/routing`, a path that stopped
  existing when the module root moved in Phase 21.

## [Phase 21] — Tracking Engine

### Changed — module topology (the Phase 16 open question, resolved)

- **The Go module root moved from `apps/api` to `apps/`** (module
  `github.com/ismagilovnail/flox/apps`). Phase 16 flagged this as a
  decision for whoever started Phase 21: Go's internal-import rule means
  `.../internal/x` is only importable by code rooted at that directory's
  parent, so `apps/tracker` could never have imported
  `apps/api/internal/routing`. Both `ARCHITECTURE.md` ("separate binaries
  inside the same Go module") and §41 ("shares internal packages") state
  the answer outright, so there was nothing left to weigh — taken rather
  than re-litigated. `git mv` throughout, so history is preserved.
  - `apps/api/cmd/api/main.go` → `apps/api/main.go`; run with `go run ./api`.
  - `apps/api/internal/…` → `apps/internal/…`.
  - Migrations stay with the control plane: `go tool goose -dir api/migrations`.
  - The directory layout CLAUDE.md specifies is unchanged; only `go.mod` moved.
- **`apps/web/go.mod`** (new, 4 lines): the module root now sits above
  `apps/web`, so `go build ./...` was compiling and vetting a stray `.go`
  file shipped inside `web/node_modules` by an npm package — letting a JS
  dependency break the Go build. A nested module stub makes the toolchain
  skip that subtree. Found empirically right after the move, not
  theorised.

### Added

- **`apps/tracker`** (§41): the hot-path click/redirect binary.
  `GET /t/{trackingID}` runs parse → classify → route → record async →
  302 redirect, with no analytics query and no wait on persistence
  anywhere on the path. Measured on the dev stack over 200 requests:
  **p50 1.1ms, p95 1.4ms** against §56's p50 < 20ms / p95 < 50ms budget.
- **`internal/event`** (§43): the full ~20-type event model, present from
  day one and never truncated (CLAUDE.md non-negotiable #2), with the five
  CPA statuses as distinct enum members. A test asserts the model against
  §43's list so a later edit can't quietly drop a type, and asserts a
  generic `CONVERSION` type does *not* exist.
- **`internal/eventbuf`** (§41): buffered batch writer whose one hard
  guarantee is that `Enqueue` never blocks — a single non-blocking channel
  send, so a stalled sink drops events (counted, and logged, never
  silently) rather than making a user wait on a redirect. Proven by a test
  that wires a permanently-stalled sink to a 4-deep buffer and asserts
  1000 `Enqueue` calls still return promptly. Batches flush on size or
  interval, and `Close` drains rather than discarding accepted events.
  §43's durable queue + ClickHouse consumer arrive with `apps/worker`
  (Phase 24); until then the sink is an honest structured-log writer
  behind the same `Sink` interface.
- **`internal/routingstore`**: loads a campaign's routing configuration
  out of Postgres into `internal/routing`'s pure types — a separate
  package specifically so the routing engine keeps its no-database
  property. Rebuilds the recursive AND/OR filter tree from the flat
  `filter_groups`/`filter_conditions` rows in two queries rather than one
  per node, and resolves whether an offer destination's offer is still
  active (the check §58's "inactive offers" case needs).
- **`ConfigVersion`** is derived from the newest `updated_at` across the
  campaign and its routing objects. §39 asks for versioned configuration
  and no version column exists; this is a real monotonic version that
  changes exactly when the configuration changes, which is what
  cache-invalidation and "which config produced this decision" actually
  need.
- **Migration `00011`**: §39-STICKY's three flags (`sticky_flow`,
  `sticky_flow_keep_click_id`, `sticky_flow_skip_inactive`) on
  `campaigns`. Not in §35's table list, so Phase 17 correctly didn't
  invent them; the tracker is the first code that reads them, so they land
  now — schema following the code that needs it.
- **`config.LoadTracker`**: same shape and env vars as the API's loader,
  differing only in which URL variable supplies the port and the default
  OTel service name, so the two share one loader instead of growing
  near-identical copies that drift.

### Notable implementation decisions

- **Tracking links resolve by host + slug, never slug alone.**
  `tracking_links` is unique on `(domain_id, slug)`, so two organizations
  may each own the slug `summer` on their own domain — resolving by slug
  alone would be a cross-tenant data leak.
- **Two of Phase 19's three "caller-level" §58 cases are now implemented
  here**, where the data actually lives: an unresolvable tracking link and
  a non-`active` campaign both 404 without ever reaching the routing
  engine. The third (in-app WebView bounce) remains outstanding and
  belongs with the PWA install funnel.
- **`stickyFlowKeepClickId` still doesn't reach `internal/routing`.**
  Phase 19 documented that it affects attribution only, never flow
  selection. The tracker needs it, so `routingstore.CampaignRouting`
  returns it *alongside* the routing config rather than smuggling it in —
  and the tracker parses the full `setId:flowId[:clickId]` cookie itself,
  handing the engine only the two fields a routing decision depends on.
- **§42's unfilled-FB-subs rule is implemented by having no special case
  at all**: whatever subs arrive are persisted, whatever don't stay empty
  strings — no placeholder, no "unknown campaign", no inference.
  `event.Subs.SubCount()` makes subs-less traffic measurable, which is the
  point.
- **302, not 301.** A permanently-cached redirect would bypass the tracker
  on the next click, losing both the event and any routing change.
- **No per-request logging middleware on the tracker's router** — the path
  is latency-budgeted and every click already produces a structured event
  asynchronously; a second synchronous log line per click would be
  duplicate work on the hot path.

### Verified

- Live end-to-end against real Postgres with a seeded
  campaign → tracking link → 3 stream sets (bot block / nested
  `AND(device, OR(country US, CA))` / catch-all) → flows graph:
  - desktop → catch-all set → 302 to its flow destination;
  - unknown slug → 404;
  - Googlebot → classified `bot=1`, matched the block set, and routed to
    the campaign's **safe fallback** (§73's fallback model) rather than
    the real destination;
  - mobile with unknown geo → nested `OR(country…)` correctly failed, so
    the `AND` failed and it fell through to the catch-all — proving the
    filter tree really was rebuilt from Postgres and evaluated;
  - replayed sticky cookie → `sticky_applied=true` and the **original
    `click_id` reused**, a real attribution chain across a return visit.
- 205 events emitted, zero dropped, zero errors or warnings in the log.
- `go build`/`vet`/`gofmt`/`test -race` clean across the whole module.

### Fixed

- N/A this phase — no defects found in the implementation. Two mistakes
  were caught and corrected in the *test scaffolding* while validating:
  ULID literals in a seed script used the Crockford-excluded letters
  `O`/`L` (the `ulid` domain's CHECK constraint caught them, which is
  exactly what it is for), and a seed comment mispredicted that a blocked
  bot would end with no destination — it correctly received the campaign
  fallback instead.

### Known issues

- None new. Phase 10's unresolved crash-loop report carries over
  (unrelated). The in-app WebView bounce (§73) is still unimplemented, now
  tracked in `apps/tracker/README.md` rather than as an open architectural
  question.

### Files changed

- `apps/go.mod`, `apps/go.sum`, `apps/internal/**`, `apps/api/main.go` (moved)
- `apps/tracker/{main,handler,params,sticky}.go` + tests (new)
- `apps/internal/{event,eventbuf,routingstore}/**` (new)
- `apps/api/migrations/00011_campaign_sticky_flags.sql` (new)
- `apps/web/go.mod` (new — node_modules exclusion)
- `ARCHITECTURE.md`, `docs/architecture.md`, `apps/api/README.md`,
  `apps/api/migrations/README.md`, `apps/tracker/README.md` (modified)

## [Phase 20] — Traffic Classifier

### Added

- **`internal/classifier`** (§40): turns raw request signal (IP,
  User-Agent, `Accept-Language`) into `routing.Attributes`, keyed by the
  exact `routing.FilterField` constants stream-set filters already
  evaluate against — imported from Phase 19's package, not redeclared, so
  the two can never drift apart on a field's name. Feeds directly into
  `Router.Resolve` with no adapter layer, verified end-to-end in
  `integration_test.go`.
- **`GeoProvider`/`ASNProvider`/`BotDetector`** interfaces (§74/§75,
  non-negotiable #11 — no vendor lock-in), matching the spec's placeholder
  signatures with real method shapes. Defaults are honest about not being
  wired to a vendor: `NoopGeoProvider`/`NoopASNProvider` return empty
  results rather than fabricated geo/ASN data; `HeuristicBotDetector`
  flags well-known crawlers by a generic User-Agent substring list
  (Googlebot, curl, python-requests, …) — explicitly the provider-neutral
  technique §73 allows, not the ad-network moderator/reviewer detection it
  forbids — and always reports `IsProxy: false` rather than guess without
  a real IP-reputation vendor.
- **Local User-Agent parsing** (`useragent.go`, stdlib `regexp` only — RE2,
  non-negotiable #8): device/platform/os/os_version/browser/browser_version
  bucketed into the same small fixed vocabulary the frontend's
  `FIELD_VOCAB` already defines. No external UA-parsing dependency — the
  target vocabulary is already small and fixed, so hand-written
  substring/regex matching (with browser-detection order handled
  carefully: Edge and Samsung Internet checked before Chrome, Chrome
  before Safari, since all three embed each other's tokens) covers it
  completely.
- **`os_version`/`browser_version` populated even though §40's field list
  doesn't name them** — they're already real, filterable
  `routing.FilterField`s exposed since Phase 8's filter builder; leaving
  them dead would be a gap, not fidelity to the spec's (representative,
  not exhaustive) list.
- **`connection_type` always `"unknown"`** — no reliable
  wifi/cellular/ethernet signal exists server-side without a paid
  network-intelligence vendor or a client-side JS beacon, neither of which
  exist yet. Honest default, not a guess; matches the frontend's own
  vocabulary option of the same name.
- Device classification's one intentional non-"leave it empty" default:
  absent any mobile/tablet marker, `Device` defaults to `"desktop"` — "not
  mobile, not tablet" is itself a meaningful, safe-to-assume signal
  (the convention every real UA parser uses), unlike platform/os/browser
  where no such safe default exists.

### Fixed

- **A test expectation, not the implementation**: while writing
  `useragent_test.go`, an "unrecognized UA" case initially expected
  `Device: ""`, which failed against the (correct, intentional)
  `desktop`-default behavior described above. Fixed the test's
  expectation and tightened `UAResult`'s doc comment to state the
  device-defaulting rule explicitly, rather than changing working code to
  match a wrong assumption.

### Known issues

- None new. Phase 10's unresolved crash-loop report carries over
  (unrelated to this phase). `apps/tracker`/`apps/worker` module topology
  remains an open decision (documented Phase 16, still unresolved, not
  blocking) — `internal/classifier`'s eventual caller, alongside
  `internal/routing`.

### Files changed

- `apps/api/internal/classifier/*` (new)
- `docs/architecture.md` (modified — Phase 20 section)

## [Phase 19] — Routing Engine

### Added

- **`internal/routing`** (§38, §6-SHARED Strategy A): the single source of
  truth for routing decisions — stream-set priority, AND/OR nested filter
  evaluation, weighted flow selection, sticky assignment. Deliberately
  independent of `net/http` and any database driver (§38: "keep routing
  independent from HTTP handlers") — `Resolve` is a pure function of
  already-loaded config + already-classified request attributes →
  decision.
- **`Router.Resolve`** matches §38's exact spec'd interface and
  `RouteResult` shape byte for byte. **`Engine.Explain`** (a second method
  on the concrete engine, not part of the `Router` interface) runs the
  identical evaluation and additionally returns the full per-stream-set/
  per-flow `Explanation` §72 requires ("why did this match, why not that
  one, why this flow, why fallback, sticky from where") — the shape the
  frontend Routing Simulator already renders — without growing
  `RouteResult` beyond what §38 specifies. `Resolve`/`Explain` share one
  internal evaluation function, so they can't disagree.
- **Filter evaluation, weighted pick, and destination resolution ported
  faithfully from the existing frontend mock** (`lib/filters.ts`,
  `lib/routing-simulate.ts`) — same 31 `FilterField`s, same 16
  `FilterOperator`s (including the exact `norm()`/`compareValues()`
  semantics), same weighted-pick and fallback-cascade logic. Strategy A
  means the frontend's TypeScript version is a mock to be *replaced*
  (Phase 27), not a second implementation to keep in sync — porting the
  behavior faithfully now is what makes that swap invisible to users.
- **One deliberate correctness improvement over the frontend mock**: the
  Go engine checks `Destination.OfferActive` before using an offer
  destination, falling through to fallback if the offer is inactive — §58
  explicitly requires an "inactive offers" test case, and the current
  frontend mock never implemented this check at all.
- **`stickyFlowKeepClickId` doesn't reach this package at all** — it only
  affects whether the caller reuses an old `click_id` for attribution,
  which has zero effect on which flow gets selected. `RoutingConfig`
  doesn't carry it; documented in both the code and `docs/routing.md` so
  its absence reads as a decision, not an oversight.
- **Sticky is verifiably Redis-independent by construction**: this package
  has no cache dependency at all, so a sticky decision is a pure function
  of `(req.Sticky, req.Config)` on every call — there's nothing an "eviction"
  could invalidate on this side. Test proves identical results across
  repeated calls with the same sticky state.
- **Conformance fixture** (§6-SHARED, §58): `internal/routing/fixture_test.go`
  covers all 17 required cases — AND/OR/nested groups, priority
  (first-match wins), fallback (stream-set level before campaign level),
  weighted distribution (±2% over 10k picks, real seeded PRNG via an
  injectable `Engine.Rand01`, not a fixed value), sticky (+ skipInactive
  both branches), inactive flows/offers, missing-destination fallback
  cascade, and ISO-code-mismatch (proving no fuzzy UK/GB coercion — a
  literal, case-insensitive-only match). Three cases (`inactive campaigns`,
  `invalid tracking links`, `in-app WebView bounce`) are explicitly
  documented as out of this package's scope via `t.Skip` with reasoning
  inline, not silently absent — all three happen in the caller
  (`apps/tracker`, Phase 21) before `Resolve` is ever invoked.

### Fixed

- N/A this phase — no defects found. `go build`/`vet`/`gofmt`/`test`
  clean, full suite stable across 5 repeated runs and under `go test -race`.

### Known issues

- None new. Phase 10's unresolved crash-loop report carries over
  (unrelated to this phase). `apps/tracker`/`apps/worker` module topology
  remains an open decision (documented Phase 16, still unresolved, not
  blocking) — `internal/routing`'s eventual consumers.

### Files changed

- `apps/api/internal/routing/*` (new)
- `docs/routing.md` (rewritten — real conformance fixture table)
- `docs/architecture.md` (modified — Phase 19 section)

## [Phase 18] — Campaign API

### Added

- **Campaign API** (§37): `internal/campaign/` — `handler.go` → `service.go`
  → `repository.go` → `model.go`, per CLAUDE.md's Go architecture (thin
  handlers, business logic in the service, repository is just SQL).
  `GET/POST /campaigns`, `GET/PATCH/DELETE /campaigns/:id`, `POST
  /campaigns/:id/{duplicate,pause,activate}`. Every query org-scoped
  (§36-TENANCY).
- **`internal/tenant`**: `X-Organization-Id`-header middleware — the
  session-derived org-scoping stand-in until Phase 28 auth lands (fully
  documented as a deliberate, temporary substitute in `apps/api/README.md`,
  not a shortcut: handlers physically cannot read org scope from anywhere
  else, which is the property §36-TENANCY actually protects).
- **Cross-tenant FK check**: a campaign's `traffic_source_id` foreign key
  only proves that row exists *somewhere* — nothing stops it belonging to a
  different org unless checked explicitly. `Repository.TrafficSourceBelongsToOrg`
  verifies it against the caller's org before every create/update; this is
  the pattern every future domain package with a cross-table reference
  needs to repeat.
- **`internal/apierror`**: one JSON error envelope (`{code, message,
  fields?}`) shared by every handler, so `/campaigns` and every future
  `/offers`, `/networks`, etc. render errors the same shape.
- **`internal/idgen`**: ULID generation (`oklog/ulid/v2`) + the same
  format validation `apps/api/migrations`'s `ulid` domain enforces, so an
  application-generated id and a database CHECK constraint can never
  disagree about what's valid.
- **`internal/postgres`**: pgx pool constructor, pinged once at startup so
  a misconfigured `DATABASE_URL` fails fast instead of surfacing on the
  first request.
- **`GET /ready` now genuinely pings Postgres** — the check Phase 16
  promised ("starts checking real dependencies once Phase 17+ wires them
  in") and deliberately hadn't faked yet. Returns `503` with the failing
  check named if the ping fails.
- **Pause/activate domain rules** (§37: "validate using domain rules," not
  bare setters): idempotent from the target state, rejected with `409` from
  `archived` — an archived campaign has to be explicitly edited back to
  another status via `PATCH`, not casually reactivated by a toggle.
  `Duplicate` mirrors the frontend's `stores/campaigns.ts`: `"{name}
  (Copy)"`, forced back to `draft`, stats/tracking reset.
- **Cross-tenant isolation test** (CLAUDE.md DoD requirement for API
  phases): `internal/campaign/repository_test.go`, gated on `DATABASE_URL`
  (skips cleanly without it — `go test ./...` never needs a live DB).
  Creates a campaign for org A, proves org B's list/get/update/delete all
  see nothing of it, and that org A can't attach a campaign to org B's
  traffic source.

### Fixed

- **Test-cleanup bug in the isolation test itself**, caught before this
  phase closed: `defer pool.Close()` in the test body runs at function
  return, which is *before* `t.Cleanup`-registered callbacks run — so the
  pool was already closed by the time each seeded test org's delete-cleanup
  query fired, silently leaking `organizations`/`campaigns` rows into the
  dev database on every test run (confirmed leaked rows via `psql`, fixed
  by registering the pool's close via `t.Cleanup` instead, ordered by LIFO
  to run after the org-delete cleanups, re-verified via row counts before
  and after a run).
- **`internal/tenant`'s missing-header error** originally used a hand-rolled
  JSON string that didn't match `apierror`'s envelope shape — caught via
  manual curl testing, fixed to construct a real `apierror.Error`.

### Known issues

- None new. Phase 10's unresolved crash-loop report carries over
  (unrelated to this phase). `apps/tracker`/`apps/worker` module topology
  remains an open decision (documented Phase 16, still unresolved, not
  blocking).

### Files changed

- `apps/api/internal/campaign/*` (new)
- `apps/api/internal/{tenant,apierror,idgen,postgres}/*` (new)
- `apps/api/internal/httpserver/{server.go,health.go}` (modified — DB ping, route mounting)
- `apps/api/cmd/api/main.go` (modified — wires DB pool + campaign routes)
- `apps/api/README.md`, `docs/architecture.md` (modified — Phase 18 sections)

## [Phase 17] — Database

### Added

- **10 goose migrations** in `apps/api/migrations/` implementing §35's full
  core table list: `organizations`, `users`, `memberships`, `roles`,
  `permissions`, `traffic_sources`, `campaigns`, `stream_sets`,
  `filter_groups`, `filter_conditions`, `flows`, `landings`, `pwas`,
  `postlandings`, `networks`, `offers`, `offer_links`, `domains`,
  `tracking_links`, `pixels`, `postbacks`, `cost_entries`, `fx_rates`,
  `api_keys`, `audit_logs` — plus `stream_set_pixels`, a join table for
  `StreamSet.pixels: string[]` that the explicit table list didn't spell
  out but a many-to-many relationship needs. `go tool goose` (vendored as a
  Go tool dependency, no separate install).
- **ULID everywhere**: a `ulid` Postgres domain (text + Crockford-base32
  format CHECK) defined once in `00001`, used as every table's primary key
  — "one standard, consistently, everywhere" per §35, not a mix.
- **`organization_id` denormalized onto every tenant-scoped table,
  including child tables** (`offer_links`, `filter_conditions`, `flows`,
  `stream_set_pixels`, …) rather than left implicit via a join to the
  parent — §36-TENANCY calls cross-tenant isolation "a hard security
  invariant, not a convention," and a repository query that filters a
  child table directly by `organization_id` can't leak data even if a
  future join condition is written wrong.
- **`updated_at` maintained by a trigger** (`set_updated_at()`), not
  application code, on every mutable table — can't go stale from a
  forgotten `SET updated_at = now()`.
- **Filter tree**: `filter_groups` (self-referencing via `parent_group_id`,
  `stream_set_id` denormalized onto every node) + `filter_conditions`,
  matching the frontend's recursive `FilterGroupNode`/`FilterCondition`
  shape (`lib/filters.ts`) exactly enough that Phase 27 integration should
  be a straight read/write mapping, not a redesign.
- **`flows`**: the frontend's `LandingStage`/`PwaStage`/`PostlandingStage`/
  `Destination` structs flattened into nullable columns gated by their own
  `*_enabled`/`destination_kind` flag, with a CHECK constraint enforcing
  the destination discriminated-union shape (`offer` requires network+offer
  IDs and no URL; `redirect` requires a URL and no network/offer IDs).
- **`postbacks`**: the durable dedup ledger for §45/non-negotiable
  invariant #3 — `UNIQUE (organization_id, click_id, status)`, with a
  partial-index exemption (`WHERE NOT network_accepts_duplicates`) for
  networks with the `acceptDuplicates` override. Deliberately does *not*
  duplicate the rich per-attempt log (message, payload) — that belongs in
  ClickHouse's `postback_events`, outside this Postgres-only phase.
- **`cost_entries`**: one row per (campaign, traffic_source-or-none, day),
  enforced with two partial unique indexes rather than a
  `COALESCE`-sentinel expression index (avoids needing a fake placeholder
  ULID to satisfy the `ulid` domain's format CHECK).
- **`fx_rates`**: the one non-`organization_id`-scoped, non-ULID table —
  natural composite key `(currency, rate_date)`, since an exchange rate is
  an objective market fact, not something each tenant has its own copy of.
- **`campaigns` has no `tracking_domain`/`tracking_id` columns** even
  though the frontend mock has them — that's a `(campaign, domain, slug)`
  row in `tracking_links` instead, so one campaign with links on multiple
  domains never desyncs a domain string stored in two places.
- No tables for Tags/Custom Metrics/Report Presets/Referral/Content
  Gallery — those v3 "secondary" frontend phases (14.5–14.9) aren't in
  §35's core table list; not guessed at here.

### Validated

- Brought up a real Postgres via `infra/docker-compose.dev.yml`, ran
  `goose up` (all 10 migrations, clean), `goose down-to 0` (full rollback,
  clean — zero orphaned objects, confirming every `-- +goose Down` block is
  correct), then `goose up` again.
- Ran a full smoke-test transaction: inserted a complete two-org dataset —
  campaign → stream_set → nested AND/OR filter tree → two flows (one
  `offer` destination, one `redirect`) → domain/tracking_link → pixel →
  cost_entry → api_key → audit_log → postback — for Org A, then verified
  Org B's queries return zero rows against every one of those tables
  (cross-tenant isolation, DoD requirement for data-model phases). Also
  verified, as expected failures: the postback dedup constraint rejects a
  true duplicate but the `accept_duplicates` override allows one; the
  `flows` destination CHECK rejects a malformed `offer` row with a URL set;
  the `cost_entries` dedup constraint rejects a same-day double-entry. A
  recursive CTE read-back of the filter tree round-trips correctly.
- `go build`/`vet`/`gofmt` clean.

### Fixed

- N/A this phase — all defects (invalid Crockford-charset ULIDs in seed
  data during authoring, a fragile `COALESCE`-sentinel unique index, a
  psql `\gset`-inside-`DO`-block scripting mistake in the smoke test) were
  caught and fixed before anything ran against real Postgres, not found
  as bugs afterward.

### Known issues

- None new. Phase 10's unresolved crash-loop report carries over
  (unrelated to this phase). `apps/tracker`/`apps/worker` module topology
  remains an open decision (documented Phase 16, still unresolved, not
  blocking).

### Files changed

- `apps/api/migrations/00001..00010_*.sql` (new)
- `apps/api/migrations/README.md` (rewritten — command reference + conventions)
- `apps/api/go.mod`/`go.sum` (modified — goose tool dependency)
- `apps/api/README.md` (modified — migration commands)
- `docs/architecture.md`, `docs/domain-model.md` (modified — Phase 17 section)

## [Phase 16] — Go Backend Foundation

### Added

- **`apps/api`** (§33): new Go module (`github.com/ismagilovnail/flox/apps/api`,
  Go 1.26) — the first backend code in this repo. `cmd/api/main.go` wires
  config → logging → OpenTelemetry → HTTP server → graceful shutdown on
  SIGINT/SIGTERM.
- **`internal/config`**: loads every env var already declared in
  `.env.example` (§7/§17/§33). Only `NODE_ENV`/`API_URL`/`LOG_LEVEL`/OTel
  vars are actually consumed this phase; `DATABASE_URL`/`CLICKHOUSE_*`/
  `REDIS_URL`/`S3_*` are parsed onto `Config` now so later phases don't need
  to touch this file again, but nothing connects to them yet.
- **`internal/logging`**: `slog.Logger`, JSON handler, level from config.
- **`internal/telemetry`**: OpenTelemetry `TracerProvider` with an OTLP/HTTP
  exporter. No-op (not an error) when `OTEL_EXPORTER_OTLP_ENDPOINT` is
  unset — OTel is observability, not a hard dependency for the API to run.
  Verified the server starts and serves cleanly both with the endpoint unset
  and with it pointed at a collector that isn't actually running.
- **`internal/httpserver`**: chi router — request ID (+ echoed back as an
  `X-Request-Id` response header for client-side correlation), real IP,
  structured per-request logging (tagged with the request ID), panic
  recovery, 30s timeout, OTel HTTP instrumentation.
- **`GET /health`** — liveness only, always `200`. **`GET /ready`** —
  readiness; returns `200` unconditionally for now. It does not fake a
  dependency check: Postgres/ClickHouse/Redis don't exist until Phase 17+,
  and a check that doesn't check anything is exactly the "fake API that
  looks real" CLAUDE.md forbids. Starts checking real dependencies once
  those phases wire them in.
- **`infra/docker-compose.dev.yml`**: Postgres, ClickHouse, Redis, MinIO
  (S3-compatible) for local dev, credentials matching `.env.example`
  exactly so `cp .env.example .env` + `docker compose up` need no edits.
  Brought the full stack up and verified all four healthchecks pass;
  caught and fixed one real bug along the way — the ClickHouse healthcheck
  used `wget http://localhost:8123/ping`, which resolves `localhost` to
  `::1` inside the container and fails since the image's HTTP server isn't
  listening on IPv6; changed to `127.0.0.1`.
- `.env.example`: filled in `S3_ACCESS_KEY_ID`/`S3_SECRET_ACCESS_KEY` with
  the MinIO root credentials the compose file actually uses (throwaway
  local-dev values, not real secrets) so the two files stay consistent.

### Scope decision — module topology for `apps/tracker`/`apps/worker` is deferred, not solved

`ARCHITECTURE.md` says `apps/tracker`/`apps/worker` share `internal/routing`/
`internal/classifier` with `apps/api` as "the same Go module." §33 literally
places this module's `go.mod` at `apps/api`, and Go's internal-import
visibility rule means sibling directories (`apps/tracker`, `apps/worker`)
structurally cannot import `apps/api/internal/...` regardless of module
setup. Not a Phase 16 problem — routing/classifier logic doesn't exist until
Phase 19/20 — documented as an open decision in `apps/api/README.md` and
`docs/architecture.md` for whoever starts Phase 21, with the two concrete
options already spelled out (move the module root to `apps/`, or keep
routing/classifier under `pkg/` instead of `internal/`).

### Fixed

- ClickHouse dev-compose healthcheck (`localhost` → `127.0.0.1`), described
  above — the only defect found during this phase's validation.

### Known issues

- None new. Phase 10's unresolved crash-loop report carries over
  (unrelated to this phase).

### Files changed

- `apps/api/{go.mod,go.sum,cmd,internal,migrations/README.md,pkg/README.md,README.md}` (new)
- `infra/docker-compose.dev.yml` (new)
- `.env.example` (modified — S3 credentials filled in)
- `docs/architecture.md` (modified — Phase 16 section)

## [Phase 15] — Frontend Architecture

### Added

- `apps/web/src/hooks/` — the first genuinely cross-feature hook,
  `useCurrentMember()`, replacing an identical `CURRENT_USER_MEMBER_ID` +
  `useTeamStore((s) => s.members.find(...))` + Owner/Admin check that had
  been copy-pasted into Referral (14.8), Content Gallery (14.9), and Custom
  Metrics (14.6) independently. Single source of truth for "who am I, what
  can I manage" until real auth/sessions land in Phase 28.
- `apps/web/src/lib/api/routing.ts` — the frontend API boundary (§32)
  starts here, not as 20 speculative per-domain clients. `docs/architecture.md`
  already promised the Routing Simulator "runs against a local mock that
  implements the exact same request/response contract... in Phase 27 it is
  switched to the real endpoint with no UI changes" — but the Phase 10 call
  site was a plain synchronous function, which would have broken that
  promise (swapping sync for `fetch` always forces UI changes). It's now a
  promise-returning wrapper around the same pure mock (`lib/routing-simulate.ts`),
  with a real loading state (`SimulatorForm`'s button now disables and reads
  "Simulating..." mid-flight) — so Phase 27 only ever touches this one
  file's body.

### Scope decision — no `src/lib/api/<domain>.ts` per store, no empty `types/`/`schemas/` folders

Audited the full `apps/web/src` tree against CLAUDE.md's recommended
`app/ components/ features/ hooks/ lib/ stores/ types/ schemas/` layout.
Findings:

- **No `fetch()` exists anywhere in the codebase** (grepped) — so "don't
  scatter fetch() in components" is vacuously satisfied today. Every
  component already reads/writes data exclusively through a Zustand store's
  typed action surface, never directly — that store action surface **is**
  each domain's mock API contract, per CLAUDE.md's own REPO LAYOUT section
  listing `stores/` as a sanctioned top-level layer. Wrapping every existing
  store (campaigns, offers, networks, landings, pwa, postlanding,
  conversions, postbacks, pixels, domains, team, settings, tags,
  custom-metrics, referral, content-gallery, …) in an additional
  `lib/api/<domain>.ts` pass-through now would be a same-shaped duplicate
  with no consumer and nothing to swap it against — real per-domain API
  integration is explicitly Phase 27's job (CLAUDE.md BUILD ORDER: "Design &
  UI on mock contracts first (phases 2–15)... then integration (27)"), and
  building 20 client stubs 12 phases early is exactly the "never build
  ahead" / "no unnecessary frameworks" this file warns against. Routing
  (above) is the one exception because a concrete promise about it already
  existed in `docs/architecture.md` from Phase 0.
- **Zod schemas and mock types stay co-located per-feature**
  (`features/*/x-form-sheet.tsx`, `lib/mock/*.ts`) rather than moved into
  new top-level `types/`/`schemas/` folders. This already satisfies "domain-
  specific code belongs inside features" — a Landing form's schema is
  exactly as domain-specific as the Landing form itself. Top-level
  `types/`/`schemas/` directories would only be justified by types genuinely
  shared *across* features (e.g. a future OpenAPI-generated contract), which
  don't exist yet; creating them empty now would be pure scaffolding.
- Audited every `useXStore((s) => s.method(...))` selector call site for the
  Phase 13 stale-snapshot crash pattern (a selector that itself does
  `.filter()`/`.map()` breaks `useSyncExternalStore`). All are either
  `.find()` (safe) or already explicitly guarded with a stable-reference
  cache (`stream-sets.ts`'s `listByCampaign`, commented from when that was
  fixed pre-dating this conversation). No latent bugs found.

### Fixed

- N/A this phase — no defects found; the two mock-list-view regressions
  after the `useCurrentMember()` refactor (Referral, Content Gallery, Custom
  Metrics) and the routing simulator's new async path were all verified
  clean in the browser, no console errors.

### Known issues

- None new. Phase 10's unresolved crash-loop report carries over
  (unrelated to this phase).

### Files changed

- `apps/web/src/hooks/use-current-member.ts` (new)
- `apps/web/src/lib/api/routing.ts` (new)
- `apps/web/src/features/{referral,content-gallery,custom-metrics}/*` (modified) — use `useCurrentMember()`
- `apps/web/src/features/routing-simulator/*` (modified) — async simulate call + loading state
- `apps/web/src/lib/routing-simulate.ts` (unchanged — now called from `lib/api/routing.ts` instead of directly)

## [Phase 14.9] — Content Gallery

### Added

- **Content Gallery** (§30.8) at `/content-gallery`: a browsable, searchable,
  categorized library of system-provided templates and creative assets, plus
  team uploads. Four categories: landing/PWA/postlanding templates, and
  creative assets. Category and source (All/System/Team) filters, plus a
  free-text search over title/description/tags.
- **Real hand-off, not a simulated one**: each template's `GalleryItem`
  carries a payload shaped exactly like the target builder's form
  `defaultValues` (from the Phase 12 Landing/PWA/Postlanding form sheets).
  "Use this" navigates to `/landings|pwa|postlanding?gallery=<id>` — reusing
  the same "URL hands off a fully-formed query" pattern Phase 14.7's View
  Statistics established — and the target list page opens its existing
  create sheet pre-filled from the template, sheet title annotated "— from
  {template}". No parallel template-authoring UI was built; the existing
  builders do the work.
- **No real asset library exists yet** (Phase 12 never built one), so a
  creative asset's "Use this" copies its hosted URL to the clipboard instead
  of a builder hand-off — an honest minimal action rather than inventing a
  second, unbuilt system.
- **Team uploads** (§36-TENANCY: private to the workspace) are scoped to
  creative assets only — there's no template-authoring flow to upload a
  landing/PWA/postlanding template into, so upload is restricted to the one
  category that makes sense today. No real object storage in this
  frontend-first phase: an upload records an already-hosted URL, the same
  "URL stands in for upload" convention already used for PWA icons — real S3
  integration is Phase 27's job, not this one's. The uploader can remove
  their own upload; Owner/Admin can remove any team upload. System items are
  read-only.
- Preview tiles are generated CSS gradients + a category icon, not fake
  hosted images — the gallery has no real image pipeline, and CLAUDE.md
  forbids mock APIs that look real.
- Seeded 10 system items (3 landing, 2 PWA, 2 postlanding templates, 3
  creative assets) and 2 team items, across all four categories.

### Fixed

- N/A this phase — full flow verified in the browser: search, category/source
  filters, preview, "Use this" into all three builders (landing tested
  end-to-end through actual creation), asset URL copy, team upload, and
  team-item removal. No console errors.

### Known issues

- None new. Phase 10's unresolved crash-loop report carries over
  (unrelated to this phase).

### Files changed

- `apps/web/src/lib/mock/content-gallery.ts` (new) — data model + seed data
- `apps/web/src/stores/content-gallery.ts` (new) — Zustand store
- `apps/web/src/features/content-gallery/*` (new) — view, card, preview tile,
  preview dialog, upload dialog
- `apps/web/src/app/(app)/content-gallery/page.tsx` (modified) — wired to `ContentGalleryView`
- `apps/web/src/features/{landings,pwa,postlanding}/*-list.tsx` (modified) —
  read `?gallery=<id>` to pre-fill the create sheet from a template
- `apps/web/src/app/(app)/{landings,pwa,postlanding}/page.tsx` (modified) —
  wrapped in `<Suspense>` for `useSearchParams()`

## [Phase 14.8] — Referral Program

### Added

- **Referral dashboard** (§27.6/§30.7) at `/referral`: one referral
  account per team (tenant-scoped, matching every other §36-TENANCY
  surface), not per-user — FLOX pays a commission to the workspace for
  referring other advertisers to the platform, so it's scoped like the
  workspace itself. `referralCode`/`referralLink` derived from the org
  name via the existing `slugify()` helper; copy-to-clipboard with a
  toast confirmation.
- **Referred Signups** table: name, email, status badge
  (invited/signed_up/converted), relative signup date.
- **Earnings History**: an immutable, append-only transaction ledger
  (§54-style audit trail) — `accrual` | `adjustment` | `payout_paid`
  entries, each attributed to a team member and timestamped. Balances
  are never stored or edited directly; `computeBalances()` derives
  Total earned / Total paid / Pending payout / Available balance from
  the transaction + payout logs via a pure function, so the numbers can
  never drift out of sync with the log that produced them.
- **Payouts** state machine: `pending → approved → paid`, plus a
  `rejected` branch (a reasonable low-cost extension of the spec's
  linear 3-state description — every approval flow needs a "no").
  Owner/Admin (reusing Team's canonical roles, not a new per-feature
  role vocabulary) can approve, reject (with a required reason), or
  mark paid; marking paid appends a matching negative `payout_paid`
  ledger entry. Terminal states (`paid`/`rejected`) render no actions.
- **Request payout** dialog, pre-filled to the full available balance,
  validated `0 < amount <= availableBalance`. **Add adjustment** dialog
  (Owner/Admin only): signed amount + required reason, for manual
  corrections.
- All amounts are flat USD — deliberately *not* run through §50-FX
  original-currency/event-date normalization, since referral payouts
  are FLOX's own commission structure, not tenant traffic revenue.

### Fixed

- N/A this phase — full request → approve → mark-paid flow, balance
  math, and ledger immutability all verified correct in the browser on
  the first pass; no console errors.

### Known issues

- None new. Phase 10's unresolved crash-loop report carries over
  (unrelated to this phase).

### Files changed

- `apps/web/src/lib/mock/referral.ts` (new) — data model + `computeBalances()`
- `apps/web/src/stores/referral.ts` (new) — Zustand ledger store
- `apps/web/src/features/referral/*` (new) — dashboard, tables, dialogs, row actions
- `apps/web/src/app/(app)/referral/page.tsx` (modified) — wired to `ReferralDashboard`

## [Phase 14.7] — Report Presets + Directory Stats

### Added

- **Report Presets** (§27.5), built on top of the Phase 5 Analytics
  Explorer rather than a parallel report system: save the current
  {dimensions ("columns"), metrics, groupBy ("grouping"), period,
  timezone} as a named preset, apply it back, edit (rename + resave
  current config), delete. One system default preset (starred, can't be
  renamed or deleted) is seeded and visible to everyone.
- `period` is stored **relative** ("last 7/30/90 days") wherever the
  current date range matches a known relative window, not as a frozen
  from/to pair — reapplying a preset next month still means "the last 30
  days," not a stale historical window. Falls back to a fixed custom
  range only when the current selection doesn't match a relative window.
  Because the mock "now" is a fixed constant already used throughout
  `analytics-view.tsx`, reapplying a preset is fully deterministic —
  directly satisfying "applying a preset reproduces an identical report."
- **View Statistics directory drill-in** (§27.5): a row action on
  Networks, Offers, and Traffic Sources (the three of the spec's four
  named surfaces — Campaigns, CPA Networks, Offers, Flows, Traffic
  Sources — that have both a real list view and a stable entity name;
  Flows skipped for the same reason Phase 14.5 skipped them: no Flows
  list view exists to put a row action on) that hands off a fully-formed
  report query via URL search params (`/analytics?dim=&val=&tab=line`) —
  a navigation + pre-filter, per spec, not a client-side recompute.
  `AnalyticsView` reads `dim`/`val`/`tab` on mount (validated against a
  small allowlist) to seed its initial filter and default to the Line
  tab, which already aggregates by day — satisfying "grouping by day"
  with the chart that already exists rather than inventing a new
  dimension. `analytics/page.tsx` wraps the view in `<Suspense>`, which
  `useSearchParams()` requires.
- **Fixed a pre-existing, silent mock-data disconnect while wiring this
  up**: `mock/analytics.ts`'s `network`/`offer` dimension pools (Phase 5)
  were self-contained fake names ("MaxBounty", "Sweeps Gold US", …) that
  never matched the real Network/Offer entities Phase 11 introduced
  later — so a "View Statistics" link built against real entity names
  would have landed on a correctly filtered but permanently *empty*
  report. Realigned both pools to the real seeded names (same array
  length, pure string swap, no change to the generator logic); `source`
  already coincidentally matched Traffic Source names, left as-is; `flow`
  left untouched since Flows aren't wired to this feature.

### Fixed

- N/A this phase (see "Added" above for the mock-data alignment, which
  is a data-content fix rather than a code-logic fix).

### Known issues

- Full browser smoke test passed (extension connected): applied both
  seeded presets and confirmed dimensions/metrics/groupBy/date-range all
  updated correctly; saved a new preset from the current view and
  confirmed it appeared with edit/delete controls (the default preset
  correctly has neither); "View Statistics" from both a Network and an
  Offer row landed on `/analytics` with the right filter chip, the Line
  tab active, and non-empty real trend data. No console errors.
- Report Presets don't capture `filters` or `sort`/`compare` — only the
  five fields §27.5 explicitly lists ({columns, metrics, grouping,
  period, timezone}). This is a deliberate scope match to the spec's
  literal field list, not an oversight, but worth knowing if a preset
  seems to "forget" an ad hoc filter you had applied when you saved it.

## [Phase 14.6] — Custom Metrics builder

### Added

- **`src/lib/formula-engine.ts`** — a real tokenizer/parser/evaluator/
  validator (§30.5), not an `eval()`/`Function()` shortcut. Operators
  `+ − × ÷ ( )` plus comparisons; functions `DIV, EMPTYIF, IF, ROUND, ABS,
  MIN, MAX` with contextual hints; `{metric_id}` tokens. Division is safe
  everywhere — bare `/` and `DIV()` alike — division by zero or a null
  input yields empty, never throws, exactly as mandated. `validateFormula`
  enforces the single-data-source constraint and rejects LTV metrics
  outright, both explicitly required by §30.5.
- **Critical finding, not duplicated**: `features/analytics/registry.ts`
  (Phase 5) already *is* the §50 Metrics Registry — its own comment says
  so ("matches §50 — never recompute ad hoc elsewhere"). The Custom
  Metrics catalog (`lib/mock/custom-metrics-registry.ts`) re-exports its
  13 `METRICS` verbatim as the live "Traffic / Performance" category and
  only *adds* what Phase 5 didn't need: CPA Funnel, Fraud (`bots`,
  `click_all` — §30.5's own Bot Share example uses these ids), and Push,
  all catalog-only (`live: false`, no tracker/Push module exists yet to
  compute them) — plus LTV, catalog-only **and** `insertable: false`,
  the one category §30.5 forbids in formulas outright, not just
  "unimplemented."
- Formula Builder Sheet: searchable/grouped metric catalog with
  click-to-insert-at-cursor (LTV entries shown disabled with a tooltip,
  not hidden — the constraint stays discoverable), an operator/function
  toolbar, a contextual hint when the cursor is inside a function call,
  live green-check/red-X validation, and a live preview computed against
  representative sample values. Name/group (existing-or-new)/format,
  Show-in targets, Draft/Published.
- "Show in" targets scoped to 4 concrete, real surfaces (Report Builder,
  Campaigns Table, Offers Table, Traffic Sources Table) — each checkbox is
  disabled unless every metric the formula references is actually
  available as data on that surface. Offers/Sources currently have no
  analytics numbers at all (no tracker), so those two are honestly almost
  always disabled rather than faked — a real demonstration of §30.5's
  "never render empty zeros where it cannot be computed" rule, not a cop-out.
- Seeded two metrics illustrating both lifecycle states genuinely: "Margin
  per Click" (§30.5's own example, Published + Active, live on Report
  Builder and Campaigns Table) and "Bot Share" (§30.5's other example,
  kept as **Draft** because `{bots}`/`{click_all}` aren't tracked by
  anything yet — an honest use of "drafts are invisible until published,"
  not a placeholder that pretends to work).
- Lifecycle/governance: Draft/Published, an independent Active toggle
  ("hide from pickers without deleting"), duplicate, and deletion blocked
  while published or exposed on any surface (archive/deactivate instead)
  — enforced in `stores/custom-metrics.ts`.
- Role access reuses Team's existing canonical roles
  (Owner/Admin/Manager/Buyer/Analyst/Viewer, §52) rather than inventing
  §30.5's separate Owner/Tech/Lead/Buyer vocabulary: Owner/Admin manage
  any metric, Manager creates and manages only their own, Buyer/Analyst/
  Viewer see only published+active metrics read-only. Gating logic is
  real (checks the current mock user's role from `useTeamStore`) even
  though the single seeded identity is always Owner — noted below.
- Live computation wired into two real surfaces, proving "computes
  correctly" isn't just a UI toggle: `ReportTable` (Analytics) appends a
  column per report_builder-targeted metric evaluated against each row's
  own `ReportRow.metrics`; the Campaigns `DataTable` does the same against
  each campaign's clicks/conversions/revenue/spend/profit/roi. Same
  formula engine, same evaluator, both places.

### Fixed

- **Crash found during the in-browser smoke test**: typing an
  in-progress/invalid formula (e.g. mid-keystroke) threw an uncaught
  `FormulaError` and crashed the Sheet's render. Root cause:
  `FormulaInput` computed its own validation and reported it to the
  parent through a `useEffect` callback, so the parent's lifted
  `validation` state was always one render behind the live `formula`
  value — on the render where `formula` had just become invalid, the
  parent still evaluated it against its stale "valid" state, and
  `evaluateFormula`/`parseFormula` (unlike `validateFormula`) don't catch
  parse errors. Fixed by making the parent compute validation once,
  synchronously, from `formula` itself (`useMemo`) and pass the result
  down as a prop — one source of truth, no lifted-state race. Documented
  the failure mode in both files as a comment so the pattern isn't
  reintroduced.

### Known issues

- Role-restricted paths (Manager/Buyer/Analyst/Viewer) are implemented
  and exercised by code path but not interactively verified in the
  browser — the only seeded mock identity is the Team Owner, so testing
  the restricted views would require a role-switcher UI this phase didn't
  build. The permission checks themselves read the real (mock) team
  member/role data, not a hardcoded flag.
- Full browser smoke test otherwise passed (extension connected):
  click-to-insert from the catalog, live validation and live preview,
  safe division against zero confirmed via direct DOM/JS inspection
  (screenshots were unreliable this session due to viewport-size
  cutoff — verified state via `get_page_text`/`javascript_tool` instead),
  the single-data-source rejection (`{clicks} + {push_sent}`), and LTV
  metrics rendering as non-interactive (not `<button>`) catalog rows. No
  console errors after the fix above.

## [Phase 14.5] — Tags (cross-entity)

### Added

- Tags (§30.6): a color-label system spanning exactly the seven entities
  the spec names — Campaigns, Networks, Offers, Flows, Traffic Sources,
  PWA, Landings — and nothing else (Postlanding/Pixels/Conversions/
  Domains/Team stay untagged, matching the spec's list precisely).
- Data model mirrors §30.6's generic `tags` table + polymorphic
  `taggables` join exactly: `lib/mock/tags.ts` (Tag, decorative
  `TAG_COLORS` palette deliberately kept separate from the design
  system's semantic success/warning/danger/info tokens) and
  `lib/mock/tag-assignments.ts` (the join side). One `stores/tags.ts`
  owns both, with `setEntityTags` (single-item replace-all) and
  `bulkEditTags(entityType, entityIds, toAdd, toRemove)` implementing
  §30.6's bulk rule precisely: pre-check is the intersection of tags
  across the whole selection; confirming adds newly-checked tags to
  every selected item and removes previously-common tags that got
  unchecked; tags only some items had, left untouched by the user, are
  never touched.
- **ONE tag component, ONE filter, reused across all seven entities per
  the spec's explicit "do not reimplement per entity"** — everything
  lives in `features/tags/`: `use-entity-tags.ts` (safe hook — selects
  raw stable arrays and filters in a `useMemo`, learned from the Phase 13
  crash), `tag-badge-list.tsx` (the shared "Tags column" cell: ≤3 shown,
  >3 "+N", "Add tags" affordance when empty), `tag-picker-popover.tsx`
  (search, checkboxes, inline quick-create when the typed name has no
  match, inline rename/recolor, delete), `tag-filter-control.tsx` (OR
  filter with colored dots), `bulk-tag-dialog.tsx`, `filter-by-tags.ts`.
  Six list pages (Campaigns/Networks/Offers/Traffic Sources/PWA/Landings)
  wire the exact same five pieces; Flows (no list page — nested inside
  Stream Set cards) get `TagBadgeList` inline in `flow-editor.tsx` only,
  since bulk-select and filtering are list-view concepts Flows don't
  have.
- Extended `components/ui/data-table.tsx` with opt-in row selection
  (`enableRowSelection`, `getRowId`, a `bulkActions` render-prop, and a
  `filters` toolbar slot) using TanStack v9's `rowSelectionFeature` —
  strictly additive, so every pre-existing `<DataTable>` call site
  (Postback Logs, Conversions, Pixels, Postlanding, …) keeps working
  with zero changes since the new props all default to off.
- Deliberate scope decision: skipped a second "Manage Tags" entry point
  in each row's `⋮` menu. The spec offers it as an alternative ("via the
  Tags column **or** the row's ⋮ Manage Tags"), and the Tags column
  click already fully covers single-item tag management; a redundant
  second popover-from-a-closed-dropdown entry point would have meant
  six more rounds of anchor-positioning boilerplate for identical
  functionality.

### Fixed

- N/A this phase.

### Known issues

- Full browser smoke test passed (extension connected): tag column +
  picker + quick-create + propagation into the filter list on Campaigns,
  OR-semantics filtering, bulk-select → Edit Tags with the intersection/
  add/remove algorithm verified end-to-end, flow-level tagging inside
  the Flow Builder, and a second entity (PWA) spot-checked for the same
  behavior. No console errors. Re-ran the Phase 13 selector-antipattern
  grep sweep before testing — clean.
- No dedicated tag-management page — tags are created/renamed/recolored/
  deleted entirely inline from any picker they appear in (by design, per
  spec; there's no separate admin UI called for). Deleting a tag from
  one picker removes it (and all its assignments) everywhere at once,
  which is correct per "edit tag name/color propagates everywhere" but
  is a real destructive action with no confirmation step beyond the
  picker's own delete icon — worth a confirm dialog if this becomes a
  frequent misclick in practice.

## [Phase 14] — Domains / Team / Settings

### Added

- Domains (§30): `/domains` — the §30 note that this is "a real module, not
  a text field" is modeled directly: `purpose` (tracking/pwa/fallback),
  `registrar`/`dnsProvider` enums, `expiresAt`/`verifiedAt` tracking, and
  separate mock "Verify ownership" / "Issue SSL" actions rather than raw
  editable status fields (you don't hand-set SSL status, you issue a
  cert). Seeded with the exact 3 domain strings `mock/campaigns.ts` already
  used as `TRACKING_DOMAINS`, plus a PWA domain and a fallback domain.
  Domain removal is a real hard delete (with confirm) — unlike every other
  entity phase so far, there's no "archived" status in the spec for
  domains, and hard delete is the honest action here.
- **Closed the last remaining "known mock placeholder" loop from Phase 6**:
  `campaign-form.tsx`'s Source and Tracking domain selects now read live
  from `useTrafficSourcesStore` (Phase 11) and `useDomainsStore` (filtered
  to `purpose: "tracking"`) instead of the static `SOURCES`/
  `TRACKING_DOMAINS` arrays in `mock/campaigns.ts`. That module keeps
  those two arrays only for its own non-reactive seed generator, same
  pattern as every prior phase's mock-list closure.
- Team (§30, roles from §52): `/team` with Members / Roles & Permissions /
  Activity tabs. Roles (`Owner/Admin/Manager/Buyer/Analyst/Viewer`) and
  permission keys (`campaign.read`, `offer.write`, `settings.write`, etc.)
  match §52 (Phase 28 Auth/RBAC) exactly, so the Roles & Permissions
  reference table is the real vocabulary, not a placeholder — Phase 28
  just adds server-side enforcement on top. The seeded Owner is the actual
  mock signed-in user (`Nail Ismagilov`, `nailismagilovnick@gmail.com`)
  already used in `components/shell/user-menu.tsx` — same person, not a
  stand-in. Owner's role can't be changed and has no remove/suspend
  actions, matching real SaaS conventions for a workspace's sole owner.
- Settings (§30): `/settings` with Organization / API Keys / Integrations
  / Security tabs. API key creation shows the full key exactly once
  (only the prefix persists afterward) via a two-step create → reveal
  dialog flow. Integrations panel doubles as the visible home for the
  Facebook/TikTok/Google Ads connections `TrafficSource.costIntegration`
  (Phase 11) and Domains' registrar/DNS providers (this phase) point at —
  connect/disconnect is mocked, matching the "OAuth wiring lands in Phase
  27-COST" note already established. **Custom Metrics is intentionally
  NOT here** despite §30 listing it under Settings — CLAUDE.md's build
  order makes it its own later phase (14.6); adding it now would be
  building ahead.

### Fixed

- N/A this phase.

### Known issues

- Full browser smoke test passed (extension connected): Domains list +
  Issue SSL action, Team's all 3 tabs including role-change and the
  Owner-has-no-actions guard, Settings' all 4 tabs including the API-key
  reveal-once flow, and the campaign creation form's now-live Source/
  Tracking-domain pickers. No console errors. Re-ran the Phase 13
  selector-antipattern grep sweep before testing — clean, nothing new to
  fix this time.
- Team/Settings/Domains are pure frontend state (Zustand, in-memory) with
  no backend behind "Verify ownership," "Issue SSL," "Connect"
  integration, or API key creation — all are explicitly mock actions
  pending their real phases (27, 27-COST, 28). Documented in-code at each
  call site so this isn't discovered by surprise later.

## [Phase 13] — Conversions / Postbacks / Pixels UI

### Added

- Conversions (§29, §43): `/conversions` list (click ID, campaign, offer,
  CPA status badge, revenue+currency, postback status, event time — all
  cross-referenced live from the Campaigns/Offers stores) linking to
  `/conversions/[id]`, a detail page with the exact §29 timeline (Click →
  Landing → PWA → Offer → Conversion → Postback) as a vertical stepper, plus
  a "Resend postback" action. `status` is a proper `CpaStatus` enum with
  the CLAUDE.md-authoritative token values (`CPA_HOLD`/`CPA_ACCEPT`/
  `CPA_REDEP`/`CPA_DECLINE`/`CPA_TRASH`) — never collapsed into one
  "conversion" type (invariant #2).
- Postbacks (§29, §45): `/postbacks` with four tabs.
  - **Outgoing** reuses `NetworkList`/`NetworkFormSheet` as-is (outgoing
    postback config IS the Network entity's `postbackUrl` — no second CRUD
    for the same data).
  - **Incoming** is new: a per-network reference card showing the URL to
    hand that network so they can report conversions into FLOX
    (`api.floxlink.io/postback/{networkId}?...`), with copy-to-clipboard
    and a mapped-status-count badge.
  - **Event Mapping** is a new, editable per-network table translating a
    network's own raw status string to the canonical `CpaStatus` — what
    the real Conversion Engine (Phase 23) will run at ingest time.
  - **Logs** is a new `DataTable` of every incoming/outgoing postback
    attempt (success/duplicate/error) with a Replay row action, per §45's
    "log every postback... with replay ability."
- Extended the Network entity (Phase 11) with `acceptDuplicates: boolean`
  — the §45 per-network dedup override — as a toggle in the existing
  `NetworkFormSheet`, plus a "Dedup" column on both the Networks page and
  the reused Outgoing Postbacks panel.
- Pixels (§29): `/pixels` — the same list/create-edit-Sheet/row-actions
  shape as Landings/PWA/Postlanding. Client-side ad-platform pixels
  (Facebook/TikTok/Snap/X/generic S2S) fired on a curated event subset.
  Explicitly documented as distinct from a Stream Set's raw `pixels:
  string[]` S2S URLs (§23/§24) — different concepts, not touched here.

### Fixed

- **Infinite-render crash on `/postbacks` → Event Mapping tab**: found via
  the in-browser smoke test (Claude-in-Chrome connected this session).
  `NetworkMappingCard` selected `useEventMappingsStore((s) =>
  s.listByNetwork(networkId))` — `listByNetwork` returns a fresh
  `.filter()`'d array on every call, which breaks `useSyncExternalStore`'s
  snapshot-stability check (new reference each read, even with no `set()`
  call) and threw `Uncaught Error: Maximum update depth exceeded`,
  crashing the tab outright — the same failure class already documented
  and correctly avoided in `stores/stream-sets.ts`'s `listByCampaign`
  (which sidesteps it with a cache), just not learned from here. Fixed by
  selecting the raw `mappings` array and filtering locally in a
  `useMemo`. Also deleted the equivalent unused `listByNetwork` off
  `stores/offers.ts` and `stores/event-mappings.ts` — both were dead code
  and the exact loaded gun for the next person to pick up the same way;
  confirmed via a full-codebase grep that no other `useXStore((s) =>
  s.method(...))` or inline `.filter()`/`.map()` selector exists anywhere,
  including in Phase 9/10's code. This does **not** explain the Phase 10
  crash-loop report — that one has no reproduction and no `.filter()`-as-
  selector pattern anywhere near it — so that stays logged as unresolved,
  not retroactively closed by this fix.

### Known issues

- None new this phase — full browser smoke test passed after the fix
  above (Conversions list + detail, all four Postbacks tabs including
  adding an Event Mapping row and replaying a log entry, Pixels list), no
  console errors.

## [Phase 12] — Landing / PWA / Postlanding UI

### Added

- Landings, PWAs, and Postlandings are now real, team-managed entities
  (§28) with their own pages (`/landings`, `/pwa`, `/postlanding`) — same
  list/create-edit-Sheet/row-actions/Zustand-store shape established in
  Phase 11 for Offers/Networks/Sources.
- Landing editor models the §28 `internal`/`external` split for real: an
  `external` landing takes a URL you already control; an `internal` one
  takes a content textarea and the Sheet derives + live-previews a
  `cdn.floxlink.io/lnd/{slug}` hosted URL from the name via the new
  `slugify()` (`lib/utils.ts`) as you type. Verified in-browser that the
  slug preview updates live and that submitting an internal landing with
  no content is correctly rejected before persisting a broken entity, per
  the `.superRefine` in `landing-form-sheet.tsx`.
- PWA editor fields are the real Web App Manifest (name, short_name,
  theme_color, background_color, icon, start_url) rather than a
  fictionalized subset — the Sheet renders a live, read-only
  `manifest.json` preview generated from those exact fields, plus a
  color-swatch input alongside each hex field. Includes the §73-required,
  provider-neutral `bounceInAppWebview` toggle: bounce in-app WebView
  traffic (FB/IG/TikTok/Telegram) to the external browser so the install
  prompt can fire. This is explicitly NOT vendor-specific moderator
  detection, which §73 forbids — the toggle only ever describes bouncing
  a generic in-app WebView, never detecting a specific ad network's
  reviewer.
- Postlanding editor's `events` field is a multi-select over a curated,
  postlanding-relevant subset of the §43 event model (`PWA_INSTALL`,
  `NOTIFICATION_REQUEST/SUBSCRIBE/DECLINE`, `TG_JOIN`, `TG_START`) —
  string values chosen to match the canonical list in CLAUDE.md exactly,
  so nothing needs renaming when Phase 13 (Conversions/Postbacks/Pixels)
  introduces the full enum.
- **Closed the equivalent of the Phase 9 known issue for the last three
  Flow Builder pickers**: `flow-funnel.tsx` now reads Landings/PWAs/
  Postlandings live from `useLandingsStore`/`usePwasStore`/
  `usePostlandingsStore` instead of the static `flow-entities.ts` mock
  list (which now holds only the flow-level `PwaType`/`PWA_TYPES` display
  mode — internal/external/ios_app, NOT one of the real entities). Also
  fixed the PWA node's preview URL, which was a hardcoded
  `pwa.floxlink.io/install/{id}` string — it now resolves the selected
  PWA's real `startUrl`. `stream-sets.ts` (the module-level stream-set
  mock generator, not a component) keeps seeding from the static
  `LANDINGS`/`PWAS` arrays exported by the new mock files.
- IDs were kept identical to the old `flow-entities.ts` placeholders
  (`lnd_prelander_a`, `pwa_sweeps`, `psl_thankyou`, etc.) so every existing
  seeded stream-set flow keeps resolving without a data migration.

### Fixed

- N/A this phase.

### Known issues

- Verified in a real browser this time (the Claude-in-Chrome extension
  connected, unlike Phase 11's session) — clicked through creating a
  Landing, confirmed its live slug preview and validation, confirmed the
  PWA manifest preview renders correctly, and confirmed a landing created
  in the Landings UI shows up immediately in a campaign's Flow Builder
  picker within the same session (state resets on a hard navigation/
  reload, same known limitation the Campaigns store already has — not a
  regression). No console errors during any of it. This still isn't the
  exact repro path from the Phase 10 crash-loop report, so that one stays
  logged as unresolved rather than closed.
- Internal-landing content is a raw HTML textarea, not a real page
  builder/WYSIWYG editor — matches the spec's field list (§28 lists
  "internal/external" as the Landing distinction, not a builder
  requirement) but is worth flagging if a future phase expects richer
  authoring.

## [Phase 11] — Offers / Networks / Sources UI

### Added

- Traffic Sources, Networks, and Offers are now real, team-managed entities
  (§27) with their own top-level pages (`/traffic-sources`, `/networks`,
  `/offers`) — list (`DataTable`, search/sort/paginate), create/edit (a
  `Sheet` form, following the Stream Set Builder's pattern rather than a
  full detail route since none of these three need per-entity analytics
  tabs), row actions (edit/pause/resume/duplicate/archive with an archive
  confirm dialog), Zustand store per entity (`stores/{networks,offers,
  traffic-sources}.ts`) mirroring the Campaigns store's CRUD shape.
- Modeled the real hierarchy from §27: Network → Offer → Offer Link. An
  Offer carries a `links: OfferLink[]` field array (primary + backups) in
  its form, editable independently with add/remove.
- `src/lib/macros.ts` — the ONE shared macro/placeholder resolver (§27):
  `{click_id}`, `{status}`, `{revenue}`, `{currency}`, `{payout}`, `sub1..10`,
  etc., plus `resolveMacros()`. `src/components/shared/macro-picker.tsx` is
  the reusable insert-a-token popover, wired into the Offer link URL,
  Network postback URL, and Source tracking template fields. Both are built
  to be reused as-is by Postback templates and Pixel payloads in Phase 13 —
  no second token list.
- `src/lib/countries.ts` — static ISO country/currency reference for the
  Offer GEO multi-select (`components/ui/multi-select.tsx`, previously only
  used by the filter builder and analytics).
- **Closed a Phase 9 known issue**: the Flow Builder's Network/Offer
  pickers (`flow-funnel.tsx`, `stream-set-list.tsx`, `stream-set-row.tsx`,
  `stream-set-form-sheet.tsx`) now read live from `useNetworksStore` /
  `useOffersStore` instead of the static `flow-entities.ts` mock list —
  creating/editing/pausing an offer in the new Phase 11 UI is immediately
  reflected in the Flow Builder. `flow-entities.ts` keeps only
  Landing/PWA/Postlanding placeholders (real in Phase 12). The stream-set
  mock generator (`mock/stream-sets.ts`, a module-level pure function, not
  a component) still seeds from the static `OFFERS`/`NETWORKS` arrays
  exported by the new mock files — same non-reactive-seed behavior it had
  before, just sourced from the new location.
- Mock data IDs (`net_afftrust`, `off_sweeps_us`, etc.) were kept identical
  to the old `flow-entities.ts` placeholders so every existing seeded
  stream-set flow keeps resolving to the same offer/network without a data
  migration.

### Fixed

- N/A this phase.

### Known issues

- No visual browser smoke test was possible this phase — the Claude-in-Chrome
  extension reported "not connected" in this environment. Validated instead
  via `tsc --noEmit` (clean), `eslint` (clean), `next build` (clean, all 3
  new routes prerender), and `curl` against a running dev server confirming
  200s and correct SSR content on `/traffic-sources`, `/networks`, `/offers`,
  and a campaign detail page (including that a stream-set flow still
  resolves "US Sweeps — CPA $12" through the new offers store). This does
  not re-exercise the Phase 10 client-side navigation crash-loop report —
  that one needs a real browser tab and DevTools console to chase down;
  still unresolved.
- Offer/Network/Source archive is a status flag, not a hard delete (matches
  the Campaigns pattern) — nothing currently blocks deleting the last
  network, which would leave the Offer form's network picker empty and the
  "New Offer" button disabled; acceptable for mock data, revisit once
  Offers/Networks have real referential integrity in Phase 17+.
- Payout is shown/entered in the offer's own currency with no FX
  conversion to USD anywhere in this UI (§50-FX is backend/event-time work,
  out of scope for an entity-management screen with no events yet).

## [Phase 10] — Routing Simulator

### Added

- Routing Simulator (§26), a third tab (Overview / **Simulator** / Settings)
  on the campaign detail page — routing is campaign-scoped, so it lives
  there rather than as a new top-level sidebar route not in §17's list.
- `src/lib/routing-simulate.ts` — a pure `simulateRoute(streamSets,
  campaignFallbackUrl, request) → result` function implementing the exact
  request/response contract the future `POST /routing/simulate` Go
  endpoint (Phase 19) will expose, per the §6-SHARED / invariant #1
  architecture note: not a second permanent routing engine, but the only
  place this logic *can* live before Phase 19 exists, designed so Phase 27
  swaps this call for a `fetch` with zero UI changes. Evaluates the real
  nested filter tree (all 16 operators from §22, reusing Phase 8's exact
  types), walks stream sets in priority order (first active match wins),
  weighted-picks a flow among active ones, and resolves flow → stream-set
  fallback → campaign fallback → none.
- Input form covers every §26 field, reusing Phase 8's `FIELD_GROUPS`/
  `FIELD_VOCAB`/`BOOLEAN_FLAG_FIELDS` from `lib/filters.ts` — the simulator
  and the filter builder share one field vocabulary, not two.
- Result view: a pipeline stepper (Request → Classification → Campaign →
  Stream Set → Filters → Flow → Destination per §26), a pass/fail trace
  per stream set — including *why* non-matching ones didn't match, not
  just the winner — flow candidates with normalized probabilities and the
  selected one marked, the resolved destination with copy, and a sticky
  note that's honest about sticky config not existing in the data model
  yet rather than faking cookie-based behavior (§80 — no fake APIs that
  look real).
- Core evaluator logic (AND/OR/nested-group semantics, `IN`/`IS` matching)
  spot-checked against the seeded §23 fixture (`country IS US AND device
  IN [mobile, tablet] AND (OS IS Android OR OS IS iOS)`) via a standalone
  script mirroring the algorithm, since Jest/Vitest isn't wired up yet —
  all 5 cases passed.

### Fixed

- `DndContext` (Phase 7, `stream-set-list.tsx`) had no `id` prop, so
  dnd-kit's internal `useUniqueId` counter — which is module-level, not
  reset per-mount — produced a different `aria-describedby` id on the
  client than the server rendered, a confirmed hydration mismatch
  (dnd-kit's own SSR guidance: pass a stable `id` when using `DndContext`
  with SSR). Fixed with `id={\`stream-sets-${campaignId}\`}`.

### Known issues

- **Unresolved**: while smoke-testing this phase, a real browser tab
  connected to the dev server (not one of my own curl requests — curl
  can't drive client-side navigation) hit a repeating
  `Uncaught TypeError: Cannot read properties of undefined (reading
  'length') at Array.map (<anonymous>)` after navigating
  `/overview → /campaigns → /campaigns/[id]`. The dnd-kit `id` fix above
  eliminated the hydration-mismatch warning that preceded it the first
  time, but the crash itself recurred on a second capture without that
  warning, so it isn't fully explained by that fix. Terminal-forwarded
  browser errors don't carry source-mapped stack frames, so the exact
  call site couldn't be pinned down from the dev server log; production
  builds don't forward console output at all, and curl-only testing can't
  reproduce a client-side remount. Typecheck/lint/build are clean and a
  targeted static audit of every `.map()` call added or touched this
  phase found nothing unsafe. If this recurs, the browser's own DevTools
  console will have the real stack trace — that's the fastest path to a
  fix.
- Sticky assignment is explanatory text only — no real cookie/session
  state exists to simulate against yet.
- The weighted flow pick is a live `Math.random()` draw per Simulate
  click, not deterministic/repeatable — matches real routing behavior,
  but re-running the same inputs can select a different flow.

## [Phase 9] — Flow Builder

### Added

- Visual per-flow funnel (§24-25) replacing the flat name/URL/weight row
  from Phase 7-8: optional Landing (+ "show as PWA" toggle) → optional PWA
  (+ type: internal/external/ios_app) → optional Postlanding → a required
  terminal step that's either an **Offer** (network + offer + offer-URL
  carrying the `{click_id}` macro) or a **Redirect** (plain URL, no CPA
  attribution) — the segmented toggle between them lives on the terminal
  node itself. A dashed ghost **Fallback** node closes the funnel, showing
  the Stream Set's existing `fallbackUrl` (Phase 7) rather than a new
  per-flow field — all six §25 node types, one data source per concept.
- Every node supports the §25 capability set: enable/disable (optional
  stages only — the terminal step is always active), inline configuration
  (picker fields appear directly under the node header when enabled), a
  status badge (`Skipped`/`Needs setup`/`Configured`), a copyable preview
  URL, and a small deterministic mock analytics line (seeded per
  flow+stage — a real per-node metric needs the tracker event stream from
  Phase 16+, so this is a placeholder demonstrating the capability, not
  live data).
- Weight is now an arbitrary raw integer instead of Phase 7's "must sum to
  100" constraint, matching §24 exactly: the editor shows the raw weight
  next to the engine-normalized percentage (`weight / Σweights × 100`),
  and the Stream Set row's flow tags show the normalized % too.
- Per-flow **Duplicate** (§24's "duplicated" node/flow state), alongside
  the existing enable/disable and remove.
- `src/lib/mock/flow-entities.ts` — placeholder Network/Offer/Landing/PWA/
  Postlanding option lists so the funnel pickers have something to bind
  to. These become real, team-managed entities in Phase 11-12; the Flow
  shape (`networkId`/`offerId`/`landingId`/`pwaId`/`postlandingId`) is
  designed not to change when that happens, only the picker's data source.
- `src/features/stream-sets/flow-node.tsx` — the generic node card reused
  by all six node types; `flow-funnel.tsx` composes them per flow;
  `flow-editor.tsx` wraps one flow's header (name/weight/active/duplicate/
  remove) around its funnel, collapsible.

### Changed

- `Flow` (in `lib/mock/stream-sets.ts`) went from
  `{destinationType, destinationUrl}` to the funnel shape described above
  (`landing`, `pwa`, `postlanding`, `destination: {kind: "offer"|"redirect"}`).
  Mock stream sets regenerated to exercise it: set 0's first flow uses the
  full Landing→PWA→Offer chain, set 2 (bot/proxy block) uses a Redirect
  terminal instead of an Offer.

### Known issues

- Landing/PWA/Postlanding/Network/Offer pickers are the Phase 9 mock lists
  above, not real entities — real management UI is Phase 11 (Sources/
  Networks/Offers) and Phase 12 (Landing/PWA/Postlanding).
- Per-node "analytics summary" is a seeded mock number, not wired to any
  real event data yet.

## [Phase 8] — Filter Builder

### Added

- Recursive AND/OR filter tree (§22-23) replacing Phase 7's flat placeholder:
  `MATCH ALL`/`MATCH ANY` group pills, `+ Condition`/`+ Group` at every
  depth, arbitrary nesting — matches the §23 example structure exactly
  (`country IS US AND device IN [mobile, tablet] AND (OS IS Android OR OS
  IS iOS)`), which one of the Phase 7 mock stream sets now demonstrates.
- Full §22 field list (30 fields, grouped Geo/Device/Fraud/Traffic/Custom
  in the field picker) and all 16 operators, including `BETWEEN` (two-input
  range) and `MATCHES` (regex).
- Typed value inputs instead of one generic text box: `MultiSelect` (reused
  from Phase 5) for enum-like fields on `IN`/`NOT_IN`, a Yes/No toggle for
  `bot`/`proxy` (they're boolean-like flags, not free text, per §22's
  note), a plain Select for enum fields on single-value operators,
  no input for `EXISTS`/`NOT_EXISTS`.
- Save-time validation surfaced inline: ISO-3166 alpha-2 country codes
  (flags the classic `UK` mistake with "use GB"), and a client-side RE2-
  compatibility heuristic for `MATCHES` patterns (rejects lookaround,
  backreferences, atomic groups, possessive quantifiers) — a first pass
  only; real enforcement is Go's `regexp` (RE2) at save time per §5, which
  is why the check function's own doc comment says not to trust it as the
  source of truth.
- `src/lib/filters.ts` — the field/operator vocab, recursive tree types
  (`FilterNode = FilterCondition | FilterGroupNode`) and pure tree
  utilities (`addConditionToGroup`, `addGroupToGroup`, `updateCondition`,
  `updateGroupJoiner`, `removeNode`, `cloneWithNewIds`, `describeFilterTree`)
  that both the builder UI and the Stream Set row summary share — single
  implementation, per §6-SHARED.
- Stream Set row summary now renders top-level conditions as `FilterChip`s
  (Phase 3) plus a collapsed `(N)` badge per nested group, with the full
  plain-language tree (`describeFilterTree`) on hover via `Tooltip` — the
  static half of §72's explainability goal; the interactive "why did this
  match" surface is still the Phase 10 simulator.
- `src/lib/id.ts` — extracted the shared `genId` helper (was duplicated
  inline in `lib/mock/stream-sets.ts`) so `lib/filters.ts` doesn't import
  from `lib/mock/*`, avoiding a mock→domain→mock import cycle.

### Changed

- `StreamSet.filters: FilterCondition[]` + `joiner` (Phase 7's flat shape)
  is now `StreamSet.rootFilter: FilterGroupNode`, a real tree. `Campaign`/
  `Flow` shapes are unaffected.

### Known issues

- `useForm<StreamSetFormValues>`'s resolver needs an explicit `Resolver<T>`
  cast in `stream-set-form-sheet.tsx` — react-hook-form's `Path<T>` type
  can't fully resolve a self-referential union
  (`FilterNode = FilterCondition | FilterGroupNode`), so the zodResolver's
  inferred generic mismatches the plain type. Compile-time inference gap
  only, not a runtime issue — the filter tree isn't registered as RHF
  field paths anyway (it's one `Controller`-managed value mutated via the
  pure tree utilities above).
- The RE2 heuristic and country-code check run inline in the value editor,
  not through react-hook-form's per-field error state — deeply nested
  union array paths aren't practical to index into for that. The zod
  `superRefine` still blocks submission as a safety net either way.

## [Phase 7] — Stream Sets

### Added

- Stream Set management (§21) embedded in the campaign detail page's
  Overview tab, replacing the Phase 6 `EmptyState` stub: priority-ordered
  cards (`@dnd-kit` drag reorder, persisted via `useStreamSetsStore.reorder`),
  enable/disable `Switch`, duplicate, and a create/edit `Sheet` form.
  Semantics are stated explicitly in the UI copy — evaluated top-to-bottom
  by priority, first match wins, no match falls through to the campaign
  fallback — matching the explainability goal in §72 (the interactive
  "why did this match" surface is the Phase 10 simulator; this phase keeps
  it legible via plain-language copy and the same `FilterChip`/`FilterGroup`
  components already built in Phase 3 for exactly this purpose).
- Filters/Flows/Pixels editors are intentionally **flat**, not the final
  builders: filters are one AND/OR-joined list (no nested groups — that's
  Phase 8's 13-operator nested rule engine), flows are a weighted list
  pointing at a destination URL directly (no node graph or offer picker —
  that's Phase 9 plus the Offers/Landings/PWA entities from Phase 11-12).
  Same `FilterCondition`/`Flow` field shapes Phase 8-9 will consume, so the
  data contract doesn't change when the real builders land, per the
  single-source-of-truth rule in §6-SHARED.
- Flow weight editor sums live and warns (non-blocking) when weights don't
  total 100%.
- `src/lib/mock/stream-sets.ts` — deterministic per-campaign generator
  (seeded from the campaign id, same pattern as the Phase 6 daily-trend
  generator) plus the shared `FilterField`/`FilterOperator`/
  `FlowDestinationType` vocab §22 will reuse.
- `src/stores/stream-sets.ts` — Zustand store keyed by campaign id
  (`addStreamSet`, `updateStreamSet`, `setStatus`, `duplicateStreamSet`,
  `reorder`).

### Fixed

- `listByCampaign` originally lazily generated a campaign's stream sets
  and called `set()` **inside the selector function itself** — i.e. during
  React's render phase. Combined with `generateStreamSets` returning a new
  array reference on every call, this broke `useSyncExternalStore`'s
  snapshot-stability contract: each render's snapshot read as "changed"
  from the last, so React kept re-rendering, which kept re-invoking the
  selector, forever. Confirmed live — a real connected browser tab hit the
  campaign detail page and spammed ~8k identical `TypeError`s/sec in a
  reconnect-retry loop before the fix. Fixed by making `listByCampaign` a
  pure read (no `set()` call) backed by a module-level generation cache
  keyed by campaign id, so an unmaterialized campaign's snapshot is
  referentially stable across repeated selector calls; store mutations
  still go through `set()`, but only ever from event handlers, never from
  a render-phase selector.

### Known issues

- Stream sets are mock/in-memory like campaigns (Phase 6) — reset on a
  hard reload.
- Fallback URL is per-stream-set only in this phase; the campaign-level
  fallback from Phase 6 is what's actually used when no set matches at
  all (§73's "no stream set matches → campaign fallback" case). The
  per-set fallback here covers a narrower case (set matched, no flow
  resolved) and isn't yet wired into the Phase 10 simulator, since that's
  real routing evaluation and out of scope until Phase 10/19.

## [Phase 6] — Campaigns

### Added

- Campaign list (§20) at `/campaigns`: `DataTable` with Name/Status/Source/
  Clicks/Conversions/Revenue/Spend/Profit/ROI/Updated columns, search,
  sort, pagination. Profit/ROI render "—" (never a false $0/0%) for the
  ~12% of mock campaigns generated with no spend, per §27-COST.
- Row actions (§20): Open, Pause/Resume, Duplicate (navigates to the new
  copy), Copy tracking URL (toast with the composed
  `https://{trackingDomain}/t/{trackingId}` URL), Archive (confirm dialog,
  destructive). Duplicate and Archive share `useCampaignsStore` mutations
  used by both the list row menu and the detail page.
- Campaign creation at `/campaigns/new` and detail/settings at
  `/campaigns/[id]` (Overview + Settings tabs). Overview shows the same
  8 stat cards as the dashboard for one campaign, a 30-day revenue trend
  chart, and a "Stream Sets" card stubbed as `EmptyState` until Phase 7-9
  build routing. Settings reuses the creation form (`CampaignForm`,
  react-hook-form + zod) with an added Status field and a danger-zone
  Archive action.
- `src/stores/campaigns.ts` — Zustand store (`addCampaign`,
  `updateCampaign`, `setStatus`, `duplicateCampaign`, `getById`) seeded
  from `src/lib/mock/campaigns.ts`'s deterministic generator so the
  detail page resolves an id consistently within a session.

### Known issues

- Detail page 404s (`ErrorState`, with a link back to the list) if the
  mock store resets — e.g. a hard reload after `addCampaign` — since
  campaign data is in-memory only until Phase 16+ wires the real API.
- Stream Sets/Filters/Flows are not built yet; every campaign routes
  100% to its fallback URL until Phase 7-9.

## [Phase 5] — Analytics

### Added

- Analytics explorer (§19) at `/analytics`: controls for date range, timezone,
  dimensions (16, multi-select), metrics (13, multi-select), filters
  (dimension = value, AND-joined), group-by, sort, and a compare-to-previous-
  period toggle. Four views on one shared aggregation: Table (dynamic
  `DataTable` columns), Line (any selected metric over time), Bar (top-8
  breakdown by the group-by dimension), and Funnel — the full 8-step event
  model (§43) `SOURCE_CLICK → ... → CPA_REDEP` with per-step and per-total
  conversion %, not a generic 3-step funnel.
- `src/components/ui/multi-select.tsx` — reusable Popover+Command checklist
  (dimensions/metrics here; the Filter Builder in Phase 8 will likely want it
  too).
- `src/features/analytics/registry.ts` — metric formulas match §50 exactly
  (`roi=(revenue-cost)/cost`, `roas=revenue/cost`, etc.); mock data marks
  ~15% of slices with no cost at all, so aggregated ROI/CPA/cost render "—"
  rather than a false $0/0%, at any grouping granularity.
- `src/lib/format.ts` — `formatUsd`/`formatInt`, both pinned to the `en-US`
  locale.

### Fixed

- Every `toLocaleString()` currency/number call across the app (Dashboard
  included, from Phase 4) used the runtime's default locale instead of a
  fixed one. In this environment that silently rendered USD as
  `14 655,87 $` instead of `$14,655.87` — locale-dependent formatting for a
  fixed-locale product. Centralized in `src/lib/format.ts` and fixed
  everywhere it was called.

### Known issues

- Timezone selector is cosmetic in the mock phase — it doesn't shift
  aggregation, since slices only carry a date, not a timestamp.

## [Phase 4] — Dashboard

### Added

- Overview dashboard (§18) at `/overview`, replacing the stub: 8-card KPI row
  (Revenue/Spend/Profit/ROI/Clicks/Conversions/CVR/CPA) with period-over-period
  trend deltas, 4 time-series charts (Revenue/Spend/Profit/Conversions) via
  Apache ECharts, and 4 top-N tables (Campaigns/Offers/Countries/Flows) on the
  existing `DataTable`. `DateRangePicker` drives both the charts and the KPI
  comparison window (current period vs. the equal-length period before it).
- `src/lib/mock/dashboard.ts` — deterministic (seeded-PRNG, not `Math.random`)
  mock data generator, so the statically-exported page is reproducible across
  builds. One campaign carries no spend on purpose, to exercise §27-COST: ROI
  renders as "—", never a false 0%.
- `src/lib/chart-theme.ts` — ECharts option tokens reusing the exact design
  tokens (dark/light), instead of a second hardcoded palette.

### Fixed

- `StatCard`'s value/delta row had no wrap or shrink handling and clipped
  against the card edge on narrow (mobile) widths; now wraps the delta to its
  own line instead of overflowing.

### Known issues

- The full ECharts bundle takes ~1.3s to parse and paint on first mount
  (confirmed deterministic via polling, not flaky) — acceptable for the mock
  phase; revisit with a lighter `echarts/core` + explicit chart-type imports
  in Phase 31 (performance).
- Top-N tables are not filtered by the selected date range (only the KPI row
  and charts are) — there's no per-row time series in the mock to filter by.

## [Phase 3] — Application Shell

### Added

- Persistent app shell (§17) under the `(app)` route group: `Sidebar`
  (workspace selector, grouped nav, expand/collapse via a persisted Zustand
  store, active-link highlighting) and `Topbar` (breadcrumbs derived from the
  route, ⌘K command menu, notifications popover, theme toggle, user menu).
- Mobile: off-canvas nav via shadcn `Sheet`, triggered from the topbar
  hamburger; same `NavContent` as desktop so the nav tree has one definition.
- `src/lib/nav.ts` — single source of truth for the nav tree, consumed by the
  sidebar, breadcrumbs, and command menu (no duplicated nav data).
- One stub page per sidebar item (`EmptyState`, "not built yet") so every
  link resolves — nothing 404s while Phases 4–14 fill in real content.

### Fixed

- `CommandDialog` (shadcn) renders `DialogContent` around `children` directly
  — it does **not** include an inner `<Command>` root the way older shadcn
  versions did. Passing `CommandInput`/`CommandList` straight into
  `CommandDialog` crashed with `Cannot read properties of undefined (reading
  'subscribe')` (cmdk's internal store context was missing). Fixed by
  wrapping the palette contents in `<Command>` inside `CommandDialog`.
- Topbar overflowed and wrapped onto a second line on narrow viewports (the
  search bar had a fixed `w-56` and breadcrumbs weren't allowed to truncate).
  Search collapses to an icon-only trigger below `sm`; breadcrumbs truncate
  instead of wrapping.

### Known issues

- Workspace selector, notifications, and user menu are mock data — wired to
  real data in Phase 27 (integration) / Phase 28 (auth).

## [Phase 2] — Design System

### Added

- Next.js 16 (App Router) + React 19 + TypeScript app scaffold in `apps/web`,
  Tailwind v4, shadcn/ui (`radix-nova` style, Radix UI primitives).
- Dark-first FLOX token system in `src/app/globals.css`: neutral surfaces,
  small radius (0.375rem), one restrained blue accent, semantic
  success/warning/danger/info tokens (light + dark), tabular-numeral utility.
  Light theme fully supported via `next-themes` (`ThemeProvider`,
  `ThemeToggle`), dark is the default (`defaultTheme="dark"`).
- Typography scale (`Display/H1/H2/H3/Body/Small/Caption/Mono`) in
  `components/ui/typography.tsx`.
- Full §16 component library: Button, IconButton, Input, Select, Checkbox,
  Radio, Switch, Textarea, Dialog, Popover, Tooltip, Dropdown, Command, Tabs,
  Badge, Tag, Avatar, Card, StatCard, DataTable (TanStack Table v9, sort /
  paginate / global search / column visibility), EmptyState, ErrorState,
  LoadingState, Skeleton, DateRangePicker, Breadcrumbs, Pagination, ChartCard
  (Apache ECharts mount point), FilterChip, FilterGroup, Alert, Toaster.
- `/style-guide` route showcasing every token and component in one page.

### Fixed

- `radix-ui` was imported by 15 generated shadcn components but never
  installed (silent gap from the CLI's dependency step); added explicitly.
  Removed the now-unused `@base-ui/react` dependency left over from the
  initial scaffold.

### Known issues

- DataTable "virtualize large sets" (UX floor) is satisfied via pagination,
  not DOM windowing — acceptable for now; revisit with
  `@tanstack/react-virtual` if a real dataset needs unpaginated scroll.
- No application shell yet (Phase 3) — `/style-guide` and `/` are standalone
  routes.

## [Phase 1] — Product Foundation

### Added

- `README.md`, `ARCHITECTURE.md`, `PRODUCT.md`, `ROADMAP.md` at repo root.
- `.env.example` covering the planned Postgres/ClickHouse/Redis/S3/auth
  configuration surface.
- Monorepo directory skeleton: `apps/{web,api,tracker,worker}`,
  `packages/{ui,config,types}`, `infra/`, each with a placeholder README
  describing its future contents.
- `docs/architecture.md`, `docs/domain-model.md`, `docs/event-model.md`,
  `docs/routing.md`.

### Fixed

- `gitignore` renamed to `.gitignore` — the file existed without a leading
  dot and was silently not being applied by git.

## [Phase 0] — Repository Inspection

### Added

- Inspected an empty repository (only `CLAUDE.md`, `docs/FLOX-master-prompt-v3.md`,
  `.idea/`, `.claude/settings.local.json` present, zero commits).
- Chose shared-domain-logic strategy **A** (§6-SHARED): Go core is the single
  source of truth for routing decisions; the Routing Simulator consumes the
  `POST /routing/simulate` contract (mocked during frontend-first phases,
  wired to the real endpoint in Phase 27).
- Identified that `gitignore` (no leading dot) was not being honored by git.
