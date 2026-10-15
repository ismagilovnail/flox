# ROADMAP

Mandatory development order (§9 of the master spec). One phase at a time —
see `CLAUDE.md` for the current phase and protocol. Status legend: `done` ·
`in progress` · `planned`.

**Every phase in §9's fixed order is complete.** All work past Phase 34 is
user-scoped and picked explicitly per session — §9 does not prescribe it, so
it is tracked in the second table rather than given invented phase numbers.
Statuses below were re-verified against the tree during the 2026-09-05 audit
(see CHANGELOG); the per-phase detail for each row is its own CHANGELOG entry.

## §9 phases

| Phase | Name | Status |
|---|---|---|
| 0 | Repository inspection | done |
| 1 | Product foundation | done |
| 2 | Design system | done |
| 3 | Application shell | done |
| 4 | Dashboard | done |
| 5 | Analytics UI | done |
| 6 | Campaign UI | done |
| 7 | Stream Set UI | done |
| 8 | Filter Builder | done |
| 9 | Flow Builder | done |
| 10 | Routing Simulator | done |
| 11 | Offers / Networks / Sources UI | done |
| 12 | Landing / PWA / Postlanding UI | done |
| 13 | Conversions / Postbacks / Pixels UI | done |
| 14 | Domains / Team / Settings | done |
| 14.5 | Tags (cross-entity) | done |
| 14.6 | Custom Metrics builder | done |
| 14.7 | Report Presets + directory stats | done |
| 14.8 | Referral Program | done |
| 14.9 | Content Gallery | done |
| 15 | Frontend integration architecture | done |
| 16 | Go backend foundation | done |
| 17 | PostgreSQL schema | done |
| 18 | Campaign API | done |
| 19 | Routing engine | done |
| 20 | Traffic classifier | done |
| 21 | Tracking engine | done |
| 22 | Attribution engine | done |
| 23 | Conversion engine | done |
| 24 | Postback engine | done |
| 25 | Analytics pipeline | done |
| 26 | ClickHouse analytics | done |
| 26.5 | LTV & cohort engine | done |
| 27 | Frontend/backend integration | done |
| 27-COST | Cost ingestion | done |
| 28 | Authentication / RBAC + tenant isolation | done |
| 29 | Observability | done |
| 30 | Security hardening | done |
| 31 | Performance optimization | done |
| 32 | E2E testing | done |
| 33 | Production deployment | done |
| 34 | Final audit | done |

Phases 2–14.9 were built frontend-first against mock contracts, as §83
requires. Phase 34's audit found seven of those had never been taken through
a backend-integration phase; every one has since been closed (see the second
table). No feature in `apps/web` reads mock data today — the one file left under
`lib/mock/` is the role/permission vocabulary (`team.ts`).

## Post-34, user-scoped

Not part of §9's order; each was scoped with the user at the start of its own
session.

