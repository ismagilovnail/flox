# Stream Sets, Filters & Flows CRUD (§21/§39, Phase 7-9)

Full write path for Stream Sets, their recursive AND/OR filter trees, and
their weighted Flows — the last piece of §84's core workflow ("Create
Campaign → Stream Set → Filters → Flow → Simulate Routing") that had no
real backend. Chosen after Networks & Offers landed, since both are Flow
destination dependencies.

## The read path already existed — this phase only adds the write path

`apps/internal/routingstore` + `apps/internal/routing` already load this
exact schema and evaluate real routing decisions on the tracker's hot
path (CLAUDE.md #1: routing decisions have exactly one implementation).
`apps/internal/streamset` never duplicates that logic — it only writes
rows for the existing reader to load, and reuses
`routing.FilterField`/`FilterOperator`/`Joiner`/`DestinationKind`/
`StreamSetStatus` directly rather than redefining the same enums twice.

## `FilterNode`: a flattened union, like `routing.Trace` already is

`routing/trace.go`'s own doc comment explains the pattern first:
"mirrors the frontend's ConditionTrace | GroupTrace union as a single
flattened struct... keeps this trivially JSON-serializable." `FilterNode`
follows the same shape for the same reason — one Go struct with
`omitempty` fields, discriminated by `Kind`, matching
`lib/filters.ts`'s `FilterCondition | FilterGroupNode` union on the wire
without needing a Go-side interface (which — unlike `routing.FilterNode`,
an interface with an unexported `evaluate` method — nothing outside
`internal/routing` could implement anyway).

### No `id` on the wire

`routing.FilterNode`/`Flow` don't carry ids (the engine doesn't need
them), so neither does this package's `FilterNode`. The frontend hydrates
fresh client-side ids when loading a tree for editing
(`hydrateFilterNode`/`hydrateRootFilter` in `lib/api/stream-sets.ts`) —
exactly what `filter-group-builder.tsx`'s id-addressed mutation helpers
(`addConditionToGroup`, `updateCondition`, `removeNode`, …) need — and
strips them back out before saving (`dehydrateFilterNode`). `ApiFlow`
keeps its `id` (unlike filter nodes) because `flows.id` is a real,
independently-addressable Postgres row, useful for the frontend's
`useFieldArray` keys; filter tree nodes are always replaced wholesale, so
they never needed persistent identity to begin with.

## Filter tree conditions and groups keep separate position sequences

`filter_conditions.position`/`filter_groups.position` (00006) are each
"order among sibling conditions/groups under the same parent" — two
independent sequences, not one interleaved order. `loadFilterTrees`'s
read path (already existed in `routingstore`, mirrored here for the
CRUD-facing shape) always appends a group's own conditions before its
nested sub-groups, regardless of how they were originally interleaved in
a `Children` array — since AND/OR evaluation is commutative, this never
affected routing correctness, and this phase's `insertFilterGroup`
matches that exact read-side reconstruction rather than inventing a
different write-side ordering that the reader would then re-order anyway.

## Server-side filter validation, not just the frontend's heuristic

`lib/filters.ts`'s own `checkRE2Compatible` comment says it plainly:
"a first pass only; real enforcement is compiling with RE2 at save time
on the backend." `Service.validateFilterNode` does exactly that — Go's
stdlib `regexp.Compile` on a `MATCHES` condition's value IS RE2 (CLAUDE.md
#8), so no separate heuristic check was needed server-side, just the real
compile. Country codes get the same treatment: `"UK"` rejected (GB is the
real ISO-3166 code), every other value checked against the 2-letter
alpha pattern — mirroring `validateCountryValue` server-side rather than
trusting whatever the client already checked.

## Offer destinations: network is derived, never trusted from the client

A Flow's `destination_network_id`/`destination_offer_id` are both
denormalized onto the row (the table's own CHECK constraint requires
both, or neither). `Service.resolveFlowNetworks` looks up the offer's
*own* `network_id` and uses that — ignoring whatever `networkId` the
client sent — since there is exactly one correct network for a given
offer and trusting a client-supplied pair risks a silent mismatch.
`TestOfferDestinationDerivesNetworkFromOffer` sends a deliberately wrong
network id alongside a real offer id and asserts the real one wins.

