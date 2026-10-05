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
table). No feature in `apps/web` reads mock data today — the two files left
under `lib/mock/` are the role/permission vocabulary (`team.ts`) and the
report builder's filter-value suggestion pool (`analytics.ts`, a real
documented gap, below).

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

## Known open gaps

Real, deliberately-unbuilt work — not bugs. Each is a candidate for its own
scoped phase; none blocks anything that ships today.

| Gap | Where |
|---|---|
| `cloudflare_registrar` has no adapter, though Cloudflare works as a DNS provider | `apps/internal/domain/model.go` |
| SMS/email 2FA as an alternative delivery channel (TOTP only today) | `apps/internal/totp` |
| Seven feature areas are English-only in an EN/RU app | `content-gallery`, `custom-metrics`, `domains`, `referral`, `settings`, `tags`, `team` |
| Video upload/transcoding in Content Gallery (images only today) | needs a real ffmpeg/worker pipeline |
| Flows have no top-level list view, so no "View statistics" drill-in row | flows live nested inside a campaign's edit page |
| 4 of the report builder's 16 named dimensions still unbacked (`landing`/`pwa`/`postlanding`/`offer`) | need IDs threaded through `apps/tracker`'s event emission + a ClickHouse schema migration — `stream_set` (closed) shows the pattern once a column exists; these still need one |
| Tracker doesn't serve landings yet: flows' landing stage is configuration only, `LAND_VIEW`/`LAND_CLICK` never emitted | phase B of the ZIP-landings work — `apps/tracker` serves `landing.Render(...)`, records the events, click URL continues to the offer (`docs/landings.md`) |
| Google Play URL parsing for the PWA listing preview | would need a scraper behind `apps/internal/urlsafety` |
| `TRUSTED_PROXIES` still on its default (no real reverse proxy/LB/CDN deployed yet) | set it once real infrastructure exists in front of `apps/api`/`apps/tracker` |
| `golang.org/x/crypto/openpgp` (`GO-2026-5932`) has no upstream fix | permanent, accepted, unreachable — this codebase only imports `bcrypt`; see `docs/security.md` Known limitations |

Full acceptance criteria per phase: `docs/FLOX-master-prompt-v3.md`.
Definition of Done (applies to every phase): `CLAUDE.md` → "DEFINITION OF
DONE".
