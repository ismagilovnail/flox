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
CURRENT PHASE : KEY THE GENERAL RATE LIMITER BY ORG FOR API-KEY TRAFFIC
                (picked from a 3-option menu after "продолжай" with no
                phase specified; user chose the most substantial
                remaining known limitation)
STATUS        : done. The general rate limiter (300/min,
                apps/internal/httpserver) was purely IP-keyed since it
                was built — a high-volume API-key integration shared one
                bucket with everything else behind its address. Added
                tenant.PeekIdentity: the same session/API-key resolution
                NewMiddleware already does, but never rejecting on a
                missing/invalid credential (needed since it's mounted
                globally, ahead of genuinely public routes too). Mounted
                in httpserver.New, immediately before the general
                limiter; the limiter's key function (main.go) reads the
                resolved identity — "org:"+orgID for API-key traffic,
                "ip:"+clientIP for everything else (unauthenticated,
                and session-cookie browser traffic, which isn't the
                "many requests behind one address" case this fixes).
                tenant.NewMiddleware now short-circuits when OrgID is
                already in context (set by PeekIdentity), so an
                authenticated request still costs exactly one Postgres/
                session round trip total, not two — a request
                PeekIdentity couldn't identify still falls through to
                NewMiddleware's own resolution and 401 exactly as
                before. httpserver.New's signature changed (clientIP
                param replaced by generalLimitKey/identify, both built
                in main.go); its one call site moved down, to after
                tenantMiddleware exists (identify needs authSvc/
                apiKeySvc). Added 5 new tenant-package tests (Peek's 3
                resolution cases, NewMiddleware's short-circuit +
                still-enforces-when-not-pre-identified) and 2 new
                apps/internal/httpserver integration tests against live
                Redis confirming the actual key created is org- vs
                IP-scoped. Updated docs/security.md §3 + Known
                limitations (struck through, matching the existing
                convention) and ROADMAP.md.
                VERIFIED: gofmt/go vet/go build/go test -count=1 ./...
                clean, all Go packages, live Postgres/ClickHouse/Redis/
                MinIO, incl. new tests; apps/api's own
                rbac_coverage_test.go confirms the main.go reordering
                dropped no route's RBAC gate. Live verification against
                the real dev stack: signed up a real org, created a
                real API key, hit a real endpoint, confirmed
                ratelimit:api:org:<orgID> was the actual Redis key
                created (not IP-keyed) — then confirmed a session-
                cookie request from the same org still produced the
                original ratelimit:api:ip:<addr> key, unaffected. Full
                browser pass (login, /overview, /team, /settings, all
                RBAC-gated): no regression, no console errors. All test
                fixtures (org, user, API key, Redis keys) removed
                afterward.
LAST COMMIT   : 999f123 feat(security): encrypt facebook_ads/tiktok_ads
                access_token at rest. Before it: 118bfed feat(analytics):
                add stream_set report dimension, close stale roadmap
                gap; f382d10 docs(roadmap): close the stream_set gap,
                correct the remaining unbacked-dimension count; 76b1279
                docs(roadmap): record this session's security-audit
                work, close the accessorFn gap — full detail for all in
                CHANGELOG.md. Note: the golang.org/x/crypto bump
                (GO-2026-6355/6354 closed, GO-2026-5932 permanently
                flagged/unreachable) landed between phases per
                CHANGELOG.md but was never captured in an earlier
                version of this block's LAST COMMIT line — check
                `git log` if an exact hash is needed for it.
UNCOMMITTED   : this phase's own work, about to be committed as one
                logical commit — apps/internal/tenant/tenant.go,
                apps/internal/tenant/tenant_test.go,
                apps/internal/httpserver/server.go,
                apps/internal/httpserver/server_test.go,
                apps/api/main.go, docs/security.md, ROADMAP.md,
                CHANGELOG.md, this CLAUDE.md edit. Deliberately left
                out, unrelated and predating this phase:
                apps/e2e/scenario_test.go remains unstaged from the
                older repo+plan-audit pass; whoever picks it up next
                should check it's still wanted.
NEXT          : confirm scope before starting. All 7 numbered security-
                audit findings are fully resolved; the stream_set
                dimension gap, the access_token plain-text gap, and the
                rate-limiter IP-keying gap are all closed. Known
                remaining items in ROADMAP.md's Known-open-gaps table:
                cloudflare_registrar has no adapter; SMS/email 2FA as a
                TOTP alternative; the report builder's filter-value
                dropdown is still a fixed mock pool (needs a real
                "distinct values for dimension X" endpoint); 7 feature
                areas are English-only in an EN/RU app; DataTable has no
                column resizing/CSV export; Content Gallery has no video
                upload/transcoding; Flows have no top-level list view; 4
                of 16 report dimensions (landing/pwa/postlanding/offer)
                still need real columns before they can follow
                stream_set's pattern; Google Play URL parsing for PWA
                listings. TRUSTED_PROXIES needs a real proxy deployed
                first (still blocked, no infra exists). GO-2026-5932
                (x/crypto/openpgp, no fix exists) stays a permanent,
                accepted, unreachable flag. Worth being aware of for
                future sessions: this session's own history/subagent
                tracking apparently does not persist a prior turn's
                investigation request reliably across some boundary —
                CHANGELOG.md and git log are the durable record to check
                first when a user references earlier work this session
                has no trace of.
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
