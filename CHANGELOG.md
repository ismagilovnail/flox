# CHANGELOG

Format follows [Keep a Changelog](https://keepachangelog.com/). Entries are
per-phase, matching `CLAUDE.md`'s phase protocol.

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
