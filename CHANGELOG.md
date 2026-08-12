# CHANGELOG

Format follows [Keep a Changelog](https://keepachangelog.com/). Entries are
per-phase, matching `CLAUDE.md`'s phase protocol.

## [Phase 18] — Campaign API

### Added

- **Campaign API** (§37): `internal/campaign/` — `handler.go` → `service.go`
  → `repository.go` → `model.go`, per CLAUDE.md's Go architecture (thin
  handlers, business logic in the service, repository is just SQL).
  `GET/POST /campaigns`, `GET/PATCH/DELETE /campaigns/:id`, `POST
  /campaigns/:id/{duplicate,pause,activate}`. Every query org-scoped
  (§36-TENANCY).
- **`internal/tenant`**: `X-Organization-Id`-header middleware — the
  session-derived org-scoping stand-in until Phase 28 auth lands (fully
  documented as a deliberate, temporary substitute in `apps/api/README.md`,
  not a shortcut: handlers physically cannot read org scope from anywhere
  else, which is the property §36-TENANCY actually protects).
- **Cross-tenant FK check**: a campaign's `traffic_source_id` foreign key
  only proves that row exists *somewhere* — nothing stops it belonging to a
  different org unless checked explicitly. `Repository.TrafficSourceBelongsToOrg`
  verifies it against the caller's org before every create/update; this is
  the pattern every future domain package with a cross-table reference
  needs to repeat.
- **`internal/apierror`**: one JSON error envelope (`{code, message,
  fields?}`) shared by every handler, so `/campaigns` and every future
  `/offers`, `/networks`, etc. render errors the same shape.
- **`internal/idgen`**: ULID generation (`oklog/ulid/v2`) + the same
  format validation `apps/api/migrations`'s `ulid` domain enforces, so an
  application-generated id and a database CHECK constraint can never
  disagree about what's valid.
- **`internal/postgres`**: pgx pool constructor, pinged once at startup so
  a misconfigured `DATABASE_URL` fails fast instead of surfacing on the
  first request.