| Work | Status |
|---|---|
| Phase 35 — real multi-dimension `/analytics` backend | done |
| Domains + Tracking Links CRUD | done |
| Campaign edit page two-column layout | done |
| Click Log (row-level `click_events`) | done |
| Geo/ASN provider (DB-IP Lite) | done |
| PWA live store-listing preview | done |
| Content Gallery — real media library (images) | done |
| Tags — real cross-entity backend | done |
| Custom Metrics — real backend | done |
| Settings → Organization + API Keys | done |
| Report Presets — real backend | done |
| Referral Program — real backend | done |
| Settings → Security (session timeout, IP allowlist, TOTP 2FA) | done |
| Repo-wide `accessorFn` stale-cache audit | done |
| Settings → Integrations (Namecheap / Cloudflare / Slack) | done |
| Google Ads cost integration | done |
| Registrar/DNS automation for Domains | done |
| Repo + plan audit | done |
| Security audit + finding #1 — RBAC enforcement on every mutating route | done |
| Security audit finding #2/#3 — trusted-proxy IP resolution + pending-login lockout | done |
| Security audit finding #4 — API keys: UI honesty pass, then real authentication | done |
| Security audit finding #6 — Go 1.26.6 + `golang.org/x/crypto` v0.56.0 | done |
| Security audit finding #7 — shadcn CLI moved to devDependencies | done |
| Frontend permission-gating (finding-#1 follow-up: buttons across all 5 write keys) | done |
| `offer-columns.tsx` `accessorFn` stale-cache fix (closed the one gap the repo-wide audit left open) | done |
| `MultiSelect` real `disabled` support (closed domain-form.tsx's permission gap) | done |
| `stream_set` analytics dimension (closed the stale "no stream_set_id on events" gap — the column already existed and was populated; only the whitelist/registry needed it) | done |
| `facebook_ads`/`tiktok_ads` `access_token` retrofitted to real AES-256-GCM encryption (Phase D, `docs/ad-account-connections.md`) — no schema change needed, the column was already `text` | done |
| General rate limit no longer stays IP-keyed for API-key traffic — `tenant.PeekIdentity` resolves identity before the limiter runs, keying by organization instead (`docs/security.md` §3) | done |
| Report builder's filter-value dropdown suggests real distinct values — new `POST /analytics/dimension-values` endpoint (`chstore.DistinctValues`); the mock pool now only backs the loading placeholder and the 5 still-unbacked dimensions | done |
| DataTable gets real column resizing (opt-in per-column, zero visual change until a user drags a handle) and CSV export (all filtered/sorted rows, not just the current page) — `components/ui/data-table.tsx`, repo-wide across all 22 call sites | done |
| Landings as uploaded ZIP bundles (reference: adset.pro custom landings) — upload/re-upload keeping the ID, logo/title, template editor with live preview, injected `trackUrl`/`engine_onClickButton` runtime (`docs/landings.md`) | done (phase A) |
| PWA stage served by the tracker: installable page (manifest + SW), PWA_VIEW/INSTALL/OPEN, WebView bounce, constructor "Test page", `pwa` report dimension — neutral page, no app-store imitation (`docs/pwa-serving.md`) | done |
| `offer` report dimension: `RouteResult.OfferID` (only when the click really went to the offer), `offer_id` on events (`chstore/schema/009`), through landing links and attribution; offers list "View statistics" fixed to pass the id | done |
| `landing` report dimension: `landing_id` column on click/tracking/conversion events, set by the tracker and carried through attribution; names instead of ids in the report UI (`docs/analytics-report-api.md`) | done |
| Tracker serves landings: internal pages rendered on `/t/`, signed `/lc/` click-through, first real `LAND_VIEW`/`LAND_CLICK` events; external landings redirect to their URL (`docs/landings.md`, "Serving visitors") | done (phase B) |
| Analytics: real `source` dimension; report presets keep filters, column order and widths; unsaved report edits survive a reload; PWA install / registration / purchase / repurchase metrics with step rates and cost per step; hidden columns saved with presets (`00047`) | done |
| Organization timezone is the default for analytics and the overview (`timezone` on `/analytics/dashboard` and `/breakdown`); Russian timezones in the picker; tzdata embedded in the API | done |
| Unique clicks count visitors (campaign + IP + User-Agent), not `click_id`s | done |
| Stream-set filter list: country and language dropdowns, language matches by subtag, ASN normalized; `region`/`city`/`proxy`/`connection_type` dropped from the list; end-to-end filter matrices (`apps/e2e/filters_test.go`) | done |
| Directory tables, Click Log and Overview remember columns and can reorder them; creating a network/offer/traffic source returns to the list; campaign statistics links | done |
| Tracking link builder with Facebook/TikTok macro presets (`lib/tracking-link-params.ts`) | done |
| Conversions list: offer, flow, tags, payout/currency, status history, hidden `utm_*`/`sub1`–`sub10`/USD/txn id columns | done |
| Marketer audit (`docs/audit-marketer-2026-10-06.md`) — findings only; #2 (timezone), #8 (link builder), #9 (conversions list) since closed | done |
| Dev compose subnet pinned to `10.99.0.0/24` (docker's auto-pick `192.168.0.0/20` shadowed the host LAN and broke host → container traffic) | done |
| Offer daily cap enforced in routing (`internal/offercap`, org-timezone day, capped offers leave the flow draw; audit #1); today's count and "cap reached" badge in the offers list, alerts | done |
| Blocked traffic and bots in analytics: `blocked`/`bots`/`blockedRate`/`botRate` metrics + four Overview tiles (audit #3) | done |
| HTTPS for tracking domains: Caddy on-demand TLS, worker SSL check, https links (audit #4, `docs/domains-tracking-links.md`) | done |
| LTV / Cohorts screen on the existing engine; Push and Reports removed from the menu until they exist (audit #5) | done |
| Overview: Top Offers card; the KPI comparison names the previous period it is against (audit #11) | done |
| No phase/spec references in interface texts (audit #6); settings, team, domains, referral, tags, custom metrics, content gallery translated — the whole app is EN/RU (audit #7) | done |
| Funnel metrics in the default analytics columns and the system default preset (in the org timezone); custom-metric formulas accept every report metric, kept in sync by a test (audit #12) | done |
| Alerts: spend over, ROI below, offer capped / paused, SSL expiring → the bell and Slack; rules under Settings → Alerts (audit #10, `docs/alerts.md`). Campaign/source reports keep rows that have spend but no events | done |

## Known open gaps

Real, deliberately-unbuilt work — not bugs. Each is a candidate for its own
scoped phase; none blocks anything that ships today.

| Gap | Where |
|---|---|
| Brave is indistinguishable from Chrome by User-Agent (audit #13) | platform limit, not fixable |
| `golang.org/x/crypto/openpgp` (`GO-2026-5932`) has no upstream fix | permanent, accepted, unreachable — this codebase only imports `bcrypt`; see `docs/security.md` Known limitations |
| `TRUSTED_PROXIES` still on its default (no real reverse proxy/LB/CDN deployed yet) | set it once real infrastructure exists in front of `apps/api`/`apps/tracker` |
| Restore drill against production | after the first deployment (`docs/backup.md`) |
| CI | the owner's decision (CLAUDE.md) |
| Renewing a Cloudflare Registrar domain from FLOX (read-only today: expiry and auto-renew) | Cloudflare's API has no renewal yet — `docs/domains-tracking-links.md` "Cloudflare Registrar" |
| SMS/email 2FA as an alternative delivery channel (TOTP only today) | `apps/internal/totp` |
| Video upload/transcoding in Content Gallery (images only today) | needs a real ffmpeg/worker pipeline |
| Pixels: a "send test event" button | sending works for Facebook/TikTok/Snapchat/generic — `docs/pixels.md` "Not done" |
| PWA types `external`/`ios_app`, landing `asPwa` | no target-URL fields exist for the two types; `docs/pwa-serving.md` "Not done" |
| Google Play URL parsing for the PWA listing preview | would need a scraper behind `apps/internal/urlsafety` |
| Telegram: changing a connected bot's token; TG_JOIN by channel in reports | `docs/telegram.md` "Not done" |
| A sidebar item named "Reports" (§17) | removed on purpose (audit #5): the report builder is Analytics |

Closed since this list was first written (2026-10-11 → 2026-10-15): push
(scheduled, triggered, recurring broadcasts; metrics in Push → Statistics,
custom metrics and the report builder), cost CSV import and per-country
spend, reports in the organization's currency, LTV filters by source and
offer and the decline/trash metrics, Content Gallery system content, tags
on flows, flow-node analytics, the empty-subs diagnostic, API-key
rotation, wider audit logs, the inactive-campaign tracker test, "Copy
tracking URL" in the campaign row, IOS_INSTALL and TG_* events with
pixels, DataGrid, and the `types/`/`schemas/` layers — see CHANGELOG.md.

Full acceptance criteria per phase: `docs/FLOX-master-prompt-v3.md`; their
state on 2026-10-11, phase by phase: `docs/spec-conformance-2026-10-11.md`.
Definition of Done (applies to every phase): `CLAUDE.md` → "DEFINITION OF
DONE".
