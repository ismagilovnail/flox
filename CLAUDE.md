# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

> FLOX's operating manual + progress tracker, read at the start of every
> session. Keep it lean. The full spec is `docs/FLOX-master-prompt-v3.md` —
> read it before implementing any phase.

---

## PROJECT

**FLOX** — production-grade SaaS traffic tracking & routing platform.
Tagline: **Track. Route. Optimize.**

Full requirements: **`docs/FLOX-master-prompt-v3.md`** (authoritative; sections
referenced below as §N). What each phase did, in detail: `CHANGELOG.md`.

---

## CURRENT STATE — UPDATE THIS EVERY PHASE

```
CURRENT PHASE : PHASE 30.90 — UNI-4 PART 3, SHOPIFY WEBHOOKS (closed)
STATUS        : Backlog = product audits 2026-10-15 and 2026-10-24
                (docs/audit-product-*.md), tracked on the local dev panel
                tools/devpanel (make devpanel → 127.0.0.1:8099; memory:
                flox-audit-tracker — keep statuses current). What each
                closed item did: CHANGELOG.md.
                Open: UNI-4 part 4 (WooCommerce, on shop_connections);
                OPS-1..3 (prod, restore, load — need the owner's hosting,
                skipped by the user for now); BIZ-3 and BIZ-4 part 2 are the
                user's. Not built: BIZ-1 autopay/discounts/invoices/staff
                admin, Microsoft Ads upload, REL-1 disk spool (declined).
                No third-party integration (Google Ads, Metrica, Direct, VK,
                Shopify, YooKassa) has run against a live account: fake
                servers + live error probes only.
LAST COMMIT   : feat(shopify): orders and refunds by webhook (UNI-4 part 3).
UNCOMMITTED   : docs/import-export-plan.md (INT-2, stays uncommitted).
NEXT          : wait for the user.
                Do not offer: GAP-2 (Google Play import, won't-do), INT-2
                (the owner does it later), a BIZ-3 interview script. CI
                (DO1) is the user's own decision — do not set it up. No
                browser checks without the user's OK (memory:
                flox-no-browser-without-ok). To try payments for real: a
                YooKassa test shop's YOOKASSA_SHOP_ID/SECRET_KEY in
                infra/.env.
```

> At the end of every phase: update the four lines above, add a CHANGELOG entry,
> commit, then STOP and wait. Do not roll into the next phase automatically.
> Rules a phase leaves for later code go in CODEBASE RULES below, not here.

---

## COMMANDS

```
# repo root — the dev stack (infra/docker-compose.dev.yml) and checks
make up | make status | make help
make dev-fresh CONFIRM=yes        # DESTRUCTIVE: wipe dev data + demo accounts
make dev-status                   # what the dev stores hold
make migrate-up                   # goose, apps/api/migrations
make check                        # gofmt -l + go vet + go test + vitest + eslint + tsc
make audit-web                    # npm audit gate (prod deps)
make devpanel                     # backlog dashboard, 127.0.0.1:8099

# apps/ — Go. Without this env, ClickHouse/e2e/MinIO/Redis/NATS tests
# t.Skip and the run still says ok. Check with -v that TestFullFunnel PASSed.
DATABASE_URL='postgres://flox:flox@localhost:5432/flox?sslmode=disable' \
CLICKHOUSE_URL=http://localhost:8123 CLICKHOUSE_DATABASE=flox CLICKHOUSE_USER=flox \
REDIS_URL=redis://localhost:6379 NATS_URL=nats://localhost:4222 \
S3_ENDPOINT=http://localhost:9000 S3_PUBLIC_ENDPOINT=http://localhost:9000 S3_REGION=us-east-1 \
S3_BUCKET=flox S3_ACCESS_KEY_ID=flox-minio-admin S3_SECRET_ACCESS_KEY=flox-minio-secret \
go test -count=1 ./...                         # or: -run TestName ./internal/<pkg>/
# e2e signs up through the real API; after a few runs the auth rate limit
# makes every e2e test skip — clear it first:
docker exec infra-redis-1 sh -c 'redis-cli --scan --pattern "ratelimit:*" | xargs -r redis-cli DEL'

# apps/web
npx vitest run [src/path/file.test.tsx] | npx tsc --noEmit | npx eslint . | npm run build
```

Dev stack: web :3000, api :8080 (`/v1/...`), tracker :8081, Postgres
:5432, ClickHouse HTTP :8123 (not 9000 — that is MinIO), Redis :6379, NATS
:4222, MailHog :8025. Accounts `{owner,admin,manager,buyer,analyst,viewer}@flox.local`
/ `flox-demo-2026` (`make fixtures`). A scripted POST/PATCH/PUT/DELETE needs
`Origin: http://localhost:3000` (same-origin check). The api container is a
baked image — after Go changes: `docker compose -f infra/docker-compose.dev.yml
up -d --build api`. tracker and worker hot-reload with air: saving their
code (a mutation test included) changes the live dev binaries, and they run
half-written code against dev data. A new env var goes into the compose
file and the container is recreated *before* the code that reads it is
saved (air does not pick up env).

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
object storage · NATS JetStream (tracker events → worker; conversions stay in
the Postgres queue — docs/event-stream.md).

**Architecture:** modular monolith. `apps/tracker` and `apps/worker` are separate
binaries inside the same Go module, sharing `internal/routing` and
`internal/classifier`. No microservices up front. No unnecessary frameworks.