- **`GET /ready` now genuinely pings Postgres** — the check Phase 16
  promised ("starts checking real dependencies once Phase 17+ wires them
  in") and deliberately hadn't faked yet. Returns `503` with the failing
  check named if the ping fails.
- **Pause/activate domain rules** (§37: "validate using domain rules," not
  bare setters): idempotent from the target state, rejected with `409` from
  `archived` — an archived campaign has to be explicitly edited back to
  another status via `PATCH`, not casually reactivated by a toggle.
  `Duplicate` mirrors the frontend's `stores/campaigns.ts`: `"{name}
  (Copy)"`, forced back to `draft`, stats/tracking reset.
- **Cross-tenant isolation test** (CLAUDE.md DoD requirement for API
  phases): `internal/campaign/repository_test.go`, gated on `DATABASE_URL`
  (skips cleanly without it — `go test ./...` never needs a live DB).
  Creates a campaign for org A, proves org B's list/get/update/delete all
  see nothing of it, and that org A can't attach a campaign to org B's
  traffic source.

### Fixed

- **Test-cleanup bug in the isolation test itself**, caught before this
  phase closed: `defer pool.Close()` in the test body runs at function
  return, which is *before* `t.Cleanup`-registered callbacks run — so the
  pool was already closed by the time each seeded test org's delete-cleanup
  query fired, silently leaking `organizations`/`campaigns` rows into the
  dev database on every test run (confirmed leaked rows via `psql`, fixed
  by registering the pool's close via `t.Cleanup` instead, ordered by LIFO
  to run after the org-delete cleanups, re-verified via row counts before
  and after a run).
- **`internal/tenant`'s missing-header error** originally used a hand-rolled
  JSON string that didn't match `apierror`'s envelope shape — caught via
  manual curl testing, fixed to construct a real `apierror.Error`.

### Known issues

- None new. Phase 10's unresolved crash-loop report carries over
  (unrelated to this phase). `apps/tracker`/`apps/worker` module topology
  remains an open decision (documented Phase 16, still unresolved, not
  blocking).

### Files changed

- `apps/api/internal/campaign/*` (new)
- `apps/api/internal/{tenant,apierror,idgen,postgres}/*` (new)
- `apps/api/internal/httpserver/{server.go,health.go}` (modified — DB ping, route mounting)
- `apps/api/cmd/api/main.go` (modified — wires DB pool + campaign routes)
- `apps/api/README.md`, `docs/architecture.md` (modified — Phase 18 sections)

## [Phase 17] — Database

### Added

- **10 goose migrations** in `apps/api/migrations/` implementing §35's full
  core table list: `organizations`, `users`, `memberships`, `roles`,
  `permissions`, `traffic_sources`, `campaigns`, `stream_sets`,
  `filter_groups`, `filter_conditions`, `flows`, `landings`, `pwas`,
  `postlandings`, `networks`, `offers`, `offer_links`, `domains`,
  `tracking_links`, `pixels`, `postbacks`, `cost_entries`, `fx_rates`,
  `api_keys`, `audit_logs` — plus `stream_set_pixels`, a join table for
  `StreamSet.pixels: string[]` that the explicit table list didn't spell
  out but a many-to-many relationship needs. `go tool goose` (vendored as a
  Go tool dependency, no separate install).
- **ULID everywhere**: a `ulid` Postgres domain (text + Crockford-base32
  format CHECK) defined once in `00001`, used as every table's primary key
  — "one standard, consistently, everywhere" per §35, not a mix.
- **`organization_id` denormalized onto every tenant-scoped table,
  including child tables** (`offer_links`, `filter_conditions`, `flows`,
  `stream_set_pixels`, …) rather than left implicit via a join to the
  parent — §36-TENANCY calls cross-tenant isolation "a hard security
  invariant, not a convention," and a repository query that filters a
  child table directly by `organization_id` can't leak data even if a
  future join condition is written wrong.
- **`updated_at` maintained by a trigger** (`set_updated_at()`), not
  application code, on every mutable table — can't go stale from a
  forgotten `SET updated_at = now()`.
- **Filter tree**: `filter_groups` (self-referencing via `parent_group_id`,
  `stream_set_id` denormalized onto every node) + `filter_conditions`,
  matching the frontend's recursive `FilterGroupNode`/`FilterCondition`
  shape (`lib/filters.ts`) exactly enough that Phase 27 integration should
  be a straight read/write mapping, not a redesign.
- **`flows`**: the frontend's `LandingStage`/`PwaStage`/`PostlandingStage`/
  `Destination` structs flattened into nullable columns gated by their own
  `*_enabled`/`destination_kind` flag, with a CHECK constraint enforcing
  the destination discriminated-union shape (`offer` requires network+offer
  IDs and no URL; `redirect` requires a URL and no network/offer IDs).
- **`postbacks`**: the durable dedup ledger for §45/non-negotiable
  invariant #3 — `UNIQUE (organization_id, click_id, status)`, with a
  partial-index exemption (`WHERE NOT network_accepts_duplicates`) for
  networks with the `acceptDuplicates` override. Deliberately does *not*
  duplicate the rich per-attempt log (message, payload) — that belongs in
  ClickHouse's `postback_events`, outside this Postgres-only phase.
- **`cost_entries`**: one row per (campaign, traffic_source-or-none, day),
  enforced with two partial unique indexes rather than a
  `COALESCE`-sentinel expression index (avoids needing a fake placeholder
  ULID to satisfy the `ulid` domain's format CHECK).
- **`fx_rates`**: the one non-`organization_id`-scoped, non-ULID table —
  natural composite key `(currency, rate_date)`, since an exchange rate is
  an objective market fact, not something each tenant has its own copy of.
- **`campaigns` has no `tracking_domain`/`tracking_id` columns** even
  though the frontend mock has them — that's a `(campaign, domain, slug)`
  row in `tracking_links` instead, so one campaign with links on multiple
  domains never desyncs a domain string stored in two places.
- No tables for Tags/Custom Metrics/Report Presets/Referral/Content
  Gallery — those v3 "secondary" frontend phases (14.5–14.9) aren't in
  §35's core table list; not guessed at here.

### Validated

- Brought up a real Postgres via `infra/docker-compose.dev.yml`, ran
  `goose up` (all 10 migrations, clean), `goose down-to 0` (full rollback,
  clean — zero orphaned objects, confirming every `-- +goose Down` block is
  correct), then `goose up` again.
- Ran a full smoke-test transaction: inserted a complete two-org dataset —
  campaign → stream_set → nested AND/OR filter tree → two flows (one
  `offer` destination, one `redirect`) → domain/tracking_link → pixel →
  cost_entry → api_key → audit_log → postback — for Org A, then verified
  Org B's queries return zero rows against every one of those tables
  (cross-tenant isolation, DoD requirement for data-model phases). Also
  verified, as expected failures: the postback dedup constraint rejects a
  true duplicate but the `accept_duplicates` override allows one; the
  `flows` destination CHECK rejects a malformed `offer` row with a URL set;
  the `cost_entries` dedup constraint rejects a same-day double-entry. A
  recursive CTE read-back of the filter tree round-trips correctly.
- `go build`/`vet`/`gofmt` clean.

### Fixed

- N/A this phase — all defects (invalid Crockford-charset ULIDs in seed
  data during authoring, a fragile `COALESCE`-sentinel unique index, a
  psql `\gset`-inside-`DO`-block scripting mistake in the smoke test) were
  caught and fixed before anything ran against real Postgres, not found
  as bugs afterward.

### Known issues

- None new. Phase 10's unresolved crash-loop report carries over
  (unrelated to this phase). `apps/tracker`/`apps/worker` module topology
  remains an open decision (documented Phase 16, still unresolved, not
  blocking).

### Files changed

- `apps/api/migrations/00001..00010_*.sql` (new)
- `apps/api/migrations/README.md` (rewritten — command reference + conventions)
- `apps/api/go.mod`/`go.sum` (modified — goose tool dependency)
- `apps/api/README.md` (modified — migration commands)
- `docs/architecture.md`, `docs/domain-model.md` (modified — Phase 17 section)

## [Phase 16] — Go Backend Foundation

### Added

- **`apps/api`** (§33): new Go module (`github.com/ismagilovnail/flox/apps/api`,
  Go 1.26) — the first backend code in this repo. `cmd/api/main.go` wires
  config → logging → OpenTelemetry → HTTP server → graceful shutdown on
  SIGINT/SIGTERM.
- **`internal/config`**: loads every env var already declared in
  `.env.example` (§7/§17/§33). Only `NODE_ENV`/`API_URL`/`LOG_LEVEL`/OTel
  vars are actually consumed this phase; `DATABASE_URL`/`CLICKHOUSE_*`/
  `REDIS_URL`/`S3_*` are parsed onto `Config` now so later phases don't need
  to touch this file again, but nothing connects to them yet.
- **`internal/logging`**: `slog.Logger`, JSON handler, level from config.
- **`internal/telemetry`**: OpenTelemetry `TracerProvider` with an OTLP/HTTP
  exporter. No-op (not an error) when `OTEL_EXPORTER_OTLP_ENDPOINT` is
  unset — OTel is observability, not a hard dependency for the API to run.
  Verified the server starts and serves cleanly both with the endpoint unset
  and with it pointed at a collector that isn't actually running.
- **`internal/httpserver`**: chi router — request ID (+ echoed back as an
  `X-Request-Id` response header for client-side correlation), real IP,
  structured per-request logging (tagged with the request ID), panic
  recovery, 30s timeout, OTel HTTP instrumentation.
- **`GET /health`** — liveness only, always `200`. **`GET /ready`** —
  readiness; returns `200` unconditionally for now. It does not fake a
  dependency check: Postgres/ClickHouse/Redis don't exist until Phase 17+,
  and a check that doesn't check anything is exactly the "fake API that
  looks real" CLAUDE.md forbids. Starts checking real dependencies once
  those phases wire them in.
- **`infra/docker-compose.dev.yml`**: Postgres, ClickHouse, Redis, MinIO
  (S3-compatible) for local dev, credentials matching `.env.example`
  exactly so `cp .env.example .env` + `docker compose up` need no edits.
  Brought the full stack up and verified all four healthchecks pass;
  caught and fixed one real bug along the way — the ClickHouse healthcheck
  used `wget http://localhost:8123/ping`, which resolves `localhost` to
  `::1` inside the container and fails since the image's HTTP server isn't
  listening on IPv6; changed to `127.0.0.1`.
- `.env.example`: filled in `S3_ACCESS_KEY_ID`/`S3_SECRET_ACCESS_KEY` with
  the MinIO root credentials the compose file actually uses (throwaway
  local-dev values, not real secrets) so the two files stay consistent.

### Scope decision — module topology for `apps/tracker`/`apps/worker` is deferred, not solved

`ARCHITECTURE.md` says `apps/tracker`/`apps/worker` share `internal/routing`/
`internal/classifier` with `apps/api` as "the same Go module." §33 literally
places this module's `go.mod` at `apps/api`, and Go's internal-import
visibility rule means sibling directories (`apps/tracker`, `apps/worker`)
structurally cannot import `apps/api/internal/...` regardless of module
setup. Not a Phase 16 problem — routing/classifier logic doesn't exist until
Phase 19/20 — documented as an open decision in `apps/api/README.md` and
`docs/architecture.md` for whoever starts Phase 21, with the two concrete
options already spelled out (move the module root to `apps/`, or keep
routing/classifier under `pkg/` instead of `internal/`).

### Fixed

- ClickHouse dev-compose healthcheck (`localhost` → `127.0.0.1`), described
  above — the only defect found during this phase's validation.

### Known issues

- None new. Phase 10's unresolved crash-loop report carries over
  (unrelated to this phase).

### Files changed

- `apps/api/{go.mod,go.sum,cmd,internal,migrations/README.md,pkg/README.md,README.md}` (new)
- `infra/docker-compose.dev.yml` (new)
- `.env.example` (modified — S3 credentials filled in)
- `docs/architecture.md` (modified — Phase 16 section)

## [Phase 15] — Frontend Architecture

### Added

- `apps/web/src/hooks/` — the first genuinely cross-feature hook,
  `useCurrentMember()`, replacing an identical `CURRENT_USER_MEMBER_ID` +
  `useTeamStore((s) => s.members.find(...))` + Owner/Admin check that had
  been copy-pasted into Referral (14.8), Content Gallery (14.9), and Custom
  Metrics (14.6) independently. Single source of truth for "who am I, what
  can I manage" until real auth/sessions land in Phase 28.
- `apps/web/src/lib/api/routing.ts` — the frontend API boundary (§32)
  starts here, not as 20 speculative per-domain clients. `docs/architecture.md`
  already promised the Routing Simulator "runs against a local mock that
  implements the exact same request/response contract... in Phase 27 it is
  switched to the real endpoint with no UI changes" — but the Phase 10 call
  site was a plain synchronous function, which would have broken that
  promise (swapping sync for `fetch` always forces UI changes). It's now a
  promise-returning wrapper around the same pure mock (`lib/routing-simulate.ts`),
  with a real loading state (`SimulatorForm`'s button now disables and reads
  "Simulating..." mid-flight) — so Phase 27 only ever touches this one
  file's body.

### Scope decision — no `src/lib/api/<domain>.ts` per store, no empty `types/`/`schemas/` folders

Audited the full `apps/web/src` tree against CLAUDE.md's recommended
`app/ components/ features/ hooks/ lib/ stores/ types/ schemas/` layout.
Findings:

- **No `fetch()` exists anywhere in the codebase** (grepped) — so "don't
  scatter fetch() in components" is vacuously satisfied today. Every
  component already reads/writes data exclusively through a Zustand store's
  typed action surface, never directly — that store action surface **is**
  each domain's mock API contract, per CLAUDE.md's own REPO LAYOUT section
  listing `stores/` as a sanctioned top-level layer. Wrapping every existing
  store (campaigns, offers, networks, landings, pwa, postlanding,
  conversions, postbacks, pixels, domains, team, settings, tags,
  custom-metrics, referral, content-gallery, …) in an additional
  `lib/api/<domain>.ts` pass-through now would be a same-shaped duplicate
  with no consumer and nothing to swap it against — real per-domain API
  integration is explicitly Phase 27's job (CLAUDE.md BUILD ORDER: "Design &
  UI on mock contracts first (phases 2–15)... then integration (27)"), and
  building 20 client stubs 12 phases early is exactly the "never build
  ahead" / "no unnecessary frameworks" this file warns against. Routing
  (above) is the one exception because a concrete promise about it already
  existed in `docs/architecture.md` from Phase 0.
- **Zod schemas and mock types stay co-located per-feature**
  (`features/*/x-form-sheet.tsx`, `lib/mock/*.ts`) rather than moved into
  new top-level `types/`/`schemas/` folders. This already satisfies "domain-
  specific code belongs inside features" — a Landing form's schema is
  exactly as domain-specific as the Landing form itself. Top-level
  `types/`/`schemas/` directories would only be justified by types genuinely
  shared *across* features (e.g. a future OpenAPI-generated contract), which
  don't exist yet; creating them empty now would be pure scaffolding.
- Audited every `useXStore((s) => s.method(...))` selector call site for the
  Phase 13 stale-snapshot crash pattern (a selector that itself does
  `.filter()`/`.map()` breaks `useSyncExternalStore`). All are either
  `.find()` (safe) or already explicitly guarded with a stable-reference
  cache (`stream-sets.ts`'s `listByCampaign`, commented from when that was
  fixed pre-dating this conversation). No latent bugs found.

### Fixed

- N/A this phase — no defects found; the two mock-list-view regressions
  after the `useCurrentMember()` refactor (Referral, Content Gallery, Custom
  Metrics) and the routing simulator's new async path were all verified
  clean in the browser, no console errors.

### Known issues

- None new. Phase 10's unresolved crash-loop report carries over
  (unrelated to this phase).

### Files changed

- `apps/web/src/hooks/use-current-member.ts` (new)
- `apps/web/src/lib/api/routing.ts` (new)
- `apps/web/src/features/{referral,content-gallery,custom-metrics}/*` (modified) — use `useCurrentMember()`
- `apps/web/src/features/routing-simulator/*` (modified) — async simulate call + loading state
- `apps/web/src/lib/routing-simulate.ts` (unchanged — now called from `lib/api/routing.ts` instead of directly)

## [Phase 14.9] — Content Gallery

### Added

- **Content Gallery** (§30.8) at `/content-gallery`: a browsable, searchable,
  categorized library of system-provided templates and creative assets, plus
  team uploads. Four categories: landing/PWA/postlanding templates, and
  creative assets. Category and source (All/System/Team) filters, plus a
  free-text search over title/description/tags.
- **Real hand-off, not a simulated one**: each template's `GalleryItem`
  carries a payload shaped exactly like the target builder's form
  `defaultValues` (from the Phase 12 Landing/PWA/Postlanding form sheets).
  "Use this" navigates to `/landings|pwa|postlanding?gallery=<id>` — reusing
  the same "URL hands off a fully-formed query" pattern Phase 14.7's View
  Statistics established — and the target list page opens its existing
  create sheet pre-filled from the template, sheet title annotated "— from
  {template}". No parallel template-authoring UI was built; the existing
  builders do the work.
- **No real asset library exists yet** (Phase 12 never built one), so a
  creative asset's "Use this" copies its hosted URL to the clipboard instead
  of a builder hand-off — an honest minimal action rather than inventing a
  second, unbuilt system.
- **Team uploads** (§36-TENANCY: private to the workspace) are scoped to
  creative assets only — there's no template-authoring flow to upload a
  landing/PWA/postlanding template into, so upload is restricted to the one
  category that makes sense today. No real object storage in this
  frontend-first phase: an upload records an already-hosted URL, the same
  "URL stands in for upload" convention already used for PWA icons — real S3
  integration is Phase 27's job, not this one's. The uploader can remove
  their own upload; Owner/Admin can remove any team upload. System items are
  read-only.
- Preview tiles are generated CSS gradients + a category icon, not fake
  hosted images — the gallery has no real image pipeline, and CLAUDE.md
  forbids mock APIs that look real.
- Seeded 10 system items (3 landing, 2 PWA, 2 postlanding templates, 3
  creative assets) and 2 team items, across all four categories.

### Fixed

- N/A this phase — full flow verified in the browser: search, category/source
  filters, preview, "Use this" into all three builders (landing tested
  end-to-end through actual creation), asset URL copy, team upload, and
  team-item removal. No console errors.

### Known issues

- None new. Phase 10's unresolved crash-loop report carries over
  (unrelated to this phase).

### Files changed

- `apps/web/src/lib/mock/content-gallery.ts` (new) — data model + seed data
- `apps/web/src/stores/content-gallery.ts` (new) — Zustand store
- `apps/web/src/features/content-gallery/*` (new) — view, card, preview tile,
  preview dialog, upload dialog
- `apps/web/src/app/(app)/content-gallery/page.tsx` (modified) — wired to `ContentGalleryView`
- `apps/web/src/features/{landings,pwa,postlanding}/*-list.tsx` (modified) —
  read `?gallery=<id>` to pre-fill the create sheet from a template
- `apps/web/src/app/(app)/{landings,pwa,postlanding}/page.tsx` (modified) —
  wrapped in `<Suspense>` for `useSearchParams()`

## [Phase 14.8] — Referral Program

### Added

- **Referral dashboard** (§27.6/§30.7) at `/referral`: one referral
  account per team (tenant-scoped, matching every other §36-TENANCY
  surface), not per-user — FLOX pays a commission to the workspace for
  referring other advertisers to the platform, so it's scoped like the
  workspace itself. `referralCode`/`referralLink` derived from the org
  name via the existing `slugify()` helper; copy-to-clipboard with a
  toast confirmation.
- **Referred Signups** table: name, email, status badge
  (invited/signed_up/converted), relative signup date.
- **Earnings History**: an immutable, append-only transaction ledger
  (§54-style audit trail) — `accrual` | `adjustment` | `payout_paid`
  entries, each attributed to a team member and timestamped. Balances
  are never stored or edited directly; `computeBalances()` derives
  Total earned / Total paid / Pending payout / Available balance from
  the transaction + payout logs via a pure function, so the numbers can
  never drift out of sync with the log that produced them.
- **Payouts** state machine: `pending → approved → paid`, plus a
  `rejected` branch (a reasonable low-cost extension of the spec's
  linear 3-state description — every approval flow needs a "no").
  Owner/Admin (reusing Team's canonical roles, not a new per-feature
  role vocabulary) can approve, reject (with a required reason), or
  mark paid; marking paid appends a matching negative `payout_paid`
  ledger entry. Terminal states (`paid`/`rejected`) render no actions.
- **Request payout** dialog, pre-filled to the full available balance,
  validated `0 < amount <= availableBalance`. **Add adjustment** dialog
  (Owner/Admin only): signed amount + required reason, for manual
  corrections.
- All amounts are flat USD — deliberately *not* run through §50-FX
  original-currency/event-date normalization, since referral payouts
  are FLOX's own commission structure, not tenant traffic revenue.

### Fixed

- N/A this phase — full request → approve → mark-paid flow, balance
  math, and ledger immutability all verified correct in the browser on
  the first pass; no console errors.

### Known issues

- None new. Phase 10's unresolved crash-loop report carries over
  (unrelated to this phase).

### Files changed

- `apps/web/src/lib/mock/referral.ts` (new) — data model + `computeBalances()`
- `apps/web/src/stores/referral.ts` (new) — Zustand ledger store
- `apps/web/src/features/referral/*` (new) — dashboard, tables, dialogs, row actions
- `apps/web/src/app/(app)/referral/page.tsx` (modified) — wired to `ReferralDashboard`

## [Phase 14.7] — Report Presets + Directory Stats

### Added

- **Report Presets** (§27.5), built on top of the Phase 5 Analytics
  Explorer rather than a parallel report system: save the current
  {dimensions ("columns"), metrics, groupBy ("grouping"), period,
  timezone} as a named preset, apply it back, edit (rename + resave
  current config), delete. One system default preset (starred, can't be
  renamed or deleted) is seeded and visible to everyone.
- `period` is stored **relative** ("last 7/30/90 days") wherever the
  current date range matches a known relative window, not as a frozen
  from/to pair — reapplying a preset next month still means "the last 30
  days," not a stale historical window. Falls back to a fixed custom
  range only when the current selection doesn't match a relative window.
  Because the mock "now" is a fixed constant already used throughout
  `analytics-view.tsx`, reapplying a preset is fully deterministic —
  directly satisfying "applying a preset reproduces an identical report."
- **View Statistics directory drill-in** (§27.5): a row action on
  Networks, Offers, and Traffic Sources (the three of the spec's four
  named surfaces — Campaigns, CPA Networks, Offers, Flows, Traffic
  Sources — that have both a real list view and a stable entity name;
  Flows skipped for the same reason Phase 14.5 skipped them: no Flows
  list view exists to put a row action on) that hands off a fully-formed
  report query via URL search params (`/analytics?dim=&val=&tab=line`) —
  a navigation + pre-filter, per spec, not a client-side recompute.
  `AnalyticsView` reads `dim`/`val`/`tab` on mount (validated against a
  small allowlist) to seed its initial filter and default to the Line
  tab, which already aggregates by day — satisfying "grouping by day"
  with the chart that already exists rather than inventing a new
  dimension. `analytics/page.tsx` wraps the view in `<Suspense>`, which
  `useSearchParams()` requires.
- **Fixed a pre-existing, silent mock-data disconnect while wiring this
  up**: `mock/analytics.ts`'s `network`/`offer` dimension pools (Phase 5)
  were self-contained fake names ("MaxBounty", "Sweeps Gold US", …) that
  never matched the real Network/Offer entities Phase 11 introduced
  later — so a "View Statistics" link built against real entity names
  would have landed on a correctly filtered but permanently *empty*
  report. Realigned both pools to the real seeded names (same array
  length, pure string swap, no change to the generator logic); `source`
  already coincidentally matched Traffic Source names, left as-is; `flow`
  left untouched since Flows aren't wired to this feature.

### Fixed

- N/A this phase (see "Added" above for the mock-data alignment, which
  is a data-content fix rather than a code-logic fix).

### Known issues

- Full browser smoke test passed (extension connected): applied both
  seeded presets and confirmed dimensions/metrics/groupBy/date-range all
  updated correctly; saved a new preset from the current view and
  confirmed it appeared with edit/delete controls (the default preset
  correctly has neither); "View Statistics" from both a Network and an
  Offer row landed on `/analytics` with the right filter chip, the Line
  tab active, and non-empty real trend data. No console errors.
- Report Presets don't capture `filters` or `sort`/`compare` — only the
  five fields §27.5 explicitly lists ({columns, metrics, grouping,
  period, timezone}). This is a deliberate scope match to the spec's
  literal field list, not an oversight, but worth knowing if a preset
  seems to "forget" an ad hoc filter you had applied when you saved it.

## [Phase 14.6] — Custom Metrics builder

### Added

- **`src/lib/formula-engine.ts`** — a real tokenizer/parser/evaluator/
  validator (§30.5), not an `eval()`/`Function()` shortcut. Operators
  `+ − × ÷ ( )` plus comparisons; functions `DIV, EMPTYIF, IF, ROUND, ABS,
  MIN, MAX` with contextual hints; `{metric_id}` tokens. Division is safe
  everywhere — bare `/` and `DIV()` alike — division by zero or a null
  input yields empty, never throws, exactly as mandated. `validateFormula`
  enforces the single-data-source constraint and rejects LTV metrics
  outright, both explicitly required by §30.5.
- **Critical finding, not duplicated**: `features/analytics/registry.ts`
  (Phase 5) already *is* the §50 Metrics Registry — its own comment says
  so ("matches §50 — never recompute ad hoc elsewhere"). The Custom
  Metrics catalog (`lib/mock/custom-metrics-registry.ts`) re-exports its
  13 `METRICS` verbatim as the live "Traffic / Performance" category and
  only *adds* what Phase 5 didn't need: CPA Funnel, Fraud (`bots`,
  `click_all` — §30.5's own Bot Share example uses these ids), and Push,
  all catalog-only (`live: false`, no tracker/Push module exists yet to
  compute them) — plus LTV, catalog-only **and** `insertable: false`,
  the one category §30.5 forbids in formulas outright, not just
  "unimplemented."
- Formula Builder Sheet: searchable/grouped metric catalog with
  click-to-insert-at-cursor (LTV entries shown disabled with a tooltip,
  not hidden — the constraint stays discoverable), an operator/function
  toolbar, a contextual hint when the cursor is inside a function call,
  live green-check/red-X validation, and a live preview computed against
  representative sample values. Name/group (existing-or-new)/format,
  Show-in targets, Draft/Published.
- "Show in" targets scoped to 4 concrete, real surfaces (Report Builder,
  Campaigns Table, Offers Table, Traffic Sources Table) — each checkbox is
  disabled unless every metric the formula references is actually
  available as data on that surface. Offers/Sources currently have no
  analytics numbers at all (no tracker), so those two are honestly almost
  always disabled rather than faked — a real demonstration of §30.5's
  "never render empty zeros where it cannot be computed" rule, not a cop-out.
- Seeded two metrics illustrating both lifecycle states genuinely: "Margin
  per Click" (§30.5's own example, Published + Active, live on Report
  Builder and Campaigns Table) and "Bot Share" (§30.5's other example,
  kept as **Draft** because `{bots}`/`{click_all}` aren't tracked by
  anything yet — an honest use of "drafts are invisible until published,"
  not a placeholder that pretends to work).
- Lifecycle/governance: Draft/Published, an independent Active toggle
  ("hide from pickers without deleting"), duplicate, and deletion blocked
  while published or exposed on any surface (archive/deactivate instead)
  — enforced in `stores/custom-metrics.ts`.
- Role access reuses Team's existing canonical roles
  (Owner/Admin/Manager/Buyer/Analyst/Viewer, §52) rather than inventing
  §30.5's separate Owner/Tech/Lead/Buyer vocabulary: Owner/Admin manage
  any metric, Manager creates and manages only their own, Buyer/Analyst/
  Viewer see only published+active metrics read-only. Gating logic is
  real (checks the current mock user's role from `useTeamStore`) even
  though the single seeded identity is always Owner — noted below.
- Live computation wired into two real surfaces, proving "computes
  correctly" isn't just a UI toggle: `ReportTable` (Analytics) appends a
  column per report_builder-targeted metric evaluated against each row's
  own `ReportRow.metrics`; the Campaigns `DataTable` does the same against
  each campaign's clicks/conversions/revenue/spend/profit/roi. Same
  formula engine, same evaluator, both places.

### Fixed

- **Crash found during the in-browser smoke test**: typing an
  in-progress/invalid formula (e.g. mid-keystroke) threw an uncaught
  `FormulaError` and crashed the Sheet's render. Root cause:
  `FormulaInput` computed its own validation and reported it to the
  parent through a `useEffect` callback, so the parent's lifted
  `validation` state was always one render behind the live `formula`
  value — on the render where `formula` had just become invalid, the
  parent still evaluated it against its stale "valid" state, and
  `evaluateFormula`/`parseFormula` (unlike `validateFormula`) don't catch
  parse errors. Fixed by making the parent compute validation once,
  synchronously, from `formula` itself (`useMemo`) and pass the result
  down as a prop — one source of truth, no lifted-state race. Documented
  the failure mode in both files as a comment so the pattern isn't
  reintroduced.

### Known issues

- Role-restricted paths (Manager/Buyer/Analyst/Viewer) are implemented
  and exercised by code path but not interactively verified in the
  browser — the only seeded mock identity is the Team Owner, so testing
  the restricted views would require a role-switcher UI this phase didn't
  build. The permission checks themselves read the real (mock) team
  member/role data, not a hardcoded flag.
- Full browser smoke test otherwise passed (extension connected):
  click-to-insert from the catalog, live validation and live preview,
  safe division against zero confirmed via direct DOM/JS inspection
  (screenshots were unreliable this session due to viewport-size
  cutoff — verified state via `get_page_text`/`javascript_tool` instead),
  the single-data-source rejection (`{clicks} + {push_sent}`), and LTV
  metrics rendering as non-interactive (not `<button>`) catalog rows. No
  console errors after the fix above.

## [Phase 14.5] — Tags (cross-entity)

### Added

- Tags (§30.6): a color-label system spanning exactly the seven entities
  the spec names — Campaigns, Networks, Offers, Flows, Traffic Sources,
  PWA, Landings — and nothing else (Postlanding/Pixels/Conversions/
  Domains/Team stay untagged, matching the spec's list precisely).
- Data model mirrors §30.6's generic `tags` table + polymorphic
  `taggables` join exactly: `lib/mock/tags.ts` (Tag, decorative
  `TAG_COLORS` palette deliberately kept separate from the design
  system's semantic success/warning/danger/info tokens) and
  `lib/mock/tag-assignments.ts` (the join side). One `stores/tags.ts`
  owns both, with `setEntityTags` (single-item replace-all) and
  `bulkEditTags(entityType, entityIds, toAdd, toRemove)` implementing
  §30.6's bulk rule precisely: pre-check is the intersection of tags
  across the whole selection; confirming adds newly-checked tags to
  every selected item and removes previously-common tags that got
  unchecked; tags only some items had, left untouched by the user, are
  never touched.
- **ONE tag component, ONE filter, reused across all seven entities per
  the spec's explicit "do not reimplement per entity"** — everything
  lives in `features/tags/`: `use-entity-tags.ts` (safe hook — selects
  raw stable arrays and filters in a `useMemo`, learned from the Phase 13
  crash), `tag-badge-list.tsx` (the shared "Tags column" cell: ≤3 shown,
  >3 "+N", "Add tags" affordance when empty), `tag-picker-popover.tsx`
  (search, checkboxes, inline quick-create when the typed name has no
  match, inline rename/recolor, delete), `tag-filter-control.tsx` (OR
  filter with colored dots), `bulk-tag-dialog.tsx`, `filter-by-tags.ts`.
  Six list pages (Campaigns/Networks/Offers/Traffic Sources/PWA/Landings)
  wire the exact same five pieces; Flows (no list page — nested inside
  Stream Set cards) get `TagBadgeList` inline in `flow-editor.tsx` only,
  since bulk-select and filtering are list-view concepts Flows don't
  have.
- Extended `components/ui/data-table.tsx` with opt-in row selection
  (`enableRowSelection`, `getRowId`, a `bulkActions` render-prop, and a
  `filters` toolbar slot) using TanStack v9's `rowSelectionFeature` —
  strictly additive, so every pre-existing `<DataTable>` call site
  (Postback Logs, Conversions, Pixels, Postlanding, …) keeps working
  with zero changes since the new props all default to off.
- Deliberate scope decision: skipped a second "Manage Tags" entry point
  in each row's `⋮` menu. The spec offers it as an alternative ("via the
  Tags column **or** the row's ⋮ Manage Tags"), and the Tags column
  click already fully covers single-item tag management; a redundant
  second popover-from-a-closed-dropdown entry point would have meant
  six more rounds of anchor-positioning boilerplate for identical
  functionality.

### Fixed

- N/A this phase.

### Known issues

- Full browser smoke test passed (extension connected): tag column +
  picker + quick-create + propagation into the filter list on Campaigns,
  OR-semantics filtering, bulk-select → Edit Tags with the intersection/
  add/remove algorithm verified end-to-end, flow-level tagging inside
  the Flow Builder, and a second entity (PWA) spot-checked for the same
  behavior. No console errors. Re-ran the Phase 13 selector-antipattern
  grep sweep before testing — clean.
- No dedicated tag-management page — tags are created/renamed/recolored/
  deleted entirely inline from any picker they appear in (by design, per
  spec; there's no separate admin UI called for). Deleting a tag from
  one picker removes it (and all its assignments) everywhere at once,
  which is correct per "edit tag name/color propagates everywhere" but
  is a real destructive action with no confirmation step beyond the
  picker's own delete icon — worth a confirm dialog if this becomes a
  frequent misclick in practice.

## [Phase 14] — Domains / Team / Settings

### Added

- Domains (§30): `/domains` — the §30 note that this is "a real module, not
  a text field" is modeled directly: `purpose` (tracking/pwa/fallback),
  `registrar`/`dnsProvider` enums, `expiresAt`/`verifiedAt` tracking, and
  separate mock "Verify ownership" / "Issue SSL" actions rather than raw
  editable status fields (you don't hand-set SSL status, you issue a
  cert). Seeded with the exact 3 domain strings `mock/campaigns.ts` already
  used as `TRACKING_DOMAINS`, plus a PWA domain and a fallback domain.
  Domain removal is a real hard delete (with confirm) — unlike every other
  entity phase so far, there's no "archived" status in the spec for
  domains, and hard delete is the honest action here.
- **Closed the last remaining "known mock placeholder" loop from Phase 6**:
  `campaign-form.tsx`'s Source and Tracking domain selects now read live
  from `useTrafficSourcesStore` (Phase 11) and `useDomainsStore` (filtered
  to `purpose: "tracking"`) instead of the static `SOURCES`/
  `TRACKING_DOMAINS` arrays in `mock/campaigns.ts`. That module keeps
  those two arrays only for its own non-reactive seed generator, same
  pattern as every prior phase's mock-list closure.
- Team (§30, roles from §52): `/team` with Members / Roles & Permissions /
  Activity tabs. Roles (`Owner/Admin/Manager/Buyer/Analyst/Viewer`) and
  permission keys (`campaign.read`, `offer.write`, `settings.write`, etc.)
  match §52 (Phase 28 Auth/RBAC) exactly, so the Roles & Permissions
  reference table is the real vocabulary, not a placeholder — Phase 28
  just adds server-side enforcement on top. The seeded Owner is the actual
  mock signed-in user (`Nail Ismagilov`, `nailismagilovnick@gmail.com`)
  already used in `components/shell/user-menu.tsx` — same person, not a
  stand-in. Owner's role can't be changed and has no remove/suspend
  actions, matching real SaaS conventions for a workspace's sole owner.
- Settings (§30): `/settings` with Organization / API Keys / Integrations
  / Security tabs. API key creation shows the full key exactly once
  (only the prefix persists afterward) via a two-step create → reveal
  dialog flow. Integrations panel doubles as the visible home for the
  Facebook/TikTok/Google Ads connections `TrafficSource.costIntegration`
  (Phase 11) and Domains' registrar/DNS providers (this phase) point at —
  connect/disconnect is mocked, matching the "OAuth wiring lands in Phase
  27-COST" note already established. **Custom Metrics is intentionally
  NOT here** despite §30 listing it under Settings — CLAUDE.md's build
  order makes it its own later phase (14.6); adding it now would be
  building ahead.

### Fixed

- N/A this phase.

### Known issues

- Full browser smoke test passed (extension connected): Domains list +
  Issue SSL action, Team's all 3 tabs including role-change and the
  Owner-has-no-actions guard, Settings' all 4 tabs including the API-key
  reveal-once flow, and the campaign creation form's now-live Source/
  Tracking-domain pickers. No console errors. Re-ran the Phase 13
  selector-antipattern grep sweep before testing — clean, nothing new to
  fix this time.
- Team/Settings/Domains are pure frontend state (Zustand, in-memory) with
  no backend behind "Verify ownership," "Issue SSL," "Connect"
  integration, or API key creation — all are explicitly mock actions
  pending their real phases (27, 27-COST, 28). Documented in-code at each
  call site so this isn't discovered by surprise later.

## [Phase 13] — Conversions / Postbacks / Pixels UI

### Added

- Conversions (§29, §43): `/conversions` list (click ID, campaign, offer,
  CPA status badge, revenue+currency, postback status, event time — all
  cross-referenced live from the Campaigns/Offers stores) linking to
  `/conversions/[id]`, a detail page with the exact §29 timeline (Click →
  Landing → PWA → Offer → Conversion → Postback) as a vertical stepper, plus
  a "Resend postback" action. `status` is a proper `CpaStatus` enum with
  the CLAUDE.md-authoritative token values (`CPA_HOLD`/`CPA_ACCEPT`/
  `CPA_REDEP`/`CPA_DECLINE`/`CPA_TRASH`) — never collapsed into one
  "conversion" type (invariant #2).
- Postbacks (§29, §45): `/postbacks` with four tabs.
  - **Outgoing** reuses `NetworkList`/`NetworkFormSheet` as-is (outgoing
    postback config IS the Network entity's `postbackUrl` — no second CRUD
    for the same data).
  - **Incoming** is new: a per-network reference card showing the URL to
    hand that network so they can report conversions into FLOX
    (`api.floxlink.io/postback/{networkId}?...`), with copy-to-clipboard
    and a mapped-status-count badge.
  - **Event Mapping** is a new, editable per-network table translating a
    network's own raw status string to the canonical `CpaStatus` — what
    the real Conversion Engine (Phase 23) will run at ingest time.
  - **Logs** is a new `DataTable` of every incoming/outgoing postback
    attempt (success/duplicate/error) with a Replay row action, per §45's
    "log every postback... with replay ability."
- Extended the Network entity (Phase 11) with `acceptDuplicates: boolean`
  — the §45 per-network dedup override — as a toggle in the existing
  `NetworkFormSheet`, plus a "Dedup" column on both the Networks page and
  the reused Outgoing Postbacks panel.
- Pixels (§29): `/pixels` — the same list/create-edit-Sheet/row-actions
  shape as Landings/PWA/Postlanding. Client-side ad-platform pixels
  (Facebook/TikTok/Snap/X/generic S2S) fired on a curated event subset.
  Explicitly documented as distinct from a Stream Set's raw `pixels:
  string[]` S2S URLs (§23/§24) — different concepts, not touched here.

### Fixed

- **Infinite-render crash on `/postbacks` → Event Mapping tab**: found via
  the in-browser smoke test (Claude-in-Chrome connected this session).
  `NetworkMappingCard` selected `useEventMappingsStore((s) =>
  s.listByNetwork(networkId))` — `listByNetwork` returns a fresh
  `.filter()`'d array on every call, which breaks `useSyncExternalStore`'s
  snapshot-stability check (new reference each read, even with no `set()`
  call) and threw `Uncaught Error: Maximum update depth exceeded`,
  crashing the tab outright — the same failure class already documented
  and correctly avoided in `stores/stream-sets.ts`'s `listByCampaign`
  (which sidesteps it with a cache), just not learned from here. Fixed by
  selecting the raw `mappings` array and filtering locally in a
  `useMemo`. Also deleted the equivalent unused `listByNetwork` off
  `stores/offers.ts` and `stores/event-mappings.ts` — both were dead code
  and the exact loaded gun for the next person to pick up the same way;
  confirmed via a full-codebase grep that no other `useXStore((s) =>
  s.method(...))` or inline `.filter()`/`.map()` selector exists anywhere,
  including in Phase 9/10's code. This does **not** explain the Phase 10
  crash-loop report — that one has no reproduction and no `.filter()`-as-
  selector pattern anywhere near it — so that stays logged as unresolved,
  not retroactively closed by this fix.

### Known issues

- None new this phase — full browser smoke test passed after the fix
  above (Conversions list + detail, all four Postbacks tabs including
  adding an Event Mapping row and replaying a log entry, Pixels list), no
  console errors.

## [Phase 12] — Landing / PWA / Postlanding UI

### Added

- Landings, PWAs, and Postlandings are now real, team-managed entities
  (§28) with their own pages (`/landings`, `/pwa`, `/postlanding`) — same
  list/create-edit-Sheet/row-actions/Zustand-store shape established in
  Phase 11 for Offers/Networks/Sources.
- Landing editor models the §28 `internal`/`external` split for real: an
  `external` landing takes a URL you already control; an `internal` one
  takes a content textarea and the Sheet derives + live-previews a
  `cdn.floxlink.io/lnd/{slug}` hosted URL from the name via the new
  `slugify()` (`lib/utils.ts`) as you type. Verified in-browser that the
  slug preview updates live and that submitting an internal landing with
  no content is correctly rejected before persisting a broken entity, per
  the `.superRefine` in `landing-form-sheet.tsx`.
- PWA editor fields are the real Web App Manifest (name, short_name,
  theme_color, background_color, icon, start_url) rather than a
  fictionalized subset — the Sheet renders a live, read-only
  `manifest.json` preview generated from those exact fields, plus a
  color-swatch input alongside each hex field. Includes the §73-required,
  provider-neutral `bounceInAppWebview` toggle: bounce in-app WebView
  traffic (FB/IG/TikTok/Telegram) to the external browser so the install
  prompt can fire. This is explicitly NOT vendor-specific moderator
  detection, which §73 forbids — the toggle only ever describes bouncing
  a generic in-app WebView, never detecting a specific ad network's
  reviewer.
- Postlanding editor's `events` field is a multi-select over a curated,
  postlanding-relevant subset of the §43 event model (`PWA_INSTALL`,
  `NOTIFICATION_REQUEST/SUBSCRIBE/DECLINE`, `TG_JOIN`, `TG_START`) —
  string values chosen to match the canonical list in CLAUDE.md exactly,
  so nothing needs renaming when Phase 13 (Conversions/Postbacks/Pixels)
  introduces the full enum.
- **Closed the equivalent of the Phase 9 known issue for the last three
  Flow Builder pickers**: `flow-funnel.tsx` now reads Landings/PWAs/
  Postlandings live from `useLandingsStore`/`usePwasStore`/
  `usePostlandingsStore` instead of the static `flow-entities.ts` mock
  list (which now holds only the flow-level `PwaType`/`PWA_TYPES` display
  mode — internal/external/ios_app, NOT one of the real entities). Also
  fixed the PWA node's preview URL, which was a hardcoded
  `pwa.floxlink.io/install/{id}` string — it now resolves the selected
  PWA's real `startUrl`. `stream-sets.ts` (the module-level stream-set
  mock generator, not a component) keeps seeding from the static
  `LANDINGS`/`PWAS` arrays exported by the new mock files.
- IDs were kept identical to the old `flow-entities.ts` placeholders
  (`lnd_prelander_a`, `pwa_sweeps`, `psl_thankyou`, etc.) so every existing
  seeded stream-set flow keeps resolving without a data migration.

### Fixed

- N/A this phase.

### Known issues

- Verified in a real browser this time (the Claude-in-Chrome extension
  connected, unlike Phase 11's session) — clicked through creating a
  Landing, confirmed its live slug preview and validation, confirmed the
  PWA manifest preview renders correctly, and confirmed a landing created
  in the Landings UI shows up immediately in a campaign's Flow Builder
  picker within the same session (state resets on a hard navigation/
  reload, same known limitation the Campaigns store already has — not a
  regression). No console errors during any of it. This still isn't the
  exact repro path from the Phase 10 crash-loop report, so that one stays
  logged as unresolved rather than closed.
- Internal-landing content is a raw HTML textarea, not a real page
  builder/WYSIWYG editor — matches the spec's field list (§28 lists
  "internal/external" as the Landing distinction, not a builder
  requirement) but is worth flagging if a future phase expects richer
  authoring.

## [Phase 11] — Offers / Networks / Sources UI

### Added

- Traffic Sources, Networks, and Offers are now real, team-managed entities
  (§27) with their own top-level pages (`/traffic-sources`, `/networks`,
  `/offers`) — list (`DataTable`, search/sort/paginate), create/edit (a
  `Sheet` form, following the Stream Set Builder's pattern rather than a
  full detail route since none of these three need per-entity analytics
  tabs), row actions (edit/pause/resume/duplicate/archive with an archive
  confirm dialog), Zustand store per entity (`stores/{networks,offers,
  traffic-sources}.ts`) mirroring the Campaigns store's CRUD shape.
- Modeled the real hierarchy from §27: Network → Offer → Offer Link. An
  Offer carries a `links: OfferLink[]` field array (primary + backups) in
  its form, editable independently with add/remove.
- `src/lib/macros.ts` — the ONE shared macro/placeholder resolver (§27):
  `{click_id}`, `{status}`, `{revenue}`, `{currency}`, `{payout}`, `sub1..10`,
  etc., plus `resolveMacros()`. `src/components/shared/macro-picker.tsx` is
  the reusable insert-a-token popover, wired into the Offer link URL,
  Network postback URL, and Source tracking template fields. Both are built
  to be reused as-is by Postback templates and Pixel payloads in Phase 13 —
  no second token list.
- `src/lib/countries.ts` — static ISO country/currency reference for the
  Offer GEO multi-select (`components/ui/multi-select.tsx`, previously only
  used by the filter builder and analytics).
- **Closed a Phase 9 known issue**: the Flow Builder's Network/Offer
  pickers (`flow-funnel.tsx`, `stream-set-list.tsx`, `stream-set-row.tsx`,
  `stream-set-form-sheet.tsx`) now read live from `useNetworksStore` /
  `useOffersStore` instead of the static `flow-entities.ts` mock list —
  creating/editing/pausing an offer in the new Phase 11 UI is immediately
  reflected in the Flow Builder. `flow-entities.ts` keeps only
  Landing/PWA/Postlanding placeholders (real in Phase 12). The stream-set
  mock generator (`mock/stream-sets.ts`, a module-level pure function, not
  a component) still seeds from the static `OFFERS`/`NETWORKS` arrays
  exported by the new mock files — same non-reactive-seed behavior it had
  before, just sourced from the new location.
- Mock data IDs (`net_afftrust`, `off_sweeps_us`, etc.) were kept identical
  to the old `flow-entities.ts` placeholders so every existing seeded
  stream-set flow keeps resolving to the same offer/network without a data
  migration.

### Fixed

- N/A this phase.

### Known issues

- No visual browser smoke test was possible this phase — the Claude-in-Chrome
  extension reported "not connected" in this environment. Validated instead
  via `tsc --noEmit` (clean), `eslint` (clean), `next build` (clean, all 3
  new routes prerender), and `curl` against a running dev server confirming
  200s and correct SSR content on `/traffic-sources`, `/networks`, `/offers`,
  and a campaign detail page (including that a stream-set flow still
  resolves "US Sweeps — CPA $12" through the new offers store). This does
  not re-exercise the Phase 10 client-side navigation crash-loop report —
  that one needs a real browser tab and DevTools console to chase down;
  still unresolved.
- Offer/Network/Source archive is a status flag, not a hard delete (matches
  the Campaigns pattern) — nothing currently blocks deleting the last
  network, which would leave the Offer form's network picker empty and the
  "New Offer" button disabled; acceptable for mock data, revisit once
  Offers/Networks have real referential integrity in Phase 17+.
- Payout is shown/entered in the offer's own currency with no FX
  conversion to USD anywhere in this UI (§50-FX is backend/event-time work,
  out of scope for an entity-management screen with no events yet).

## [Phase 10] — Routing Simulator

### Added

- Routing Simulator (§26), a third tab (Overview / **Simulator** / Settings)
  on the campaign detail page — routing is campaign-scoped, so it lives
  there rather than as a new top-level sidebar route not in §17's list.
- `src/lib/routing-simulate.ts` — a pure `simulateRoute(streamSets,
  campaignFallbackUrl, request) → result` function implementing the exact
  request/response contract the future `POST /routing/simulate` Go
  endpoint (Phase 19) will expose, per the §6-SHARED / invariant #1
  architecture note: not a second permanent routing engine, but the only
  place this logic *can* live before Phase 19 exists, designed so Phase 27
  swaps this call for a `fetch` with zero UI changes. Evaluates the real
  nested filter tree (all 16 operators from §22, reusing Phase 8's exact
  types), walks stream sets in priority order (first active match wins),
  weighted-picks a flow among active ones, and resolves flow → stream-set
  fallback → campaign fallback → none.
- Input form covers every §26 field, reusing Phase 8's `FIELD_GROUPS`/
  `FIELD_VOCAB`/`BOOLEAN_FLAG_FIELDS` from `lib/filters.ts` — the simulator
  and the filter builder share one field vocabulary, not two.
- Result view: a pipeline stepper (Request → Classification → Campaign →
  Stream Set → Filters → Flow → Destination per §26), a pass/fail trace
  per stream set — including *why* non-matching ones didn't match, not
  just the winner — flow candidates with normalized probabilities and the
  selected one marked, the resolved destination with copy, and a sticky
  note that's honest about sticky config not existing in the data model
  yet rather than faking cookie-based behavior (§80 — no fake APIs that
  look real).
- Core evaluator logic (AND/OR/nested-group semantics, `IN`/`IS` matching)
  spot-checked against the seeded §23 fixture (`country IS US AND device
  IN [mobile, tablet] AND (OS IS Android OR OS IS iOS)`) via a standalone
  script mirroring the algorithm, since Jest/Vitest isn't wired up yet —
  all 5 cases passed.

### Fixed

- `DndContext` (Phase 7, `stream-set-list.tsx`) had no `id` prop, so
  dnd-kit's internal `useUniqueId` counter — which is module-level, not
  reset per-mount — produced a different `aria-describedby` id on the
  client than the server rendered, a confirmed hydration mismatch
  (dnd-kit's own SSR guidance: pass a stable `id` when using `DndContext`
  with SSR). Fixed with `id={\`stream-sets-${campaignId}\`}`.

### Known issues

- **Unresolved**: while smoke-testing this phase, a real browser tab
  connected to the dev server (not one of my own curl requests — curl
  can't drive client-side navigation) hit a repeating
  `Uncaught TypeError: Cannot read properties of undefined (reading
  'length') at Array.map (<anonymous>)` after navigating
  `/overview → /campaigns → /campaigns/[id]`. The dnd-kit `id` fix above
  eliminated the hydration-mismatch warning that preceded it the first
  time, but the crash itself recurred on a second capture without that
  warning, so it isn't fully explained by that fix. Terminal-forwarded
  browser errors don't carry source-mapped stack frames, so the exact
  call site couldn't be pinned down from the dev server log; production
  builds don't forward console output at all, and curl-only testing can't
  reproduce a client-side remount. Typecheck/lint/build are clean and a
  targeted static audit of every `.map()` call added or touched this
  phase found nothing unsafe. If this recurs, the browser's own DevTools
  console will have the real stack trace — that's the fastest path to a
  fix.
- Sticky assignment is explanatory text only — no real cookie/session
  state exists to simulate against yet.
- The weighted flow pick is a live `Math.random()` draw per Simulate
  click, not deterministic/repeatable — matches real routing behavior,
  but re-running the same inputs can select a different flow.

## [Phase 9] — Flow Builder

### Added

- Visual per-flow funnel (§24-25) replacing the flat name/URL/weight row
  from Phase 7-8: optional Landing (+ "show as PWA" toggle) → optional PWA
  (+ type: internal/external/ios_app) → optional Postlanding → a required
  terminal step that's either an **Offer** (network + offer + offer-URL
  carrying the `{click_id}` macro) or a **Redirect** (plain URL, no CPA
  attribution) — the segmented toggle between them lives on the terminal
  node itself. A dashed ghost **Fallback** node closes the funnel, showing
  the Stream Set's existing `fallbackUrl` (Phase 7) rather than a new
  per-flow field — all six §25 node types, one data source per concept.
- Every node supports the §25 capability set: enable/disable (optional
  stages only — the terminal step is always active), inline configuration
  (picker fields appear directly under the node header when enabled), a
  status badge (`Skipped`/`Needs setup`/`Configured`), a copyable preview
  URL, and a small deterministic mock analytics line (seeded per
  flow+stage — a real per-node metric needs the tracker event stream from
  Phase 16+, so this is a placeholder demonstrating the capability, not
  live data).
- Weight is now an arbitrary raw integer instead of Phase 7's "must sum to
  100" constraint, matching §24 exactly: the editor shows the raw weight
  next to the engine-normalized percentage (`weight / Σweights × 100`),
  and the Stream Set row's flow tags show the normalized % too.
- Per-flow **Duplicate** (§24's "duplicated" node/flow state), alongside
  the existing enable/disable and remove.
- `src/lib/mock/flow-entities.ts` — placeholder Network/Offer/Landing/PWA/
  Postlanding option lists so the funnel pickers have something to bind
  to. These become real, team-managed entities in Phase 11-12; the Flow
  shape (`networkId`/`offerId`/`landingId`/`pwaId`/`postlandingId`) is
  designed not to change when that happens, only the picker's data source.
- `src/features/stream-sets/flow-node.tsx` — the generic node card reused
  by all six node types; `flow-funnel.tsx` composes them per flow;
  `flow-editor.tsx` wraps one flow's header (name/weight/active/duplicate/
  remove) around its funnel, collapsible.

### Changed

- `Flow` (in `lib/mock/stream-sets.ts`) went from
  `{destinationType, destinationUrl}` to the funnel shape described above
  (`landing`, `pwa`, `postlanding`, `destination: {kind: "offer"|"redirect"}`).
  Mock stream sets regenerated to exercise it: set 0's first flow uses the
  full Landing→PWA→Offer chain, set 2 (bot/proxy block) uses a Redirect
  terminal instead of an Offer.

### Known issues

- Landing/PWA/Postlanding/Network/Offer pickers are the Phase 9 mock lists
  above, not real entities — real management UI is Phase 11 (Sources/
  Networks/Offers) and Phase 12 (Landing/PWA/Postlanding).
- Per-node "analytics summary" is a seeded mock number, not wired to any
  real event data yet.

## [Phase 8] — Filter Builder

### Added

- Recursive AND/OR filter tree (§22-23) replacing Phase 7's flat placeholder:
  `MATCH ALL`/`MATCH ANY` group pills, `+ Condition`/`+ Group` at every
  depth, arbitrary nesting — matches the §23 example structure exactly
  (`country IS US AND device IN [mobile, tablet] AND (OS IS Android OR OS
  IS iOS)`), which one of the Phase 7 mock stream sets now demonstrates.
- Full §22 field list (30 fields, grouped Geo/Device/Fraud/Traffic/Custom
  in the field picker) and all 16 operators, including `BETWEEN` (two-input
  range) and `MATCHES` (regex).
- Typed value inputs instead of one generic text box: `MultiSelect` (reused
  from Phase 5) for enum-like fields on `IN`/`NOT_IN`, a Yes/No toggle for
  `bot`/`proxy` (they're boolean-like flags, not free text, per §22's
  note), a plain Select for enum fields on single-value operators,
  no input for `EXISTS`/`NOT_EXISTS`.
- Save-time validation surfaced inline: ISO-3166 alpha-2 country codes
  (flags the classic `UK` mistake with "use GB"), and a client-side RE2-
  compatibility heuristic for `MATCHES` patterns (rejects lookaround,
  backreferences, atomic groups, possessive quantifiers) — a first pass
  only; real enforcement is Go's `regexp` (RE2) at save time per §5, which
  is why the check function's own doc comment says not to trust it as the
  source of truth.
- `src/lib/filters.ts` — the field/operator vocab, recursive tree types
  (`FilterNode = FilterCondition | FilterGroupNode`) and pure tree
  utilities (`addConditionToGroup`, `addGroupToGroup`, `updateCondition`,
  `updateGroupJoiner`, `removeNode`, `cloneWithNewIds`, `describeFilterTree`)
  that both the builder UI and the Stream Set row summary share — single
  implementation, per §6-SHARED.
- Stream Set row summary now renders top-level conditions as `FilterChip`s
  (Phase 3) plus a collapsed `(N)` badge per nested group, with the full
  plain-language tree (`describeFilterTree`) on hover via `Tooltip` — the
  static half of §72's explainability goal; the interactive "why did this
  match" surface is still the Phase 10 simulator.
- `src/lib/id.ts` — extracted the shared `genId` helper (was duplicated
  inline in `lib/mock/stream-sets.ts`) so `lib/filters.ts` doesn't import
  from `lib/mock/*`, avoiding a mock→domain→mock import cycle.

### Changed

- `StreamSet.filters: FilterCondition[]` + `joiner` (Phase 7's flat shape)
  is now `StreamSet.rootFilter: FilterGroupNode`, a real tree. `Campaign`/
  `Flow` shapes are unaffected.

### Known issues

- `useForm<StreamSetFormValues>`'s resolver needs an explicit `Resolver<T>`
  cast in `stream-set-form-sheet.tsx` — react-hook-form's `Path<T>` type
  can't fully resolve a self-referential union
  (`FilterNode = FilterCondition | FilterGroupNode`), so the zodResolver's
  inferred generic mismatches the plain type. Compile-time inference gap
  only, not a runtime issue — the filter tree isn't registered as RHF
  field paths anyway (it's one `Controller`-managed value mutated via the
  pure tree utilities above).
- The RE2 heuristic and country-code check run inline in the value editor,
  not through react-hook-form's per-field error state — deeply nested
  union array paths aren't practical to index into for that. The zod
  `superRefine` still blocks submission as a safety net either way.

## [Phase 7] — Stream Sets

### Added

- Stream Set management (§21) embedded in the campaign detail page's
  Overview tab, replacing the Phase 6 `EmptyState` stub: priority-ordered
  cards (`@dnd-kit` drag reorder, persisted via `useStreamSetsStore.reorder`),
  enable/disable `Switch`, duplicate, and a create/edit `Sheet` form.
  Semantics are stated explicitly in the UI copy — evaluated top-to-bottom
  by priority, first match wins, no match falls through to the campaign
  fallback — matching the explainability goal in §72 (the interactive
  "why did this match" surface is the Phase 10 simulator; this phase keeps
  it legible via plain-language copy and the same `FilterChip`/`FilterGroup`
  components already built in Phase 3 for exactly this purpose).
- Filters/Flows/Pixels editors are intentionally **flat**, not the final
  builders: filters are one AND/OR-joined list (no nested groups — that's
  Phase 8's 13-operator nested rule engine), flows are a weighted list
  pointing at a destination URL directly (no node graph or offer picker —
  that's Phase 9 plus the Offers/Landings/PWA entities from Phase 11-12).
  Same `FilterCondition`/`Flow` field shapes Phase 8-9 will consume, so the
  data contract doesn't change when the real builders land, per the
  single-source-of-truth rule in §6-SHARED.
- Flow weight editor sums live and warns (non-blocking) when weights don't
  total 100%.
- `src/lib/mock/stream-sets.ts` — deterministic per-campaign generator
  (seeded from the campaign id, same pattern as the Phase 6 daily-trend
  generator) plus the shared `FilterField`/`FilterOperator`/
  `FlowDestinationType` vocab §22 will reuse.
- `src/stores/stream-sets.ts` — Zustand store keyed by campaign id
  (`addStreamSet`, `updateStreamSet`, `setStatus`, `duplicateStreamSet`,
  `reorder`).

### Fixed

- `listByCampaign` originally lazily generated a campaign's stream sets
  and called `set()` **inside the selector function itself** — i.e. during
  React's render phase. Combined with `generateStreamSets` returning a new
  array reference on every call, this broke `useSyncExternalStore`'s
  snapshot-stability contract: each render's snapshot read as "changed"
  from the last, so React kept re-rendering, which kept re-invoking the
  selector, forever. Confirmed live — a real connected browser tab hit the
  campaign detail page and spammed ~8k identical `TypeError`s/sec in a
  reconnect-retry loop before the fix. Fixed by making `listByCampaign` a
  pure read (no `set()` call) backed by a module-level generation cache
  keyed by campaign id, so an unmaterialized campaign's snapshot is
  referentially stable across repeated selector calls; store mutations
  still go through `set()`, but only ever from event handlers, never from
  a render-phase selector.

### Known issues

- Stream sets are mock/in-memory like campaigns (Phase 6) — reset on a
  hard reload.
- Fallback URL is per-stream-set only in this phase; the campaign-level
  fallback from Phase 6 is what's actually used when no set matches at
  all (§73's "no stream set matches → campaign fallback" case). The
  per-set fallback here covers a narrower case (set matched, no flow
  resolved) and isn't yet wired into the Phase 10 simulator, since that's
  real routing evaluation and out of scope until Phase 10/19.

## [Phase 6] — Campaigns

### Added

- Campaign list (§20) at `/campaigns`: `DataTable` with Name/Status/Source/
  Clicks/Conversions/Revenue/Spend/Profit/ROI/Updated columns, search,
  sort, pagination. Profit/ROI render "—" (never a false $0/0%) for the
  ~12% of mock campaigns generated with no spend, per §27-COST.
- Row actions (§20): Open, Pause/Resume, Duplicate (navigates to the new
  copy), Copy tracking URL (toast with the composed
  `https://{trackingDomain}/t/{trackingId}` URL), Archive (confirm dialog,
  destructive). Duplicate and Archive share `useCampaignsStore` mutations
  used by both the list row menu and the detail page.
- Campaign creation at `/campaigns/new` and detail/settings at
  `/campaigns/[id]` (Overview + Settings tabs). Overview shows the same
  8 stat cards as the dashboard for one campaign, a 30-day revenue trend
  chart, and a "Stream Sets" card stubbed as `EmptyState` until Phase 7-9
  build routing. Settings reuses the creation form (`CampaignForm`,
  react-hook-form + zod) with an added Status field and a danger-zone
  Archive action.
- `src/stores/campaigns.ts` — Zustand store (`addCampaign`,
  `updateCampaign`, `setStatus`, `duplicateCampaign`, `getById`) seeded
  from `src/lib/mock/campaigns.ts`'s deterministic generator so the
  detail page resolves an id consistently within a session.

### Known issues

- Detail page 404s (`ErrorState`, with a link back to the list) if the
  mock store resets — e.g. a hard reload after `addCampaign` — since
  campaign data is in-memory only until Phase 16+ wires the real API.
- Stream Sets/Filters/Flows are not built yet; every campaign routes
  100% to its fallback URL until Phase 7-9.

## [Phase 5] — Analytics

### Added

- Analytics explorer (§19) at `/analytics`: controls for date range, timezone,
  dimensions (16, multi-select), metrics (13, multi-select), filters
  (dimension = value, AND-joined), group-by, sort, and a compare-to-previous-
  period toggle. Four views on one shared aggregation: Table (dynamic
  `DataTable` columns), Line (any selected metric over time), Bar (top-8
  breakdown by the group-by dimension), and Funnel — the full 8-step event
  model (§43) `SOURCE_CLICK → ... → CPA_REDEP` with per-step and per-total
  conversion %, not a generic 3-step funnel.
- `src/components/ui/multi-select.tsx` — reusable Popover+Command checklist
  (dimensions/metrics here; the Filter Builder in Phase 8 will likely want it
  too).
- `src/features/analytics/registry.ts` — metric formulas match §50 exactly
  (`roi=(revenue-cost)/cost`, `roas=revenue/cost`, etc.); mock data marks
  ~15% of slices with no cost at all, so aggregated ROI/CPA/cost render "—"
  rather than a false $0/0%, at any grouping granularity.
- `src/lib/format.ts` — `formatUsd`/`formatInt`, both pinned to the `en-US`
  locale.

### Fixed

- Every `toLocaleString()` currency/number call across the app (Dashboard
  included, from Phase 4) used the runtime's default locale instead of a
  fixed one. In this environment that silently rendered USD as
  `14 655,87 $` instead of `$14,655.87` — locale-dependent formatting for a
  fixed-locale product. Centralized in `src/lib/format.ts` and fixed
  everywhere it was called.

### Known issues

- Timezone selector is cosmetic in the mock phase — it doesn't shift
  aggregation, since slices only carry a date, not a timestamp.

## [Phase 4] — Dashboard

### Added

- Overview dashboard (§18) at `/overview`, replacing the stub: 8-card KPI row
  (Revenue/Spend/Profit/ROI/Clicks/Conversions/CVR/CPA) with period-over-period
  trend deltas, 4 time-series charts (Revenue/Spend/Profit/Conversions) via
  Apache ECharts, and 4 top-N tables (Campaigns/Offers/Countries/Flows) on the
  existing `DataTable`. `DateRangePicker` drives both the charts and the KPI
  comparison window (current period vs. the equal-length period before it).
- `src/lib/mock/dashboard.ts` — deterministic (seeded-PRNG, not `Math.random`)
  mock data generator, so the statically-exported page is reproducible across
  builds. One campaign carries no spend on purpose, to exercise §27-COST: ROI
  renders as "—", never a false 0%.
- `src/lib/chart-theme.ts` — ECharts option tokens reusing the exact design
  tokens (dark/light), instead of a second hardcoded palette.

### Fixed

- `StatCard`'s value/delta row had no wrap or shrink handling and clipped
  against the card edge on narrow (mobile) widths; now wraps the delta to its
  own line instead of overflowing.

### Known issues

- The full ECharts bundle takes ~1.3s to parse and paint on first mount
  (confirmed deterministic via polling, not flaky) — acceptable for the mock
  phase; revisit with a lighter `echarts/core` + explicit chart-type imports
  in Phase 31 (performance).
- Top-N tables are not filtered by the selected date range (only the KPI row
  and charts are) — there's no per-row time series in the mock to filter by.

## [Phase 3] — Application Shell

### Added

- Persistent app shell (§17) under the `(app)` route group: `Sidebar`
  (workspace selector, grouped nav, expand/collapse via a persisted Zustand
  store, active-link highlighting) and `Topbar` (breadcrumbs derived from the
  route, ⌘K command menu, notifications popover, theme toggle, user menu).
- Mobile: off-canvas nav via shadcn `Sheet`, triggered from the topbar
  hamburger; same `NavContent` as desktop so the nav tree has one definition.
- `src/lib/nav.ts` — single source of truth for the nav tree, consumed by the
  sidebar, breadcrumbs, and command menu (no duplicated nav data).
- One stub page per sidebar item (`EmptyState`, "not built yet") so every
  link resolves — nothing 404s while Phases 4–14 fill in real content.

### Fixed

- `CommandDialog` (shadcn) renders `DialogContent` around `children` directly
  — it does **not** include an inner `<Command>` root the way older shadcn
  versions did. Passing `CommandInput`/`CommandList` straight into
  `CommandDialog` crashed with `Cannot read properties of undefined (reading
  'subscribe')` (cmdk's internal store context was missing). Fixed by
  wrapping the palette contents in `<Command>` inside `CommandDialog`.
- Topbar overflowed and wrapped onto a second line on narrow viewports (the
  search bar had a fixed `w-56` and breadcrumbs weren't allowed to truncate).
  Search collapses to an icon-only trigger below `sm`; breadcrumbs truncate
  instead of wrapping.

### Known issues

- Workspace selector, notifications, and user menu are mock data — wired to
  real data in Phase 27 (integration) / Phase 28 (auth).

## [Phase 2] — Design System

### Added

- Next.js 16 (App Router) + React 19 + TypeScript app scaffold in `apps/web`,
  Tailwind v4, shadcn/ui (`radix-nova` style, Radix UI primitives).
- Dark-first FLOX token system in `src/app/globals.css`: neutral surfaces,
  small radius (0.375rem), one restrained blue accent, semantic
  success/warning/danger/info tokens (light + dark), tabular-numeral utility.
  Light theme fully supported via `next-themes` (`ThemeProvider`,
  `ThemeToggle`), dark is the default (`defaultTheme="dark"`).
- Typography scale (`Display/H1/H2/H3/Body/Small/Caption/Mono`) in
  `components/ui/typography.tsx`.
- Full §16 component library: Button, IconButton, Input, Select, Checkbox,
  Radio, Switch, Textarea, Dialog, Popover, Tooltip, Dropdown, Command, Tabs,
  Badge, Tag, Avatar, Card, StatCard, DataTable (TanStack Table v9, sort /
  paginate / global search / column visibility), EmptyState, ErrorState,
  LoadingState, Skeleton, DateRangePicker, Breadcrumbs, Pagination, ChartCard
  (Apache ECharts mount point), FilterChip, FilterGroup, Alert, Toaster.
- `/style-guide` route showcasing every token and component in one page.

### Fixed

- `radix-ui` was imported by 15 generated shadcn components but never
  installed (silent gap from the CLI's dependency step); added explicitly.
  Removed the now-unused `@base-ui/react` dependency left over from the
  initial scaffold.

### Known issues

- DataTable "virtualize large sets" (UX floor) is satisfied via pagination,
  not DOM windowing — acceptable for now; revisit with
  `@tanstack/react-virtual` if a real dataset needs unpaginated scroll.
- No application shell yet (Phase 3) — `/style-guide` and `/` are standalone
  routes.

## [Phase 1] — Product Foundation

### Added

- `README.md`, `ARCHITECTURE.md`, `PRODUCT.md`, `ROADMAP.md` at repo root.
- `.env.example` covering the planned Postgres/ClickHouse/Redis/S3/auth
  configuration surface.
- Monorepo directory skeleton: `apps/{web,api,tracker,worker}`,
  `packages/{ui,config,types}`, `infra/`, each with a placeholder README
  describing its future contents.
- `docs/architecture.md`, `docs/domain-model.md`, `docs/event-model.md`,
  `docs/routing.md`.

### Fixed

- `gitignore` renamed to `.gitignore` — the file existed without a leading
  dot and was silently not being applied by git.

## [Phase 0] — Repository Inspection

### Added

- Inspected an empty repository (only `CLAUDE.md`, `docs/FLOX-master-prompt-v3.md`,
  `.idea/`, `.claude/settings.local.json` present, zero commits).
- Chose shared-domain-logic strategy **A** (§6-SHARED): Go core is the single
  source of truth for routing decisions; the Routing Simulator consumes the
  `POST /routing/simulate` contract (mocked during frontend-first phases,
  wired to the real endpoint in Phase 27).
- Identified that `gitignore` (no leading dot) was not being honored by git.
