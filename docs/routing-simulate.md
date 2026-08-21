# Wiring the Routing Simulator to `/routing/simulate` (§6-SHARED Strategy A, final slice)

The last piece of the promise `ARCHITECTURE.md`'s §6-SHARED decision made
back in Phase 0: the frontend Routing Simulator "runs against a local mock
that implements the exact same request/response contract... switched to
the real endpoint once a source/offer/stream-set/flow backend exists to
simulate against." Networks, Offers, and Stream Sets/Filters/Flows CRUD
all landed earlier this session; this phase is the thin HTTP wrapper and
the frontend's switch off the mock — nothing about the routing *decision*
changed, since `routingstore.LoadRoutingConfig` + `routing.Router.Explain`
already did all the real work (Phase 19/26).

## What's new

- **`apps/internal/routingsimulate`** (new package) — `Service.Simulate`
  loads a campaign's real routing config via `routingstore`, calls
  `routing.Engine.Explain` (never `Resolve`: the simulator's whole point
  is the trace `Resolve` alone doesn't return), and reshapes the result
  into the frontend's wire contract. `Handler` mounts `POST
  /campaigns/{campaignId}/routing/simulate`, tenant-scoped like every
  other handler.
- **`routing.Explanation` gained one field**: `DestinationLabel` — the
  same human label (`"Offer"`, `"Redirect"`, `"Stream Set fallback"`,
  `"Campaign fallback"`, `"No destination configured"`)
  `resolveDestination` already computed internally for `RouteResult
  .Reason`, now also surfaced structurally. `trySticky` and
  `freshEvaluate` both already called `resolveDestination` and had the
  label sitting in a local variable — this just stores it instead of
  discarding it, so the simulate response's `destination.label` can never
  disagree with what the engine actually decided.
- **`routing.Trace`/`StreamSetEvaluation`/`FlowCandidate` gained JSON
  tags** matching the frontend's field names exactly (`streamSetId`,
  `flowId`, `normalizedPercent`, …) — reused directly in the simulate
  response with no separate DTO layer, exactly as `trace.go`'s own doc
  comment (written in an earlier phase) said they were for: "keeps this
  trivially JSON-serializable for the future /routing/simulate response
  with no extra marshaling code."
- **Frontend**: `lib/routing-simulate.ts` (the mock — pure-function
  filter evaluation, FNV-1a visit hashing, weighted pick, the works) is
  deleted outright, not kept running alongside the real thing. Its type
  definitions moved into `lib/api/routing.ts`, which already existed as
  the one pre-built seam for this exact moment (its own doc comment: "a
  plain synchronous function call... can't keep that promise [of a later
  swap]... wrapping it as a promise-returning call today... means Phase
  27 only ever changes this file's body" — true). `routing-simulator-
  view.tsx` drops its `useStreamSetsStore` read (the server now loads
  the campaign's stream sets itself) and calls `simulateRoute(campaignId,
  request)` instead of the old `(streamSets, fallbackUrl, request)`
  signature. `simulator-form.tsx` and `stream-set-trace.tsx` needed only
  an import-path change — both were already fully pure and prop-driven.
  `simulator-result.tsx` needed two small edits: `result.selectedFlow`
  (a boolean check only, never its fields) became
  `result.flowCandidates.some(c => c.selected)`, and the `kind`-keyed
  `DESTINATION_LABEL` lookup was dropped in favor of rendering
  `result.destination.label` directly — the backend already sends the
  exact display string, so keeping a parallel frontend enum→label map
  would only be a second place that string could drift from.

## `stores/stream-sets.ts` and `lib/mock/stream-sets.ts`: deleted, not just left alone

`docs/stream-sets.md` explicitly kept these in place because the Routing
Simulator was their one remaining reader. That stopped being true the
moment `routing-simulator-view.tsx` switched to the real API — a repo-wide
grep for `useStreamSetsStore` / `@/stores/stream-sets` /
`@/lib/mock/stream-sets` after the switch found zero remaining importers,
so both files are now genuinely dead code, not files someone might still
need. Deleted, matching this session's standing "drop it, don't fake it"
precedent rather than leaving an orphaned mock/store pair to bit-rot.

## Sticky is never simulated

`Service.Simulate` always passes `Sticky: nil` to `Explain` — the
simulator has no real `sf_{campaignId}` cookie to honor, and fabricating
one would produce a result indistinguishable from a real returning
visitor's, which is actively misleading for a debugging tool. Instead the
response's `stickyNote` is generated from the campaign's real
`sticky_flow` flag: if enabled, the note says a returning visitor's
cookie would override this pick; if not, it says every visit is
evaluated fresh. (The old mock's sticky note was a static, increasingly
stale string written before sticky assignment existed at all — see
`docs/stream-sets.md`'s own note about this being a known inconsistency.)

## The visit key can make an all-empty simulate request a real 422

`deriveVisitKey` is reimplemented in Go, byte-for-byte matching the old
frontend mock's algorithm (sorted non-empty `field=value` pairs joined by
`|`) — continuity across the mock-to-real switch, though not load-bearing
for correctness now that the server is the only place a real pick
happens. If the derived key is empty **and** the matched stream set has
more than one eligible weighted flow, `pickWeighted` returns
`ErrNoVisitKey` (CLAUDE.md invariant, `docs/routing.md`'s "missing visit
key" case) — deliberately *not* papered over with a fake key, because
doing so would give the simulator a false sense of determinism real
traffic doesn't have. `Service.Simulate` surfaces this as a
`422 validation` error with a message telling the operator to fill in at
least one attribute. In practice this rarely fires through the actual UI:
`emptySimulateRequest()` defaults `bot`/`proxy` to `"0"`, which are
non-empty values, so the derived key is virtually never truly empty
unless a stream set's *only* eligible flows tie on every other signal too.

## Two real bugs caught during manual browser verification

Both are the same underlying mistake, hit twice in the same session,
because Go's `encoding/json` `omitempty` doesn't check nil-ness for
slices — it omits **any** zero-length slice, `nil` or not:

1. **`routingsimulate.Response.FlowCandidates`** — when no stream set
   matches, `routing.Explanation.FlowCandidates` is a genuine, untouched
   `nil` (no weighted draw ever ran), which encodes as JSON `null`.
   `SimulatorResult`'s `result.flowCandidates.some(...)` call is
   unconditional and crashed on it. Fixed by normalizing `nil` to
   `[]routing.FlowCandidate{}` in `Service.Simulate` before building the
   response. `TestSimulateNoMatchFallsBackToCampaignFallback` now asserts
   `FlowCandidates` is non-nil.

2. **`routing.Trace.Children` and `streamset.FilterNode.Children`** —
   both had `omitempty` on the `Children` tag. An empty top-level filter
   group ("no filters — matches all traffic", a normal, UI-documented
   configuration per `stream-set-form-sheet.tsx`'s own help text)
   produces a real, **non-nil**, zero-length `Children` on both types —
   `make([]T, 0)` in the engine's `FilterGroup.evaluate` and the
   repository's tree-building `build()` closures. `omitempty` hid it from
   the wire anyway, because it keys off `len() == 0`, not nil-ness. The
   frontend's `hydrateFilterNode` and `FilterTraceView` both call
   `.map()`/`.length` on `children` unconditionally, so this crashed not
   just the Simulator but the Stream Sets card on the whole campaign
   detail page for any campaign with an empty-filter stream set — a
   config the UI actively encourages ("An empty top-level group matches
   all traffic"). Fixed by dropping `omitempty` from `Children` on both
   types, and — for the repository's read path specifically — making
   sure the `build()` closures start `Children` at `[]FilterNode{}`
   rather than relying on `append` to a nil slice happening to stay
   non-nil (it does, but a stray future refactor could easily reintroduce
   nil there without a test noticing). Both types' create/update paths
   echo the client's own JSON-decoded input straight back
   (`s.RootFilter = in.RootFilter`), which is unaffected — JSON `[]`
   always decodes to a non-nil empty Go slice, so only the *read* path
   (List/Get, rebuilt from Postgres rows) was ever actually broken.
   `TestEmptyRootGroupChildrenIsNeverNilOnTheWire` (streamset) and
   `TestEmptyGroupTraceChildrenEncodesAsEmptyArrayNotNull` (routing) both
   guard this now.

Neither bug was reachable through this session's earlier manual
verification passes because no prior phase had exercised an *empty*
root filter group end-to-end through a real HTTP round-trip — every
Stream Sets test fixture up to this point included at least one
condition.

## Verified

Backend: `go build/vet/gofmt/test ./...` all green, including 5 new
`routingsimulate` tests (match + weighted pick, no-match campaign
fallback with the `FlowCandidates` non-nil regression assertion,
ambiguous-tie 422, sticky-note reflecting the campaign's real
`sticky_flow` flag, cross-tenant isolation) and 2 new regression tests in
`streamset`/`routing` for the `omitempty` bug. The full pre-existing
`routing` conformance fixture (18 cases now) still passes unchanged.

Frontend: `tsc --noEmit`/`eslint` clean. Full manual browser pass against
the real running `api`+`web` dev servers: created a network, offer, and
campaign; built a stream set with a `country IS US` filter and one offer
flow — simulated a matching request (correct stream set/flow/destination
rendered) and a non-matching one (correct fallback, no crash); edited the
stream set to an empty root filter with two 50/50-weighted redirect
flows — simulated with `bot=0,proxy=0` (the form's defaults) and got a
deterministic weighted pick; enabled `sticky_flow` on the campaign
directly in Postgres and confirmed the sticky note text updated live.
Both real bugs above were caught and fixed during this same pass, not
found separately. Test campaign (cascading to its stream set), offer,
and network removed via the real `DELETE` endpoints afterward.

## What's left on `docs/frontend-integration.md`'s deferred list

Routing Simulator is off the "still mocked" list now. Remaining:
conversions/postbacks list-management pages, the `/analytics` report
builder, and `/ltv-cohorts` — see that doc for current status.