How the pieces meet (details: `ARCHITECTURE.md`, `docs/event-stream.md`):

- **apps/tracker** is everything public: redirects (`/t/...`), landings and
  PWAs on tracking domains, postbacks (`/postback/{networkId}`, secret),
  page conversions (`/flox.js`, `/c/{networkId}`), store webhooks
  (`/shopify/{networkId}`, HMAC), Telegram webhooks. It publishes click
  events to NATS JetStream; conversions are decided synchronously by
  `internal/conversion` (Postgres) and queued for the worker.
- **apps/worker** drains events into ClickHouse (`internal/chstore`, schema
  in `internal/chstore/schema`), delivers outgoing postbacks and pixels, and
  runs the timers: cost sync (`internal/costsync` + `internal/adaccount/*`),
  offline conversions (`internal/offlineconv`), FX, retention, alerts.
- **apps/api** is the control plane behind the session/API key; reports read
  ClickHouse through `chconn.TenantConn` (per-plan limits).
- Postgres holds every setting and the conversion ledger (`postbacks`);
  ClickHouse holds clicks/events; Redis is cache and rate limits only.

---

## REPO LAYOUT

```
apps/
  go.mod     ONE Go module for everything below except web
  api/       control-plane API (main + migrations/ + route-level tests)
  tracker/   public hot path (package main)
  worker/    async processor (package main)
  internal/  shared domain packages (routing, conversion, chstore, …)
  cmd/       one-off binaries (devfixtures, billing, …)
  e2e/       full-funnel tests against the running dev stack
  web/       Next.js frontend (the only non-Go app; own package.json)
packages/    ui/ config/ types/ (barely used)
docs/        spec (FLOX-master-prompt-v3.md), per-domain docs (§76), audits
infra/       docker-compose (dev, test), k8s, Caddy, Prometheus alerts
tools/devpanel/  separate Go module: local backlog dashboard (SQLite)
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
   txn id was sent — except a re-approval after DECLINE/TRASH carrying a txn
   id no earlier ACCEPT of the click carried, and the reversal of that
   re-approval (§45 RE-APPROVAL, DATA-3); a refund with a repeat purchase's
   txn_id reverses that one, refund_amount part of one (UNI-4). **Status never goes back to CPA_HOLD** — nightly partner
   replays re-send it after approval and would take revenue out of a closed
   report (§45 STATUS PROGRESSION). Long Redis TTL + durable DB unique
   constraint. Store original currency + USD value at event time.
   `acceptDuplicates` override per network — it does not bypass the
   progression rule.

4. **Sticky = cookie is truth, Redis is cache only (§39-STICKY).** Cookie
   `sf_{campaignId}` = `setId:flowId[:clickId].sig` (HMAC, STICKY_COOKIE_KEY;
   unsigned/forged = ignored). Redis-only sticky is forbidden
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

## CODEBASE RULES (tests enforce them, or they bit before)

- A new API route lands in `internal/openapi/openapi.yaml` or in
  `notInPublicSpec` (`api/openapi_test.go`). An id-named JSON field in a
  request needs a `referenceCases` entry or a `notReferences` reason
  (`api/tenant_isolation_routes_test.go`, which also walks every route with
  another org's ids).
- Buyers (TEAM-1): a new read of campaign data goes through
  `internal/campaignscope` and is listed in `api/campaign_scope_test.go`;
  campaign writes go through `campaign.Access.RequireOwned`.
- ClickHouse from apps/api: `chconn.Settings`, never
  `clickhouse.WithSettings`. A new insert path into
  `click_events`/`tracking_events` appends `retention_months`. Source click
  ids are read with `chstore.SourceClickIDExpr` (`notEmpty()`, not `!= ''`,
  or the bloom index is skipped). A test that seeds past events and reads a
  rollup-able report uses `NewRawEventStore` (the rollup starts at a
  cutover).
- chi: never `r.Route("/{id}", …)` beside a handler that owns `/{id}` — it
  takes `/networks/{id}` itself away; register sub-resources as full paths.
- Postgres index on a big table: `CREATE INDEX CONCURRENTLY` in its own
  migration with `-- +goose NO TRANSACTION`.
- Recoverable secrets: AES-256-GCM, one key per domain
  (`TOTP_`, `INTEGRATION_`, `ADACCOUNT_`, `PIXEL_`,
  `SHOP_WEBHOOK_ENCRYPTION_KEY`); responses carry only a last-4 preview. A new
  key goes into both compose files, k8s, `.env.example` and
  `docs/environment.md`.
- Ad platforms: a sync error is a 503 with the platform's reason, never the
  request URL (Facebook's carries the token). VK tokens are minted once and
  kept (`Credentials.SaveTokens`; 5 per client at most). Metrica uploads are
  never restated. `googleads.APIVersion` must move before Google sunsets it
  (~a year after release).
- Billing limits never touch the tracker. A network with postbacks cannot be
  deleted (409) — archive it.
- Known flakes under the full parallel run (pass alone):
  `TestRawRetentionFollowsThePlan`, `TestLimitsStopAHeavyQuery`,
  `TestTranscodeFailures`. `internal/chstore/retention.go` is not
  gofmt-clean (older). Restarting the dev worker re-runs the FX sync, and
  cbr.ru answers bursts with 429.

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
2. Read the relevant §sections in `docs/FLOX-master-prompt-v3.md` for this phase.
3. Confirm the phase plan (§2) before writing code.
4. Implement → validate → document → update CURRENT STATE → commit → STOP.
