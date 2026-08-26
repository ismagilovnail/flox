# Ad Account Connections (§74/§27-COST)

# Phase A: credential storage

First slice of the FB/TikTok ad-spend import candidate. Split into two
phases (confirmed via `AskUserQuestion` before starting): **this phase**
builds the storage/CRUD half — an operator connects an ad account by
pasting a token, fully verifiable end-to-end in this environment. A
later, separate phase builds the real Facebook Ads / TikTok Ads API
clients and a sync job — those can only be verified structurally (unit
tests against a fake HTTP transport) since no live Meta/TikTok app
credentials exist here.

## Why manual token paste, not OAuth

Facebook's and TikTok's OAuth consent flows both require a registered
app with a public HTTPS callback URL. Neither exists for this project in
this environment, and building OAuth redirect/callback endpoints that
can never be exercised against a real app would be unverifiable,
decorative code — this session's standing rule against faking
integrations that "look real." An operator instead pastes a long-lived
access token + ad account id directly, the same MVP pattern plenty of
affiliate tools use. Nothing about the storage shape (`Connection`,
`ConnectInput`) or the `CostProvider` interface below depends on how the
token was obtained — OAuth can replace the paste-a-token UI later
without touching either.

## `traffic_sources.cost_integration` already existed — this phase is what plugs into it

`CostIntegration` (`none`/`manual`/`facebook_ads`/`tiktok_ads`, migration
00002) has recorded *intent* since Traffic Sources' own CRUD phase — its
own doc comment says so explicitly: "this field doesn't create
[entries]." This phase is the thing that was missing: connecting real
credentials for a source whose intent is already `facebook_ads`/
`tiktok_ads`.

## Schema: one connection per traffic source, no separate status column

`ad_account_connections` (migration 00018): `id`, `organization_id`,
`traffic_source_id` (`UNIQUE`), `ad_account_id`, `access_token`,
timestamps. `UNIQUE` on `traffic_source_id` matches `cost_integration`'s
own singular "this source uses at most one integration" design. No
`status` column: the row's existence *is* "connected" — connecting
upserts by `traffic_source_id` (`ON CONFLICT DO UPDATE`, same
re-submit-to-replace precedent as `cost_entries`), disconnecting is a
plain `DELETE`, reconnecting after that just inserts a fresh row. No
`provider` column either — deliberately not stored redundantly against
`traffic_sources.cost_integration`, to avoid the two ever drifting;
`Service.Connect` reads the source's own `cost_integration` at write
time instead (`Repository.TrafficSourceCostIntegration`, a raw
cross-domain query — same pattern as `streamset`'s own `*BelongsToOrg`
methods, not a package import) and rejects `none`/`manual`.

## `access_token` is stored in plain text — and never leaves this package as such

No KMS/envelope-encryption infrastructure exists anywhere in this
codebase yet. `api_keys` (migration 00010), the only prior
credential-shaped table, sidesteps the problem entirely by only ever
needing to *verify* a value (a one-way hash suffices); this table's
token must be read back out in full for a later phase's sync to call
Facebook's/TikTok's API, so a hash can't be used here. The real control
this phase applies is at the Go API boundary: `Connection` (the
JSON-response struct) has no `AccessToken` field at all — not
`omitempty`'d, structurally absent — and `repository.go`'s
`selectColumns` never selects the raw column into anything that gets
marshaled; only a SQL-computed `right(access_token, 4)` (`TokenPreview`)
is ever read back. A future sync job reads the real token through a
separate, never-serialized `Credentials` struct instead (see below).
Encryption-at-rest for this column is real follow-up hardening work, not
silently deferred — flagged here and in the migration's own comment
rather than left unmentioned.

## `CostProvider`: declared now, called by nobody yet

```go
type CostProvider interface {
	DailySpend(ctx context.Context, creds Credentials, from, to time.Time) ([]DailySpendRecord, error)
}
```

