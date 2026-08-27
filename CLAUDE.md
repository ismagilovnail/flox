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
CURRENT PHASE : PHASE 30 — SECURITY HARDENING
STATUS        : done — §54's four defenses, each closing a gap that had
                zero authentication/throttling/host-validation before
                this phase. Full detail in docs/security.md; summary:
                (1) Incoming postback auth: networks.postback_secret_hash
                (migration 00021, hashed at rest, DEFAULT '' sentinel
                that can never match any hashed input). apps/tracker's
                POST /postback/{networkId} used to trust the path ULID
                alone; now requires ?secret=... checked with
                subtle.ConstantTimeCompare against the stored hash
                before recording any conversion. Raw secret is returned
                exactly once — network.Service.Create's CreateResult and
                the new RegeneratePostbackSecret response — never from
                List/Get/Update/Duplicate. Duplicate gets its own fresh
                secret, never the source's. apps/web: PostbackSecretDialog
                (reveal-once, copy button) after create and after the new
                "Regenerate incoming secret" row action.
                (2) CSRF: new tenant.RequireSameOrigin closes the gap
                SameSite=Lax alone doesn't (a cross-site fetch/XHR with
                credentials:"include" can still attach a Lax cookie in
                some browsers) — rejects a mutating request whose Origin
                header is PRESENT and != APP_URL; an ABSENT Origin is
                allowed (non-browser client, not a browser silently
                omitting it). Mounted before session resolution in
                apps/api/main.go, standalone on pre-session signup/
                login/accept-invite too (blocks login CSRF).
                (3) Rate limiting: new apps/internal/ratelimit, a fixed-
                window Redis counter, FAILS OPEN on a Redis error (same
                stance as every other Redis-optional path in this
                codebase — a limiter that failed closed would turn a
                Redis outage into a full API outage). General 300/min-
                per-IP across all of apps/api (httpserver, exempting
                /health//ready//metrics via an exemptPaths wrapper — see
                Fixed below), a stricter 20/15min-per-IP on
                /auth/login+/auth/signup. Deliberately NOT on apps/
                tracker's redirect hot path (§41 budget) — a synchronous
                Redis round trip on every click would blow it outright.
                (4) SSRF: new apps/internal/urlsafety. ValidateURL is
                the cheap save-time check (network.Service create/
                update: scheme + literal-IP-range rejection).
                SafeDialContext is the real defense — set as apps/
                worker's postback-delivery http.Client's Transport.
                DialContext, it resolves the host and validates the IP
                AT CONNECTION TIME, then dials that exact validated IP
                (never the hostname again), closing the DNS-rebinding
                gap a save-time-only check can't catch (a hostname
                resolving to a public IP at save time can be repointed
                at a private/metadata IP by delivery time). Forbidden:
                loopback/private/link-local(+169.254.169.254 metadata)/
                unspecified.
                FIXED DURING THIS PHASE'S OWN VALIDATION: apps/internal/
                httpserver.New originally called r.Use(limiter...) AFTER
                /health//ready//metrics were already registered on the
                same chi mux — chi panics on that ordering ("all
                middlewares must be defined before routes"), so apps/api
                failed to start at all. The original comment's reasoning
                ("Use only affects routes added after it in the same
                router scope") is true for an inline Group/With sub-
                router but NOT for the root mux itself. Caught because
                this phase's manual pass actually tried to start the
                real binary rather than stopping at go build (build/vet
                don't catch a startup-time panic). Fixed: Use moved
                before route registration; a new exemptPaths(mw,
                paths...) wrapper keeps the three infra routes bypassing
                the limiter without depending on registration order.
                Verified: gofmt/go build/go vet/go test -count=1 ./...
                green repo-wide (forced, not cached), incl. 4 new test
                files (internal/ratelimit, internal/urlsafety,
                internal/tenant/tenant_test.go, tracker/
                postback_secret_test.go) + new cases in internal/
                network/network_test.go. tsc --noEmit/eslint/next build
                clean on apps/web. Full manual pass against the REAL
                running api+tracker binaries (after fixing the startup
                panic above, migration 00021 already applied to the dev
                DB): signed up a real user, created a real network,
                confirmed the returned postbackSecret gates apps/
                tracker's postback endpoint end to end (missing secret
                -> 401, wrong secret -> 401, correct secret -> passes
                auth and reaches real business logic), confirmed
                RegeneratePostbackSecret invalidates the old secret
                immediately and the new one works. Confirmed
                RequireSameOrigin: cross-origin Origin -> 403, matching-
                origin -> 200s/normal errors, no Origin -> also passes
                (curl, no browser). Confirmed urlsafety.ValidateURL
                rejects both a cloud-metadata URL (169.254.169.254) and
                a loopback URL at network-create time (422). Confirmed
                the /auth/login rate limit actually trips (429) once the
                20-per-15min threshold is crossed. All fixtures (user,
                org, network, sessions) deleted afterward, confirmed
                zero rows remaining.
LAST COMMIT   : feat(security): postback secrets, CSRF, rate limiting,
                SSRF protection (Phase 30)
NEXT          : confirm scope before starting. No open known issues
                remain for Security Hardening. Known limitations
                documented in docs/security.md: no CAPTCHA/device-
                fingerprinting on auth endpoints (rate limiting is the
                only brute-force defense); rate limiting is per-IP only,
                not distributed-attacker-aware (an infra/WAF concern);
                no CSP/security-header hardening (out of scope, not
                requested).

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
