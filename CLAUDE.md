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
CURRENT PHASE : PHASE 28B — AUTH / RBAC + TENANT ISOLATION (frontend half)
STATUS        : done — wires apps/web to Phase 28A's real auth API. New
                login/signup/accept-invite pages (none existed before,
                not even mocked), route protection, and the pre-existing
                mock Team page switched onto the real /team/* endpoints.
                NEXT_PUBLIC_DEV_ORG_ID is gone entirely.
                New (auth) route group (bare centered layout, no sidebar/
                topbar): /login, /signup, /accept-invite (reads ?token=
                via PageProps' searchParams — a Server Component prop —
                not useSearchParams, avoiding that hook's own Suspense-
                boundary requirement). New "auth" i18n namespace (RHF+Zod
                forms, matching this project's current convention) — the
                pre-existing Team feature files were left on their
                original hardcoded English rather than partially
                retrofitting i18n into only what this phase touched.
                lib/api/client.ts: credentials:"include" replaces the
                X-Organization-Id header; httpserver's CORS gained
                AllowCredentials:true + a single explicit AllowedOrigins
                entry (fetch spec forbids "*" with credentials).
                New src/proxy.ts (THIS NEXT.JS VERSION, 16.3, RENAMED
                middleware.js TO proxy.js — middleware.js still works but
                is deprecated) — redirects based on session-cookie
                PRESENCE only (checked, documented explicitly as a UX
                convenience, never the enforcement boundary); new
                components/shell/auth-guard.tsx is the defense-in-depth
                layer behind it that actually calls GET /auth/me, catching
                a present-but-expired/revoked cookie (suspended/removed
                mid-session, or past 30-day expiry) and redirecting
                client-side instead of leaving a signed-out visitor on an
                app shell whose every fetch silently 401s.
                Team page: new lib/api/team.ts + hooks/use-team.ts replace
                stores/team.ts FOR THE TEAM PAGE ONLY.
                hooks/use-current-member.ts and stores/team.ts itself were
                DELIBERATELY LEFT UNTOUCHED — custom-metrics/content-
                gallery/referral (still fully mock, not wired to a real
                backend) key their own "who am I" role-gating against that
                mock roster's ids; rewiring identity there ahead of their
                own backend-integration phase would have silently broken
                their "is this mine" checks against still-mock seed data.
                lib/mock/team.ts's Role/ROLES/PERMISSIONS/ROLE_PERMISSIONS
                stayed exactly as-is (still accurate, still what
                RolesPermissionsPanel displays) — only TeamMember/
                TEAM_MEMBERS/ActivityEntry/TEAM_ACTIVITY were left behind
                by the real Team page. Invite/resend show the real link in
                a copy-to-clipboard reveal dialog (same pattern as
                api-keys-panel.tsx's key reveal), not a fake "email sent"
                toast.
                RBAC-aware navigation — a REAL GAP FOUND VIA MANUAL
                TESTING, not requested up front: a signed-in Viewer could
                click "Team" and land on a bare permission-denied
                ErrorState card, exactly what §52's "frontend permissions
                are only UX" is supposed to prevent. Fixed with
                NavItem.requiredPermission (lib/nav.ts, set to "team.read"
                on Team only) + visibleNavGroups(groups, permissions),
                applied in both nav-content.tsx (sidebar) and
                command-menu.tsx (⌘K), reading useMe().data?.permissions.
                MemberList/member-columns.tsx additionally hide "Invite
                member", disable the role Select, and hide row-actions
                when lacking team.write (e.g. Manager: has team.read, not
                team.write) — server 403s remain the real enforcement,
                this just stops offering controls that were never going
                to work.
                TWO REAL BUGS CAUGHT DURING MANUAL TESTING (not code
                review): (1) AcceptInviteForm's useForm({values:{...}})
                re-applied a FRESH object literal on every render, so
                every keystroke into the password field silently reset
                the whole form — symptom was "submit does nothing."
                Fixed: defaultValues + a one-time effect/ref-guarded
                form.setValue prefill instead. (2) internal/auth's own Go
                test suite created real rows in the shared dev Postgres
                but NEVER DELETED THEM (every other package's test suite
                does) — 3 earlier test runs during Phase 28A had left 51
                organizations + dozens of orphaned users rows sitting in
                the dev DB, only discovered when post-testing cleanup row
                counts looked wrong. Fixed: t.Cleanup added to
                signupOrg/uniqueEmail; the pre-existing backlog (and this
                phase's own manual-test fixtures) purged by hand — dev DB
                confirmed back to its pre-testing state (1 pre-existing
                Phase 27 Dev Org fixture, 0 users, 0 sessions).
                Verified: gofmt/go build/vet/test ./... unchanged and
                green (no Go code changed except the test-cleanup fix).
                next typegen (needed once, for /accept-invite's PageProps
                type) / tsc --noEmit / eslint . / vitest run (21 tests,
                unchanged) / next build all clean. Full manual browser
                pass (Claude-in-Chrome) against the real running apps/api
                + apps/web dev servers ON MATCHING DEFAULT PORTS (8080/
                3000 — required for CORS/cookies to actually work; Phase
                28A's own curl pass had used a nonstandard 18080 for
                apps/api, which would silently fail CORS from a real
                3000-origin browser): signed-out /overview correctly
                redirected to /login; signup created a real org and
                rendered the full app shell with the REAL org name
                (WorkspaceSelector) and REAL user initials/name/email
                (UserMenu, no more MOCK_USER); /team showed one real
                member with genuine "last active"; invited a Viewer
                through the real sheet, got a real accept-invite link;
                opened it in a second tab, the public preview correctly
                showed org/role (localized to Russian per the browser's
                Accept-Language, confirming the new i18n namespace
                loads), accepting logged the invitee straight in; the
                Viewer's sidebar correctly had NO "Team" link at all and
                their /auth/me showed exactly ["analytics.read",
                "campaign.read"]; back as Owner, suspended then removed
                the Viewer via the real row-actions menu, member count
                updated live with no reload; Activity tab showed all 4
                real audit entries with human-readable labels and correct
                actor names; logout cleared the cookie and a subsequent
                direct /team visit redirected to /login again; spot-
                checked content-gallery still renders correctly against
                its own untouched mock, confirming use-current-member/
                stores/team.ts were genuinely left alone. Zero console
                errors throughout. All fixtures from both this phase's
                and Phase 28A's manual passes deleted afterward.
LAST COMMIT   : feat(auth): wire apps/web to real sessions and RBAC
                (Phase 28B)
NEXT          : confirm scope before starting. No open known issues
                remain for Auth/RBAC. Known limitations documented in
                docs/auth.md (unchanged from Phase 28A): no RBAC on pre-
                existing domain routes (an explicit separate follow-up,
                confirmed out of scope for both 28A and 28B), no email
                delivery/password reset/MFA/API-key auth/org switcher/
                session-cleanup job — none requested. Phase 28
                (Auth/Organizations/RBAC) is now fully done end-to-end.

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