The §74 extensibility interface a later phase's real Facebook Ads/TikTok
Ads adapters implement. Declared in this phase specifically so this
phase's storage shape is provably sufficient for that future caller, not
guessed at — `Credentials` (a separate type from `Connection`, holding
exactly `AdAccountID`+`AccessToken`) is what an actual API call needs;
`DailySpendRecord` returns amounts in the ad platform's own native
currency, never pre-converted (USD normalization via `fx_rates`, §50-FX,
stays a single point of truth, same as every other cost value in this
system — happens once, at the point a record is written into
`cost_entries`, not inside each provider adapter).

## API: one nested sub-resource, no id in the URL

`GET/PATCH/DELETE /traffic-sources/{id}/connection` — no `{connectionId}`
segment, since a traffic source has at most one. `PATCH` (not `PUT`,
despite `PATCH` here fully replacing the row) to match this codebase's
own established convention for "replace in place" — Stream Sets' whole-
`flows`-array replacement is also a `PATCH`, and this project has never
used `PUT` anywhere else.

## Frontend: lives inside the existing Traffic Source edit sheet

New `AdAccountConnectionSection`, rendered inside `SourceFormSheet` only
in edit mode (`sourceId` set) and only when the **live, not-yet-saved**
form value of `costIntegration` is `facebook_ads`/`tiktok_ads`
(`useWatch`, not the original `defaultValues` — so switching the
dropdown shows/hides the section immediately, before the surrounding
form is even saved). Connecting before saving that dropdown change is
correctly rejected server-side (`Service.Connect` reads the *persisted*
`cost_integration`, still whatever it was) — confirmed live during
manual verification, not a bug: the section's own description text says
plainly that it stores a credential, not that it's independent of the
surrounding form's save state.

### A real bug, caught and fixed: nested `<form>` leaked the token into the URL

`AdAccountConnectionSection` originally rendered its own `<form
onSubmit={...}>` for its two-field connect form. It's nested inside
`SourceFormSheet`'s own outer `<form>` (the section renders inside the
main form's JSX, not in a portal) — HTML forbids nested `<form>`
elements, and Chrome's actual behavior for it isn't "the inner form is
ignored": the inner submit button's default browser behavior fires
against whichever form the browser actually parsed, submitting as a
plain GET with every field — including the raw access token — appended
to the URL as a query string. Reproduced live during this phase's manual
verification (`?adAccountId=...&accessToken=...` appeared in the address
bar after clicking "Connect"). This is exactly the
"never place sensitive data in URL parameters" case.

Fixed by removing the inner `<form>` entirely — a plain `<div>`, with the
"Connect" button changed from `type="submit"` to `type="button"` calling
`form.handleSubmit(submit)` directly from `onClick`, and a manual
`onKeyDown` handler on the wrapping `<div>` to keep Enter-to-submit
working without a `<form>` element to grant it for free. Verified via
`read_network_requests` after the fix: a real `PATCH .../connection`
request, no GET-with-query-params navigation, and the address bar stayed
plain throughout a full connect/disconnect cycle afterward.

## Verified

