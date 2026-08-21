# Event Mappings CRUD (§29, migration 00012)

The second of the three separable pieces "Conversions/Postbacks" turned
out to be (see `docs/conversions.md` for the first, the Conversions list
+ detail/timeline). Per-network translation from a network's own raw
postback status string (whatever they send, e.g. `"ftd"`, `"lead"`,
`"chargeback"`) to the canonical §43 CpaStatus. The `event_mappings`
table already existed (migration 00012) and already had a real reader —
`apps/internal/conversion.PostgresMapper.MapStatus`, which
`apps/internal/conversion.Service` calls at postback-ingest time on
`apps/tracker`'s hot path (Phase 23, `docs/conversion.md`). What was
missing was purely the write side: a CRUD API for the team-editable
config surface.

## `apps/internal/eventmapping` writes rows for the existing reader — never duplicates its lookup

Same relationship `apps/internal/streamset` has to
`apps/internal/routingstore`: this package never re-implements
`MapStatus`'s case-insensitive lookup logic, it only inserts/deletes the
rows that lookup already reads. `FloxStatus` reuses `event.Type` directly
(validated via the already-existing `event.Type.IsCPA()`) rather than
redeclaring a parallel enum — the same call every other domain package in
this codebase makes for an enum that already exists elsewhere
(`routing.FilterField`, `routing.Joiner`, ...).

## Shape: `GET/POST /event-mappings`, `DELETE /event-mappings/{id}` — org-wide, not per-network

The frontend panel shows one card per network with its own mappings
inline, matching the old mock's UI exactly — so `GET /event-mappings`
returns every mapping across the whole organization in one request
(`ORDER BY network_id, created_at`), and the panel groups them by
`networkId` client-side, same as the mock's `EVENT_MAPPINGS` array did.
No `PATCH`: the UI only ever adds or removes a mapping, never edits one
in place, so `Update` was never built — CLAUDE.md's "don't build
capabilities the UI doesn't use" discipline, mirroring why
`apps/internal/streamset`'s Reorder exists but no per-condition PATCH
does.

## Duplicate detection is the database's job, not a check-then-insert race

`event_mappings_network_status_idx` (00012's unique index on
`(network_id, lower(network_status))`) already exists — `Create` just
inserts and catches Postgres's `23505` (unique_violation) into a real
`apierror.Conflict`, the same `errors.As(err, &pgErr)` pattern
`network`/`offer`/`trafficsource` already use for their own constraint
violations (until now, always `23503` FK violations; this is the
codebase's first `23505` catch). A separate SELECT-then-INSERT would have
the same race a concurrent double-click could hit.

## `EventMappingPanel` also switched off the stale mock networks list

The panel already imported `CPA_STATUSES` from the real
`lib/api/conversions.ts` (from the Conversions phase), but was still
reading `useNetworksStore` — the pure mock store with fabricated ids like
`net_afftrust`, wired up before Networks CRUD landed and never updated.
Since the whole point of this phase is "manage mappings for your real
networks," leaving that stale would have meant the panel showed networks
whose ids couldn't possibly match anything `event_mappings.network_id`
could reference. Switched to the real `useNetworks()` hook.

## A real, now-visible inconsistency: `IncomingPostbacksPanel` still reads the mock store

`stores/event-mappings.ts` and `lib/mock/event-mappings.ts` are **not**
deleted, unlike the Routing Simulator phase's `stores/stream-sets.ts` —
they're still imported by two other, still-mocked corners of the
Postbacks page: `IncomingPostbacksPanel` (the "mapped count" badge per
network) and `lib/mock/postback-logs.ts` (the deferred Postback Logs
mock's `EVENT_MAPPINGS` cross-reference). Both are the deferred Postback
Logs domain, out of this phase's scope. The result: the "Event Mapping"
tab on `/postbacks` now manages real data, while the "Incoming" tab
(same page) still shows the old mock networks (AffTrust CPA, AdCombo,
MyLead, Direct advertiser) with a "N statuses mapped" badge sourced from
the mock store — a real, visible inconsistency until Postback Logs lands,
documented here rather than papered over. It does not crash; it simply
shows disconnected, stale data, the same class of inconsistency
`docs/stream-sets.md` documented for the Routing Simulator before that
phase landed.

## Verified

Backend: `go build/vet/gofmt/test ./...` all green — 5 new
`eventmapping` tests (create/list/delete round-trip, invalid FloxStatus
rejected, case-insensitive duplicate rejected with a real `conflict`
apierror, unknown network id rejected, full cross-tenant isolation
including create-against-another-org's-network and delete-as-a-
different-org).

Frontend: `tsc --noEmit`/`eslint` clean.

Full manual browser pass against the real running `api` + `web` dev
servers: created two real networks; added a mapping (`lead` →
`CPA_HOLD`) through the real form and confirmed it landed in Postgres via
a direct API call; attempted a case-insensitive duplicate (`LEAD`) and
confirmed the real `409` (network request inspected directly, not just
inferred from the UI) with the row count staying at one; removed the
mapping and confirmed it was gone both in the UI and via the API. The
`IncomingPostbacksPanel` inconsistency above was confirmed live (renders
correctly, just disconnected data) rather than assumed. Test networks
removed via the real `DELETE` endpoint afterward (their one event mapping
had already been removed through the UI, and would have cascaded
regardless — `event_mappings.network_id` is `ON DELETE CASCADE`).

## Deliberately deferred

Postback Logs (incoming/outgoing delivery log, ClickHouse
`postback_events`, replay-capable per §45) — the third and last piece of
this domain, scoped out when this phase's slice was negotiated. See
`docs/frontend-integration.md`.
