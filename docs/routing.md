# Routing

## Model

```
Campaign
    ↓
Stream Set (ordered by priority, evaluated top-down, FIRST match wins)
    ↓
Filter group (AND / OR, nested)
    ↓
Flow group (weighted, pickWeighted)
    ↓
Destination
```

No stream set matches → campaign fallback / safe destination.

## Evaluation order (conceptual, §39)

```
request
↓ resolve tracking link → resolve campaign → load configuration (versioned)
↓ in-app WebView check → bounce to external browser if needed (PWA install mechanics)
↓ sticky lookup (if enabled) → restore prior flow (+ click_id if configured)
↓ classify request
↓ bot / proxy filtering
↓ evaluate Stream Sets top-down by priority
↓ evaluate filters (AND/OR, nested)
↓ select eligible flow → apply weighted selection (pickWeighted, deterministic
  by visit key — see "Weighted selection" below)
↓ persist sticky assignment (if enabled)
↓ resolve destination (or fallback / safe destination if no set matched)
↓ return RouteResult
```

Routing is deterministic **always**, not only when sticky is enabled (§38 —
see "Weighted selection" below), and every decision must be explainable: why
matched / why not / why this flow / why fallback / sticky applied from where.

## Sticky assignment semantics (§39-STICKY — critical)

```
SOURCE OF TRUTH: client cookie sf_{campaignId} = "setId:flowId[:clickId]".
  Survives Redis eviction, restarts, cross-session returns.

REDIS: cache/acceleration ONLY, never the authority. Cold Redis still yields
  the correct sticky flow via the cookie.

CONFIG FLAGS:
  stickyFlow             enable/disable
  stickyFlowKeepClickId  reuse the original click_id on return (attribution)
  stickyFlowSkipInactive if true, keep the saved flow even if now inactive;
                         if false, drop the cookie and re-pick
```

Redis-only sticky is forbidden — it silently corrupts A/B test data on
eviction with no visible error.

## WebView bounce vs. moderator cloaking (§73 — keep separate)

- **Allowed and required**: bouncing in-app WebView traffic (FB/IG/TikTok/
  Telegram browsers) to the external browser so the PWA install prompt can
  fire. Provider-neutral technical necessity.
- **Forbidden**: vendor-specific ad-network moderator/reviewer detection
  ("cloaking"). Classification stays provider-agnostic; no platform-specific
  behavior is hard-coded into routing.

## Shared-domain-logic strategy (§6-SHARED)

Decision recorded in [`/ARCHITECTURE.md`](../ARCHITECTURE.md): **Strategy
A** — the Go routing engine (`internal/routing`, Phase 19) is the single
source of truth. The frontend Routing Simulator (Phase 10) is a thin UI
over `POST /campaigns/{campaignId}/routing/simulate`
(`apps/internal/routingsimulate`, landed the same phase as this doc's
last update — see [`docs/routing-simulate.md`](routing-simulate.md));
during frontend-first phases it ran against a local mock of the
identical contract, since replaced entirely, not kept running alongside
the real endpoint.

## `internal/routing` (Phase 19 — landed)

`apps/internal/routing` is the engine. It is deliberately independent
of `net/http` and of any database driver (§38): `Resolve` is a pure
function of `RequestContext` (already-classified request attributes +
already-loaded, versioned campaign configuration) → `RouteResult`.
Tracking-link resolution, campaign lookup, traffic classification (Phase
20), and the in-app WebView bounce all happen in the caller (`apps/tracker`,
Phase 21), before `Resolve` is ever invoked.

```go
type Router interface {
    Resolve(ctx context.Context, req RequestContext) (RouteResult, error)
}
```

