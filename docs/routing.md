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
↓ select eligible flow → apply weighted selection (pickWeighted)
↓ persist sticky assignment (if enabled)
↓ resolve destination (or fallback / safe destination if no set matched)
↓ return RouteResult
```

Routing is deterministic where configuration requires deterministic
behavior, and every decision must be explainable: why matched / why not /
why this flow / why fallback / sticky applied from where.

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
source of truth. The frontend Routing Simulator (Phase 10) and Filter
Builder previews are thin UI over `POST /routing/simulate`; during
frontend-first phases they run against a local mock of the identical
contract, switched to the real endpoint in Phase 27 with no UI changes.

## `internal/routing` (Phase 19 — landed)

`apps/api/internal/routing` is the engine. It is deliberately independent
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

## Conformance fixture (§6-SHARED, §58)

`apps/api/internal/routing/fixture_test.go` is the fixture — a table of
`(request context, configuration) → expected RouteResult`, run against the
real Go engine. There is no separate TypeScript implementation running the
same table: per Strategy A, the frontend Routing Simulator is a thin UI
over this engine's future HTTP wrapper (`/routing/simulate`, Phase 27), not
a second decision implementation to keep in sync.

| §58 case | Test | Notes |
|---|---|---|
| AND | `TestFilterEvaluation_AND` | |
| OR | `TestFilterEvaluation_OR` | |
| nested groups | `TestFilterEvaluation_NestedGroups` | OR wrapping a nested AND |
| priority (first-match wins) | `TestPriority_FirstMatchWins` | proves sort-by-priority, not declaration order, governs |
| fallback | `TestFallback_CampaignLevel`, `TestFallback_StreamSetLevelBeforeCampaign` | stream-set fallback wins over campaign fallback |
| weighted routing (±2% / 10k picks) | `TestWeightedRouting_DistributionWithin2PercentOver10kPicks` | real seeded PRNG via injectable `Engine.Rand01`, not a fixed value |
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

All 17 cases pass or are explicitly documented as out of this package's
scope (`t.Skip` with the reasoning inline, not silently absent). Verified
stable across repeated runs and under `go test -race`.
