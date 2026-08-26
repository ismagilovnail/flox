# CLAUDE.md — FLOX

> This file is read at the start of every session. Keep it lean.
> The full spec is `FLOX-master-prompt-v3.md` — read it before implementing any
> phase. This file is the operating manual + progress tracker, not the spec.

---

## PROJECT

**FLOX** — production-grade SaaS traffic tracking & routing platform.
Tagline: **Track. Route. Optimize.**

Full requirements: **`FLOX-master-prompt-v3.md`** (authoritative; sections
referenced below as §N).

---

## CURRENT STATE — UPDATE THIS EVERY PHASE

```
CURRENT PHASE : PHASE 29 — OBSERVABILITY
STATUS        : done — structured logging and OpenTelemetry tracing
                already existed (Phase 16+); this phase's real gap was
                Prometheus metrics, closed via a new apps/internal/metrics
                package (prometheus/client_golang, confirmed via
                AskUserQuestion over the OTel metrics SDK — simpler, no
                collector hop needed just to export as Prometheus
                anyway). All nine §53 tracked metrics now exist:
                tracking_requests/tracking_latency (apps/tracker's
                track() handler, outcome-labeled: redirected/blocked/
                not_found/error), routing_latency (internal/routing.
                Engine.Resolve, timed wherever it's called — hot path AND
                /routing/simulate, each in their own process), event_
                processing_latency (eventqueue.Flusher's ClickHouse
                InsertBatch call), event_queue_depth (new eventqueue.
                PollDepth goroutine in apps/worker, 15s ticker, a new
                PostgresQueue.Depth() counting 'queued'+'processing' so
                it doesn't dip to zero mid-flush), event_loss/postback_
                success/postback_failure/analytics_latency — see below
                and docs/observability.md for exactly how each maps to
                real metric names.
                event_loss is DELIBERATELY TWO COUNTERS, NOT ONE:
                enqueued (apps/tracker, from eventbuf.Writer.Stats() —
                exposed via new metrics.RegisterEventBufStats as
                CounterFuncs reading eventbuf's OWN existing atomics,
                zero changes to eventbuf's internals) vs persisted
                (apps/worker, from eventqueue.Flusher) — the two sides
                live in DIFFERENT BINARIES, so a dashboard derives loss
                via PromQL rate-subtraction; pre-computing and storing a
                single "loss" value would need one process to know both
                sides' counts, which is backwards from how Prometheus
                counters are supposed to work. Confirmed by reading
                eventbuf.Writer.write() directly: a failed sink write is
                never retried, so dropped/failed really are permanent
                loss, while eventqueue.Flusher's failed ClickHouse insert
                requeues forever (no dead-letter) — NOT loss, tracked
                separately as events_requeued_total.
                postback_deliveries_total{outcome} has THREE outcome
                values (success/retrying/dead), not §53's literal two —
                "retrying" (will be attempted again) vs "dead" (exhausted
                MaxAttempts, genuinely lost) already exists as postback.
                DeliveryStatus and collapsing it at the metric layer
                would throw away information a dashboard can always
                re-derive (sum outcome!="success") but never recover.
                apps/tracker gained middleware.RequestID + an X-Request-Id
                echo (NOT the full per-request logging middleware apps/
                api uses — a context value + one header write, negligible
                next to a synchronous slog call per click, which Phase
                21's own comment on this router already ruled out as
                "duplicate work" on the §41 hot path; this phase respects
                that same p50<20ms/p95<50ms budget by construction, every
                new hot-path metric call is an in-memory atomic, nothing
                that blocks). apps/worker's postback delivery http.Client
                now wraps otelhttp.NewTransport — each delivery attempt
                becomes its own trace (root span, no inbound request to
                inherit context from). Deeper custom spans elsewhere
                (routing decisions, conversion recording) are an
                explicit, separate follow-up, not done here — apps/api
                already had otelhttp.NewHandler covering every inbound
                request before this phase.
                /metrics added to all three binaries (apps/worker's bare
                http.HandlerFunc became a small http.ServeMux since a
                HandlerFunc alone serves only one path).
                New prometheus service in infra/docker-compose.dev.yml +
                infra/prometheus.yml (confirmed via AskUserQuestion, for
                real-scrape verification over curl-only) — scrapes all
                three binaries via host.docker.internal (they run on the
                HOST via `go run`, matching this whole repo's dev
                workflow; extra_hosts:host-gateway makes that hostname
                resolve on Linux Docker Engine).
                Verified: gofmt/go build/vet/test ./... green repo-wide,
                incl. 2 new test files (internal/metrics: /metrics serves
                all 9 metric families, RegisterEventBufStats against a
                real eventbuf.Writer; internal/eventqueue: new Depth()
                test covering enqueue/claim/delete transitions). Full
                manual pass: started all three binaries + a REAL
                Prometheus container, confirmed all 3 scrape targets
                "up" via /api/v1/targets (the real integration risk in
                this phase, worked first try). Seeded one real org/
                campaign/domain/tracking-link fixture, hit it 3x + one
                unknown slug: tracking_requests_total{outcome=
                "redirected"}=3, {outcome="not_found"}=1 confirmed via
                Prometheus's OWN QUERY API (not just curl), each real
                redirect carrying a genuine X-Request-Id header. Same 3
                clicks traced end to end: tracker's events_enqueued_total
                =3 -> worker's events_persisted_total=3 (as 3 separately
                job-labeled series in ONE Prometheus query, proving the
                two-counters-two-binaries design actually works, not
                just compiles), event_queue_depth back to 0 once drained.
                Inserted one real postback_deliveries row pointing at an
                intentionally unreachable URL — worker's Deliverer
                attempted it for real (connection refused), postback_
                deliveries_total{outcome="retrying"}=1 matched the row's
                own delivery_status in Postgres exactly. Signed up a real
                user (Phase 28's real auth) and called a real analytics
                endpoint: analytics_query_latency_seconds_count{endpoint=
                "campaign_daily"}=1. Zero unexpected errors in any
                binary's logs throughout. All fixtures deleted afterward,
                confirmed zero count on every LIKE 'Metrics%' pattern.
LAST COMMIT   : feat(observability): Prometheus metrics, tracker request
                IDs, traced postback delivery (Phase 29)
NEXT          : confirm scope before starting. No open known issues
                remain for Observability. Known limitations documented in
                docs/observability.md: no custom tracing spans beyond
                apps/api's existing inbound-request instrumentation +
                this phase's new postback-delivery outbound span (an
                explicit, separate follow-up); apps/tracker still has no
                per-request logging or inbound otelhttp wrapping (§41
                latency budget, deliberate, unchanged since Phase 21); no
                Grafana dashboard (Prometheus's own query UI only, per
                AskUserQuestion scope) — none requested.

```