Backend: `go build/vet/gofmt/test ./...` all green — new
`adaccount_test.go` covers connect/get/reconnect-replaces-in-place/
disconnect, rejecting a source whose `cost_integration` is `none`/
`manual` (and accepting `tiktok_ads`), invalid-shape validation (empty
`adAccountId`/`accessToken`), and full cross-tenant isolation
(get/disconnect/connect-against-another-org's-source).

Frontend: `tsc --noEmit`/`eslint`/`vitest run` (21 tests, unchanged — no
new frontend unit tests this phase, same reasoning as Pixels: the new
logic is UI wiring over an established form/query/mutation pattern, not
new business logic to unit-test)/`next build` (production) all clean.

Full manual browser pass against the real running `api`+`web` dev
servers: opened a pre-existing "Facebook Ads" traffic source (fixture,
`cost_integration` originally `none`) for edit, switched the dropdown to
`facebook_ads` — the connection section appeared immediately, live;
attempted to connect before saving (correctly rejected, `422`, confirmed
`cost_integration` was still `none` in Postgres); saved the source
(confirmed `cost_integration` became `facebook_ads` in Postgres);
reopened the edit sheet and connected a fake token — **this is where the
nested-`<form>` bug above was caught, fixed, and re-verified** (`PATCH`
now returns `200`, was previously a silent GET-with-leaked-token
navigation); reloaded the page fresh and confirmed the connected state
(masked token preview, real ad account id) round-tripped through the
real Postgres-backed API; disconnected (`DELETE` → `204`, confirmed `0`
rows in `ad_account_connections` afterward) and confirmed the section
reverted to its empty connect form. No console errors at any point.
`cost_integration` reverted to `none` on the fixture directly in
Postgres afterward, restoring it to its pre-phase state.

---

# Phase B: real Facebook Ads / TikTok Ads API adapters + sync

The second half promised by Phase A above. User confirmed via
`AskUserQuestion` this phase should build the real adapters + a sync,
still only structurally verifiable (no live Meta/TikTok app credentials
exist in this environment) — and separately confirmed a real
architectural gap raised mid-phase (below) via a second
`AskUserQuestion`.

## The gap Phase A's `CostProvider` didn't account for: which FLOX campaign?

`cost_entries.campaign_id` is `NOT NULL` (migration 00009), but a
platform's Insights API reports spend broken down by *its own* campaign
id, and one traffic source's connected ad account can fund more than one
FLOX campaign (`campaigns.traffic_source_id` is many-to-one, not
one-to-one) — nothing tied a synced spend row to a specific FLOX
campaign. Resolved by adding an optional `external_campaign_id` column
to `campaigns` (migration 00019, `text NOT NULL DEFAULT ''` — same "empty
string means unset" convention as `fallback_url`/`notes` on the same
table) that an operator pastes the ad platform's own campaign id into.
No uniqueness constraint: an operator can deliberately map two FLOX
campaigns to one ad-platform campaign (e.g. split-testing two FLOX
setups against one real ad spend) and the sync attributes the full day's
spend to both — `campaign.Repository.ListByExternalID` (scoped to
`(organizationId, trafficSourceId, externalCampaignId)`, never a bare org-
wide match, since a connection's own results only ever cover one traffic
source) returns a slice for exactly this reason. A record whose
`ExternalCampaignID` matches nothing produces no `cost_entries` row for
that day — CLAUDE.md invariant #6 ("no cost for a slice shows as `—`,
never a false zero") applies directly; an ad account will always report
spend for campaigns FLOX doesn't know about.

`adaccount.CostProvider` was revised accordingly, before anything called
it: `DailySpendByCampaign` (not an account-level total) returning
`DailyCampaignSpendRecord{Date, ExternalCampaignID, Amount, Currency}` —
amounts stay in the ad platform's own native currency, USD normalization
via `fx_rates` (§50-FX) still happens exactly once, at the point a record
is written into `cost_entries`, same as every other cost value in this
system.

## Real HTTP adapters, real request shapes, structurally verified

`internal/adaccount/facebookads` — Facebook Graph API Marketing Insights
(`GET /act_{id}/insights`, `level=campaign`, `time_increment=1`,
`time_range` JSON-encoded, `fields=campaign_id,spend,account_currency`),
following `paging.next` (capped at `maxPages=500`) and parsing the real
Graph API error-body shape (`{"error":{"code","type","message",
"fbtrace_id"}}`).

`internal/adaccount/tiktokads` — TikTok Business API (`GET
/report/integrated/get/`, `report_type=BASIC`, `data_level=
AUCTION_CAMPAIGN`, `dimensions=["campaign_id","stat_time_day"]`,
`metrics=["spend"]`, `Access-Token` header not a query param — TikTok's
own convention, unlike Facebook's), paginating on `page_info.total_page`.
TikTok's reporting endpoint does **not** return currency per row (it's an
account-level property); a second call to `GET /advertiser/info/` fetches
it once per sync and stamps every record with it.

Both adapters take an injectable `BaseURL`/`HTTPClient` specifically so
`*_test.go` can point them at an `httptest.Server` instead of the real
host — this project has no live Meta/TikTok app credentials, so unit
tests are the only verification the request/response shapes get. (They
also, incidentally, got verified against the **real** Graph/Business API
during this phase's manual pass below — with intentionally-invalid
tokens, so the only thing exercised for real was "does this project's
request reach the real endpoint and get a real, correctly-parsed error
back," never a real spend pull.)

## `cost.Source`: cost_entries finally records where a value came from

`cost_entries.source` (migration 00009's own `CHECK` constraint has
allowed `facebook_ads`/`tiktok_ads` since that migration) had never
actually been written as anything but the hardcoded literal `'manual'`
until this phase. `cost.Service.Upsert` (the only HTTP-reachable write
path — `upsertRequest` in `handler.go` has no `source` field to decode
into) always still writes `SourceManual`. A new `cost.Service.
UpsertFromSync` — Go-only, never wired to any HTTP route — takes `source
cost.Source` as its own explicit parameter and rejects `SourceManual`/any
invalid value. This was a deliberate design correction made mid-
implementation: `Source` was initially just a field on the shared
`UpsertInput` struct, then reverted specifically because that would make
"an HTTP client can't set an arbitrary source" true only by the
incidental fact that `upsertRequest` doesn't happen to populate that
field — a trust-by-omission gap, not a structural guarantee. Splitting
into two methods with source only ever an explicit parameter on the
trusted path is the same "give a trusted-only write its own explicit
entry point" pattern the previous section's `Credentials` type already
established (never on `Connection`, the API-response type).

## `internal/costsync`: the orchestrator

New package, `handler → service`, no `repository.go` of its own (reads
through `adaccount.Repository`, `campaign.Repository`, and writes through
`cost.Service` directly — it owns no table). `Service.Sync(ctx, orgID,
trafficSourceID, from, to)`:

1. Reads the traffic source's `cost_integration` and picks the matching
   `Providers.FacebookAds`/`TikTokAds` (`§74`/invariant #11: providers
   behind interfaces — this is the sync's one vendor-specific branch).
2. Reads the real access token via a new `adaccount.Repository.
   CredentialsByTrafficSourceID` — deliberately added to `*Repository`
   directly, never to `adaccount.Service` (the public, HTTP-facing type),
   the same "trusted caller holds the repository directly" split as
   `cost.Service.UpsertFromSync` above.
3. Calls the provider, matches each record via `campaign.Repository.
   ListByExternalID`, writes one `cost_entries` row per matched campaign
   via `cost.Service.UpsertFromSync` — unmatched records are silently
   skipped (capped list of up to 20 unmatched external ids returned in
   `Result` for the UI to surface, plus a truncation flag).

`POST /traffic-sources/{id}/connection/sync` — mounted in the **same**
`srv.Mux().Route("/traffic-sources/{id}/connection", ...)` block as
`adaccount`'s own `GET/PATCH/DELETE`, not a second `Route()` call: chi
panics if two separate `Route()` calls claim the identical literal
pattern, so `adAccountHandler.Register(r)` and `costSyncHandler.
Register(r)` both run inside one closure. Query params `from`/`to`
(`YYYY-MM-DD`) default to a 30-day lookback (`to=today`, `from=today-29`)
— the same default window `cost` handler's own `parseRange` already
uses, since ad platforms commonly revise very recent days' reported
spend and a "Sync now" without explicit dates should re-pull that whole
window, not just today.

## Frontend

`Campaign.externalCampaignId` added to the campaign form (`campaign-
form.tsx`, both create and edit — `campaign-detail-view.tsx`'s Settings
tab pre-fills it from the loaded campaign) and to `CreateCampaignInput`/
the `Campaign` type in `lib/api/campaigns.ts`.

`AdAccountConnectionSection` (Phase A) gained a "Sync now" button, shown
only once a connection exists (next to "Disconnect"), and an inline
result summary card below it (`sync.data`, no extra `useState` needed —
the mutation's own `data` is enough): records fetched, entries written,
and a capped, `Badge`-rendered list of unmatched external campaign ids
with a hint pointing at where to set them. `useSyncAdAccount` invalidates
every `["cost-entries", ...]` query on success (broad, not scoped to one
campaign id — the caller only knows `trafficSourceId`, a sync can write
to any number of campaigns under it, and this is a low-frequency manual
action so the broad invalidation isn't a real cost).

## Verified

Backend: `gofmt`/`go build ./...`/`go vet ./...`/`go test ./...` all
green, including new tests in `internal/cost` (`TestUpsertRecordsManualSource`,
`TestUpsertFromSync` incl. its two rejection cases),
`internal/adaccount/facebookads` and `.../tiktokads` (httptest.Server-
backed: single page, pagination, real error-body shape, malformed spend
value), and `internal/costsync` (matched-campaign write, unmatched
skip-and-report, two-campaigns-share-one-external-id both get the full
day's spend, not-connected error, no-provider-for-cost-integration
error) — all against a real Postgres, per this project's existing
integration-test convention.

Frontend: `tsc --noEmit`/`eslint`/`vitest run` (21 tests, unchanged)/
`next build` all clean.

Full manual verification against the real running `api`+`web` dev
servers, **including real network calls to the actual Facebook Graph API
and TikTok Business API** (this environment has outbound internet
access) with intentionally-invalid tokens:

- `curl`-level: created a `facebook_ads` traffic source, confirmed `sync`
  404s before any connection exists, connected a fake token, created a
  campaign with a matching `externalCampaignId`, triggered `sync` — got
  back a real Graph API `OAuthException` (code 190, "Invalid OAuth access
  token"), logged server-side with full detail, `500` returned to the
  client with a generic message (no token or internal detail leaked).
  Repeated for `tiktok_ads` — real Business API error (code 40105,
  "Access token is incorrect or has been revoked") from the
  `advertiser/info/` call specifically, confirming the two-call
  (currency-then-report) design's first call is what's reached first.
  Server stayed up and serving after both.
- Browser-level: switched a fixture traffic source's `costIntegration` to
  `facebook_ads` live in the edit sheet, saved, connected a fake
  credential (`PATCH` → `200`), clicked "Sync now" — real `POST .../sync`
  fired, got the same real Graph API `OAuthException` back, and the UI
  correctly rendered an error toast ("Couldn't sync ad spend" / "internal
  server error") rather than crashing or showing a blank state. Created a
  campaign through `/campaigns/new` with `externalCampaignId` filled in,
  confirmed the value round-tripped through the real API (`GET
  /campaigns/{id}` after create) and pre-filled correctly on the
  campaign's own Settings tab.
- Cleanup: deleted the test campaign, disconnected both fake ad account
  connections, reverted the Facebook Ads fixture's `cost_integration`
  back to `none` — matching state before this phase's manual pass.

# Phase C: sync scheduler

Phase B's "Sync now" button (`POST .../connection/sync`) was manual-only
by design — no scheduler existed. This phase automates it: a background
job in `apps/worker` re-syncs every connected ad account on a fixed
interval, with no change to the manual endpoint (an operator can still
force an immediate sync between scheduled runs).

## Why a ticker, not `PollLoop`

`apps/worker`'s three existing loops (`postback.Deliverer`,
`eventqueue.Flusher`, `postbacklog.Flusher`) all share one shape: claim a
batch of *due* rows from a queue table, process them, and idle briefly
if the batch was empty or partial (`PollLoop(ctx, batchSize, idle)`).
That shape fits a backlog that grows between polls. Ad-spend sync has no
such backlog — there's no per-connection "due" state, just "everyone
gets synced again every N hours" — so `costsync.Scheduler.RunLoop(ctx,
interval)` uses a plain `time.Ticker` instead: run once immediately on
start, then again every `interval`, forever, until `ctx` is done.

## New: `adaccount.Repository.ListAllConnections`

Every other method on `adaccount.Repository` takes an `orgID` and scopes
its query to it — correct for a handler serving one tenant's request,
but wrong for a scheduler that must find *every* connected ad account
across *every* org on its own timer, with no request to scope from.
`ListAllConnections(ctx) ([]ConnectionRef, error)` is deliberately the
one unscoped query in this package: it lives directly on `*Repository`
(never on `Service`, same "only a trusted Go-only caller holding the
repository directly" pattern as `CredentialsByTrafficSourceID`, Phase B)
and is never reachable from any HTTP route. `ConnectionRef` carries only
`OrganizationID`/`TrafficSourceID` — exactly what `costsync.Service.Sync`
needs to run for one connection; the credential lookup itself still goes
through the existing, per-connection `CredentialsByTrafficSourceID` once
`Sync` is called for that ref.

## `costsync.Scheduler`

`NewScheduler(svc *Service, connections connectionLister, logger
*slog.Logger) *Scheduler` — `connectionLister` is a one-method interface
(`ListAllConnections`) satisfied by `*adaccount.Repository`, narrowed the
same way `Providers`' own `adaccount.CostProvider` interface is, so tests
substitute a fake instead of a real Postgres pool.

- `RunOnce(ctx) (int, error)` lists every connection, then calls
  `Service.Sync` for each one in turn using the same 30-day
  (`defaultLookbackDays`) window an on-demand "Sync now" defaults to —
  both ad platforms commonly revise very recent days' reported spend, so
  every scheduled run re-pulls the whole window rather than tracking a
  "since last sync" cursor. **One connection's `Sync` failing (expired
  token, a transient API error, a traffic source that got disconnected
  mid-run) is logged and skipped — it never aborts the rest of the
  batch.** Losing org B's sync to org A's bad token would be a silent,
  cross-tenant-shaped failure mode; CLAUDE.md invariant #5 (tenant
  isolation) reads most naturally as "no org's data leaks to another,"
  but this is the adjacent case — no org's *background job* should be
  starvable by another org's broken credential either.
- `RunLoop(ctx, interval)` wraps `RunOnce` in the ticker described above.
  A `RunOnce` error (e.g. the initial `ListAllConnections` query itself
  failing — a DB blip) is logged and the loop waits for its next tick
  rather than exiting; a scheduler that gave up permanently after one bad
  listing would silently stop syncing *everyone's* spend until the whole
  worker process was restarted, which is worse than trying again in
  `costSyncInterval`.

## Wiring: `apps/worker/main.go`

`costSyncInterval = 6 * time.Hour` (a plain const, same convention as the
existing poll loops' `postbackPollBatchSize`/`eventPollIdle` etc. — no
env var, since none of the other three loops' timing is
env-configurable either). The worker constructs the same
`adaccount.Repository`/`campaign.Repository`/`cost.Service`/
`costsync.Providers{FacebookAds: facebookads.New(), TikTokAds:
tiktokads.New()}` combination `apps/api/main.go` already builds for the
manual endpoint, wraps it in `costsync.NewScheduler(...)`, and launches
`go costSyncScheduler.RunLoop(ctx, costSyncInterval)` alongside the three
existing `go x.PollLoop(...)` calls.

## Verified

Backend: `gofmt`/`go build ./...`/`go vet ./...`/`go test ./...` all
green, including new tests — `adaccount.TestListAllConnections` (cross-
org, and confirms an unconnected traffic source is excluded) and
`costsync`'s `TestSchedulerRunOnceSyncsEveryConnection` (two different
orgs, two different providers, one `RunOnce`),
`TestSchedulerRunOnceContinuesPastAFailingConnection` (a broken
connection ahead of a good one in the list; the good one's cost entry
still gets written), and `TestSchedulerRunLoopStopsOnContextCancel` —
all against real Postgres per this project's integration-test
convention, plus one pure-unit test needing no database at all.

Full manual pass: started `apps/worker` against the real dev Postgres/
ClickHouse/Redis stack and confirmed, from its own logs, that
`RunLoop`'s immediate first tick ran with zero connections
(`"connections_attempted":0`) on a clean database. Then, with the worker
stopped, seeded one real row directly in Postgres — an org, a
`facebook_ads` traffic source, and an `ad_account_connections` row with
an intentionally-invalid token — and restarted the worker: its first
tick picked up the seeded connection and made a **real network call to
the live Facebook Graph API**, which came back with the same genuine
`OAuthException` (code 190, "Invalid OAuth access token") Phase B's own
manual pass got, logged with the connection's `organization_id`/
`traffic_source_id` and the full error, followed immediately by
`"scheduled ad spend sync run finished","connections_attempted":1` — the
loop correctly treats a failed sync as "attempted," not a crash, and
keeps running. Test fixtures (org, cascade-deleted traffic source +
connection) were deleted afterward.