`RouteResult` is §38's exact spec'd shape (`CampaignID`, `StreamSetID`,
`FlowID`, `Destination`, `Reason`, `StickyApplied`, `ConfigVersion`) — the
production hot-path return value. Deeper explainability (§72: "why did this
match, why not that one, why this flow, why fallback, sticky from where")
comes from a second method on the concrete `*Engine`, `Explain`, which runs
the identical evaluation and additionally returns an `Explanation`
(`StreamSetEvaluations`, `FlowCandidates`, `StickyNote`) — the shape the
frontend Routing Simulator already renders. `Resolve` and `Explain` share
one internal evaluation function, so they can never disagree.

`stickyFlowKeepClickId` is the one §39-STICKY flag `RoutingConfig`
deliberately does not carry: it only affects whether the caller reuses an
old `click_id` for attribution, which has no effect on which flow gets
selected — nothing for the routing decision itself to consume.

## Weighted selection (§38)

The flow draw is **deterministic**, not random. `pickWeighted(flows, visitKey)`
takes `RequestContext.VisitKey` and maps it through an unseeded FNV-1a/64 hash
onto the eligible flows' weight ranges.

Why a hash and not an RNG — the draw has to hold two properties at once, and a
random roll only gives the first:

- over many visits the observed shares must match the configured weights, or an
  A/B test between two landers measures the tracker rather than the landers;
- one visit must always resolve to the same flow, however many times it is
  replayed. A retried request that lands elsewhere shows the visitor a second
  page and leaves the conversion attributable to two variants at once.

A uniform hash gives the first; being a pure function gives the second. It is
FNV-1a specifically and **not** `hash/maphash`, which is seeded randomly per
process: two tracker replicas behind one load balancer would disagree about the
same visit, and every restart would re-bucket every visitor.

This is independent of sticky (§39-STICKY). The cookie remains the source of
truth for a returning visitor and short-circuits before the draw; the hash
covers what happens before a cookie exists and if it is lost.

**Who builds the key.** The caller, because "the same visit" is an HTTP-layer
question `internal/routing` is deliberately blind to:

| Caller | Key |
|---|---|
| `apps/tracker` | `campaignID \| clientIP \| userAgent` — the triple that stays constant across a retry, prefetch or duplicate delivery. The campaign id is included so one visitor is bucketed independently per campaign. |
| Routing Simulator (temporary mock) | the filled-in request fields, sorted. Changing a field re-rolls the pick; re-running an unchanged request does not. |

The shared contract is only *same key + same weights → same flow*; deriving the
key is each caller's own business.

**Eligibility is decided before the draw.** Only flows that are active *and*
carry a positive weight take part, and shares are relative to the sum of those
weights rather than to 100. Pausing one arm of a split therefore hands its
traffic to the others immediately, with no need to re-balance the remaining
weights first.

**A missing key is refused, not guessed.** With more than one eligible flow and
an empty `VisitKey`, `pickWeighted` returns `ErrNoVisitKey`. Hashing the empty
string would send every affected visit to whichever single bucket that one hash
value falls in — 100% of traffic to one arm while every dashboard still showed
the configured split. A 502 on the first request is found in minutes; a
corrupted experiment is found after the campaign is over. A lone eligible flow
is not a draw and needs no key.

## Conformance fixture (§6-SHARED, §58)

`apps/internal/routing/fixture_test.go` is the fixture — a table of
`(request context, configuration) → expected RouteResult`, run against the
real Go engine. There is no separate TypeScript implementation running the
same table: per Strategy A, the frontend Routing Simulator is a thin UI
over this engine's HTTP wrapper (`POST
/campaigns/{campaignId}/routing/simulate`), not a second decision
implementation to keep in sync.

| §58 case | Test | Notes |
|---|---|---|
| AND | `TestFilterEvaluation_AND` | |
| OR | `TestFilterEvaluation_OR` | |
| nested groups | `TestFilterEvaluation_NestedGroups` | OR wrapping a nested AND |
| priority (first-match wins) | `TestPriority_FirstMatchWins` | proves sort-by-priority, not declaration order, governs |
| fallback | `TestFallback_CampaignLevel`, `TestFallback_StreamSetLevelBeforeCampaign` | stream-set fallback wins over campaign fallback |
| weighted routing (±2% / 10k picks) | `TestWeightedRouting_DistributionWithin2PercentOver10kPicks` | 10k **distinct** visit keys, not 10k repeats of one — the pick is deterministic, so the distribution property only exists across the key space |
| weighted routing (determinism) | `TestWeightedRouting_DeterministicAcrossCallsInstancesAndRestarts`, `TestVisitHash_StableAcrossBuilds` | a fresh `Engine{}` per call stands in for both another replica and a restart, since the engine holds no state and no seed; the hash vectors are pinned against an independent FNV-1a/64 implementation |
| weighted routing (eligibility before the draw) | `TestWeightedRouting_EligibilityDecidedBeforeTheDraw`, `TestWeightedRouting_AllFlowsIneligibleFallsBack` | only active, positive-weight flows enter the draw; an ineligible flow never absorbs share |
| weighted routing (missing visit key) | `TestWeightedRouting_MissingVisitKeyIsRefusedNotGuessed`, `TestWeightedRouting_SingleEligibleFlowNeedsNoVisitKey` | refused with `ErrNoVisitKey` when a real split is at stake; a lone candidate is not a draw and needs no key |
| sticky routing (cookie survives Redis flush) | `TestSticky_SurvivesAcrossCallsWithNoHiddenState` | this package has no Redis/cache dependency at all — a sticky decision is a pure function of `(req.Sticky, req.Config)` every call, so there's nothing "eviction" could invalidate on this side |
| sticky keepClickId | `TestSticky_KeepClickId_NotThisPackagesConcern` (skipped, documented) | see above — not this package's concern |
| sticky skipInactive true/false | `TestSticky_SkipInactive` | both branches |
| inactive flows | `TestInactiveFlow_ExcludedFromSelection` | |
| inactive offers | `TestInactiveOffer_TreatedAsMissingDestination` | the frontend mock doesn't check this; the Go engine is deliberately stricter here — see `Destination.OfferActive` |
| inactive campaigns | `TestInactiveCampaigns_CallerLevelConcern` (skipped, documented) | `RoutingConfig` carries no campaign-level status; the caller only loads config for campaigns it already confirmed are active |
| missing destinations | `TestMissingDestination_CascadesThroughFallbacks` | cascades redirect/offer → stream-set fallback → campaign fallback → empty (never an error) |
| invalid tracking links | `TestInvalidTrackingLinks_CallerLevelConcern` (skipped, documented) | never reaches this package — the caller doesn't invoke `Resolve` if the tracking link doesn't resolve |
| ISO code mismatch (UK vs GB) | `TestISOCodeMismatch_NoFuzzyCoercion` | proves no semantic country-code aliasing — a `UK` value must not match a `GB` filter; this engine surfaces a classifier bug rather than papering over it |
| in-app WebView bounce | `TestInAppWebViewBounce_CallerLevelConcern` (skipped, documented) | a pre-routing HTTP redirect based on User-Agent, entirely `apps/tracker`'s job |
| empty-group trace JSON shape | `TestEmptyGroupTraceChildrenEncodesAsEmptyArrayNotNull` | regression: `Trace.Children` must never encode as JSON `null` for an empty group — see [`docs/routing-simulate.md`](routing-simulate.md) |

All 18 cases pass or are explicitly documented as out of this package's
scope (`t.Skip` with the reasoning inline, not silently absent). Verified
stable across repeated runs and under `go test -race`.
