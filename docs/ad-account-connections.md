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

## `access_token` was stored in plain text — and never left this package as such

Retrofitted to real AES-256-GCM encryption in Phase D, below — this
section describes Phase A's original design, at a time when no
encryption infrastructure existed anywhere in this codebase yet.
`api_keys` (migration 00010), the only prior
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

# Phase D: Google Ads, the third real `CostProvider`

Picked off this doc's own "candidate" list (user-scoped, well after
Phase C) as its own separately-scoped phase — Google Ads had been
deliberately deferred out of the Settings > Integrations phase
(`docs/settings-integrations.md`), which only ever gave it a read-only
aggregate card, precisely because it belongs here instead: a third
per-traffic-source cost integration, matching Facebook/TikTok Ads
exactly (`trafficsource.CostIntegration`'s own singular design).

## Why Google Ads needed real schema changes, not just a third adapter

Facebook/TikTok Ads both accept a single long-lived access token pasted
directly. Google Ads' own API has no equivalent — its access tokens
expire in about an hour. An operator instead pastes a **refresh token**
(minted once via Google's own OAuth 2.0 Playground, using their own
Google Ads developer token and a Google Cloud OAuth client they already
have — no registered public callback exists in this environment, same
constraint Phase A's own manual-token-paste decision already documents)
alongside that developer token and OAuth client id/secret. Every real
sync call first exchanges the refresh token for a short-lived access
token via the real OAuth 2.0 refresh-token grant, then calls the Ads API
with it — nothing about that exchange is persisted, since the refresh
token itself doesn't expire under normal use.

Migration 00034 widens two enums (`traffic_sources.cost_integration` and
`cost_entries.source`, both to add `'google_ads'`) and adds four nullable
columns to `ad_account_connections`: `developer_token`, `client_id`,
`client_secret`, `refresh_token` (`access_token` itself becomes nullable
— a google_ads row never sets it). `adaccount.ConnectInput`/
`Connection`/`Credentials` all grew the same four fields;
`Service.Connect` branches on the traffic source's own `cost_integration`
to require the right subset (`accessToken` for facebook_ads/tiktok_ads,
the other four for google_ads) — the exact same "read cost_integration,
require its own real shape" pattern Phase A's `Connect` already used for
the two-provider case, now extended to three.

## Real encryption, not a repeat of `access_token`'s own plain-text gap

`access_token` (Phase A) was plain text — no encryption infrastructure
existed anywhere in this codebase at the time (that migration's own doc
comment says so explicitly). At the time this section was written it
still was, unchanged; retrofitting it was flagged as separate hardening
work, not this phase's job — Phase D, below, is that work. But `apps/internal/
totp` and `apps/internal/integrations` have since built real AES-256-GCM
encryption twice over, each with its own dedicated key — so this phase's
three new secret fields (`developer_token`, `client_secret`,
`refresh_token`) get the same real treatment via a new `apps/internal/
adaccount/crypto.go` and its own `ADACCOUNT_ENCRYPTION_KEY`, deliberately
separate from `TOTP_ENCRYPTION_KEY`/`INTEGRATION_ENCRYPTION_KEY` (a
compromise or rotation of one should never force rotating the others).
`client_id` stays plain text — it's a public OAuth identifier, not a
secret, the same convention OAuth 2.0 itself draws between a client's id
and its secret. Needed by both `apps/api` (Connect/the manual sync
endpoint) and `apps/worker` (the scheduled sync) — both parse the key at
startup with the same "fail loudly, never run with a missing or
well-known key" bar every other required secret in this codebase holds
itself to.

`TokenPreview` (the masked "•••• last4" the UI shows) can no longer be
computed purely in SQL the way Phase A's `right(access_token, 4)` did —
`refresh_token` is ciphertext in the database, and slicing ciphertext
would show four meaningless bytes, not four meaningful characters.
`Repository.scanConnection` became a method (needs the repository's own
key) that decrypts `refresh_token` in Go purely to compute its own
preview when `access_token` is absent, never returning the decrypted
value itself.

## `apps/internal/adaccount/googleads`: the real adapter

Structurally the same shape as `facebookads`/`tiktokads` (`Adapter`
struct with exported, test-overridable base-URL fields + `HTTPClient`,
a `New()` constructor, `var _ adaccount.CostProvider = (*Adapter)(nil)`)
but with two real HTTP hosts instead of one: `oauth2.googleapis.com`
(the token refresh) and `googleads.googleapis.com` (the actual Ads API,
REST `googleads.APIVersion` — v26 since BUG-10, one GAQL `googleAds:search` query per page). Unlike
`tiktokads`' own two-call shape (a separate advertiser-info call for
currency), Google Ads' GAQL lets `customer.currency_code` be selected
directly alongside `campaign`/`segments`/`metrics` fields in the same
query — one call per page, not two. `metrics.cost_micros` is Google
Ads' own money encoding (1,000,000 micros = 1 unit of the account's
currency), divided down in Go before becoming a
`DailyCampaignSpendRecord.Amount`, same "raw platform value in, no
pre-conversion" contract every adapter here already follows (currency
normalization to USD still only ever happens once, at the
`cost_entries` write, via `fx_rates`).

Two real gaps this phase's own code fixed along the way, found by
tracing the request path rather than assumed clean: `trafficsource.
CostIntegration.Valid()` (a completely separate whitelist from the
`traffic_sources.cost_integration` CHECK constraint, in a different
package) still only listed the original two providers — creating a
`google_ads` traffic source 422'd with "must be one of none, manual,
facebook_ads, tiktok_ads" even after the migration and the `adaccount`
package's own changes landed, caught live via curl, not by inspection.
`costsync.Providers`/`sourceByCostIntegration` also needed their own
third case each (`GoogleAds adaccount.CostProvider` field,
`"google_ads": cost.SourceGoogleAds`) — `costsync.Service.Sync` would
otherwise have silently found no provider for a google_ads source's
sync request.

## Verified

Backend: `gofmt`/`go build ./...`/`go vet ./...`/`go test ./...` all
green repo-wide, including new tests in `adaccount` (`google_ads is
allowed` alongside the existing tiktok_ads case, a dedicated test that
google_ads rejects a bare `accessToken` and a request missing just
`refreshToken`, and `TestGoogleAdsCredentialsRoundTripEncryption` —
connects real plaintext, reads the *raw* `refresh_token` column
directly via SQL and asserts it does **not** equal the plaintext,
`CredentialsByTrafficSourceID` decrypts it back to the exact original,
and `TokenPreview` shows the real last-4 of the refresh token) and a new
`googleads` package test suite (a real two-server harness — one fake
`oauth2.googleapis.com`, one fake `googleads.googleapis.com` — covering
the happy path incl. `costMicros` division and customer-id dash-
stripping, `nextPageToken` pagination, a failing token refresh, a real
Google Ads API error shape, and a malformed `costMicros` value), plus
`trafficsource.TestCostIntegrationValid` (a plain unit test, no database
needed) for the whitelist bug above.

Real, live, end-to-end verification against the actual running
containerized dev stack (`docker compose -f docker-compose.dev.yml up
api worker` after adding `ADACCOUNT_ENCRYPTION_KEY` to both services,
plus the same key added to the long-neglected `docker-compose.test.yml`
— which turned out to be missing `TOTP_ENCRYPTION_KEY`/
`INTEGRATION_ENCRYPTION_KEY` too, a pre-existing gap from those two
phases that would have crash-looped that compose file's own `api`
service on its very first boot attempt, fixed as part of this phase
since it was already touching this exact concern): created a real
`google_ads` traffic source, confirmed a validation-error round-trip
(missing Google Ads fields), connected real (if fake) credentials and
confirmed the response's `tokenPreview` matched the refresh token's
real last four characters, confirmed the raw `refresh_token`/
`client_secret`/`developer_token` columns in Postgres are genuinely
encrypted (not the plaintext) while `client_id` is genuinely plain, and
triggered "Sync now" — which made a **real live HTTP call to
`oauth2.googleapis.com`** and got back Google's own real error,
`"invalid_client: The OAuth client was not found."`, logged server-side
and surfaced as a normal (if generic) 500 to the caller, no panic, no
partial state. Real browser pass (Claude-in-Chrome) against the same
disposable test account used for the Integrations phase: the cost-
integration dropdown lists "Google Ads" as a fourth option, selecting it
before saving shows the correct badge/hint text (including a corrected,
no-longer-stale claim about there being no scheduled sync — Phase C's
own scheduler has existed since before this phase started, the copy
just never caught up), saving reveals the real five-field Google Ads
form (customer ID/developer token/OAuth client id/OAuth client
secret/refresh token, not the two-field Facebook/TikTok shape),
connecting renders the connected state with the real masked preview,
and "Sync now" fires the same real (failing, since the pasted
credentials are fake) request confirmed above with no console errors or
crash. Disconnect correctly returns the source to its pre-connection
state. All test fixtures (the traffic source, the test org used for the
curl pass) deleted afterward.

## NEXT

- A real, live Google Ads developer account + OAuth client would let
  this adapter be verified against the genuine Ads API end to end, the
  same gap Facebook/TikTok Ads still have.

## Phase D: `access_token` retrofitted to real encryption, closing the last plain-text gap

The one deliberate non-goal this doc's own `NEXT` section used to list.
`access_token` (facebook_ads/tiktok_ads) now goes through the exact same
`crypto.go`/`ADACCOUNT_ENCRYPTION_KEY` machinery Google Ads' own
developer_token/client_secret/refresh_token already used — no schema
change needed, since `ad_account_connections.access_token` was already
`text` and ciphertext is text too (no migration; the column's stored
*format* changed, not its type). `repository.go`'s `selectColumns`
dropped its `right(access_token, 4)` SQL-level slice (that only worked
against plaintext) — `scanConnection` now decrypts whichever secret
column is present (`access_token` for facebook_ads/tiktok_ads,
`refresh_token` for google_ads) in Go, purely to compute `TokenPreview`,
mirroring the pattern `refresh_token`'s own preview already used.
`Connect` encrypts `access_token` via the same `encryptOptional` helper
the other three fields use; `CredentialsByTrafficSourceID` decrypts it
back via `decryptOptional`. No API contract change — `Connection` never
had an `AccessToken` field to begin with (write-only from the start),
and the request shape a browser sends to `Connect` is unchanged; only
what Postgres stores changed.

Verified: `TestAccessTokenRoundTripEncryption` (mirrors
`TestGoogleAdsCredentialsRoundTripEncryption`) connects a real plaintext
access token, reads the raw `access_token` column directly via SQL and
asserts it does **not** equal the plaintext, confirms
`CredentialsByTrafficSourceID` decrypts it back to the exact original,
and confirms `TokenPreview` still shows the real last-4 characters. The
existing `TestConnectGetDisconnect` (asserting `TokenPreview` on a fresh
Connect) needed no changes — the preview's *value* is unaffected by
where in the stack it gets sliced from. `gofmt`/`go vet`/`go build`/
`go test -count=1 ./...` clean repo-wide against live Postgres/
ClickHouse/Redis/MinIO. No frontend change: `apps/web`'s
`ad-account-connections.ts` only ever sends `accessToken` on `Connect`
and never reads one back — the wire contract this package's own
`Connection` doc comment already documented as write-only stays exactly
that.

## Offline conversions (audit UNI-2 part 2)

A Google Ads connection can also report conversions back to Google:
docs/offline-conversions.md. It goes through the Data Manager API, not the
Ads API, and needs the refresh token minted with the
`https://www.googleapis.com/auth/datamanager` scope as well.

## Ads API version (audit BUG-10)

Until BUG-10 the cost adapter called Ads API `v17`. Google lists v17 among
its sunset versions, so the cost sync would have failed against any real
account. It now calls `googleads.APIVersion` = v26, the latest (released
14 October 2026). The release notes of v23 to v26 change nothing it reads:
the GAQL fields `campaign.id`, `customer.currency_code`,
`customer.time_zone`, `segments.date`, `metrics.cost_micros`, plus
`nextPageToken` and the error shape. Not checked against the live API.

Google ships a major version about every quarter and sunsets each one about
a year later. Move the constant on before then:
https://developers.google.com/google-ads/api/docs/sunset-dates. A version no
longer served answers 404 with an HTML page, and the sync error then names
the version it called.
