# CHANGELOG

Format follows [Keep a Changelog](https://keepachangelog.com/). Entries are
per-phase, matching `CLAUDE.md`'s phase protocol.

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