> At the end of every phase: update the four lines above, add a CHANGELOG entry,
> commit, then STOP and wait. Do not roll into the next phase automatically.

---

## THE ONE RULE

**Work strictly one phase at a time.** Never build ahead. Never generate large
code before validating the current phase. Every phase follows:

```
Inspect → Plan → Implement → Run → Test → Fix → Review → Document → STOP
```

Only advance when the current phase meets its Definition of Done (§79).
Phase order is fixed — see §9. Do not reorder or skip.

---

## PHASE PROTOCOL (per §2)

Open each phase with:

```
PHASE X — NAME — GOAL
What will be built
Files created/changed
Dependencies
Acceptance criteria
```

Close each phase with:

```
COMPLETED / TESTS / KNOWN ISSUES / FILES CHANGED / NEXT PHASE
```

Run typecheck, lint, unit tests, and build where applicable before reporting
complete. Never skip validation. Do not ask permission to continue unless a real
architectural blocker exists — but never make silent destructive changes.

---

## STACK (do not swap without asking)

**Frontend:** Next.js · React · TypeScript · Tailwind · shadcn/ui · Radix ·
TanStack Query/Table · React Hook Form · Zod · Zustand · Lucide · date-fns ·
Apache ECharts.

**Backend:** Go · chi · pgx · sqlc (where sensible) · goose migrations ·
slog/zerolog · OpenTelemetry.

**Data:** PostgreSQL (control plane) · ClickHouse (high-volume events + analytics)
· Redis (cache, rate limits, sticky CACHE only, postback dedup) · S3-compatible
object storage.

**Architecture:** modular monolith. `apps/tracker` and `apps/worker` are separate
binaries inside the same Go module, sharing `internal/routing` and
`internal/classifier`. No microservices up front. No unnecessary frameworks.

---

## REPO LAYOUT

