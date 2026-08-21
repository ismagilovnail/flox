# ARCHITECTURE

High-level architecture summary. Full technical detail lives in
[`docs/architecture.md`](docs/architecture.md) and the referenced docs below;
this file is the stable entry point.

## Four layers

```
CONTROL PLANE       campaigns, flows, filters, offers, sources, domains, costs
      ↓
TRACKING ENGINE      incoming request → classification → routing → redirect
      ↓
EVENT / ATTRIBUTION  click → event → conversion → postback → attribution
      ↓
ANALYTICS ENGINE     metrics, dimensions, reports, dashboards, LTV/cohorts
```

## System style: modular monolith

One Go module. Logical modules (auth, organization, campaign, routing,
classifier, tracking, attribution, conversion, postback, pixel, landing, pwa,
postlanding, offer, network, domain, analytics, reports, ltv, push, cost,
audit, integration) live under `internal/` and are NOT split into
microservices up front.

`apps/tracker` and `apps/worker` are separate **binaries** inside the same
module so the hot click/redirect path can be deployed and scaled
independently — but they import the same `internal/routing` and
`internal/classifier` packages as `apps/api`. No routing or classification
logic is duplicated between binaries.

Incoming postbacks (`GET/POST /postback/{networkId}`, §45) are served by
`apps/tracker` too, via `internal/conversion` — a network calling FLOX is
external traffic like a click, just not on the same p50 < 20ms budget.
Outgoing postbacks (FLOX notifying a network, Phase 24) are `apps/worker`'s
job.

## Repository layout

```
apps/
  go.mod     ONE Go module for api + tracker + worker (root moved here in Phase 21)
  internal/  shared Go packages — routing, classifier, event, config, …
  web/       Next.js frontend — the only non-Go app
  api/       Go control-plane API (+ migrations/)
  tracker/   Go hot-path click/redirect service
  worker/    Go async event/postback processor
packages/
  ui/ config/ types/
docs/        architecture, event-model, routing, ltv, metrics, etc.
infra/       docker-compose, deployment
```

The Go module root sits at `apps/` rather than `apps/api/` precisely so the
three binaries can share `apps/internal/...`: Go's internal-import rule
only permits importing `.../internal/x` from code rooted at that
directory's parent, so a module rooted at `apps/api` could never have let
`apps/tracker` import the routing engine.

Frontend internal structure: `app/ components/ features/ hooks/ lib/ stores/
types/ schemas/`. Domain code lives in `features/`. All server calls go
through `src/lib/api`.

Go structure per module: `handler → service → repository → domain`. Handlers
stay thin; business logic never lives in handlers or in React components.

## Data architecture

- **PostgreSQL** — control plane: users, organizations, campaigns, stream
  sets, filters, flows, sources, networks, offers, landings, pwas,
  postlandings, domains, tracking links, pixels, postbacks, cost_entries,
  fx_rates, integrations, api_keys, audit_logs.
- **ClickHouse** — high-volume events: click_events, tracking_events,
  conversion_events, cost_events, postback_events (the real §48 five-table
  design, landed Phase 26), plus `ltv_events` (a materialized view over
  conversion_events driving the FTD/Reg cohort engine, §26.5, landed Phase
  26.5) and further analytical aggregates. Sort keys lead with
  `organization_id`, partitioned by date. `cost_events` stays schema-only
  even after Phase 27-COST: at manual-entry volume, campaign-detail spend
  is answered directly from Postgres `cost_entries` (a `GROUP BY
  entry_date`, no cross-database join needed) — the ClickHouse sync this
  table was reserved for becomes worth building once FB/TikTok ad-spend
  import (§74) actually produces ClickHouse-scale volume, not before. No
  TTL yet on any table (no retention policy exists in this project's
  docs). See `docs/analytics-pipeline.md`, `docs/ltv-cohorts.md`,
  `docs/cost-ingestion.md`.
- **Redis** — cache, rate limits, short-lived sessions, job coordination,
  postback dedup keys, and sticky-assignment **cache only** (never source of
  truth — see below).
- **S3-compatible object storage** — assets, uploads, content gallery items.

## Shared domain logic — decision (§6-SHARED)

**Strategy A**, chosen in Phase 0:

> The Go core (`internal/routing`) is the single source of truth for
> routing/filter/sticky/metric decisions. The Routing Simulator (frontend
> Phase 10) is a thin UI over `POST /campaigns/{campaignId}/routing/
> simulate` (`apps/internal/routingsimulate`) — real as of this phase. During
> frontend-first phases (2–15) it ran against a local mock that implemented
> the exact same request/response contract; that mock (`lib/routing-
> simulate.ts`) is deleted, not kept running alongside the real endpoint —
> see [`docs/routing-simulate.md`](docs/routing-simulate.md). There is no
> second (TypeScript) implementation of routing/filter/sticky logic.

Both sides are validated against one shared conformance fixture (a table of
inputs → expected route decisions), documented in
[`docs/routing.md`](docs/routing.md) and implemented as Go tests once the
routing engine (Phase 19) exists.

## Non-negotiable invariants

Summarized here; full detail and rationale in `CLAUDE.md`:

1. Single source of truth for routing (this document, above).
2. Full event model — ~20 event types, CPA statuses as an enum, never
   collapsed (see [`docs/event-model.md`](docs/event-model.md)).
3. Postback dedup key = `(click_id, status, event_ref)`, not `click_id`
   alone and not `(click_id, status)` alone — `event_ref` is the network's
   transaction id, used only for `CPA_REDEP` (see
   [`internal/conversion`](apps/internal/conversion), Phase 23/§45).
4. Sticky = cookie is truth, Redis is cache only.
5. Tenant isolation — every tenant-scoped table has `organization_id`,
   enforced in the repository layer.
6. Cost or it doesn't exist — no cost for a slice shows ROI as "—".
7. Currency normalized at event time, not query time.
8. Regex = RE2 only (Go stdlib `regexp`), validated at save time.
9. Redirect hot path: p50 < 20ms / p95 < 50ms, excl. third-party latency.
10. WebView bounce (PWA install mechanics) is provider-neutral and required;
    vendor-specific moderator cloaking is forbidden.
11. External providers (geo, ASN, bot, cost, conversion, pixel, registrar,
    DNS, FX) sit behind interfaces — no vendor lock-in.
12. Custom metrics build on the registry safely (division-by-zero → empty,
    never an error).

## Package management

No JS package manager lockfile exists yet. This machine has npm 11.x but not
pnpm/yarn. Default choice: **npm workspaces** for the `apps/web` +
`packages/*` monorepo, set up when `apps/web` is scaffolded (Phase 2). This
is a reversible choice, not a stack change — revisit if it becomes a
bottleneck.
