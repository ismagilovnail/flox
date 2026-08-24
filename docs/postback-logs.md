# Postback Logs (§29, §45/§46) — wired to a real backend; outgoing and incoming replay both shipped

The third and last of the three separable pieces "Conversions/Postbacks"
turned out to be (see `docs/conversions.md` and `docs/event-mappings.md`
for the first two). Inspection found the "Outgoing" tab was *already*
real — it just reuses `NetworkList`, since a network's `postbackUrl`/
`acceptDuplicates` fields (§27, extended §45) are the outgoing postback
config, no second table needed. The "Incoming" tab needed only a small
fix (see below). The genuinely new work was the "Logs" tab: a real,
ClickHouse-backed read layer over `postback_events` (§48), mirroring
`apps/internal/analytics`/`apps/internal/conversions`.

## Original phase's scope: read-only, "Replay" deliberately deferred

A second `AskUserQuestion` narrowed scope once inspection revealed the
old mock's "Replay" button was a genuine *write* action, not just a UI
affordance to fake or drop: replaying an **incoming** row can be
correctly implemented as re-submitting `(networkID, clickID, rawStatus,
revenue, currency)` through the same `apps/internal/conversion.Service
.Record` path a real network retry would hit (dedup/status-progression
rules apply normally — a re-submit of an already-`success` row correctly
comes back `duplicate`, not a special case). Replaying an **outgoing**
row can be correctly implemented as looking up the matching Postgres
`postbacks` row (by org + click_id + status[+ event_ref]) to get its id,
then re-enqueuing a fresh delivery through the existing
`apps/internal/postback.Store.Enqueue` path. Both are real, buildable,
no-new-schema actions — but a second capability that mutates real state
(re-triggers conversion recording, re-enqueues a live delivery) deserved
its own reviewable phase rather than riding along with a pure read view.
Dropped from this phase's UI entirely (`postback-log-columns.tsx`'s
`actions` column and its `RotateCcwIcon` replay button), not left
disabled or faked.

## `apps/internal/postbacklogs` (plural) — not to be confused with `apps/internal/postbacklog` (singular)

`apps/internal/postbacklog` already existed as Phase 24's write-side
queue/producer — the Postgres-backed `FOR UPDATE SKIP LOCKED` queue that
carries both directions' attempt outcomes into ClickHouse. This new
package is purely a browser-facing read layer over the table that
producer already fills; the naming mirrors the `conversion`/`conversions`
(singular write engine / plural read layer) split from the Conversions
phase exactly.

## One list, both directions mixed — matching the frontend's single table