```
apps/
  web/       Next.js frontend (the only non-Go app)
  api/       Go control-plane API
  tracker/   Go hot-path click/redirect service
  worker/    Go async event/postback processor
packages/
  ui/ config/ types/
docs/        architecture, routing, event-model, ltv, metrics, etc. (§76)
infra/       docker-compose, deployment
FLOX-master-prompt-v3.md   ← full spec
CLAUDE.md                  ← this file
```

Frontend internal structure: `app/ components/ features/ hooks/ lib/ stores/
types/ schemas/`. Domain code lives in `features/`. All server calls go through
`src/lib/api` — never scatter `fetch()` in components (§32, §70).

Go structure per module: `handler → service → repository → domain`. Handlers are
thin; business logic never lives in handlers or in React components (§34, §71).

---

## NON-NEGOTIABLE INVARIANTS

Read the referenced section before touching related code.

1. **Single source of truth for routing (§6-SHARED, §26).** Routing/filter/sticky
   /metric decision logic exists ONCE (in Go). The routing simulator is a thin UI
   over the `/routing/simulate` contract — NOT a second TS implementation. Both
   sides pass one shared conformance fixture.

2. **Full event model (§43).** ~20 event types incl. CPA statuses
   HOLD/ACCEPT/REDEP/DECLINE/TRASH as an enum. Design the ClickHouse schema for
   all of them from day one — adding types later is a live-data migration. Never
   collapse conversions into one "conversion" type.

3. **Postback dedup key = (click_id, status, event_ref)** (§45) — NOT click_id
   alone, and NOT (click_id, status). `event_ref` = network txn id for CPA_REDEP
   (the only repeatable status), empty string for every other status even if a
   txn id was sent. **Status never goes back to CPA_HOLD** — nightly partner
   replays re-send it after approval and would take revenue out of a closed
   report (§45 STATUS PROGRESSION). Long Redis TTL + durable DB unique
   constraint. Store original currency + USD value at event time.
   `acceptDuplicates` override per network — it does not bypass the
   progression rule.

4. **Sticky = cookie is truth, Redis is cache only (§39-STICKY).** Cookie
   `sf_{campaignId}` = `setId:flowId[:clickId]`. Redis-only sticky is forbidden
   (silently corrupts A/B tests).

5. **Tenant isolation (§36-TENANCY).** Every tenant-scoped table has
   `organization_id`; every query filters on it; enforcement is in the repository
   layer. `organization_id` comes from the session/API key, never the request
   body. ClickHouse sort keys lead with it. Cross-tenant isolation test is part of
   DoD for API/data phases.

6. **Cost or it doesn't exist (§27-COST).** Profit/ROI/ROAS need spend. Manual
   cost entry first; FB/TikTok import later. No cost for a slice → show ROI as
   "—", never compute against zero.

7. **Currency at event time (§50-FX).** Normalize to USD using the rate on the
   event date, never the current rate. Store both original and USD.

8. **Regex = RE2 only (§5, §22).** Go stdlib `regexp`. Validate user patterns at
   save time, never on the hot path. No PCRE libs.

9. **Redirect hot path stays fast (§41, §56).** tracking p50 < 20ms / p95 < 50ms
   (excl. third-party latency). Persist events async (buffered batch → queue).
   Never run analytics queries or block on outbound partner calls in the redirect.

10. **WebView bounce ≠ moderator cloaking (§73).** Bouncing in-app WebView
    (FB/IG/TikTok/Telegram) to the external browser so the PWA install prompt can
    fire is a REQUIRED, provider-neutral capability. Vendor-specific moderator
    detection is FORBIDDEN. Keep them separate.

11. **Providers behind interfaces (§74–75).** Geo, ASN, bot, cost, conversion,
    pixel, registrar, DNS, FX — all swappable. No vendor lock-in in core logic.

12. **Custom metrics build on the registry, safely (§30.5, §50).** User formulas
    reference registry metrics by stable id; division-by-zero yields empty, never
    an error; one formula = one data source (no mixing push with regular); LTV not
    allowed in formulas; team-private.

---

## EVENT MODEL QUICK REF (authoritative list in §43)

