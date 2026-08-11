# CHANGELOG

Format follows [Keep a Changelog](https://keepachangelog.com/). Entries are
per-phase, matching `CLAUDE.md`'s phase protocol.

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