## Priority: never client-supplied, always append-then-reorder

`stream-set-schema.ts` has no `priority` field at all — `Create` always
appends a new stream set after every existing one for the campaign
(`len(existing) + 1`), matching `stores/stream-sets.ts`'s own
`addStreamSet` exactly. Reordering is a separate, explicit action: `POST
.../stream-sets/reorder` takes the full `orderedIds` array a drag-end
event produces and rewrites `priority = index+1` for all of them in one
transaction — not N individual per-row PATCHes.

## Landing/PWA/Postlanding stages: restored (Flow CRUD follow-on phase)

Originally dropped for the reason below; **un-dropped** in a later phase
once `internal/landing`, `internal/pwa`, and `internal/postlanding` all
existed for real. `flows.landing_id`/`landing_as_pwa`/`pwa_id`/`pwa_type`/
`postlanding_id` (migration 00006, previously unused by any CRUD) are now
read/written by `streamset.Flow`'s `Landing`/`Pwa`/`Postlanding` fields —
each an always-present struct (`{enabled, ...}`) rather than a pointer, so
toggling a stage off in the UI keeps its previous pick around instead of
losing it, matching the columns' own independent `*_enabled` vs. nullable
`*_id` split.

`Service.checkFlowStagesBelongToOrg` confirms every non-empty landing/pwa/
postlanding id belongs to the caller's org — same "never trust a
client-supplied foreign id" reasoning as `resolveFlowNetworks` for
offer/network ids (CLAUDE.md #5) — checked whenever an id is present, not
only when its stage is enabled, since a disabled stage's id is still
persisted. `pwa_type` is nullable with a CHECK constraint that only NULL
(never `""`) satisfies for an unset stage — `nullIfEmpty` in
`repository.go` converts the empty-string wire value accordingly before
every insert.

Frontend: `flow-funnel.tsx` (new) renders the full chain — Landing → PWA →
Postlanding → the Offer-or-Redirect Destination (delegated to
`flow-destination-editor.tsx`, unchanged) → Fallback — reusing the
existing `FlowNode` component (`toggleable`/`configured`/`previewUrl`
props were already generic; nothing there needed to change) and fetching
real `useLandings()`/`usePwas()`/`usePostlandings()` alongside the
existing `useNetworks()`/`useOffers()`. `stream-set-schema.ts` validates
each stage the same way the destination union already did:
`landing.enabled && !landingId` → inline error on the picker, same for
`pwa`'s id and type, same for `postlanding`'s id.

Still out: **per-flow Pixels**. `stream_set_pixels` (migration 00008)
attaches a pixel to the *Stream Set*, not the Flow — CLAUDE.md's own
phrasing ("per-flow Pixels") is imprecise; the schema has always scoped
pixels one level up. No `internal/pixel` package exists yet either way,
so this stays a separate, still-blocked phase regardless of naming.

## A real render-loop bug, caught and fixed during manual verification

The offer picker inside the new stream set form would select correctly
for an instant, then silently reset to empty. Root cause: React Hook
Form's `useFieldArray().update()` is documented (by RHF itself) to
unregister and re-register the field row on every call — which remounts
that row's whole subtree, including its Select components, on every
keystroke or selection. Mid-remount, the Radix `<Select>` being torn down
fired a stray `onValueChange("")` that raced the real selection and won.

Confirmed live: a mount/unmount effect log showed
`FlowDestinationEditor` mounting and unmounting around *every* field
change, and the offer Select's `onValueChange` firing three times per
click — once with the real offer id, twice more with `""` immediately
after. Fixed by switching every per-flow field edit from
`flowArray.update(index, {...flow, ...patch})` to
`setValue(`flows.${index}`, {...flow, ...patch})` — `setValue` patches
the field in place with no remount. A secondary, defensive fix also
landed: the offer `<Select>`'s controlled `value` had briefly been an
empty string (`destination.offerId || undefined`, later just
`destination.offerId`) — Radix's own docs warn against an empty
`SelectItem` value for the same class of reason, so a `NO_OFFER` sentinel
now stands in for "nothing chosen yet" on the wire instead of handing
Radix `""` directly, even with the remount bug fixed.

## Frontend: a fourth mock/store pair left untouched, for a documented reason (superseded)

`lib/mock/stream-sets.ts`/`stores/stream-sets.ts` stayed in place when
this phase closed — not because other still-mocked features imported
them (they didn't, apart from one), but because **the Routing Simulator
tab on the campaign detail page read the same mock store** and was
explicitly out of scope this phase. The Stream Sets card on a campaign's
Overview tab showed real data while the Simulator tab still simulated
against old mock-generated stream sets — a real, visible inconsistency,
documented here rather than papered over.

This is now history: once the Routing Simulator phase switched
`routing-simulator-view.tsx` off the mock store, both files became
genuinely unimported anywhere in the app and were deleted outright — see
[`docs/routing-simulate.md`](routing-simulate.md).

## Verified

Backend: `go build/vet/gofmt/test ./...` all green, including 6 new
`streamset` tests (CRUD round-trip with a nested filter tree, rejecting
empty flows/a condition-as-root/an invalid country code/an invalid RE2
pattern/an incomplete BETWEEN, offer-destination network derivation,
reorder rewriting priority, duplicate keeping status and copying the
tree+flows with fresh ids, full cross-tenant isolation).

Frontend: `tsc --noEmit`/`eslint` clean. Full manual browser pass against
the real running `api`+`web` dev servers: created a network, offer, and
campaign; built a stream set through the complete form (name, one
country-IS-US filter condition, one flow targeting the real offer);
verified the created row's filter chip and flow tag rendered correctly
and the underlying API response had the exact filter tree and resolved
network id; edited it and confirmed every field — including the
hydrated, still-editable filter tree and the offer selection — pre-filled
from the real record; duplicated it (copy correctly kept the filter tree,
flow, and `active` status); toggled status to `paused` (confirmed live);
reordered two stream sets via the real `POST .../reorder` endpoint and
confirmed the UI reflected the new priorities on reload. Test campaign
(cascading to its stream sets), offer, and network removed via the real
`DELETE` endpoints afterward.

## Deliberately deferred (landed since)

Wiring the Routing Simulator to `/routing/simulate` was scoped out of
this phase deliberately, to keep it reviewable — not because it was
hard, since `routingstore.LoadRoutingConfig` and `routing.Router.Explain`
already did essentially all the work. It landed in its own phase; see
[`docs/routing-simulate.md`](routing-simulate.md).

## Verified (Flow funnel stages follow-on phase)

Backend: `go build/vet/gofmt/test ./...` all green, including 2 new
`streamset` tests (`TestFlowFunnelStagesRoundTrip`: all three stages
enabled with real seeded landing/pwa/postlanding ids, `AsPwa`/`PwaType`
set, round-trips identically through Create then a fresh Get;
`TestFlowFunnelStageValidation`: a stage enabled without its id, an
invalid `pwaType`, and a disabled stage referencing another org's
postlanding id are all rejected).

Frontend: `tsc --noEmit`/`eslint`/`vitest run` (21 tests, unchanged —
this phase added no new frontend unit tests, only backend integration
tests, since the new logic is almost entirely wiring existing generic
components to new fields) /`next build` (production) all clean.

Full manual browser pass against the real running `api`+`web` dev
servers: created a throwaway landing, PWA, and postlanding; opened the
pre-existing `i18n Test Set` stream set's edit form (a fixture from an
earlier phase) and enabled all three funnel stages on its one flow —
selected the real landing (+ "Show as PWA"), the real PWA (type
defaulted to "Internal" the instant the stage was toggled on, confirmed
switching it to "External"), and the real postlanding; saved; reloaded
the page fresh and reopened the edit form — every stage's `Configured`
badge, selected entity, and `asPwa`/`pwaType` value round-tripped exactly
through the real Postgres-backed API, not just an optimistic client
cache. Reverted the fixture's flow back to its original all-disabled
state directly in Postgres afterward (not deletable through the UI, no
hard-delete for stream sets' nested flow fields) and deleted the three
throwaway landing/pwa/postlanding rows via `DELETE FROM ...` once their
FK references were cleared, restoring the shared `i18n Test Set` fixture
to exactly its pre-phase state for future sessions.
