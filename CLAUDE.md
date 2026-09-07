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
CURRENT PHASE : REAL API-KEY AUTHENTICATION
                (audit finding #4, closed; follows the finding-#1
                frontend-button-gating phase)
STATUS        : done. Finding #4's first pass (earlier phase) left the
                UI honestly saying "not live yet" and deferred the real
                feature. SCOPED WITH THE USER before writing code, two
                decisions: (a) a key authenticates against the exact
                same route tree a session does — no separate "public
                API"; (b) the flat read/write/admin scope maps onto the
                18-permission vocabulary as coarse tiers (read = every
                *.read key; write = read + every *.write key EXCEPT
                team.write/settings.write; admin = everything) rather
                than inheriting the creating user's own role.
                THE MECHANISM: tenant.NewMiddleware — the single choke
                point all ~25 tenant-scoped route groups already share
                via one shared tenantMiddleware in main.go — now checks
                Authorization: Bearer flx_live_... before falling back
                to the session cookie. A match calls the new
                apikey.Service.Authenticate (hash lookup, revoked_at IS
                NULL check, best-effort last_used_at stamp — previously
                a dead column, confirmed by finding #4's own earlier
                research) and populates the SAME context (tenant.OrgID/
                UserID) a session would, plus a new tenant.APIKeyScope.
                Zero downstream handlers changed — 82 mutating routes
                across 17 packages, untouched. RequirePermission branches
                on APIKeyScope: present skips the membership-table
                HasPermission DB query entirely, evaluating
                apiKeyScopeAllows (pure string-suffix logic) instead.
                userID on an API-key request is the key's creator
                (created_by_user_id) — audit attribution only, never
                re-checked against memberships; a key deliberately
                outlives its creator's own membership (an integration
                shouldn't break because the employee who set it up
                left), matching how Stripe/GitHub keys work. CSRF's
                RequireSameOrigin needed zero changes — it already treats
                an absent Origin header as a non-browser client.
                FRONTEND: api-keys-panel.tsx's "not live yet" copy
                (finding #4's own earlier change) reverted to accurate —
                the create dialog spells out what each tier grants, the
                key-reveal dialog now shows the Authorization: Bearer
                <key> header format.
                VERIFIED: go build/vet/test -count=1 ./... clean, 41
                packages, live Postgres. New tests: apikey.Test
                AuthenticateValidKey/RejectsRevokedKey/
                RejectsUnknownOrGarbageKey; auth.TestRequirePermission
                APIKeyScope (6 cases, real live-mounted team.read/
                team.write keys). tsc/eslint/vitest(21/21) clean on the
                frontend copy change.
                LIVE END-TO-END AGAINST THE REAL RUNNING API: rebuilt the
                dev Docker image (no source volume mount on apps/api,
                unlike apps/web) to pick up the Go changes. Signed up a
                real org, verified via a real mailhog link, created three
                real keys (read/write/admin) through the real session-
                authenticated /api-keys endpoint. GET /campaigns and GET
                /team/members (team.read): 200 for all three scopes.
                POST /campaigns (campaign.write): 403 for read; write and
                admin got PAST the permission check (422 on missing
                fields, then a genuine 201 once given a real
                trafficSourceId) — proving the write path is actually
                unblocked, not merely not-403. POST /team/members/invite
                and PATCH /security (team.write/settings.write): 403 for
                BOTH read and write, granted only for admin — the exact
                tier-exclusion the scoping decision called for, confirmed
                against a live request. A garbage Bearer token 401'd; the
                read key 401'd immediately after a real DELETE
                /api-keys/{id}. Test org (its campaign, traffic source,
                keys, user) deleted afterward via SQL, FK order respected
                (campaigns -> traffic_sources -> api_keys -> audit_logs
                -> sessions -> memberships -> users -> organizations).
                DOCS: docs/auth.md gets a full "API key authentication"
                section (mirrors "RBAC route coverage"'s style) plus two
                stale bullets struck through (the frontend-button-gating
                gap, closed by the prior phase; "no API-key auth", closed
                by this one). docs/security.md gets a new §6 plus its
                finding-#4 Known-limitations bullet struck through.
                NOT addressed, called out honestly: the general rate
                limit stays IP-keyed, so a high-volume legitimate
                integration shares one bucket with anything else behind
                its IP — fixing that needs auth to resolve before the
                general limiter runs (it currently mounts earlier in the
                chain), a bigger restructuring than this phase scoped.
LAST COMMIT   : (this phase's own work is uncommitted, see below). Before
                it: 13ae03c feat(web): permission-gate action buttons
                across the API's five write keys (finding-#1 follow-up);
                734e54d chore(deps): move shadcn's CLI to devDependencies
                (audit finding #7); d75198d chore(deps): bump Go to
                1.26.6 (audit finding #6); e757aaa fix(settings): stop
                claiming API keys authenticate calls (audit finding #4,
                first pass); 2058b1b fix(security): trusted-proxy IP
                resolution & pending-login lockout (audit findings #2,
                #3); 1e45953 fix(auth): enforce RBAC on every mutating
                route (audit finding #1) — full detail for all in
                CHANGELOG.md.
UNCOMMITTED   : this phase's own work, about to be committed as one
                logical commit — apps/internal/tenant/tenant.go (bearer-
                token branch + APIKeyScope); apps/internal/apikey/
                {model,crypto,repository,service}.go (Authenticate,
                FindByHash, UpdateLastUsed, logger dependency) +
                apikey_test.go (new tests + newService helper);
                apps/internal/auth/service.go (RequirePermission's scope
                branch + apiKeyScopeAllows) + auth_test.go (new fakes +
                TestRequirePermissionAPIKeyScope); apps/api/main.go
                (apiKeySvc constructed once, threaded into
                tenant.NewMiddleware); apps/web/src/features/settings/
                api-keys-panel.tsx; docs/auth.md; docs/security.md;
                CHANGELOG.md; this CLAUDE.md edit. Deliberately left out,
                unrelated and predating this phase: ROADMAP.md,
                apps/e2e/scenario_test.go, apps/web/src/lib/api/
                networks.ts and offers.ts remain unstaged from the older
                repo+plan-audit pass; whoever picks those up next should
                check they're still wanted.
NEXT          : confirm scope before starting. All 7 numbered audit
                findings are now fully resolved (finding #4 was the last
                one carrying a deferred half). Still open, none of them
                numbered findings: setting TRUSTED_PROXIES to a real
                value once an actual reverse proxy/LB/CDN is deployed
                (finding-#2 phase; confirmed via AskUserQuestion that
                none exists yet, nothing to do until one does); the 3
                golang.org/x/crypto advisories surfaced in finding-#6's
                govulncheck run (dependency hygiene, not a live
                vulnerability); the general rate limit's IP-keying not
                fitting high-volume API-key traffic well (this phase's
                own known limitation, above); domain-form.tsx's
                MultiSelect field lacking a native disabled prop (UX
                polish, not security, flagged in the finding-#1-follow-up
                phase).
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
