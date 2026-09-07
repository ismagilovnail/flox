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
CURRENT PHASE : TRUSTED-PROXY IP RESOLUTION & PENDING-LOGIN LOCKOUT
                (security audit findings #2 + #3, user-scoped, post-
                Phase-34, follows the finding-#1 RBAC phase)
STATUS        : done. SCOPED WITH THE USER before writing code, same
                pattern as finding #1: (a) fix #2 and #3 together, per
                the prior phase's own NEXT note that they compound — #2
                is what lets XFF-spoofing bypass the auth rate limit that
                #3's unlimited-guessing bug needs to matter; (b) this
                deployment has no real reverse proxy/LB/CDN in front of
                it yet, so TRUSTED_PROXIES defaults to
                clientip.DefaultTrustedProxies (loopback + RFC 1918/4193
                private ranges) rather than a real address range — safe
                because a public peer is never in that set, so a spoofed
                header from the open internet is still ignored; (c) for
                #3, invalidate the pending login on the FIRST wrong code
                rather than adding an attempt-counter column — simpler,
                no migration, and the cost (a typo forces a fresh login)
                was judged acceptable.
                FINDING #2: every client-IP read in the tree —
                ratelimit.ClientIP, apps/tracker's own clientIP helper,
                and chi's middleware.RealIP mounted in front of both —
                took X-Forwarded-For's LEFTMOST entry unconditionally.
                That is exactly the entry the client itself writes, so
                all three were attacker-controlled: rotating the header
                defeated the 20/15min auth rate limit outright, made the
                Security IP allowlist decorative, and let a tracker
                visitor spoof the country/ASN routing filters read and
                choose its own weighted-flow arm (§38). A COMPLETE,
                DOCUMENTED FIX for this was found already written but
                never wired in or committed — apps/apps/internal/
                clientip/clientip.go, at a doubled/wrong path — from
                context lost to a prior compaction; moved to the correct
                apps/internal/clientip/clientip.go rather than rewritten,
                after reading it in full and confirming its design
                (right-to-left trusted-chain walk, "none" to disable,
                fail-loud on a malformed TRUSTED_PROXIES entry) was
                sound. It shipped with zero tests; clientip_test.go is
                new this phase (7 cases: untrusted-peer spoofing ignored,
                trusted-chain walked right-to-left, all-trusted chain
                falls back to leftmost, TrustNothing disables XFF
                entirely, malformed entries rejected at New(), ParseList
                trims/dedupes blanks, IPv4-mapped IPv6 unmaps to the same
                identity as its IPv4 form).
                THE FIX: one *clientip.Resolver built per binary
                (apps/api/main.go, apps/tracker/main.go) from a new
                TRUSTED_PROXIES env var (config.Config.TrustedProxies,
                empty default), threaded as an explicit dependency —
                NOT a package-level global — into httpserver.New,
                auth.NewHandler, auth.NewSecurityHandler, and tracker's
                Handler struct. ratelimit.ClientIP is DELETED, not left
                beside the resolver as a second insecure way to get the
                same value; its test (TestClientIP) is deleted with it.
                chi's middleware.RealIP is unmounted from both apps/api
                (httpserver/server.go) and apps/tracker (main.go) —
                upstream deprecated it for the identical unconditional-
                trust reason (GHSA-3fxj-6jh8-hvhx, GHSA-rjr7-jggh-pgcp,
                GHSA-9g5q-2w5x-hmxf). apps/tracker's track() now resolves
                the IP once per request and threads it through
                classifier.Input, visitKey, and buildEvent (which gained
                an ip parameter) instead of re-deriving it three times.
                FINDING #3: VerifyPendingLogin (apps/internal/auth/
                service.go) left the pending_totp_logins row untouched on
                a wrong TOTP/backup code, so its 5-minute TTL allowed
                unlimited guesses against one token. Fixed: a wrong code
                now calls DeletePendingTOTPLogin before returning the
                "incorrect code" error, so the same token is dead even
                against the real code afterward — forcing a fresh Login
                call, the same trade VerifyEmail/invite tokens already
                make elsewhere in this file. TestSelfServeTOTPEnrollment-
                AndLogin used to assert the OLD behavior in a comment
                ("A wrong code doesn't consume the pending login") and
                reuse the same token with the real code afterward — that
                assertion is now inverted (same token + real code after
                one wrong guess must fail) and the test performs a
                second, fresh Login to reach a real session.
                VERIFIED: go build/vet clean; go test -count=1 ./... clean
                — 41 packages (clientip new), run with live Postgres/
                ClickHouse/Redis/MinIO so DB-backed tests (auth: 3.8s)
                actually executed. gofmt clean (fixed one pre-existing
                doc-comment indentation nit in the moved clientip.go). No
                apps/web changes this phase, so tsc/eslint/vitest weren't
                re-run.
                DOCS: docs/security.md gets a new "§5 Trusted-proxy IP
                resolution & pending-login lockout" section plus a
                TRUSTED_PROXIES caveat under Known limitations; its §3
                Rate limiting section gets a one-line cross-reference.
                .env.example documents TRUSTED_PROXIES (default empty,
                "none" to disable entirely).
                NOT done, called out honestly: findings (4) API keys
                authenticate nothing, (5) rate limiter fails open on
                Redis error (a deliberate, documented stance per
                docs/security.md — not a bug), (6) Go 1.26.5 stdlib CVEs,
                (7) shadcn CLI in prod dependencies — all still open,
                full detail in the finding-#1 commit (1e45953 below) and
                in CHANGELOG.md. Also still open from the finding-#1
                phase itself: permission-gating the action buttons in the
                frontend so a read-only role stops seeing controls that
                403.
