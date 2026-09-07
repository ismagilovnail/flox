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
CURRENT PHASE : ADD STREAM_SET ANALYTICS DIMENSION (closes "no
                stream_set_id on events" roadmap gap — query-layer/UI
                only, per a prior research pass in this same session
                that found the gap stale: the column already existed
                and was populated, only the whitelist/registry were
                missing it)
STATUS        : done. Backend: chstore.DimStreamSet ("stream_set" ->
                stream_set_id) added to reportClickColumns and
                reportConversionColumns — the 12th backed
                ReportDimension (11 original + this addition). Frontend:
                "stream_set" added to DimensionKey/DIMENSIONS/i18n
                (en/ru) and to VIEW_STATS_DIMENSIONS. Stream Sets rows
                get a "View statistics" link (stream-set-row.tsx) like
                network/offer/source already have, but passing the
                set's real .id (not .name, unlike those three
                pre-existing, partly-decorative links) since stream_set
                genuinely filters on stream_set_id — the filter chip
                shows the raw ULID as a disclosed tradeoff
                (FilterCondition has no display-label field; adding one
                was out of scope). Empty string preserved as its own
                real bucket (campaign fallback / blocked click /
                unattributed conversion), same semantics as flow_id.
                Extended chstore/report_test.go's fixture with real
                StreamSetID values incl. an empty-bucket case; added
                TestReportClicksGroupsByStreamSet and
                TestReportConversionsGroupsByStreamSetPreservesEmptyBucket
                against live ClickHouse. Updated
                docs/analytics-report-api.md's dimension count/section.
                VERIFIED: gofmt/go vet/go build/go test -count=1 ./...
                clean, all Go packages, live Postgres/ClickHouse/Redis/
                MinIO; tsc --noEmit/eslint/vitest clean on apps/web.
                Real browser pass: signed up a test org, created a
                campaign with two real Stream Sets, generated real
                clicks through apps/tracker (4 vs 3), confirmed each
                set's "View statistics" link lands on /analytics with a
                correct, distinct filtered count and no console errors.
                All test fixtures (org/user/campaign/stream
                sets/network/offer/traffic source/domain, ClickHouse
                click events) removed afterward.
LAST COMMIT   : 96dabb9 fix(ui): give MultiSelect real disabled support,
                close domain-form.tsx's permission gap. Before it:
                1c31660 fix(offers): close offer-columns.tsx's accessorFn
                stale-cache gap; 57b220d feat(auth): real API-key
                authentication (audit finding #4, closed); 13ae03c
                feat(web): permission-gate action buttons across the
                API's five write keys (finding-#1 follow-up); 734e54d
                chore(deps): move shadcn's CLI to devDependencies (audit
                finding #7); d75198d chore(deps): bump Go to 1.26.6
                (audit finding #6); e757aaa fix(settings): stop claiming
                API keys authenticate calls (audit finding #4, first
                pass); 2058b1b fix(security): trusted-proxy IP resolution
                & pending-login lockout (audit findings #2, #3); 1e45953
                fix(auth): enforce RBAC on every mutating route (audit
                finding #1) — full detail for all in CHANGELOG.md. Note:
                the golang.org/x/crypto bump (GO-2026-6355/6354 closed,
                GO-2026-5932 permanently flagged/unreachable) landed
                between phases per CHANGELOG.md but was never captured
                in this block's LAST COMMIT line by the phase that did
                it — check `git log` if the exact commit hash is needed.
UNCOMMITTED   : this phase's own work, about to be committed as one
                logical commit — apps/internal/chstore/report.go,
                apps/internal/chstore/report_test.go,
                apps/web/src/features/analytics/registry.ts,
                apps/web/src/features/analytics/analytics-view.tsx,
                apps/web/src/features/analytics/view-statistics-link.ts,
                apps/web/src/features/stream-sets/stream-set-row.tsx,
                apps/web/src/lib/mock/analytics.ts,
                apps/web/src/lib/i18n/locales/{en,ru}/analytics.json,
                apps/web/src/lib/i18n/locales/{en,ru}/streamSets.json,
                docs/analytics-report-api.md, CHANGELOG.md, this
                CLAUDE.md edit. Deliberately left out, unrelated and
                predating this phase: ROADMAP.md,
                apps/e2e/scenario_test.go, apps/web/src/lib/api/
                networks.ts and offers.ts remain unstaged from the older
                repo+plan-audit pass; whoever picks those up next should
                check they're still wanted.
NEXT          : confirm scope before starting. All 7 numbered security-
                audit findings are fully resolved; the stream_set
                dimension gap is closed. Known remaining items: TRUSTED_
                PROXIES needs a real proxy deployed first (finding-#2
                phase); the general rate limit's IP-keying not fitting
                high-volume API-key traffic (finding-#4 phase's own known
                limitation) — a real middleware-ordering restructuring.
                GO-2026-5932 (x/crypto/openpgp, no fix exists) stays a
                permanent, accepted, unreachable flag. The 4 remaining
                unbacked registry dimensions (landing/pwa/postlanding/
                offer) still need IDs threaded through apps/tracker's
                event emission plus a schema migration before they could
                follow the same pattern as stream_set — a separate,
                larger phase, not a quick whitelist fix like this one
                was. Worth being aware of for future sessions: this
                session's own history/subagent tracking apparently does
                not persist a prior turn's investigation request reliably
                across some boundary — CHANGELOG.md and git log are the
                durable record to check first when a user references
                earlier work this session has no trace of.
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