```
SOURCE_CLICK, SOURCE_FILTER
LAND_VIEW, LAND_CLICK, POSTLANDING_VIEW, POSTLANDING_CLICK
PWA_VIEW, PWA_OPEN, PWA_INSTALL, IOS_INSTALL
NOTIFICATION_REQUEST/SUBSCRIBE/DECLINE/UNSUBSCRIBE/CLICK
TG_JOIN, TG_START
CPA_HOLD, CPA_ACCEPT, CPA_REDEP, CPA_DECLINE, CPA_TRASH
```

Canonical funnel: `SOURCE_CLICK → LAND_VIEW → LAND_CLICK → PWA_VIEW →
PWA_INSTALL → CPA_HOLD → CPA_ACCEPT → CPA_REDEP`. Chain linked by one click_id.

---

## ROUTING QUICK REF (full detail §21, §39)

```
Campaign → Stream Set (priority, first match wins, AND/OR filters, pixels)
        → Flow (weighted, pickWeighted) → destination
No set matches → campaign fallback / safe destination.
```

`pickWeighted` is **deterministic by visit key** (§38): an unseeded FNV-1a hash
of a stable property of the visit, never an RNG. Same visit → same flow on every
replica and after every restart. Independent of sticky — the cookie is still the
truth for a returning visitor (#4); the hash covers what happens before a cookie
exists. Candidates are filtered *before* the draw, so weights mean what the
operator typed.

Decisions must be explainable: why matched / why not / why this flow / why
fallback / sticky applied from where (§72).

---

## BUILD ORDER (frontend-first, §9 / §83)

Design & UI on mock contracts first (phases 2–15), then Go backend & engines
(16+), then integration (27), then LTV (26.5), auth/RBAC (28), hardening.
"Frontend-first" means UI/UX first on mock CONTRACTS — it does NOT license a
parallel routing engine in TS.

New in v3 (secondary, after core): Tags (14.5), Custom Metrics (14.6), Report
Presets + directory stats (14.7), Referral (14.8), Content Gallery (14.9).

Core workflows to prioritize (§84): Create Campaign → Stream Set → Filters →
Flow → Simulate Routing → Analytics → Track Conversion (HOLD→ACCEPT→REDEP) →
Postback → LTV.

---

## UX FLOOR (every screen, §63–68)

loading / empty / error / success states on everything. Destructive actions
confirm. Forms validate with inline errors + disabled submit + success feedback.
Tables: sort, paginate, column visibility/resize, search, filter, export;
virtualize large sets. Dark-first, dense, tabular numerals, meaningful status
colors — a trading-terminal control plane, not a generic admin dashboard.
WCAG 2.2 AA. Animation sparing (150–250ms ease-out).

---

## DEFINITION OF DONE (§79)

```
✓ implemented        ✓ no TS errors      ✓ no Go build errors
✓ lint passes        ✓ tests pass        ✓ build passes
✓ UX reviewed        ✓ responsive        ✓ loading/empty/error states
✓ tenant isolation verified (API/data phases)
✓ docs updated       ✓ CHANGELOG entry   ✓ CLAUDE.md CURRENT STATE updated
✓ logical commit (§78)
```

---

## NEVER (§80)

Build all phases at once · replace repo without inspection · duplicate business
logic (esp. a 2nd routing impl in TS) · hardcode secrets · commit `.env` · skip
tests/validation · ignore TS/Go errors · fake APIs that look real · hide errors ·
dedup on click_id alone or on (click_id, status) alone · record a postback that
moves a conversion back to CPA_HOLD · treat missing cost as zero · Redis-only sticky ·
truncate the event model · leak data across orgs · embed vendor moderator
detection · divide-by-zero into an error in custom metrics · mix push+regular
metrics in one formula · reimplement tags per entity · treat empty FB subs as
"unknown campaign".

Mocks are allowed only in frontend-first phases and must implement the same
contract as the real backend, and be explicitly replaceable.

---

## COMMITS (§78)

Logical, per-phase commits. Examples:
`feat(ui): design system` · `feat(routing): filter builder` ·
`feat(routing): sticky engine` · `feat(ltv): cohort engine` ·
`feat(api): campaign endpoints` · `feat(tracker): tracking endpoint`.
No single giant commit.

---

## SESSION START CHECKLIST

1. Read CURRENT STATE above.
2. Read the relevant §sections in `FLOX-master-prompt-v3.md` for this phase.
3. Confirm the phase plan (§2) before writing code.
4. Implement → validate → document → update CURRENT STATE → commit → STOP.