LAST COMMIT   : a5ed68a chore: update CLAUDE.md CURRENT STATE for finding
                #1 commit, drop dead campaigns mock (also carried the
                earlier repo+plan-audit's staged CLAUDE.md rewrite). Before
                it, 1e45953 fix(auth): enforce RBAC on every mutating
                route (audit finding #1) — full detail in CHANGELOG.md.
UNCOMMITTED   : this phase's own work, about to be committed as one
                logical commit — new apps/internal/clientip/{clientip.go,
                clientip_test.go} (clientip.go moved from the orphaned
                apps/apps/internal/clientip/, gofmt'd); config.go +
                .env.example's TRUSTED_PROXIES; apps/api/main.go and
                apps/tracker/main.go build and inject the resolver;
                httpserver/server.go, auth/{handler.go,security_handler.go,
                model.go}, tracker/handler.go consume it; ratelimit.go's
                ClientIP and its test deleted; auth/service.go's
                VerifyPendingLogin fix + auth_test.go's updated assertion;
                docs/security.md's new §5; CHANGELOG.md. Deliberately left
                out, unrelated and predating this phase: .env.example's
                OTHER pending changes are none (only TRUSTED_PROXIES
                touched it) — but ROADMAP.md, apps/e2e/scenario_test.go,
                apps/web/src/lib/api/networks.ts and offers.ts remain
                unstaged from the older repo+plan-audit pass; whoever
                picks those up next should check they're still wanted.
NEXT          : confirm scope before starting. Open from the audit:
                finding (4) API keys is a product decision (implement key
                auth, or remove the UI that promises it) more than a
                patch; (6) bump Go to 1.26.6 for the stdlib CVEs is a
                cheap, low-risk fix worth doing on its own; (7) moving
                shadcn's CLI to devDependencies is similarly small. Also
                open: permission-gating the frontend action buttons
                (finding-#1 phase) and setting TRUSTED_PROXIES to a real
                value once an actual reverse proxy/LB/CDN is deployed in
                front of apps/api and apps/tracker (finding-#2 phase).
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
