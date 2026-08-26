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
CURRENT PHASE : PHASE 28A — AUTH / RBAC + TENANT ISOLATION (backend half)
STATUS        : done — real authentication, sessions, organizations,
                memberships, and role-based authorization (§52), replacing
                apps/internal/tenant's original X-Organization-Id-header
                stand-in. Confirmed via 3x AskUserQuestion before starting:
                (1) split into 28A backend-only now / 28B frontend
                integration as a separate later phase; (2) server-side
                sessions in an HTTP-only cookie, not JWTs — trivially
                revocable, no rotation/denylist machinery; (3) inviting a
                team member generates a shareable accept-invite LINK, not
                a real sent email (no SMTP/email-provider integration
                exists anywhere in this project — same situation Ad
                Account Connections hit with OAuth and resolved the same
                way). A 4th AskUserQuestion mid-plan confirmed RBAC
                enforcement (RequirePermission) is wired onto this
                package's own team/membership endpoints ONLY — sweeping
                permission checks across every pre-existing domain route
                (campaigns/offers/sources/pixels/stream-sets/cost/...) is
                an explicit, separate follow-up phase, not done here.
                Tenant isolation (§36-TENANCY) is unaffected either way —
                already enforced everywhere via org-scoped repositories,
                orthogonal to role-based checks within one org.
                New migration 00020: users.password_hash (empty-string
                sentinel = "shell user, invite not yet accepted"), a
                sessions table (token stored only as a SHA-256 hash, same
                precedent as api_keys.key_hash), memberships.
                invite_token_hash/invite_token_expires_at (an invite IS a
                membership row with status='invited', not a separate
                table), and role_permissions seeded to match apps/web's
                src/lib/mock/team.ts ROLE_PERMISSIONS exactly cell-for-
                cell (verified via psql query).
                New apps/internal/auth package (handler+team_handler ->
                service -> repository -> model/crypto, same layering as
                every domain): signup (org+Owner+session in one tx),
                login (resolves which org a multi-membership user meant;
                a precomputed dummy bcrypt hash keeps a nonexistent-email
                attempt from being measurably faster than a wrong-
                password one), logout, GET /auth/me (role + full
                permission list — UX gating only per §52, "frontend
                permissions are only UX"; every mutating endpoint still
                checks server-side independently), invite / GET
                /auth/invites/{token} preview / accept-invite / resend-
                invite (resend invalidates the OLD token), and full
                membership CRUD under /team/* (list/invite/update-role-
                or-status/resend/remove + /team/activity reading
                audit_logs, migration 00010, unused until now — populated
                only by this package's own membership actions, same
                explicit-scope-boundary reasoning as the RBAC sweep).
                Every mutation refuses to touch a membership whose role is
                Owner (matches member-row-actions.tsx's own "no actions
                for the Owner row," enforced server-side too) and role can
                never be SET to Owner via this API either — exactly one
                per org, created at signup. Suspending/removing a member
                revokes their session on their VERY NEXT REQUEST
                (ResolveSession's own query requires status='active' in
                the same join) plus proactively deletes their session rows
                (belt-and-suspenders).
                apps/internal/tenant: Middleware (a fixed function reading
                X-Organization-Id) became NewMiddleware(SessionResolver,
                logger), constructed once in apps/api/main.go with
                *auth.Service — SessionResolver is declared IN tenant (not
                imported from auth) so there's no import cycle (auth ->
                tenant for OrgID/UserID; tenant does NOT import auth).
                Every other domain package's own handlers needed ZERO
                code changes — mechanical sed of all 17
                r.Use(tenant.Middleware) call sites in main.go to
                r.Use(tenantMiddleware); they still only ever read
                tenant.OrgID(ctx) (+ new tenant.UserID(ctx)).
                apps/internal/httpserver: CORS AllowCredentials flipped to
                true (cookie must travel on apps/web's cross-origin
                fetch), X-Organization-Id dropped from AllowedHeaders (no
                route accepts it anymore). New apierror.Unauthorized(401)/
                Forbidden(403) constructors (first use in this project).
                Verified: go build/vet/gofmt/test ./... green across the
                ENTIRE repo — all ~15 pre-existing domain packages' own
                tests pass unchanged (they call services/repositories
                directly in Go, never through HTTP, so the middleware
                swap touched zero of them). 20 new internal/auth tests
                against real Postgres: signup/login/logout, session
                resolution, invite->preview->accept->login (incl. "cannot
                replay an accepted token" and Analyst's actual permission
                set), invite validation (rejects Owner, rejects a
                duplicate member), resend-invalidates-old-token,
                role/status update + activity log, Owner-protection
                (cannot change own role or remove self), member removal
                revokes access, cross-tenant isolation (org B cannot
                update/remove/see org A's memberships or activity — §36-
                TENANCY's own DoD requirement), and the RequirePermission
                middleware (200 vs 403 by role). Full manual curl pass
                (no frontend to click through yet — Phase 28B's job)
                against the real running apps/api dev stack: confirmed
                X-Organization-Id-only access now 401s; signup ->
                Set-Cookie -> GET /auth/me -> GET /campaigns (an untouched
                pre-existing domain) succeeding through the SAME cookie
                with zero code changes to campaign's own package; invite
                -> public preview -> accept-invite auto-login; invited
                Viewer's /auth/me showed exactly
                ["analytics.read","campaign.read"] and their own invite
                attempt correctly 403'd; Owner suspended the Viewer whose
                EXISTING session immediately 401'd on its very next
                request with no re-login needed to observe it; Owner's own
                self-removal attempt correctly 422'd; GET /team/activity
                showed all 5 real audit entries in order; logout cleared
                the cookie and it 401'd afterward. Test fixtures (org,
                cascade-deleted user/membership/sessions) deleted
                afterward. No frontend changes this phase — apps/web still
                runs on NEXT_PUBLIC_DEV_ORG_ID + the mock Team store.
LAST COMMIT   : feat(auth): sessions, organizations, and RBAC (Phase 28A)
NEXT          : Phase 28B (frontend integration — login/signup pages, wire
                Team page to the real API, replace NEXT_PUBLIC_DEV_ORG_ID)
                has NOT been started; confirm scope before beginning it or
                anything else. Known limitations documented in
                docs/auth.md: no RBAC on pre-existing domain routes (an
                explicit separate follow-up), no email delivery/password
                reset/MFA/API-key auth/org switcher/session-cleanup job —
                none requested.
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
