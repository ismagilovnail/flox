# Ad Account Connections (§74/§27-COST) — Phase A: credential storage

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
