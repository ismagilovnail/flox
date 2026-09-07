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
CURRENT PHASE : PERMISSION-GATE THE FRONTEND ACTION BUTTONS
                (finding-#1 follow-up, user-scoped, follows the finding-#7
                shadcn-devDependencies phase)
STATUS        : done. Finding #1 enforced campaign.write/offer.write/
                source.write/content.write/settings.write on the API, but
                the frontend still showed Create/Edit/Delete/Pause/
                Archive/Duplicate/Regenerate-secret buttons to every role
                regardless of permission — a read-only role saw a
                "Create" button that then 403'd.
                SCOPED WITH THE USER before starting: whether to also
                guard the ~14 route-level new-*-view.tsx pages against
                direct navigation (hiding a list's button alone doesn't
                stop someone typing /campaigns/new and filling out the
                whole form before a 403) — user said yes, guard those
                too, roughly doubling the file count.
                BUILT: usePermission(key) hook (hooks/use-auth.ts, on top
                of useMe()'s permissions array) and PermissionDeniedState
                component (components/ui/permission-denied-state.tsx,
                matches EmptyState/ErrorState's shape), plus
                common:permissionDenied.title/.description in both en/ru
                locales. Convention matches team/member-list.tsx's
                existing !!me.data?.permissions.includes(key) — extended
                consistently to the other four write keys for the first
                time.
                campaigns/ built FIRST as the reference implementation
                (done directly, not delegated) because it had the one
                real subtlety: campaign-form.tsx/tracking-links.tsx
                already had an "archived" notice tied to a shared
                disabled prop — a plain disabled = isArchived || !canWrite
                would have shown "this campaign is archived" to a
                non-archived campaign's read-only viewer. Fixed with a
                separate `archived` prop so the notice stays accurate to
                the real reason. The other four groups (offer.write,
                source.write, content.write, settings.write) were then
                built by 4 parallel forks, each given the campaigns/
                reference files to imitate plus an explicit file list —
                none of their forms had a pre-existing disabled-tied
                notice to disambiguate, confirmed by grep before
                dispatching, so they needed no equivalent complication.
                BUG FIXED ALONG THE WAY: content-gallery/gallery-
                browser.tsx's existing role check (Owner/Admin only, and
                only gating a filter dropdown, not any mutation) was
                wrong on both counts — content.write's real seeded policy
                (migration 00035) is Owner/Admin/Manager, and upload/
                archive/restore/delete had ZERO check before this. Fixed
                to usePermission("content.write") gating the real
                mutations. gallery-folder-panel.tsx's folder CRUD also
                had zero check before and now has one.
                TAGS HANDLED ONCE: content.write also covers
                apps/internal/tags (create/update/delete/assignment all
                behind the same requireWrite). Rather than gating the
                "Edit Tags" trigger on each of the ~10 host list pages
                that embed it, tag-picker-popover.tsx and bulk-tag-
                dialog.tsx — the two shared components every host page's
                tag mutations actually route through — are gated once,
                correctly covering every call site with zero per-page
                duplication.
                VERIFIED: tsc --noEmit / eslint / vitest (21/21) all
                clean across every touched file (58 files: campaigns x7,
                offers x5, networks x5, postbacks x2, traffic-sources x6,
                landings x5, pwa x5, postlanding x5, pixels x5, content-
                gallery x2, tags x2, domains x6, plus use-auth.ts + the
                new PermissionDeniedState + 2 locale files). LIVE IN A
                REAL BROWSER against the running dev stack (not just
                static checks): stopped the infra-web-1 docker container
                to free port 3000 for a local `next dev` bound to it
                (CORS only allows APP_URL=http://localhost:3000, so a
                mismatched port silently 503'd every API call — caught
                and fixed before concluding anything was broken). Signed
                in as the existing "Browser Verify Org" Owner — New-
                campaign button visible. Invited and accepted a real
                Viewer — New buttons hidden across campaigns/offers/
                networks/pixels/landings; a real seeded traffic source's
                row actions showed only Edit + View-statistics (Pause/
                Duplicate/Archive gone); its detail form rendered every
                field and the Save/Connect buttons disabled; navigating
                directly to /campaigns/new rendered PermissionDeniedState
                ("У вас нет прав для этого действия") instead of the
                form, in the session's own Russian locale — confirming
                the i18n keys resolve. Test invite and its fixtures
                deleted afterward via SQL (audit_logs first, FK order);
                infra-web-1 restarted, ad-hoc dev server killed —
                environment back to its exact prior state.
LAST COMMIT   : (this phase's own work is uncommitted, see below). Before
                it: 734e54d chore(deps): move shadcn's CLI to
                devDependencies (audit finding #7); d75198d chore(deps):
                bump Go to 1.26.6 (audit finding #6); e757aaa
                fix(settings): stop claiming API keys authenticate calls
                (audit finding #4); 2058b1b fix(security): trusted-proxy
                IP resolution & pending-login lockout (audit findings #2,
                #3); 1e45953 fix(auth): enforce RBAC on every mutating
                route (audit finding #1) — full detail for all in
                CHANGELOG.md.
UNCOMMITTED   : this phase's own work, about to be committed as one
                logical commit — all 58 files listed under VERIFIED
                above. Deliberately left out, unrelated and predating
                this phase: ROADMAP.md, apps/e2e/scenario_test.go,
                apps/web/src/lib/api/networks.ts and offers.ts remain
                unstaged from the older repo+plan-audit pass; whoever
                picks those up next should check they're still wanted.
NEXT          : confirm scope before starting. All 7 numbered audit
                findings are resolved or explicitly deferred by user
                decision (see finding-#7 phase's own NEXT for the full
                list); this phase closes the frontend follow-up finding
                #1 left open. Still open, none of them numbered findings:
                setting TRUSTED_PROXIES to a real value once an actual
                reverse proxy/LB/CDN is deployed (finding-#2 phase);
                scoping real API-key authentication as its own phase
                whenever it's wanted (finding-#4, deferred); the 3
                golang.org/x/crypto advisories surfaced in finding-#6's
                govulncheck run (dependency hygiene, not a live
                vulnerability today). Also worth a look, found but not
                acted on this phase: domain-form.tsx's MultiSelect
                (Purpose field) has no native disabled prop and stays
                interactively editable even when the rest of the form is
                locked (its per-field disabled-prop approach, unlike
                pwa-form.tsx/postlanding-form.tsx/pixel-form.tsx's native
                `<fieldset disabled>` wrapper, which correctly locks a
                MultiSelect's button too) — the submit button still
                blocks the actual write, so this is UX polish, not a
                security gap.
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