`GET /postback-logs?from=&to=&limit=&offset=` returns incoming and
outgoing attempts together, newest first, discriminated by `direction` —
the frontend never queried them separately, so the backend doesn't
either. New `chstore` read methods (`ListPostbackAttempts`,
`CountPostbackAttempts`) are the first reads against `postback_events`;
until now the table was write-only (`InsertPostbackAttempts`, fed by
`apps/internal/postbacklog`'s flusher).

Same date-only-`to`-parses-to-midnight fix `apps/internal/conversions`
needed is applied here too (pushed to end-of-day in the handler) —
copy-pasted correctly this time since the bug and its fix were already
known from the Conversions phase, not rediscovered.

## The "Incoming" tab's stale-mock inconsistency, documented in the Event Mappings phase, is now resolved

`docs/event-mappings.md` documented that `IncomingPostbacksPanel` still
read `stores/networks.ts`/`stores/event-mappings.ts` (both mocks) even
after Event Mappings CRUD landed real, because fixing it was out of that
phase's scope. This phase closes that gap: `IncomingPostbacksPanel`
switched to the real `useNetworks()`/`useEventMappings()` hooks (both
already existed from earlier phases — no new frontend API needed for
this fix, just wiring).

## Four mock/store files deleted — all four consumers had already moved to real hooks

Once `PostbackLogsPanel`/`postback-log-columns.tsx` moved to the real
`usePostbackLogs()` hook and `IncomingPostbacksPanel` moved to the real
network/mapping hooks, a repo-wide grep found `stores/postback-logs.ts`,
`lib/mock/postback-logs.ts`, `stores/networks.ts`, and
`lib/mock/networks.ts` had zero remaining importers (each only imported
the next one in the chain, all now dead). Deleted outright, the same
"drop it, don't fake it" precedent as the Routing Simulator phase's
`stream-sets` mock/store pair.

## Verified

Backend: `go build/vet/gofmt/test ./...` all green — new `chstore`
integration tests for `ListPostbackAttempts`/`CountPostbackAttempts`
against real ClickHouse, `postbacklogs.Service` unit tests against a fake
repository (range/limit validation, pagination/ordering).

Frontend: `tsc --noEmit`/`eslint` clean.

Full manual browser pass against the real running `api` + `web` dev
servers: created a real network and a real event mapping (`ftd` →
`CPA_ACCEPT`); seeded four real `postback_events` rows (one full success
pair — incoming accepted + outgoing delivered — one incoming error with
no mapping configured, one outgoing retrying) via a throwaway,
never-committed `chstore.EventStore.InsertPostbackAttempts` call.
Confirmed: the Logs tab renders all four with correct direction icons,
the raw→mapped status display for incoming rows, mapped-status-only
display for outgoing rows, correct result badges across the wider real
vocabulary (retrying/error/success), and no Replay column; the Incoming
tab now shows the real network with its real id in the copyable URL and
a correct "1 status mapped" badge sourced from the real event mapping,
not a stale mock one. Test network and its `postback_events` rows
removed afterward.

## Outgoing Replay (follow-on phase)

Inspection for this follow-on found incoming and outgoing replay are very
different sizes, once you look past "both are real, buildable, no-new-
schema actions" (still true) to what each actually touches:

- **Outgoing** re-enqueues a fresh delivery through the exact
  `apps/internal/postback.Store.Enqueue` path a first attempt already
  takes. Self-contained in `apps/api`, which already has everything it
  needs (`db`) — no new dependency graph.
- **Incoming** re-invokes `apps/internal/conversion.Service.Record`,
  which needs mapping, the dedup/progression store, FX, attribution
  (ClickHouse click matching), an async event writer, and the outgoing-
  delivery enqueuer all wired together — the exact dependency graph only
  `apps/tracker` (the hot-path binary) has ever needed. `apps/api` (the
  control-plane binary the frontend talks to) has none of it. Wiring it
  in means either duplicating that whole graph into `apps/api`, or
  `apps/api` making an internal HTTP call to `apps/tracker`'s existing
  `/postback/{networkId}` endpoint — a real architecture decision, not a
  small one.

An `AskUserQuestion` scoped this phase to **outgoing only**; incoming
replay is deferred again, this time pending that architecture choice.

### The one new lookup: resolving a ClickHouse row back to its Postgres id

`apps/internal/postback`'s delivery queue has a `NOT NULL` FK
(`source_postback_id → postbacks.id`, migration 00014) — a first delivery
attempt already knows this id in-process (`conversion.Service.Record` has
just written it), but a replay, happening long after and starting from a
ClickHouse `postback_events` row that has no such id, does not.
`conversion.PostgresStore.FindSuccessID` resolves it by the exact same
key the dedup index uses: `(organization_id, network_id, click_id,
status, event_ref)`. `event_ref` matters here specifically —
`postback_events` previously dropped it from the browser-facing
`PostbackLog` JSON ("the UI never renders it"); it's now kept, because a
CPA_REDEP click can have more than one successful row, one per
redeposit, and matching on the wrong one would replay the wrong
delivery.

`postback.ReplayEnqueuer` adapts `postbacklogs.ReplayInput` →
`postback.EnqueueInput`, the same decoupled-interface-per-consumer
pattern `postback.Enqueuer` (→ `conversion.DeliveryEnqueuer`) already
uses — `postbacklogs` never imports `postback`'s own delivery-lifecycle
types. Unlike `Enqueuer.Enqueue` (best-effort, called from inside
`Record` where a queuing failure must never be reported back to the
network as a conversion failure), `ReplayEnqueuer.Enqueue`'s error goes
straight back to the HTTP caller — replaying IS the entire point of that
request.

`POST /postback-logs/replay-outgoing` takes the exact fields a
`PostbackLog` row already carries client-side (`networkId`, `clickId`,
`status`, `eventRef`, `url`) — no second fetch, and the replay reuses the
exact URL that was already macro-resolved and dispatched, not a
re-resolution against current network config.

### Verified

Backend: `go build/vet/gofmt/test ./...` all green — a new
`conversion.PostgresStore.FindSuccessID` integration test against real
Postgres (event_ref disambiguation between two REDEP rows, tenant
isolation, no-match), a new `postback.ReplayEnqueuer` unit test, and five
new `postbacklogs.Service.ReplayOutgoing` unit tests against fakes
(happy path, event_ref disambiguation, not-found, required-field
validation).

Frontend: `tsc --noEmit`/`eslint`/`next build` (production) all clean.

Full manual browser pass against the real running `api` + `tracker` +
`worker` + `web` dev servers — the first phase in this arc to need all
four, since a genuine outgoing delivery only exists once a real incoming
postback has been recorded and enqueued one: created a real network
whose postback URL points at an unreachable local port (so delivery
attempts fail fast and predictably), a real event mapping, then sent two
real incoming postbacks through `apps/tracker`'s actual `/postback/
{networkId}` endpoint. Confirmed `apps/worker`'s `Deliverer` picked up
both, failed against the unreachable URL, and logged `retrying`
attempts. Clicked Replay in the Logs tab; confirmed the toast, and
confirmed directly in Postgres that a **new** `postback_deliveries` row
was created (fresh id, `attempt_count` reset to 1, not a mutation of the
row being replayed) with `source_postback_id` correctly resolved to the
original success row's id — and that the worker picked the new row up on
its own next poll. Confirmed the Replay button is absent on incoming
rows. Confirmed error paths directly: malformed org header → 422,
missing required field → 422 with field detail, a status that was never
successfully recorded (no matching source row) → 404. Test network, its
Postgres rows (`networks`/`event_mappings`/`postbacks`/
`postback_deliveries`), and its ClickHouse `postback_events` rows all
removed afterward.

## Incoming Postback Replay (second follow-on phase)

The architecture decision the outgoing-replay phase deferred: `apps/api`
now builds the exact same `apps/internal/conversion.Service` dependency
graph `apps/tracker/main.go` already builds for a real network hit
(mapper, dedup/progression store — with the same Redis-cache-in-front,
best-effort-at-startup stance tracker uses — FX, ClickHouse-backed
attribution, async event writer, outgoing-delivery enqueuer, attempt
logger), rather than making an internal HTTP call to `apps/tracker`. An
`AskUserQuestion` decided this after inspection found the real
alternative — `apps/api` calling `apps/tracker`'s existing
`/postback/{networkId}` — has no operator/admin distinction from a real
network hit (no auth model, same public unauthenticated path), so a
replay would be indistinguishable from a forged network request in the
audit trail. Building the dependency graph directly instead is pure Go,
reuses the exact same `db`/`ch` connections `apps/api` already opens (no
new external infra), and matches the existing modular-monolith pattern
(`apps/tracker`/`apps/worker` already share `internal/routing`,
`internal/classifier`) — the actual conversion-recording logic stays one
shared package; only the wiring is duplicated between the two `main.go`
files, same as `internal/routing`/`internal/classifier` already are.
Gated behind the same `ch != nil` startup check every other
ClickHouse-backed handler on this server already uses, since
`postbacklogs.Repository` (the read side) needs it too — unlike
`apps/tracker`, which must start even with ClickHouse down, this whole
feature already only runs when ClickHouse connected successfully at
startup, so no `attribution.NewMemoryResolver` fallback is needed here.

### `postbacklogs` still never imports `conversion` — the adapter lives with the engine

Same decoupled-interface-per-consumer pattern `apps/internal/postback`'s
`ReplayEnqueuer` already established for outgoing replay:
`postbacklogs.IncomingNetworkLookup`/`IncomingRecorder` are this
package's own interfaces, built from primitive/local types only, never
importing `conversion`. `apps/internal/conversion/replay.go` adds
`ReplayNetworkLookup`/`ReplayRecorder`, two adapters satisfying those
interfaces — `conversion` imports `postbacklogs` (one-directionally, no
cycle, `postback` already does the same thing for outgoing), not the
other way around.

`ReplayRecorder.Record` calls the exact same `*conversion.Service.Record`
`apps/tracker`'s `PostbackHandler` calls for a real hit — dedup and
status-progression rules apply identically, so a re-submit of an
already-successful attempt correctly comes back `duplicate`, and a
successful replay triggers the same event emission and outgoing-delivery
enqueue a first success would (confirmed manually below: replaying an
error into success sent a real, retrying outgoing delivery attempt; a
second replay of the identical attempt correctly returned `duplicate`
with no second event or delivery).

### The tenant check ReplayIncoming needs that a real network hit doesn't

`apps/tracker`'s `/postback/{networkId}` has no separate tenant identity
to check the resolved network against — the URL's `{networkId}` **is**
the tenant scope for an unauthenticated public endpoint (CLAUDE.md #5).
`ReplayIncoming` arrives already inside an authenticated tenant session
(`tenant.Middleware`), so `postbacklogs.Service.ReplayIncoming` looks up
the network and explicitly compares its `OrganizationID` against the
caller's own, returning the same not-found response either way — a
caller must never be able to confirm another org's network id exists
merely by attempting a replay against it. Covered by a dedicated test
(`TestReplayIncomingNotFoundWhenNetworkBelongsToAnotherOrg`), the
explicit tenant-isolation check this phase's DoD calls for.

### `EventRef` doubles as the replay's `NetworkTxnID`

`conversion.eventRefFor` derives `event_ref` FROM the network's
transaction id, but only for `CPA_REDEP` (§45 — every other status's
`event_ref` is always empty, even when a txn id was sent). A `PostbackLog`
row's own `eventRef` is exactly that derived value, so passing it back in
as `NetworkTxnID` on replay reproduces the identical dedup key for every
status, redeposits included, without this package needing to know or
store the original raw txn id.

### `POST /postback-logs/replay-incoming`

Takes the exact fields a `PostbackLog` row already carries client-side
(`networkId`, `clickId`, `rawStatus`, `eventRef`, `revenue`, `currency`)
— no second fetch, matching `replay-outgoing`'s own shape. Returns the
outcome (`result`/`status`/`message`) so the UI can show what actually
happened, since — unlike outgoing replay's always-the-same "queued"
message — an incoming replay's result genuinely varies
(success/duplicate/ignored/error).

### Verified

Backend: `go build/vet/gofmt/test ./...` all green — new
`postbacklogs.Service.ReplayIncoming` unit tests against fakes (happy
path incl. exact field mapping, network-not-found, cross-tenant
not-found, required-field validation, not-configured), and new
`conversion` package tests for both adapters
(`ReplayNetworkLookup`'s field/error mapping,
`ReplayRecorder` running an actual replay-then-duplicate-replay sequence
through a real `*conversion.Service` built the same way every other
`conversion` test builds one).

Frontend: `tsc --noEmit`/`eslint`/`vitest run`/`next build` (production)
all clean. New `replayIncomingPostback` API function + `hooks/use-
postback-logs.ts`'s `useReplayIncomingPostback`; `PostbackReplayButton`
now branches on `direction` instead of only rendering for outgoing rows,
surfacing the incoming replay's actual result kind in the toast
description rather than a fixed message.

Full manual browser pass against the real running `api` + `tracker` +
`worker` + `web` dev servers: created a real network, sent a real
incoming postback through `apps/tracker`'s actual endpoint with no event
mapping configured (produced a genuine `error` row — "no event mapping
configured"), added the mapping, clicked Replay on that exact row in the
Logs tab. Confirmed directly in ClickHouse: the replay recorded a new
`success` row (`CPA_ACCEPT`, correctly unattributed — no real click ever
existed for this synthetic test), which correctly triggered a real
outgoing delivery attempt (`retrying`, since the test network's postback
URL is an unreachable domain — `apps/worker`'s `Deliverer` picked it up
and retried on its own poll cadence). Replayed the identical attempt a
second time and confirmed it came back `duplicate` with no second event
emitted and no second delivery triggered — the money-correctness
guarantee (CLAUDE.md #3) this whole feature exists to preserve under a
manual re-trigger, not just a network's own retry. Confirmed the original
`error` row was never mutated — replay only ever inserts new rows.
Test network, its Postgres rows (`networks`/`event_mappings`/
`postbacks`/`postback_deliveries`), and its ClickHouse `postback_events`
rows all removed afterward.

## Domain complete

Conversions (list + detail/timeline), Event Mappings (CRUD), Postback
Logs (read/write), and both outgoing and incoming Postback Replay are all
real now — the "Conversions/Postbacks" domain identified at the start of
this originally-three-phase arc is fully wired. See
`docs/frontend-integration.md` for the current overall status.
