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

## Conformance fixture

Both the Go engine and any frontend preview must pass the same fixture: a
table of `(request context, configuration) → expected RouteResult`. This
fixture is authored alongside the routing engine (Phase 19) and must cover
at minimum the cases in master spec §58:

```
AND / OR / nested filter groups
priority (first-match wins)
fallback
weighted routing (distribution within 2% of configured weights over 10k picks)
sticky routing (cookie survives Redis flush)
sticky keepClickId
sticky skipInactive true/false
inactive campaigns / flows / offers
missing destinations
invalid tracking links
ISO code mismatch (UK vs GB)
in-app WebView bounce
```

Not yet implemented — this file will gain the actual fixture table once
Phase 19 lands.
