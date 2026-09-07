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
CURRENT PHASE : RBAC ENFORCEMENT ON EVERY MUTATING ROUTE
                (security audit finding #1, user-scoped, post-Phase-34)
STATUS        : done — user asked for a code + security audit, then said
                to start with its first finding. The audit itself was
                verification-only and is summarised at the end of this
                block; this phase is the fix for finding #1.
                THE FINDING: the role -> permission policy seeded by
                migrations 00001/00020 was real in the database and
                enforced almost nowhere in the API. RequirePermission
                guarded /team, /security and the four features whose own
                later phases wired it (integrations, custom metrics, API
                keys, report presets, referral) — the other 23 tenant-
                scoped route groups had NO authorization at all beyond
                "is a member of this org." A Viewer — a role the Team UI
                presents as read-only, holding only campaign.read and
                analytics.read — could create, edit and delete campaigns,
                offers, networks, traffic sources, domains, tracking
                links and stream sets over the API; editing a stream set
                rewrites where live traffic goes. The frontend hid the
                buttons, the API accepted the calls. Proof it was never
                enforced rather than enforced-somewhere-else:
                campaign.write / offer.write / source.write existed as
                permissions rows and appeared in ZERO production code
                paths — the only references to campaign.write in the Go
                tree were inside auth_test.go.
                SCOPED WITH THE USER before writing code, two questions,
                both genuinely load-bearing (they change the migration
                and the frontend vocabulary): (a) how to name rights for
                the six entities the vocabulary couldn't reach at all —
                user picked ONE content.read/content.write pair over six
                per-entity pairs or folding into campaign.write; (b)
                whether to gate reads too — user picked writes only.
                THE FIX: 82 mutating routes across 17 packages now sit
                behind RequirePermission. Each handler gained a
                requireWrite func(http.Handler) http.Handler and wraps
                its own mutating routes with r.With(h.requireWrite) —
                not a new pattern, it is exactly the shape custommetrics/
                reportpresets/referral/domain already used; main.go
                builds one middleware per key and injects it. Mapping:
                campaign.write (campaigns, stream sets, tracking links,
                cost entries); offer.write (offers, networks, event
                mappings, postback replay); source.write (traffic
                sources, ad-account connection + spend sync);
                content.write (landings, pwas, postlandings, pixels,
                tags, content gallery); settings.write (domains — whose
                renew/sync-dns ALREADY used this key while its plain
                CRUD and verify sat open, a half-gated state nobody had
                noticed).
                Reads stay open to any org member — the dominant
                convention in this codebase already, and it keeps the
                change at zero lockout risk for live accounts. Gating
                reads too would drop Viewer from /offers and /networks
                and Analyst from /traffic-sources; that is a product
                decision, not something to smuggle into a security fix.
                Migration 00035 adds content.read/content.write.
                DELIBERATELY LEFT OPEN, documented in code at each site
                so a future audit doesn't re-flag them: /analytics' five
                POST endpoints (POST only because they carry a JSON query
                body — they read ClickHouse and write nothing; also NOT
                gated on analytics.read, since all six roles hold it and
                a check that rejects nobody reads as a control that
                isn't one); POST /routing/simulate (in-memory, persists
                nothing, and a read-only role should still be able to
                explain routing per §72); POST /referral/payouts (open to
                all roles by the Referral phase's own decision); the
                pre-session and personal-2FA auth routes.
                KEEPING IT CLOSED: apps/api/rbac_coverage_test.go parses
                every handler's Register with go/ast and fails on any
                POST/PATCH/PUT/DELETE not wrapped in requireWrite, with
                the four exceptions above as an explicit deliberatelyOpen
                allowlist carrying a reason each. Deliberately a
                structural test: a unit test of RequirePermission could
                never have caught the original bug, because the
                middleware always worked — it simply wasn't mounted.
                Verified it can actually fail by unwrapping one route and
                confirming it named campaign.delete, then restoring.
                Also new: TestPermissionGrantsMatchTheSeededPolicy pins
                the whole 6-role x 18-permission matrix against real
                Postgres (these grants are load-bearing for the first
                time now, so a wrong one is an access-control bug).
                FRONTEND DRIFT FOUND AND FIXED in the same pass, not
                incidental to the finding: lib/mock/team.ts still listed
                only the original ten permission keys long after
                custom_metric.*/report_preset.*/referral.* were seeded,
                and features/team/roles-permissions-panel.tsx renders
                that copy to admins AS the org's access policy — so the
                matrix someone reads to decide who to invite as what was
                incomplete. Now carries all eighteen, cross-checked
                programmatically against the migrations (all six roles,
                exact counts, zero drift), and marked as a manual sync
                point. NOT done, called out honestly rather than left
                implied: the action buttons themselves are still not
                permission-gated in the UI (only member-list.tsx checks
                team.write), so a read-only role now sees a "Create"
                button and gets a 403 from it — a UX gap, not a security
                one, and the natural next piece.
                VERIFIED: go build/vet clean; go test -count=1 ./...
                clean, 40 packages, run with live Postgres/ClickHouse/
                Redis/MinIO so the DB-backed tests actually EXECUTED
                rather than silently skipping (the exact trap the
                previous audit documented). tsc --noEmit / eslint (zero
                warnings) / vitest 21/21 clean.
                Live end-to-end against the running containerized API:
                signed up a real org, pulled the real verification link
                out of mailhog, invited and accepted a real Analyst.
                /auth/me returned exactly the eight read keys for Analyst
                and all eighteen for Owner. Twelve writes across
                campaigns/offers/networks/traffic-sources/domains/
                landings/pwas/pixels/tags/content-gallery/postlandings/
                event-mappings all returned 403 with the correct
                "missing permission: <key>" message; twelve reads all
                returned 200 — no lockout. Against a real campaign:
                Analyst got 403 on DELETE/PATCH/pause/stream-set-create
                and 200 on GET and routing/simulate; Owner's PATCH
                returned 200 AND the rename actually persisted on
                re-read — proving the write path is intact, not merely
                unblocked. Test org, both users and every created fixture
                deleted afterwards via SQL (audit_logs first, FK).
                THE AUDIT'S OTHER FINDINGS, not fixed this phase, listed
                so they aren't lost: (2) X-Forwarded-For is trusted
                unconditionally with no trusted-proxy concept anywhere
                (ratelimit.ClientIP + tracker's clientIP + chi's own
                RealIP) — bypasses the 20/15min auth brute-force limit
                AND makes the Security IP-allowlist decorative; (3)
                VerifyPendingLogin doesn't consume the pending token on a
                wrong code and has no attempt counter, so one 5-minute
                token allows unlimited TOTP/backup-code guesses; (4) API
                keys authenticate nothing — apikey.Repository has only
                List/Create/Revoke, no lookup-by-hash exists anywhere, yet
                the UI issues flx_live_... with scopes and a one-time
                reveal (CLAUDE.md's own "fake APIs that look real"); (5)
                the rate limiter fails open on a Redis error; (6) Go
                1.26.5 carries 7 known stdlib vulns per govulncheck (fix
                = bump to 1.26.6); (7) the shadcn CLI sits in prod
                dependencies, pulling express/MCP-SDK/qs into the
                production install tree. Audited CLEAN, verified not
                assumed: tenant isolation (swept every SQL literal in the
                tree against the tenant-table list — 5 hits, all five
                false positives), no SQL injection (both fmt.Sprintf
                query sites are a fixed internal whitelist and
                parameterized values), AES-256-GCM crypto with random
                nonces and key-length validation, constant-time login and
                postback-secret comparison, 32-byte crypto/rand session
                tokens, zero dangerouslySetInnerHTML/innerHTML/eval in
                apps/web, upload validation (extension whitelist +
                sniffed-content cross-check, no SVG), CSRF Origin checks
                on public and private mutating routes alike, no secrets
                in logs, .env gitignored.
LAST COMMIT   : 1e45953 fix(auth): enforce RBAC on every mutating route
                (audit finding #1) — 27 files: migration 00035; the
                requireWrite field/param + wrapped routes across the 17
                handler packages; apps/api/main.go's five named
                require*Write middlewares; apps/api/rbac_coverage_test.go
                (new); apps/internal/auth/auth_test.go's matrix test;
                lib/mock/team.ts's 18 keys; docs/auth.md. Scoped to this
                phase only — deliberately left out the older, unrelated
                repo+plan-audit changes that were sitting staged in the
                same working tree.
UNCOMMITTED   : two things, both predating this phase and unrelated to
                it. (a) This CLAUDE.md edit plus the deletion of
                apps/web/src/lib/mock/campaigns.ts — leftover staged
                state from the earlier repo+plan-audit pass, committed
                together right after this one. (b) Still unstaged from
                that same audit pass, untouched here: .env.example,
                ROADMAP.md, apps/e2e/scenario_test.go,
                apps/web/src/lib/api/networks.ts, apps/web/src/lib/api/
                offers.ts — whoever picks those up next should check
                whether they're still wanted before committing them.
                Also found this session, NOT part of any phase's file
                list and not committed: apps/apps/internal/clientip/
                clientip.go — a complete, documented trusted-proxy/XFF
                resolver for finding (2), but at a doubled/wrong path
                (should be apps/internal/clientip/) and wired into
                nothing (ratelimit.ClientIP, tracker's clientIP, and
                chi's RealIP all still take XFF's spoofable leftmost
                entry raw). Left in place pending a decision to resume
                finding (2).
NEXT          : confirm scope before starting. The audit's own ranking
                stands: finding (2) XFF/trusted-proxy is the next
                highest-value fix and it compounds with (3), so those two
                are best done together — and finding (2) already has a
                head start sitting at apps/apps/internal/clientip/
                clientip.go (see UNCOMMITTED above), just needs moving to
                apps/internal/clientip/ and wiring up; (4) API keys is a
                product decision (implement key auth, or remove the UI
                that promises it) more than a patch. Also open, from the
                finding-#1 phase: permission-gating the action buttons in
                the frontend so a read-only role stops seeing controls
                that 403.
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
