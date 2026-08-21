# Postback Logs (§29, §45/§46) — wired to a real backend, replay deferred

The third and last of the three separable pieces "Conversions/Postbacks"
turned out to be (see `docs/conversions.md` and `docs/event-mappings.md`
for the first two). Inspection found the "Outgoing" tab was *already*
real — it just reuses `NetworkList`, since a network's `postbackUrl`/
`acceptDuplicates` fields (§27, extended §45) are the outgoing postback
config, no second table needed. The "Incoming" tab needed only a small
fix (see below). The genuinely new work was the "Logs" tab: a real,
ClickHouse-backed read layer over `postback_events` (§48), mirroring
`apps/internal/analytics`/`apps/internal/conversions`.

## Scope: read-only this phase, "Replay" deliberately deferred

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

## Domain complete

Conversions (list + detail/timeline), Event Mappings (CRUD), and Postback
Logs (read-only, replay deferred) are all real now — the
"Conversions/Postbacks" domain identified at the start of this
three-phase arc is fully wired except for the deliberately-deferred
Replay action. See `docs/frontend-integration.md` for the current
overall status.
