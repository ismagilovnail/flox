# CHANGELOG

Format follows [Keep a Changelog](https://keepachangelog.com/). Entries are
per-phase, matching `CLAUDE.md`'s phase protocol. The one exception is
[Between phases], below: spec amendments and the code changes that follow from
them land between phases and would otherwise be invisible here.

## [Pixels] — wired to a real Postgres-backed API

### Scope

Fourth slice of the Landing/PWA/Postlanding/Pixels CRUD candidate list.
`pixels` was already migrated (migration 00008, alongside `postbacks`
and the `stream_set_pixels` join table). New `apps/internal/pixel`
package mirrors `apps/internal/postlanding` almost exactly (flat entity,
curated §43 event subset, pause/activate/duplicate/archive-via-Update) —
differs only in having no URL (a pixel's `provider` + `pixelId` identify
where a conversion is reported, not a page) and no defensive delete-FK
catch (`stream_set_pixels.pixel_id` `ON DELETE CASCADE`s, unlike
Landing/PWA/Postlanding's `RESTRICT`-guarded `flows.*_id` columns).
Explicitly out of scope: attaching pixels to a Stream Set
(`stream_set_pixels` has no CRUD wiring it to `flows`/`stream_sets` yet)
— CLAUDE.md's own "per-flow Pixels" phrasing was imprecise; the schema
has always scoped pixels to the Stream Set, not the Flow. See
`docs/pixels.md`.

### Changed

- `apps/internal/pixel`: new `model.go`/`handler.go`/`service.go`/
  `repository.go`, wired at `/pixels` in `apps/api/main.go`.
- `apps/web`: `lib/api/pixels.ts` + `hooks/use-pixels.ts` (real API
  layer); `features/pixels/*` rewired off the `stores/pixels.ts` Zustand
  mock onto the real hooks, with `LoadingState`/`ErrorState` added.
  `PIXEL_PROVIDER_I18N_KEY` co-located with `PixelProvider`, matching
  `traffic-sources.ts`'s `SOURCE_TYPE_I18N_KEY` pattern. New `pixels`
  i18n namespace (`en`+`ru`) — the pixel form was entirely untranslated
  under the mock. `stores/pixels.ts`/`lib/mock/pixels.ts` deleted
  outright.

### Verified

Backend: `go build/vet/gofmt/test ./...` all green, incl. new
`pixel_test.go` mirroring `postlanding_test.go`'s full test set (plus an
explicit test that an empty `pixelId` is allowed, matching the frontend's
pre-existing schema). Frontend: `tsc --noEmit`/`eslint`/`vitest run` (21
tests)/`next build` all clean. Full manual browser pass against the real
running `api`+`web` dev servers: created, edited, paused, duplicated,
and archived a pixel, confirming every action round-tripped through the
real API on a fresh reload; spot-checked the Russian locale on the same
page. See `docs/pixels.md`.

## [Flow CRUD] — Landing/PWA/Postlanding funnel stages restored on Flow

### Scope

Confirmed via `AskUserQuestion`: the base Flow entity (name/weight/
active/destination) already has real CRUD, nested under Stream Sets
(`docs/stream-sets.md`). What the Stream Sets phase deliberately dropped
was the Landing → PWA → Postlanding funnel stages ahead of a Flow's
Destination — the `flows` table (migration 00006) has always had the
columns (`landing_id`/`landing_as_pwa`, `pwa_id`/`pwa_type`,
`postlanding_id`), but no `internal/landing`/`pwa`/`postlanding` package
existed at the time to pick from. They all exist for real now, so this
phase wires the stages back in. Per-flow Pixels stays out: no
`internal/pixel` package exists yet, and pixels actually attach to the
Stream Set (`stream_set_pixels`, migration 00008), not the Flow.

### Changed

- `apps/internal/streamset`: `Flow`/`FlowInput` gained `Landing`/`Pwa`/
  `Postlanding` fields (`FlowLanding`/`FlowPwa`/`FlowPostlanding`, each
  an always-present `{enabled, ...}` struct so a disabled stage keeps its
  last pick rather than losing it). `Service.checkFlowStagesBelongToOrg`
  confirms every non-empty id belongs to the caller's org before it's
  persisted (CLAUDE.md #5); `validateFlows` requires an id (and, for
  PWA, a valid `pwaType`) whenever a stage is enabled.
  `repository.go`'s `insertFlows`/`loadFlows`/`loadFlowsTx` read/write
  the 7 new columns; `nullIfEmpty` converts the wire's `""` to `NULL` for
  `pwa_type`'s CHECK constraint.
- `apps/web`: new `features/stream-sets/flow-funnel.tsx` renders the
  full Landing → PWA → Postlanding → Destination → Fallback chain,
  delegating the last two nodes to the unchanged
  `flow-destination-editor.tsx`. Fetches real `useLandings()`/
  `usePwas()`/`usePostlandings()` alongside the existing networks/offers
  queries. `stream-set-schema.ts` validates each stage the same way the
  destination union already did. `lib/mock/flow-entities.ts` (the
  `PwaType`/`PWA_TYPES` enum, never actually mock data) moved into
  `lib/api/stream-sets.ts`.
- `docs/landings.md`/`docs/pwa.md`/`docs/postlanding.md`: their Delete
  sections' "no Flow CRUD exists yet to populate that column" notes
  updated — the RESTRICT FK is real and reachable now.

### Verified

Backend: `go build/vet/gofmt/test ./...` all green, incl. 2 new
`streamset` tests (full-funnel round-trip through Create+Get with real
seeded landing/pwa/postlanding ids; validation rejecting an enabled
stage with no id, an invalid `pwaType`, and a disabled stage referencing
another org's postlanding id). Frontend: `tsc --noEmit`/`eslint`/
`vitest run` (21 tests)/`next build` all clean. Full manual browser pass
against the real `api`+`web` dev servers: enabled all three stages on a
real stream set's flow, saved, reloaded fresh, and confirmed every
selection (incl. `asPwa`/`pwaType`) round-tripped through the real
Postgres-backed API. See `docs/stream-sets.md`.

## [i18n hydration fix] — server-side locale resolution closes the race deterministically

### Scope

Closes the known, non-blocking hydration-race issue the Postlanding phase
documented (mitigated, not eliminated, by a `requestIdleCallback`-delayed
`changeLanguage()` call). An `AskUserQuestion` confirmed the trade-off
before implementing: the only fully deterministic fix requires the
server to read a locale cookie before rendering, which opts every route
out of Next.js static prerendering into per-request dynamic rendering —
accepted, since every route already fetches its real data client-side.
See `docs/i18n-hydration-fix.md`.

### Changed

- `app/layout.tsx` is now `async`, reads `cookies()`/`headers()`, and
  renders `<html lang>` + `<I18nProvider initialLocale>` with a
  server-resolved locale (persisted cookie → Accept-Language → default)
  instead of always English.
- New `lib/i18n/locale.ts`: the pure locale-resolution logic
  (`resolveLocale`, `LOCALE_COOKIE`, `isSupportedLocale`,
  `SUPPORTED_LOCALES`, `DEFAULT_LOCALE`), with zero `i18next`/
  `react-i18next` imports so a Server Component can import it directly
  without pulling react-i18next's context creation into the
  `react-server` build (which doesn't export `createContext` — the
  literal failure mode hit and fixed mid-phase). `lib/i18n/config.ts`
  re-exports the same names for existing "use client" call sites.
- `lib/i18n/config.ts`'s `createI18nInstance(locale)` replaces the old
  shared module-level `i18next` singleton for the app's actual runtime
  path — a fresh, independent instance per call, since `I18nProvider`'s
  render now runs once per server request (concurrent requests, in
  principle different locales, on the same Node process) in addition to
  once per browser tab.
- `components/i18n-provider.tsx`: `I18nProvider` now takes
  `initialLocale` as a prop instead of self-detecting via
  `localStorage`/`navigator.language` in a post-mount effect; its
  `languageChanged` listener now writes `document.cookie` instead of
  `localStorage` (plain client-writable cookie, no Server Action/Route
  Handler round-trip needed for the persisted choice to reach the
  server on a future request).

### Verified

- `tsc --noEmit`/`eslint`/`vitest run` (21 tests, up from 15) all clean;
  `next build` confirms all 26 routes moved from `○ (Static)` to
  `ƒ (Dynamic)` as expected.
- `curl` (no browser JS involved) confirmed the raw server HTML itself
  is correct: a `flox-locale=ru` cookie, or an `Accept-Language: ru-RU`
  header with no cookie, both produced `<html lang="ru">` with Russian
  text already in the initial response body.
- Full manual pass against `next start` (production server) + real
  `api`: 10 full fresh browser navigations across `/postlanding`,
  `/landings`, `/pwa` (4 with `?gallery=<id>`, the exact condition the
  original race depended on) — zero hydration errors, read directly from
  the console each time. Confirmed the live language switcher still
  works instantly and persists correctly to a fresh navigation.

## [Incoming Postback Replay] — apps/api now builds the full conversion engine

### Scope

The architecture decision `docs/postback-logs.md` deferred at the end of
the outgoing-replay phase: how should an operator-triggered replay of a
past **incoming** postback call `apps/internal/conversion.Service.Record`,
the exact engine `apps/tracker`'s `/postback/{networkId}` calls for a real
network hit. An `AskUserQuestion` decided `apps/api` should build the same
dependency graph `apps/tracker/main.go` already builds (Redis-cached dedup
store, ClickHouse-backed attribution, async event writer, outgoing-
delivery enqueuer, attempt logger) rather than making an internal HTTP
call to `apps/tracker` — that endpoint has no operator/admin distinction
from a real network hit, so a replay through it would be indistinguishable
from a forged network request in the audit trail. See `docs/postback-
logs.md`.

### Added

- `apps/api/main.go` now constructs a full `conversion.Service`, gated
  behind the same `ch != nil` startup check the rest of the Postback Logs
  feature already uses.
- **`apps/internal/conversion/replay.go`**: `ReplayNetworkLookup`/
  `ReplayRecorder`, two adapters satisfying `postbacklogs`'
  `IncomingNetworkLookup`/`IncomingRecorder` interfaces — `postbacklogs`
  still never imports `conversion` (same one-directional adapter pattern
  `apps/internal/postback`'s `ReplayEnqueuer` already established for
  outgoing replay).
- `postbacklogs.Service.ReplayIncoming` + `POST /postback-logs/replay-
  incoming`: takes the exact fields a `PostbackLog` row already carries
  (`networkId`, `clickId`, `rawStatus`, `eventRef`, `revenue`, `currency`),
  explicitly checks the resolved network's `OrganizationID` against the
  caller's own (a check a real network hit doesn't need, since its
  `{networkId}` URL param **is** the tenant scope) before ever calling
  `Record`.
- Frontend: `replayIncomingPostback` + `useReplayIncomingPostback`;
  `PostbackReplayButton` now branches on `direction`, surfacing the
  replay's actual result (success/duplicate/ignored/error) in the toast
  instead of a fixed message.

### Verified

- Backend: `go build/vet/gofmt/test ./...` all green — new
  `postbacklogs.Service.ReplayIncoming` unit tests against fakes (happy
  path, network-not-found, cross-tenant not-found, required-field
  validation, not-configured) and new `conversion` adapter tests,
  including one that runs an actual replay-then-duplicate-replay sequence
  through a real `*conversion.Service`.
- Frontend: `tsc --noEmit`/`eslint`/`vitest run`/`next build` (production)
  all clean.
- Full manual browser pass against the real running `api`+`tracker`+
  `worker`+`web` dev servers: sent a real incoming postback with no event
  mapping configured (a genuine `error` row), added the mapping, replayed
  it from the Logs tab. Confirmed directly in ClickHouse: the replay
  recorded a new `success` row and triggered a real outgoing delivery
  attempt; replaying the identical attempt again correctly came back
  `duplicate` with no second event or delivery — the CLAUDE.md #3
  dedup/money-correctness guarantee holding under a manual re-trigger, not
  just a network's own retry. Test network and all its rows (Postgres and
  ClickHouse) removed afterward.

## [Postlanding] — wired to a real Postgres-backed API

### Scope

Third slice of the Landing/PWA/Postlanding/Pixels CRUD candidate, right
after [PWA](#pwa--wired-to-a-real-postgres-backed-api-fixes-an-i18n-hydration-race-shared-with-landings).
`postlandings` (migration 00004) was already flat, no children, real
schema — same shape as Landings/PWA. Per-flow Pixels stays mocked; this
phase is Postlanding only. See `docs/postlanding.md`.

### Added

- **`apps/internal/postlanding`**: `model.go`/`handler.go`/`service.go`/
  `repository.go`, mirroring `apps/internal/landing`/`apps/internal/pwa`'s
  shape almost exactly, wired at `/postlandings` in `apps/api/main.go`.
- `events`: at least one required, each value checked against a curated
  6-entry subset of the §43 event model (`ValidEventTypes`) a postlanding
  can plausibly fire on — not a duplicate of the canonical event enum,
  the frontend references the same string values with checked parity.
- Frontend: `lib/api/postlanding.ts` + `hooks/use-postlandings.ts` (real
  API layer, mirrors PWA's exactly); `postlanding-list`/`-form-sheet`/
  `-columns`/`-row-actions.tsx` rewired off the Zustand mock onto the
  real hooks, with loading/error states added.
- A `postlanding` i18n namespace (`en`+`ru`, key-set parity checked
  directly).

### Removed

- `stores/postlandings.ts` and `lib/mock/postlandings.ts` — deleted
  outright once a grep confirmed zero remaining importers.

### Known issue

- The `requestIdleCallback`-based i18n hydration fix from the PWA phase
  mitigates but does not fully eliminate the hydration race on pages
  using `useSearchParams()` inside `<Suspense>` (Landings/PWA/
  Postlanding). Manual testing this phase reproduced the same
  "Hydration failed" error intermittently (~2 of 6 fresh `/postlanding`
  navigations) and confirmed it also reproduces on `/landings` when
  specifically retested — not a new regression or Postlanding-specific,
  React auto-recovers, left as a known non-blocking issue rather than
  expanding scope into a deeper fix. See `docs/postlanding.md`.

### Verified

- Backend: `go build/vet/gofmt/test ./...` all green — `postlanding_test.go`
  mirrors `landing_test.go`'s/`pwa_test.go`'s full test set.
- Frontend: `tsc --noEmit`/`eslint`/`vitest run`/`next build` (production)
  all clean.
- Full manual browser pass against the real running `api`+`web` dev
  servers, in Russian locale: created/edited/paused/resumed/duplicated/
  archived a postlanding (events multi-select and URL validation both
  exercised, copy kept paused status/URL/events verbatim, archived row's
  action menu correctly dropped to Edit/Duplicate only); confirmed full
  Russian rendering throughout. Test postlanding rows deleted directly
  from Postgres afterward (no hard-delete in the UI for this entity).

## [PWA] — wired to a real Postgres-backed API; fixes an i18n hydration race shared with Landings

### Scope

`AskUserQuestion` picked this as the second slice of the Landing/PWA/
Postlanding/Pixels CRUD candidate, right after Landings landed: `pwas`
(migration 00004) was already flat, no children, real schema — same
shape as Landings. Postlanding and per-flow Pixels stay mocked; this
phase is PWA only. See `docs/pwa.md`.

### Added

- **`apps/internal/pwa`**: `model.go`/`handler.go`/`service.go`/
  `repository.go`, mirroring `apps/internal/landing`'s shape almost
  exactly, wired at `/pwas` in `apps/api/main.go`.
- The one real validation difference from Landing: `startUrl` is a
  relative install path (`/install/sweeps`), not an absolute URL, so
  it's deliberately not run through `isValidURL` — covered by its own
  test, `TestUpdateAcceptsRelativeStartURL`.
- Frontend: `lib/api/pwa.ts` + `hooks/use-pwas.ts` (real API layer,
  mirrors `landings`' exactly); `pwa-list`/`-form-sheet`/`-columns`/
  `-row-actions.tsx` rewired off the Zustand mock onto the real hooks,
  with loading/error states added.
- A `pwa` i18n namespace (`en`+`ru`, key-set parity checked directly).
- **Cross-cutting fix**: `components/i18n-provider.tsx` had a genuine
  React hydration race — its post-mount `i18n.changeLanguage()` call
  could fire before a `useSearchParams()`-driven `<Suspense>`
  boundary's own deferred hydration commit (needed for the Content
  Gallery `?gallery=<id>` integration on Landings/PWA/Postlanding),
  producing a real "Hydration failed" error. This pre-dated this phase
  (introduced with `I18nProvider` itself, already live on `/landings`,
  just not caught during that phase's manual testing — absent on pages
  without `useSearchParams()`, e.g. `/networks`/`/offers`).
  `React.startTransition` and a fixed short `setTimeout` both still
  raced; fixed with `requestIdleCallback` (`setTimeout(fn, 0)` Safari
  fallback), which waits for the main thread to actually go idle
  instead of guessing a constant. Retroactively fixes the same latent
  defect on the already-shipped Landings phase with no Landings code
  touched.

### Removed

- `stores/pwas.ts` and `lib/mock/pwas.ts` — deleted outright once a
  grep confirmed zero remaining importers.

### Verified

- Backend: `go build/vet/gofmt/test ./...` all green — `pwa_test.go`
  mirrors `landing_test.go`'s full test set plus
  `TestUpdateAcceptsRelativeStartURL`.
- Frontend: `tsc --noEmit`/`eslint`/`vitest run`/`next build`
  (production) all clean.
- Full manual browser pass against the real running `api`+`web` dev
  servers, in both locales: created/edited/paused/resumed/duplicated/
  archived a PWA (manifest preview matched form values live, copy kept
  non-active status when applicable, archived row's action menu
  correctly dropped to Edit/Duplicate only); confirmed full Russian
  rendering throughout. Also confirmed the hydration fix via repeated
  `/landings` ↔ `/pwa` navigation with the browser console read
  directly: zero hydration errors on either page. Test PWA row deleted
  directly from Postgres afterward (no hard-delete in the UI for this
  entity).

## [Landings] — wired to a real Postgres-backed API

### Scope

`AskUserQuestion` picked this as the smallest slice of the Landing/PWA/
Postlanding/Pixels CRUD candidate: `landings` (migration 00004) was
already flat, no children, real schema — the exact shape
`apps/internal/network` already establishes a real precedent for. PWA,
Postlanding, and per-flow Pixels stay mocked; this phase is Landings
only. See `docs/landings.md`.

### Added

- **`apps/internal/landing`**: `model.go`/`handler.go`/`service.go`/
  `repository.go`, mirroring `apps/internal/network`'s shape almost
  exactly, wired at `/landings` in `apps/api/main.go`.
- The one real business-logic addition beyond the template: an internal
  landing's `url` (`https://cdn.floxlink.io/lnd/{slug}`) moves from
  client-computed (the old mock's form-submit handler) to
  server-computed in `Service.Create`/`Update` from `Name` — a
  client-supplied `url` for `type: internal` is accepted in the request
  shape but always ignored. `Update` only recomputes `url` when `name`
  or `type` actually changed in that call; `Duplicate` goes through
  `Service.Create` (not `Repository.Create`) so a `(Copy)`'s `url` is
  recomputed for its new name, never copied verbatim. Go's `slugify`
  matches `lib/utils.ts`'s exactly, so the form's client-side preview
  and the server-persisted value never disagree.
- Frontend: `lib/api/landings.ts` + `hooks/use-landings.ts` (real API
  layer, mirrors `networks`' exactly); `landing-list`/`-form-sheet`/
  `-columns`/`-row-actions.tsx` rewired off the Zustand mock onto the
  real hooks, with loading/empty/error states added (the old mock was
  synchronous and had none — DoD requires them regardless).
- A `landings` i18n namespace (`en`+`ru`, registered in
  `lib/i18n/config.ts`) — Landings was correctly out of the dedicated
  i18n phase's scope (still mocked then), but leaving it the one
  hardcoded-English holdout next to every other now-real domain page
  the moment it goes real would be a visible regression.

### Removed

- `stores/landings.ts` and `lib/mock/landings.ts` — deleted outright
  once a grep confirmed zero remaining importers, the same "drop it,
  don't fake it" precedent every prior real-backend phase this session
  has followed.

### Verified

- Backend: `go build/vet/gofmt/test ./...` all green — `landing_test.go`
  mirrors `network_test.go`'s full test set (create/get/update/delete,
  invalid-shape validation, pause/activate transitions, duplicate keeps
  status, cross-tenant isolation) plus a dedicated server-computed-URL
  test (client value ignored on create, URL follows a rename, a
  status-only update leaves URL/content untouched).
- Frontend: `tsc --noEmit`/`eslint`/`next build` (production) all clean.
- Full manual browser pass against the real running `api`+`web` dev
  servers, in both locales: created an internal landing (server URL
  matched the client preview exactly) and an external one (client URL
  persisted untouched; an empty URL is rejected client-side); renamed
  the internal one (URL followed via a real round-trip, not just client
  state); duplicated it (copy's URL recomputed for its new name);
  archived one (the archived row's action menu correctly drops
  Pause/Archive, keeping only Edit/Duplicate — matches Networks'
  identical pattern); confirmed full Russian rendering throughout (list,
  form, row actions, archive confirmation, toasts). Test landings
  deleted afterward (no hard-delete exists in the UI for this entity).

## [Postback Replay] — outgoing deliveries can be re-enqueued from the Logs UI

### Scope

`AskUserQuestion` after inspection: incoming and outgoing replay (both
named as real, buildable, no-new-schema actions when Postback Logs
deferred them) turned out to be very different sizes. Outgoing needed
nothing `apps/api` didn't already have. Incoming needs the same
dependency graph (mapper, dedup store, FX, attribution, async event
writer, delivery enqueuer) only `apps/tracker` has ever had — wiring it
into `apps/api` means either duplicating that graph or `apps/api` calling
`apps/tracker`'s `/postback/{networkId}` internally, a real architecture
choice. User picked outgoing-only this phase; incoming deferred again
pending that choice. See `docs/postback-logs.md`.

### Added

- **`apps/internal/conversion.PostgresStore.FindSuccessID`**: resolves a
  ClickHouse `postback_events` row back to the Postgres `postbacks.id`
  `apps/internal/postback`'s delivery queue needs as its `NOT NULL
  source_postback_id` FK (migration 00014), by the exact dedup key
  (org, network, click_id, status, event_ref) — `event_ref` matters
  because a CPA_REDEP click can have more than one successful row, one
  per redeposit.
- **`apps/internal/postback.ReplayEnqueuer`**: adapts
  `postbacklogs.ReplayInput` → `postback.EnqueueInput`, the same
  decoupled-interface-per-consumer pattern `Enqueuer` (→
  `conversion.DeliveryEnqueuer`) already uses. Unlike `Enqueuer`'s
  best-effort, error-swallowing contract (correct for a call inside
  `Record`, where a queuing hiccup must never be reported as a
  conversion failure), `ReplayEnqueuer`'s error goes straight back to the
  HTTP caller — replaying is the entire point of that request.
- **`POST /postback-logs/replay-outgoing`** on `apps/internal
  /postbacklogs` (new `Service.ReplayOutgoing`): takes the exact fields a
  `PostbackLog` row already carries client-side, no second fetch, and
  reuses the exact URL already macro-resolved and dispatched rather than
  re-resolving against current network config.
- Frontend: `postback-log-columns.tsx`'s `actions` column and
  `RotateCcwIcon` button — dropped entirely in the read-only phase — are
  back, outgoing rows only; `useReplayOutgoingPostback` mutation hook;
  `lib/api/postback-logs.ts`'s `PostbackLog` keeps `eventRef` in its JSON
  now (previously dropped — "the UI never renders it" — needed to
  round-trip back for the replay lookup); new `postbacks.json`
  `logs.replayAria`/`logs.toast.*` keys, en+ru.

### Verified

- Backend: `go build/vet/gofmt/test ./...` all green — a new
  `FindSuccessID` integration test against real Postgres (event_ref
  disambiguation between two REDEP rows, tenant isolation, no-match), a
  new `ReplayEnqueuer` unit test, five new `Service.ReplayOutgoing` unit
  tests against fakes.
- Frontend: `tsc --noEmit`/`eslint`/`next build` (production) all clean.
- Full manual browser pass against the real running `api` + `tracker` +
  `worker` + `web` dev servers — the first phase in this arc needing all
  four, since a genuine outgoing delivery only exists once a real
  incoming postback creates one: created a real network whose postback
  URL points at an unreachable local port, a real event mapping, sent
  two real incoming postbacks through `apps/tracker`'s actual endpoint,
  confirmed `apps/worker`'s `Deliverer` picked both up and logged
  `retrying` attempts. Clicked Replay; confirmed the toast and, directly
  in Postgres, that a **new** `postback_deliveries` row was created
  (fresh id, `attempt_count` reset to 1, correct `source_postback_id`)
  and that the worker picked it up on its own next poll. Confirmed
  Replay is absent on incoming rows. Confirmed error paths directly:
  malformed org header → 422, missing required field → 422 with field
  detail, a status never successfully recorded → 404. Test network and
  every row it produced (Postgres `networks`/`event_mappings`/
  `postbacks`/`postback_deliveries`, ClickHouse `postback_events`) were
  deleted outright afterward — psql/curl-seeded rows with no UI path at
  all, not entities the app itself only lets you archive.

## [Frontend i18n] — client-side EN/RU internationalization foundation

### Scope

A cross-cutting frontend infra phase, added ahead of the next domain per
direct instruction: `react-i18next` foundation (`lib/i18n/config.ts`,
`I18nProvider`, `LanguageSwitcher`, `useLocale()`), then migrated every
currently-real (backend-wired) screen to it — Campaigns, Traffic
Sources, Networks, Offers, Stream Sets/Filters/Flows, Routing Simulator,
Cost, Conversions, Event Mappings, and Postback Logs, plus the shared
app shell (sidebar/topbar/breadcrumbs/command menu) and `DataTable`/
`ErrorState`/`LoadingState`. See `docs/frontend-i18n.md` for the full
namespace map, key conventions, and formatting/pluralization rules.

Session picked up mid-phase: the `en`/`ru` locale JSON (all 11
namespaces, both languages, fully translated) and the shell/`common`/
Campaigns/Traffic Sources/Networks component migrations already
existed; this pass wired the remaining six domains — Offers, Stream
Sets, Routing Simulator, Cost, Conversions, and Postbacks (Event
Mapping/Incoming/Outgoing/Logs panels) — to the pre-written keys.

### Added

- `useTranslation` wired into every remaining component in the six
  domains above: list/detail views, row actions, form sheets (Zod
  schemas converted to `buildXFormSchema(t)` factories, matching the
  existing `buildSourceFormSchema`/`buildNetworkFormSchema` pattern —
  validation messages need the live translator), and column-def
  factories (`t` threaded through as the first argument, matching
  `networkColumns`).
- `lib/filters.ts`: `FILTER_OPERATOR_I18N_KEY` (mirrors the existing
  `FIELD_GROUP_I18N_KEY`); `checkRE2Compatible`/`validateCountryValue`
  now take a `TFunction` and return translated messages.
- `lib/api/conversions.ts`: `CPA_STATUS_I18N_KEY`, the same
  `Record<Value, string>`-of-i18n-keys pattern as
  `SOURCE_TYPE_I18N_KEY`/`COST_INTEGRATION_I18N_KEY`, reused by
  Conversions, Event Mapping, and Postback Logs (all three render CPA
  status badges).

### Fixed (found during manual verification, not pre-existing scope)

- **`campaign-detail-view.tsx`** (Campaign detail page: Overview/Cost/
  Simulator/Settings tabs, stat cards, danger zone) was entirely
  untranslated — it wraps the newly-migrated Cost/Simulator/Stream Sets
  content in English chrome, so a Russian-locale user would see a
  jarring English-tabs-around-Russian-content page. Pre-written
  `campaigns.json` already had a complete `detail`/`overview` namespace
  sitting unused; wired it in.
- **`components/ui/multi-select.tsx`** (shared component, used by
  Offers' Countries picker and the Stream Sets filter builder's Values
  picker): its search placeholder was a hardcoded `Search
  ${label.toLowerCase()}...` template — once callers started passing a
  translated label (e.g. "ГЕО"), this produced a broken mixed-language
  string ("Search гео..."). Added `common.multiSelect.{none,
  searchPlaceholder}` keys and switched the component to
  `useTranslation("common")` internally (the same self-contained
  pattern `data-table.tsx` already uses), reusing
  `common.dataTable.selected`/`emptyTitle` for the count summary and
  "no results" text rather than duplicating them.

### Verified

- `tsc --noEmit`, `eslint`, `next build` (production build, all 26
  routes) all clean.
- `vitest run` — 15/15 passing (`lib/i18n/config.test.ts`, pre-existing,
  unchanged by this pass).
- Full manual browser pass against the real running `api` + `web` dev
  servers, in both locales: created a real network, offer, campaign,
  stream set (with a filter condition and a device-vocab value), and an
  event mapping. Confirmed correct `ru` rendering for all six domains —
  field-group labels, operator words, vocab values, pluralized counts
  (including a genuine `count=0` → Russian "many" form via
  `mappedCount_many`), the routing simulator's full pipeline trace, and
  the multi-select fix — then switched to `en` and confirmed the
  fallback locale still renders correctly. Confirmed backend-generated
  strings (`SimulateResult.destination.label`, `.stickyNote`) correctly
  stay in English per `docs/frontend-i18n.md`'s stated backend
  boundary. Test network/offer/campaign archived afterward (no
  hard-delete exists for these entities in the UI — archiving is this
  app's own non-destructive removal path).

## [Postback Logs] — Wired to real ClickHouse-backed API (read-only, replay deferred)

### Scope

Two rounds of `AskUserQuestion`. First confirmed Postback Logs as the
last remaining piece of the Conversions/Postbacks domain. Second split
scope further once inspection found the old mock's "Replay" button was a
genuine write action — re-invoking `apps/internal/conversion.Service
.Record` for an incoming row, or re-enqueuing a `apps/internal/postback`
delivery for an outgoing one. Both are real and buildable with no schema
changes, but are a second capability beyond a pure read view; user chose
logs-list-only this phase, replay deferred as its own follow-on. See
`docs/postback-logs.md`.

### Added

- **`apps/internal/postbacklogs`** (new, plural — distinct from the
  existing singular `apps/internal/postbacklog`, Phase 24's write-side
  queue/producer that feeds `postback_events`, untouched this phase): a
  thin read layer over ClickHouse's `postback_events`, mirroring
  `apps/internal/analytics`/`apps/internal/conversions`'s own shape. `GET
  /postback-logs` (org-wide, both directions mixed in one list,
  date-ranged, paginated).
- **`chstore` additions**: `ListPostbackAttempts`/`CountPostbackAttempts`
  — the first read methods against `postback_events` (previously
  write-only, fed by `InsertPostbackAttempts`).
- Frontend: `lib/api/postback-logs.ts` + `hooks/use-postback-logs.ts`
  (new, TanStack Query). `PostbackLogsPanel`/`postback-log-columns.tsx`
  rewritten against the real hook, with a wider, real `PostbackResult`
  vocabulary (incoming: success/duplicate/ignored/error; outgoing:
  queued/processing/success/failed/retrying/dead) replacing the old
  mock's three-value fiction.

### Fixed (closing a gap the Event Mappings phase documented)

`IncomingPostbacksPanel` was still reading the mock `useNetworksStore`/
`useEventMappingsStore` even after Event Mappings CRUD landed real —
flagged as a documented inconsistency in that phase's own changelog
entry. Switched to the real `useNetworks()`/`useEventMappings()` hooks
(both already existed), closing the gap.

### Removed

- `postback-log-columns.tsx`'s Replay action column and its
  `RotateCcwIcon` button — dropped entirely, not disabled or faked,
  matching this phase's scope decision above.
- **`stores/postback-logs.ts`**, **`lib/mock/postback-logs.ts`**,
  **`stores/networks.ts`**, **`lib/mock/networks.ts`** — once
  `PostbackLogsPanel` and `IncomingPostbacksPanel` both moved to real
  hooks, a repo-wide grep found all four had zero remaining importers
  (each only imported the next one in the chain). Deleted outright, the
  same "drop it, don't fake it" precedent as the Routing Simulator
  phase's `stream-sets` mock/store pair.

### Verified

- Backend: `go build/vet/gofmt/test ./...` all green — 2 new `chstore`
  integration tests against real ClickHouse, 2 new
  `postbacklogs.Service` unit tests against a fake repository.
- Frontend: `tsc --noEmit`/`eslint` clean.
- Full manual browser pass against the real running `api` + `web` dev
  servers: created a real network and a real event mapping, seeded 4
  real `postback_events` rows (an incoming/outgoing success pair, an
  incoming error, an outgoing retrying) via a throwaway, never-committed
  `chstore.EventStore.InsertPostbackAttempts` call. Confirmed the Logs
  tab renders all four correctly — raw-to-mapped status display for
  incoming, mapped-only for outgoing, correct result badges across the
  wider real vocabulary, no Replay column — and the Incoming tab now
  shows the real network id and a correct real mapped-count badge. Test
  network and its `postback_events` rows removed afterward.

### Domain complete

Conversions, Event Mappings, and Postback Logs (read-only) are all real
now — the "Conversions/Postbacks" domain is fully wired except for the
deliberately-deferred Replay action.

## [Event Mappings] — CRUD wired to real Postgres-backed API

### Scope

Confirmed via `AskUserQuestion` (the recommended option) as the smallest
remaining piece of the Conversions/Postbacks domain — a small,
self-contained CRUD slice with no ClickHouse involved, shaped like
Networks/Offers. See `docs/event-mappings.md`.

### Added

- **`apps/internal/eventmapping`** — writes rows for `event_mappings`
  (migration 00012), the table `apps/internal/conversion.PostgresMapper
  .MapStatus` already read at postback-ingest time (Phase 23). Never
  duplicates that lookup — the same relationship `streamset` has to
  `routingstore`. `FloxStatus` reuses `event.Type` directly (validated
  via the already-existing `event.Type.IsCPA()`), not a redeclared enum.
- `GET`/`POST /event-mappings` (org-wide, not per-network — the panel
  groups by `networkId` client-side, matching the old mock's org-wide
  array) and `DELETE /event-mappings/{id}`. No `PATCH`: the UI only ever
  adds or removes a mapping, never edits one in place.
- Duplicate detection relies on the database's existing unique index
  (`network_id, lower(network_status)`) rather than a race-prone
  check-then-insert — `Create` catches Postgres `23505` into a real
  `apierror.Conflict`, this codebase's first `23505` catch (every prior
  one was `23503` FK violations).
- Frontend: `lib/api/event-mappings.ts` + `hooks/use-event-mappings.ts`
  (new, TanStack Query). `EventMappingPanel` rewired to the real hooks.

### Fixed (a pre-existing gap, not new to this phase)

`EventMappingPanel` was still reading the mock `useNetworksStore` (stale
fabricated network ids like `net_afftrust`) even though it already
imported the real `CPA_STATUSES` from the Conversions phase — switched
to the real `useNetworks()` hook, since managing mappings for networks
that couldn't match any real `network_id` would have been pointless.

### Not changed, documented instead

`stores/event-mappings.ts` and `lib/mock/event-mappings.ts` were **not**
deleted, unlike the Routing Simulator phase's stream-sets mock/store
pair — both are still read by `IncomingPostbacksPanel` (a "mapped count"
badge) and the deferred Postback Logs mock, both out of this phase's
scope. Net result: the Postbacks page's "Event Mapping" tab now manages
real data while its "Incoming" tab still shows stale mock networks with
a mapped-count badge sourced from the mock store — confirmed live, not
just inferred, and documented in `docs/event-mappings.md` rather than
papered over.

### Verified

- Backend: `go build/vet/gofmt/test ./...` all green — 5 new
  `eventmapping` tests (CRUD round-trip, invalid `FloxStatus` rejected,
  case-insensitive duplicate rejected with a real conflict apierror,
  unknown network id rejected, full cross-tenant isolation).
- Frontend: `tsc --noEmit`/`eslint` clean.
- Full manual browser pass against the real running `api` + `web` dev
  servers: created two real networks, added a mapping through the real
  form (confirmed landed in Postgres via a direct API call), attempted a
  case-insensitive duplicate and confirmed the real `409` by inspecting
  the network request directly, removed the mapping and confirmed it was
  gone both in the UI and via the API, confirmed the Incoming tab's
  stale-but-non-crashing inconsistency live. Test networks removed via
  the real `DELETE` endpoint afterward.

## [Conversions] — List + detail/timeline wired to real ClickHouse-backed API

### Scope

Two rounds of `AskUserQuestion`. First picked "Conversions/Postbacks" as
the domain from three candidates. Inspection then found the hard backend
(dedup, delivery, ClickHouse logging) already fully built and wired into
`apps/tracker`/`apps/worker` — only a browser-facing read API was
missing, and the domain was really three separable pieces. Second
question narrowed to this slice: Conversions list + detail/timeline,
deferring Postback Logs and Event Mappings CRUD. See `docs/conversions.md`.

### Added

- **`apps/internal/conversions`** (new, plural — distinct from the
  existing singular `apps/internal/conversion`, Phase 23's write-side
  dedup/status-progression engine, untouched this phase): a thin read
  layer over ClickHouse's `conversion_events`/`click_events`/
  `tracking_events`, mirroring `apps/internal/analytics`'s own shape. `GET
  /conversions` (org-wide, date-ranged, paginated) lists CPA_* rows; `GET
  /conversions/{clickId}` returns a merged, chronological timeline of
  every funnel + conversion event for that click_id.
- **`chstore` additions**: `ListConversions`/`CountConversions`/
  `ConversionsByClickID`/`FunnelByClickID` — the first read methods on
  `EventStore` for `click_events`/`tracking_events`/`conversion_events`
  (previously write-only, fed by `InsertBatch`).
- A click_id can carry more than one `conversion_events` row (HOLD, then
  ACCEPT, then REDEP, ...) — real status history, not duplicate rows to
  dedupe. The old mock's fixed six-stage funnel (Click/Landing/PWA/Offer/
  Conversion/Postback, four stages fabricated with synthetic sentences)
  is replaced by a real, variable-length chronological list of whatever
  §43 events actually happened for that click_id.
- Frontend: `lib/api/conversions.ts` + `hooks/use-conversions.ts` (new,
  TanStack Query). `conversion-list.tsx`/`conversion-detail-view.tsx`/
  `conversion-timeline.tsx`/`conversion-columns.tsx` all rewritten
  against the real hooks. `CpaStatus`/`CPA_STATUSES` moved from
  `lib/mock/conversions.ts` to `lib/api/conversions.ts` (a real domain
  enum, not mock-specific) — every consumer, including the still-mocked
  Postback Logs/Event Mappings features, now imports it from there.

### Removed

- **Offer column**: `conversion_events` carries no `offer_id` (only
  `flow_id`, which would need a separate Postgres join to resolve — out
  of scope). Campaign and Network (both directly on the ClickHouse row)
  took its place.
- **Postback delivery status / "Resend postback"**: the deferred
  Postback Logs domain — the old mock's `postbackStatus` column, its
  StatCard, and the resend button are gone entirely, not shown as "—" or
  left disabled.
- **`stores/conversions.ts`** — no remaining importers once
  `ConversionList`/`ConversionDetailView` switched to the real hooks;
  deleted outright. `lib/mock/conversions.ts` stays, trimmed to just
  what the still-mocked Postback Logs feature cross-references.

### Fixed

`GET /conversions?to=YYYY-MM-DD` parsed `to` as midnight UTC of that
date. `internal/analytics`'s daily endpoints do the same thing safely,
because they query pre-aggregated day-granularity materialized views
where a date-only comparison is exactly right. This package queries raw
`event_at` timestamps — every event later that same day was silently
excluded (`event_at <= to` false for anything after 00:00:00). Caught
live seeding test data "today" and passing an explicit `?to=<today>`.
Fixed by pushing an explicit date-only `to` to end-of-day (`+24h-1ns`) in
the handler.

### Verified

- Backend: `go build/vet/gofmt/test ./...` all green — `chstore`
  integration tests against real ClickHouse for all four new query
  methods, `conversions.Service` unit tests against a fake repository
  (range/limit validation, chronological funnel+conversion merge, the
  no-events-found 404).
- Frontend: `tsc --noEmit`/`eslint` clean.
- Full manual browser pass against the real running `api` + `web` dev
  servers: seeded two real click_ids' worth of ClickHouse events (one
  full funnel, one bare click-then-decline) via a throwaway, never-
  committed `chstore.EventStore.InsertBatch` call against a real test
  campaign/network created through the real API. Confirmed the list's
  multi-row status history with resolved campaign/network names, both
  detail pages' real and genuinely different-length timelines, a clean
  404 error state for an unknown click_id, and that the still-mocked
  Postback Logs/Event Mapping panels (both touched by the `CpaStatus`
  move) kept rendering correctly. Test campaign, network, and their
  ClickHouse rows removed afterward.

## [Routing Simulator] — Wired to a real /routing/simulate endpoint

### Scope

Direct instruction, no `AskUserQuestion` scope negotiation needed — the
user named this exact candidate from the previous phase's own "NEXT"
list. See `docs/routing-simulate.md`.

### Added

- **`apps/internal/routingsimulate`** (new package) — `Service.Simulate`
  loads a campaign's real routing config via
  `routingstore.LoadRoutingConfig` and calls `routing.Engine.Explain`
  (never `Resolve` — the simulator's whole point is the trace `Resolve`
  doesn't return), reshaping the result into the frontend's wire
  contract. `Handler` mounts `POST /campaigns/{campaignId}/routing/
  simulate`, tenant-scoped like every other handler. No new routing
  decision logic anywhere (CLAUDE.md #1) — this is a thin reshape of an
  already-existing pure evaluation into JSON.
- **`routing.Explanation.DestinationLabel`**: the same human label
  (`"Offer"`, `"Redirect"`, `"Stream Set fallback"`, `"Campaign
  fallback"`, `"No destination configured"`) `resolveDestination`
  already computed for `RouteResult.Reason` internally, now also stored
  structurally instead of discarded.
- **`routing.Trace`/`StreamSetEvaluation`/`FlowCandidate` JSON tags**
  matching the frontend's field names exactly — reused directly in the
  simulate response with no separate DTO layer, honoring `trace.go`'s
  own doc comment written for exactly this moment.
- Sticky is never simulated (`Sticky: nil` always — a fabricated cookie
  would look identical to a real returning visitor's, which is
  misleading for a debugging tool). The response's `stickyNote` is
  generated from the campaign's real `sticky_flow` flag instead of the
  old mock's static, increasingly-stale string.
- `deriveVisitKey` reimplemented byte-for-byte in Go, matching the old
  frontend mock's algorithm. Unlike the mock, an all-empty request
  against a matched stream set with more than one tied weighted flow now
  surfaces a real `422` (`routing.ErrNoVisitKey`) instead of silently
  guessing.
- **Frontend**: `lib/routing-simulate.ts` (the mock) deleted outright —
  its type definitions moved into `lib/api/routing.ts`, the seam file
  built in an earlier phase specifically for this swap. `routing-
  simulator-view.tsx` dropped its `useStreamSetsStore` read (the server
  now loads stream sets itself) and calls `simulateRoute(campaignId,
  request)` instead of `(streamSets, fallbackUrl, request)`.
  `simulator-form.tsx`/`stream-set-trace.tsx` needed only an import-path
  change. `simulator-result.tsx` dropped the `selectedFlow` boolean
  check (now `flowCandidates.some(c => c.selected)`) and the `kind`-keyed
  `DESTINATION_LABEL` lookup (now renders `destination.label` directly).

### Removed

- `lib/mock/stream-sets.ts` / `stores/stream-sets.ts` — kept in the
  Stream Sets phase specifically because the Routing Simulator was their
  one remaining reader. Once the Simulator switched to the real API, a
  repo-wide grep found zero remaining importers of either; deleted
  outright rather than left as an orphaned pair.

### Fixed

Two real bugs caught during manual browser verification, both the same
root mistake hit twice: Go's `encoding/json` `omitempty` omits *any*
zero-length slice, nil or not — not just nil ones.

- `routingsimulate.Response.FlowCandidates` was `nil` (no weighted draw
  ever ran) when no stream set matched, encoding as JSON `null` and
  crashing `SimulatorResult`'s unconditional `.some()` call. Fixed by
  normalizing to `[]` in `Service.Simulate`.
- `routing.Trace.Children` and `streamset.FilterNode.Children` both had
  `omitempty`. An empty top-level filter group ("no filters — matches
  all traffic," a normal, UI-documented configuration) produces a real,
  non-nil, zero-length `Children` that `omitempty` hid anyway — crashing
  not just the Simulator but the Stream Sets card on *any* campaign with
  an empty-filter stream set, since `hydrateFilterNode`/`FilterTraceView`
  call `.map()`/`.length` on `children` unconditionally. Fixed by
  dropping `omitempty` from both types' `Children` tags. Neither bug was
  reachable in any prior phase's verification pass, because no earlier
  test fixture had exercised an empty root filter group through a real
  HTTP round-trip.

### Verified

- Backend: `go build/vet/gofmt/test ./...` all green — 5 new
  `routingsimulate` tests (match + weighted pick, no-match campaign
  fallback with a non-nil-`FlowCandidates` regression assertion,
  ambiguous-tie `422`, sticky-note reflecting the campaign's real
  `sticky_flow` flag, cross-tenant isolation) and 2 new regression tests
  for the `omitempty` bug (`streamset`, `routing`). The pre-existing
  18-case `routing` conformance fixture passes unchanged.
- Frontend: `tsc --noEmit`/`eslint` clean.
- Full manual browser pass against the real running `api` + `web` dev
  servers: matched and non-matched filter simulation against a real
  stream set; a weighted 50/50 tie-break with a deterministic pick;
  sticky-enabled note toggling live against a real Postgres update. Both
  bugs above were caught and fixed during this same pass. Test campaign
  (cascading to its stream set), offer, and network removed via real
  `DELETE` afterward.

## [Stream Sets, Filters & Flows CRUD] — Frontend/Backend Integration, next domain slice

### Scope

Confirmed via `AskUserQuestion` (the recommended option): Stream Sets +
Filters (recursive AND/OR tree) + Flows CRUD, dropping the Landing/PWA/
Postlanding stage pickers and per-flow Pixels from the writable form —
none of those four have a real backend yet, and their DB columns/tables
are nullable/optional specifically so a Flow can exist without them.
Wiring the Routing Simulator to a real `/routing/simulate` endpoint was
scoped out as its own smaller follow-on phase. See `docs/stream-sets.md`.

### Added

- **`apps/internal/streamset`** writes rows for the read path that
  already existed (`apps/internal/routingstore` + `apps/internal/routing`,
  used by the tracker's hot-path redirect) — never duplicates matching or
  weighted-selection logic (CLAUDE.md #1), and reuses
  `routing.FilterField`/`Operator`/`Joiner`/`DestinationKind`/
  `StreamSetStatus` directly rather than redefining the same enums twice.
- **`FilterNode`**: a flattened discriminated-union struct (mirrors how
  `routing/trace.go`'s own `Trace` type already solves the same
  frontend-union-on-the-wire problem), with no `id` — `routing.FilterNode`
  doesn't need one either. The frontend hydrates fresh client-side ids on
  load (`hydrateFilterNode`/`hydrateRootFilter`) for
  `filter-group-builder.tsx`'s id-addressed tree-mutation helpers, and
  strips them back out before saving (`dehydrateFilterNode`).
- **Server-side filter validation**: `MATCHES` conditions compiled with
  Go's real `regexp.Compile` (which *is* RE2, CLAUDE.md #8 — no separate
  heuristic needed, unlike the frontend's own client-side-only
  `checkRE2Compatible`); country codes checked against ISO-3166 alpha-2
  (rejects `"UK"`, same as the frontend's `validateCountryValue`, now
  enforced server-side too — "never trust the client as the source of
  truth" per `lib/filters.ts`'s own comment about its own heuristic).
- **Offer-destination network derivation**: a flow's `networkId` is
  looked up from the offer's own `network_id`, never trusted from the
  client — the flows table denormalizes both ids (its own CHECK
  constraint requires it), but there's exactly one correct network for a
  given offer. `TestOfferDestinationDerivesNetworkFromOffer` sends a
  deliberately wrong network id alongside a real offer id and asserts the
  real one wins.
- **Priority**: never client-supplied (no field in the form at all) —
  `Create` always appends after the campaign's existing stream sets,
  `POST .../stream-sets/reorder` takes a full dragged-list order and
  rewrites every priority in one transaction, matching
  `stores/stream-sets.ts`'s own `addStreamSet`/`reorder` exactly.
- **Frontend**: `lib/api/stream-sets.ts` + `hooks/use-stream-sets.ts` (new,
  parallel to the untouched mock) wire the existing Stream Sets card to
  the real API. `FlowFunnel` (which rendered the now-dropped Landing/PWA/
  Postlanding/Offer/Redirect/Fallback chain) is deleted outright, replaced
  by a new, leaner `FlowDestinationEditor` (Offer-or-Redirect + a
  read-only Fallback preview) reusing the generic `FlowNode` component
  unchanged. `filter-group-builder.tsx`/`filter-condition-editor.tsx`
  needed zero changes — both were already pure, store-free components
  operating only on the tree shape passed through props.
- **`docs/stream-sets.md`**; `docs/frontend-integration.md` and
  `ARCHITECTURE.md`'s §6-SHARED note updated — Stream Sets/Filters/Flows
  are off the "still mocked" list, and the Routing Simulator's own note
  now names what's actually left (a thin `/routing/simulate` handler over
  already-built primitives, not a new engine).

### Fixed

- **A real render-loop bug, caught during manual verification, not
  shipped**: the new offer picker would select correctly for an instant,
  then silently reset to empty. Root cause: React Hook Form's
  `useFieldArray().update()` is documented (by RHF itself) to unregister
  and re-register the field row on every call, remounting that row's
  whole subtree — including its Radix `<Select>`s — on every keystroke or
  selection; the remounting Select fired a stray `onValueChange("")` that
  raced the real selection and won. Confirmed live: a temporary
  mount/unmount effect log fired on every field edit, and a temporary
  handler log showed three `onValueChange` calls per click (the real
  offer id, then `""` twice more). Fixed by switching every per-flow edit
  from `flowArray.update(index, {...flow, ...patch})` to
  `setValue(`flows.${index}`, {...flow, ...patch})`, which patches the
  field in place with no remount. Hardened defensively alongside: the
  offer `<Select>`'s controlled value is never handed a raw empty string
  (Radix's own docs warn against an empty `SelectItem` value for the same
  underlying fragility) — a `NO_OFFER` sentinel stands in for "nothing
  chosen yet" on the wire instead.

### Notes

- **A pre-existing schema quirk inherited, not introduced**: `filter_
  conditions.position`/`filter_groups.position` are two independent
  ordering sequences (conditions among themselves, sub-groups among
  themselves), not one interleaved order — the read path
  (`routingstore.loadFilterTrees`) has always reconstructed a group's
  children as "conditions first, then nested groups" regardless of
  original interleaving, since AND/OR evaluation is commutative and never
  cared. This phase's `insertFilterGroup` matches that exact reconstruction
  rather than inventing a different write-side order the reader would
  just re-order anyway.
- 6 new integration tests in `apps/internal/streamset` (CRUD round-trip
  with a nested two-level filter tree, rejecting empty flows / a
  condition-as-root / an invalid country code / an invalid RE2 pattern /
  an incomplete BETWEEN, the offer-network derivation proof, reorder
  rewriting priority correctly, duplicate keeping status and copying the
  tree+flows with fresh ids, full cross-tenant isolation including
  create-against-another-org's-campaign) — all pass against real
  Postgres.
- Verified end-to-end against the real running `api` + `web` dev servers:
  created a network, offer, and campaign; built a stream set through the
  complete form (name, one filter condition, one offer-destination flow);
  confirmed via the real API response that the filter tree and resolved
  network id were exactly right; edited it and confirmed the hydrated
  filter tree and offer selection both pre-filled correctly; duplicated
  it (kept the tree, flow, and `active` status); toggled status to
  `paused` live; reordered two stream sets via the real endpoint and
  confirmed the new priorities survived a page reload. Test campaign
  (cascading to its stream sets), offer, and network removed via the real
  `DELETE` endpoints afterward — all net-new for this phase, no
  pre-existing seed data at risk.
- Backend: `go build/vet/gofmt/test ./...` all green. Frontend:
  `tsc --noEmit` and `eslint` both clean.

## [Networks & Offers CRUD] — Frontend/Backend Integration, next domain slice

### Scope

"Offers" was picked as the next slice on the assumption its network
reference could stay free-text until Networks existed separately — wrong:
`offers.network_id` is `NOT NULL REFERENCES networks (id)` (00003). This
was discovered before any code was written and surfaced via
`AskUserQuestion` rather than silently absorbing the extra scope or
silently weakening the schema to route around it. The user chose to build
Networks + Offers + nested `offer_links` together, completing §27's own
stated hierarchy (Network → Offer → Offer Link) in one phase. See
`docs/networks-offers.md`.

### Added

- **`apps/internal/network`** — flat entity CRUD, mirrors
  `internal/trafficsource` closely (same handler/service/repository
  split, same Duplicate-keeps-status reasoning). `Delete` is the mirror
  image of `trafficsource`'s own delete story: `offers.network_id`
  `CASCADE`s (deleting a network deletes its offers, by original schema
  design, not a choice this phase makes), while
  `flows.destination_network_id` (no Flow CRUD exists yet) would
  `RESTRICT` — caught defensively with the same `23503`-to-409 pattern.
- **`apps/internal/offer`** — the first non-flat domain in this session.
  `NetworkID` is validated against the owning org via
  `NetworkBelongsToOrg` before insert/update (§36-TENANCY, same pattern as
  `campaign`→`traffic_source`). `offer_links` use whole-array replace
  (delete-all/insert-all in one transaction) on every write where `Links`
  is present — matching the frontend form's `useFieldArray`, which
  submits every link on every save rather than a diff. No standalone link
  endpoint; a link is never addressed independently of its offer.
- **`OptionalCap`**: a small custom `json.Unmarshaler` distinguishing
  PATCH's three real states for `cap` — not sent (leave unchanged), sent
  as `null` (clear to uncapped), sent as a number (set that cap). A plain
  `*int` can't tell "key absent" apart from "key present with `null`"; the
  frontend side needed no equivalent trick — `UpdateOfferInput.cap` is
  typed `number | null | undefined` and relies on `JSON.stringify` already
  dropping `undefined` keys.
- **Frontend**: `lib/api/networks.ts` + `lib/api/offers.ts` (new, parallel
  to the untouched mocks) wire the existing mock CRUD UIs
  (`network-*`/`offer-*` feature components) to the real API, unchanged in
  shape. `lib/mock/{networks,offers}.ts` and `stores/{networks,offers}.ts`
  are left exactly as they were — stream-sets/postbacks/conversions
  (still fully mocked) import them transitively, same situation
  `lib/mock/campaigns.ts` was left in after Phase 27.
- **`docs/networks-offers.md`**; `docs/frontend-integration.md` updated —
  Networks and Offers are off the "still mocked" list now.

### Fixed

- **A real render-loop bug, caught during manual verification, not
  shipped**: `offer-form-sheet.tsx` crashed on open with React's "Maximum
  update depth exceeded." Every other rewritten form this session uses
  React Hook Form's `values` option so it re-syncs without a full remount;
  combined with this form's `useFieldArray` (the links list) and a
  `MultiSelect` (countries), a fresh `values` object literal on every
  render caused RHF to keep re-syncing the field array, which triggered a
  Radix Popper ref-callback state update, which caused another render,
  forever. Fixed by reverting to plain `defaultValues` (read once per
  mount) and restoring `key={target?.id ?? "new"}` on all three list
  components' form-dialog wrappers — the remount-on-target-change pattern
  the original mock-backed components already had, which this session's
  earlier rewrites (Traffic Sources, then this phase's own first pass)
  had quietly dropped when extracting the dialog into its own component.
  `values` stays correct for the two simple forms with no array fields.

### Notes

- 6 new integration tests in `apps/internal/network` (CRUD round-trip,
  invalid-URL rejection, pause/activate transitions, duplicate-keeps-
  status, a direct cascade-delete-to-offers proof, cross-tenant
  isolation) and 6 in `apps/internal/offer` (CRUD + country/currency
  normalization, no-links/no-countries/non-positive-payout/cross-org-
  network all rejected, whole-array link replace, the three-state Cap
  PATCH, duplicate keeps status and copies links with fresh ids,
  cross-tenant isolation) — all pass against real Postgres.
- Verified end-to-end against the real running `api` + `web` dev servers:
  created a network, then a full offer against it (network picker, GEO
  multi-select, payout, currency, daily cap, one tracking link) through
  the UI; edited it and confirmed every field pre-filled correctly
  including the link URL, with the Status field present (edit-only);
  paused it (status flipped live); duplicated it (copy kept every field
  including GEOs/cap, and correctly stayed `paused` rather than
  resetting). Test offers and the test network removed via the real
  `DELETE` endpoints afterward — both net-new for this phase, so no
  pre-existing seed data was at risk (unlike Traffic Sources' incident).
- Backend: `go build/vet/gofmt/test ./...` all green. Frontend:
  `tsc --noEmit` and `eslint` both clean.

## [Traffic Sources CRUD] — Frontend/Backend Integration, next domain slice

### Added

- **`apps/internal/trafficsource`** grew from Phase 27's deliberately
  read-only `List` into full CRUD (`Create`/`Get`/`Update`/`Delete`/
  `Duplicate`/`Pause`/`Activate`), mirroring `internal/campaign`'s
  handler→service→repository split exactly. Chosen over Offers/Networks/
  Stream Sets (confirmed via `AskUserQuestion`) as the smallest next
  slice: the read endpoint and a complete mock CRUD UI already existed,
  and the entity has no nested children.
- **`Service.Duplicate` keeps the source's status as-is** — unlike
  `campaign.Service.Duplicate`, which resets a copy to `"draft"`.
  `TrafficSource` has no draft-equivalent status, and the mock store this
  replaced never reset it either.
- **`Repository.Delete` turns a Postgres FK violation into a clean 409**:
  `campaigns.traffic_source_id` has no `ON DELETE` clause (defaults to
  `RESTRICT`, deliberately — a source with campaigns still pointing at it
  shouldn't silently vanish). Catches error code `23503` and returns
  `apierror.Conflict` instead of letting a raw 500 through.
- **Frontend**: `lib/api/traffic-sources.ts` + `hooks/use-traffic-sources.ts`
  fully rewritten off the mock store onto the real API; `source-list.tsx`/
  `source-form-sheet.tsx`/`source-row-actions.tsx`/`source-columns.tsx`
  wired to real queries/mutations with their existing UI shape unchanged
  (create/edit sheet, pause/resume/duplicate/archive row actions, tags
  column still local-mock same as campaigns).
- **`docs/traffic-sources.md`**; `docs/frontend-integration.md` updated —
  Traffic Sources is off the "still mocked" list now.

### Removed

- **`lib/mock/traffic-sources.ts` and `stores/traffic-sources.ts` deleted
  outright** — unlike campaigns' equivalents (Phase 27 kept
  `lib/mock/campaigns.ts`/`stores/campaigns.ts` because other still-mocked
  features — conversions, tag assignments — import them transitively),
  nothing outside the traffic-sources feature ever referenced these, so
  there was nothing to preserve.

### Notes

- 7 new integration tests in `apps/internal/trafficsource` (real Postgres,
  `DATABASE_URL`-gated): create/get/update/delete round-trip, invalid
  tracking-template URL rejected, pause/activate transitions (incl.
  idempotency from the target state and rejection from archived),
  duplicate keeps status, delete conflicts when a campaign references the
  source, and full cross-tenant isolation across get/update/delete/list.
- Verified end-to-end against the real running `api` + `web` dev servers:
  created a source through the UI, paused it (status flipped live),
  duplicated it (copy correctly kept `paused`, not reset), opened Edit
  and confirmed real pre-filled data with the Status field present
  (edit-only). The FK-conflict path was verified directly via `curl`
  against a real referencing campaign — a clean `409`, not a raw
  Postgres error.
- **A real mistake made and corrected during verification, not shipped
  silently**: one of the dev org's two seeded traffic sources ("Facebook
  Ads") was deleted while testing the delete flow, before the
  FK-referencing test case that would have caught it (no campaign existed
  at that moment to trigger the conflict path). Noticed immediately,
  recreated with the same name/type to restore dev state — see
  `docs/traffic-sources.md` for the full account.
- Backend: `go build/vet/gofmt/test ./...` all green. Frontend:
  `tsc --noEmit` and `eslint` both clean.

## [Phase 27-COST] — Cost Ingestion

### Added

- **`apps/internal/cost`** (repository/service/handler, mirrors
  `campaign`'s split) — manual cost entry MVP, closing the Spend/Profit/
  ROI/CPA gap Phase 27 documented and deliberately left open.
  `Repository.Upsert` overwrites-in-place on re-submitting the same
  (campaign, source, day), matching `cost_entries`' own two partial
  unique indexes (00009) exactly — two `ON CONFLICT` statements chosen by
  whether a traffic source is set, since Postgres can't target both
  partial indexes from one clause.
- **FX conversion reused, not reimplemented**: `cost.FXConverter` is the
  same shape as `internal/conversion.FXConverter`, satisfied structurally
  by the existing `conversion.PostgresFX` — one `fx_rates` lookup
  implementation for both packages. USD is already 1:1 special-cased
  there, so the common case (manual entries default to USD) always
  converts cleanly with no rate seeding needed.
- **Migration 00017**: `cost_entries.amount_usd` (nullable, §50-FX/
  CLAUDE.md #7 — a missing FX rate is NULL, never a silent 0, same
  pattern as `conversion_events.usd_value`). `created_by_user_id` DROP
  NOT NULL — there's no auth yet (Phase 28), so there's no real user id
  to attribute a dev-created entry to; a fabricated placeholder user
  would be a fake fact recorded as real data, which is worse than an
  honest NULL.
- **`DailyCampaignSpend`** sums `amount_usd` per day and flags days where
  `bool_and(amount_usd IS NOT NULL)` is false — a day with an unconverted
  entry is visibly incomplete, not silently understated by `SUM()`
  skipping the NULL.
- **`GET /campaigns/{id}/cost-entries/daily`** lives on `cost.Handler`
  itself, not under `/analytics`: unlike click/revenue analytics, spend
  never depends on ClickHouse, so its availability shouldn't share
  `apps/api`'s `if ch != nil` mount guard for the rest of `/analytics`.
- **Frontend**: a new "Cost" tab on the campaign detail page (add/edit/
  delete spend by day + optional source — the form doubles as the edit
  flow via the same upsert semantics as the backend). Overview's Spend/
  Profit/ROI/CPA stat cards are real: Spend is a direct sum (`$0.00` when
  genuinely empty, an honest number); Profit/ROI/CPA render `"—"`
  whenever no cost is entered for the range (CLAUDE.md #6 — never a
  false-positive ratio against an implicit zero); CPA additionally
  renders `"—"` whenever conversions are zero regardless of cost, since
  cost-per-acquisition with zero acquisitions is a division by zero, not
  a $0.00 acquisition cost.
- **`docs/cost-ingestion.md`**; `ARCHITECTURE.md` and
  `docs/frontend-integration.md` updated to reflect the closed gap.

### Notes

- **Deliberate architecture call, not a shortcut**: `cost_events`
  (ClickHouse, schema-only since Phase 26, with a comment promising this
  phase would build its sync pipeline) stays schema-only. Daily spend is
  answered directly from Postgres `cost_entries` — at manual-entry
  volume a `GROUP BY entry_date` is simpler and correct, and a
  write-through sync into a table with zero readers would be exactly the
  kind of abstraction CLAUDE.md's "don't add features beyond what the
  task requires" rules out. Revisit once FB/TikTok ad-spend import (§74,
  still unbuilt, explicitly "later" in §27-COST's own text) produces
  ClickHouse-scale volume.
- **A real bug caught during manual verification, not shipped**: the
  first CPA implementation returned `0` (not `null`) when conversions
  were zero, which rendered as `$0.00` — a plausible-looking but false
  acquisition cost. Caught live in the browser (a campaign with $150
  logged spend and zero conversions showed CPA `$0.00` instead of `"—"`),
  fixed before closing the phase, re-verified.
- 4 new integration tests in `apps/internal/cost` (real Postgres,
  `DATABASE_URL`-gated same as `campaign`/`trafficsource`): upsert
  updates in place rather than stacking, a missing FX rate stores `nil`
  not `0`, a mixed converted/unconverted day is correctly flagged
  incomplete, and full cross-tenant isolation (create/list/delete/daily
  spend all refuse another org's campaign).
- Verified end-to-end against the real running `api` + `web` dev
  servers: logged a $150 entry with zero conversions through the Cost
  tab UI, confirmed Spend/Profit/ROI populated correctly and CPA showed
  `"—"`, deleted the entry through the UI, confirmed the empty state and
  all four cards reverted. Backend also verified independently: `go
  build/vet/gofmt/test ./...` all green. Frontend: `tsc --noEmit` and
  `eslint` both clean. Test campaign removed via the real `DELETE`
  endpoint after.

## [Phase 27] — Frontend/Backend Integration

### Scope

§51's literal phase order ("auth, campaigns, sources, offers, networks,
flows, stream sets, filters, tracking, conversions, analytics, ltv/cohorts,
postbacks") assumes backend APIs that were never built — `ROADMAP.md` has
exactly one dedicated backend-API phase before this one (18, "Campaign
API"), and auth doesn't exist until Phase 28. Negotiated down to a single
concrete slice with the user via two `AskUserQuestion` rounds rather than
silently absorbing eleven phases' worth of missing backend work or silently
picking a scope: **Campaigns CRUD + real analytics on the campaign detail
page**. See `docs/frontend-integration.md`.

### Added

- **`apps/web/src/lib/api`** (`client.ts`, `campaigns.ts`,
  `traffic-sources.ts`, `analytics.ts`) — the first real fetch layer
  against the Go API, replacing `apps/web/src/lib/mock/*` for campaigns.
  `apiFetch<T>()` sends `X-Organization-Id` from
  `NEXT_PUBLIC_DEV_ORG_ID`, the frontend's temporary stand-in for auth
  (mirrors `internal/tenant`'s header — both go away in Phase 28), and
  throws a distinct `MissingDevOrgError` when unset so the failure is a
  clear message, not a silent 400.
- **`hooks/use-{campaigns,traffic-sources,campaign-analytics}.ts`** —
  TanStack Query hooks (first real use of the library since it was
  installed); `campaign-list.tsx`, `campaign-form.tsx`,
  `campaign-row-actions.tsx`, `new-campaign-view.tsx`,
  `campaign-detail-view.tsx` rewritten against them, each with real
  loading/error states.
- **`apps/internal/trafficsource`** (new) — `GET /traffic-sources`,
  tenant-scoped, backed by the `traffic_sources` table that already
  existed but had no read endpoint. The one backend addition this slice
  needed; campaign CRUD and analytics endpoints already existed from
  earlier phases.
- **CORS** (`github.com/go-chi/cors`) on `httpserver.New`, origin locked to
  the new `config.AppURL` (`APP_URL` env var, default
  `http://localhost:3000`) — required now that the browser calls the API
  cross-origin.
- **`apps/web/.env.example`** (+ `.env.local`, not committed) —
  `NEXT_PUBLIC_API_URL`, `NEXT_PUBLIC_DEV_ORG_ID`. Next.js loads env files
  from `apps/web/` itself, a separate surface from the root
  `.env.example` (Go services).
- **`CampaignDetailView`'s Overview tab** — real Revenue/Clicks/
  Conversions/CVR stat cards and a real daily revenue chart, computed from
  the Phase 25/26 analytics endpoints. `Spend`/`Profit`/`ROI`/`CPA` removed
  entirely rather than shown as `"—"`: invariant #6's `"—"` means "no cost
  entered yet," not "no cost pipeline exists yet" (that's Phase 27-COST).
- **`docs/frontend-integration.md`**; `ARCHITECTURE.md` §6-SHARED note
  corrected — the Routing Simulator was not switched to a real endpoint
  this phase (no stream-set/flow backend exists yet) and still runs on its
  mock, unchanged.

### Removed

- Mock-only `Campaign` fields with no backend equivalent:
  `trackingDomain`, `trackingId`, `clicks`, `conversions`, `revenue`,
  `spend`. The campaign list's Clicks/Revenue columns and the "Copy
  tracking URL" row action are gone, not faked with placeholder data.

### Fixed

- `apps/web/.gitignore`'s blanket `.env*` pattern was silently swallowing
  `.env.example` too (no `!.env.example` exception, unlike the root
  `.gitignore`) — added.

### Notes

- **Deliberately still mocked, not silently absorbed**: the standalone
  `/analytics` report builder (a full ad-hoc report UI with no matching
  backend contract at any granularity close to what it renders);
  `/ltv-cohorts` (a bare `<PageStub>` — no UI was ever built against the
  Phase 26.5 endpoints); `StreamSetList`/`RoutingSimulatorView` on the
  campaign detail page; every sources/offers/networks/flows/stream-sets/
  filters/routing-simulate/conversions/postbacks list-management page.
  Each needs its own dedicated backend phase before its existing mock can
  be wired up the same way campaigns were here.
- Verified end-to-end against the real running `api` + `web` dev servers,
  not just automated checks: created a campaign through the UI with the
  Source dropdown populated from real seeded `GET /traffic-sources` data,
  confirmed it lands via `POST /campaigns` and navigates to a real detail
  page whose Overview tab renders a correct zero-value empty state (not an
  error) for a campaign with no conversion events, confirmed the Settings
  tab pre-fills from the real record with its Archive control present, and
  confirmed the campaign list resolves and displays the real source name
  after creation. Backend also verified independently: `go build/vet/
  gofmt/test ./...` all green (incl. 2 new `trafficsource` tests), plus
  `curl` confirming traffic-sources list, campaigns list, and a CORS
  preflight all work against real seeded data. Frontend: `tsc --noEmit`
  and `eslint` both clean. Test campaign and its two seed traffic sources
  removed via the real `DELETE` endpoint after verification.

## [Phase 26.5] — LTV & Cohort Engine

### Added

- **`internal/ltv`**: FTD (first-deposit) and Reg (registration) cohort
  tables with §26.5's LTV windows (`d0`/`d1_7`/`d8_30`/`d31_90`). Pure Go
  over a narrow ClickHouse fetch — the same architecture `internal/routing`
  has over its fixture — deliberately, so §26.5's "numbers reconcile
  against fixtures" acceptance criterion is provable directly against this
  package's own tests, not trusted to a `dateDiff` expression buried in a
  materialized view. 11 fixture tests cover exact window bucketing
  (including the day-90/day-91 boundary), the "incomplete windows show
  partial revenue, never a clean zero" rule, conservative completeness
  across a multi-day cohort's members (never complete until the *youngest*
  member's window has elapsed), both §26.5 rate formulas exactly, and the
  Phase 23 FX-missing-contributes-zero invariant carrying through
  unchanged.
- **`ltv_events`** (schema/007): a materialized view over
  `conversion_events`, narrowed to `CPA_HOLD`/`CPA_ACCEPT`/`CPA_REDEP` —
  the only statuses any §26.5 formula references.
- **Anchor uniqueness needs no `MIN()`/`GROUP BY`**: `ClicksByFTDAnchor`/
  `ClicksByRegAnchor` rely on CLAUDE.md #3's dedup key already guaranteeing
  at most one `CPA_ACCEPT` (or `CPA_HOLD`) row per click_id — a direct,
  previously-undocumented-in-this-context consequence of Phase 23's own
  invariant, not a new rule invented here. One row per click IS the first
  occurrence, always.
- **`FTDCohort`/`RegCohort` are distinct types**, not one generic struct
  with optional fields: `ftd_to_redep_rate`/`dep_to_redep` denominate by
  `cpa_accept`, `reg_to_ftd_rate` by `cpa_hold` — attaching both rate
  families to a single shared type risked a silently-wrong denominator on
  whichever one didn't match that cohort's own anchor count. Both types
  still share deposit/window/lifetime accumulation through an embedded
  `base`, computed identically either way.
- **`LifetimeDaysAvg`/`HasLifetimeData`**: averaged only over clicks that
  actually redeposited — §26.5 defines `lifetime_days` as "days from FTD to
  last redeposit," which is undefined, not zero, for a click that never
  did. A cohort where nothing redeposited reports `HasLifetimeData: false`,
  never a misleading `0`.
- **REST**: `GET /analytics/ltv/ftd-cohorts` and `.../reg-cohorts`,
  `?period=day|week|month&from=&to=&campaignId=&networkId=&country=`,
  mounted on `apps/api` behind the existing `tenant.Middleware`. `from`/`to`
  default to the last 90 days when omitted — §26.5 itself names 90 days as
  the range a report needs for fully closed windows.
- **`docs/ltv-cohorts.md`**, `ARCHITECTURE.md` updated (§76).

### Notes

- **Deliberately out of scope, no new confirmation needed**: §26.5 names
  `source`/`offer` as filter/breakdown dimensions alongside
  campaign/network/country, but `event.Event` carries neither
  `traffic_source_id` nor `offer_id` — the same pre-existing gap
  `internal/macro`'s `{source}` token (Phase 24) and `click_events`' sort
  key (Phase 26) already documented, reached independently a fourth time.
  No frontend work either — no LTV/Cohorts UI mock exists to integrate
  with, consistent with every Go-backend phase since 23.
- End-to-end smoke test against the real compiled `api` binary and real
  seeded `conversion_events` (via `EventStore.InsertBatch`, the same path
  production traffic uses): an old two-click FTD cohort's windows summed to
  exactly the seeded revenue with every window `Complete: true`; a
  five-day-old cohort showed `d0` complete-and-populated with the later
  three windows correctly `Complete: false` and `0` revenue; the matching
  Reg cohort's `regToFtdRate` came out exactly `2/3`; a second organization
  querying the identical range got `{"cohorts":[]}` — tenant isolation held
  through the full stack, not just the repository layer.
- 20 new tests: `internal/ltv` 16 (11 pure-computation fixtures + 5
  service-layer validation/pass-through), `internal/chstore` +4 (the
  `ClicksByFTDAnchor`/`ClicksByRegAnchor` query layer, incl. dimension
  filtering and a no-match case). A from-scratch ClickHouse schema
  application (dropping `ltv_events`/`ltv_events_mv` and re-running
  `Migrate`) verified alongside the existing idempotency check.

## [Phase 26] — ClickHouse

### Added

- **The real §48 five-table schema** — `click_events`, `tracking_events`,
  `conversion_events`, `cost_events`, `postback_events` — replaces Phase
  25's deliberately disposable single `events` table.
  `schema/000_drop_phase25_schema.sql` drops it and its aggregate first;
  every table gets its own sort key reasoning documented in its own
  `schema/*.sql` file rather than one generic comment repeated five times.
  No TTL on any table — confirmed via user question before starting: no
  data retention policy exists anywhere in this project's docs
  (PRODUCT.md, ARCHITECTURE.md), and defaulting one in silently would be a
  real, possibly GDPR-relevant product decision made by omission.
- **`event.Type.IsClick()`** (new, alongside the existing `IsCPA()`):
  decides `click_events` vs `conversion_events` vs `tracking_events`
  routing. `TestEventClassificationIsExhaustiveAndDisjoint` guards that
  every type in the model lands in exactly one bucket — a future event
  type added to `event.go` without updating this classification fails a
  test instead of silently landing nowhere or in two tables at once.
  `chstore.EventStore.InsertBatch` now buckets one mixed batch into up to
  three ClickHouse batch inserts, never one insert per row.
- **`cost_events`**: schema only, confirmed out of scope for this phase.
  Mirrors Postgres `cost_entries` for cross-database-free JOINs against
  click/conversion volume, but the sync pipeline that populates it is
  Phase 27-COST's job ("Cost ingestion," ROADMAP.md) — the same pattern
  `cost_entries` itself followed, starting manual-entry-only in Phase 17
  before any FB/TikTok import existed. `ReplacingMergeTree(updated_at)`,
  not a plain `MergeTree` — the only append-only-by-default table here that
  anticipates needing to overwrite an edited entry once that sync exists.
- **`postback_events`: real ingestion, both directions** — confirmed via
  user question before starting, closing a gap migration 00008 (Phase 17)
  explicitly earmarked for once ClickHouse existed. Every exit point of
  `internal/conversion.Service.Record` (success, duplicate, ignored, error)
  and every dispatch outcome in `internal/postback.Deliverer` (success,
  retrying, dead) now reports through a new `AttemptLogger` interface —
  same decoupled, no-error-return contract as `EventSink`/
  `DeliveryEnqueuer` — to **`internal/postbacklog`**, a near-duplicate of
  `internal/eventqueue` (own Postgres `FOR UPDATE SKIP LOCKED` queue,
  `postback_attempt_queue`/migration 00016, own `Flusher`) rather than a
  shared/generic implementation: the two payload shapes
  (`chstore.PostbackAttempt` vs `event.Event`) have nothing else in common,
  and duplicating ~150 lines of already-proven code was judged lower risk
  than a generics refactor touching Phase 25's shipped `eventqueue`
  mid-project. `postback_events` is explicitly **not** the dedup/delivery
  source of truth — Postgres's `postbacks`/`postback_deliveries` still are
  — it's the read-side replay/audit log §45 requires, fed asynchronously,
  never on either direction's critical path.
- **Three materialized views**: `click_events_daily_campaign` and
  `click_events_daily_geo` (§48's two named patterns), plus
  `conversion_events_daily_campaign` (counts **and USD revenue** per
  campaign/day/status) — not named by §48 directly, but the same pattern
  applied to money instead of volume, added because CLAUDE.md #6 ("cost or
  it doesn't exist") and §27-COST's eventual ROI queries will need a
  revenue aggregate and the marginal cost of a third view was near zero.
  All three `SummingMergeTree`, verified firing *synchronously* on
  `INSERT` (no polling needed in tests). Revenue sums
  `usd_value * has_usd_value` — a conversion with no FX rate on file
  contributes exactly zero, not an approximation.
- **`internal/analytics` gains `GET /analytics/campaigns/{id}/daily-revenue`**,
  reading the new revenue aggregate — the query method existed since this
  phase's schema work but the REST endpoint was initially missed and added
  once the smoke test went looking for it.
- **Attribution's `MemoryResolver` replaced by `chstore.ClickResolver`** —
  not originally scoped for this phase's user-confirmed plan, but closes a
  promise `docs/attribution.md` made across Phases 22/23/25 ("arriving with
  the worker (Phase 24) and the analytical schema (Phase 26)") using
  infrastructure this phase already built. Found while verifying the
  schema end-to-end: `apps/tracker` never actually called
  `MemoryResolver.Record` anywhere — every conversion has been permanently
  `unknown_click` since Phase 23 shipped, silently. `ByClickID` resolves to
  the **earliest** occurrence of a click_id (handles `stickyFlowKeepClickId`
  reusing one click_id across a returning visitor's journey, §39-STICKY)
  and excludes `SOURCE_FILTER` rows (a filtered click never reached a
  destination, so a network could never legitimately reference one) — a
  lookup-eligibility decision, not new attribution policy; the matching
  logic itself is entirely `internal/attribution`'s own, unchanged.
  Best-effort at tracker startup (falls back to `MemoryResolver` — same
  degraded-but-honest stance as Redis elsewhere) since the redirect path
  must never depend on ClickHouse being up.
- **`internal/chstore/schema/*.sql`**, **`docs/analytics-pipeline.md`**
  (rewritten for the real schema), **`docs/attribution.md`** (the "Later"
  promise marked landed), **`ARCHITECTURE.md`** updated (§76).

### Fixed

- Nothing broken, but one real functional gap closed in passing: see
  "Attribution's `MemoryResolver` replaced" above — every conversion was
  silently unattributed in any environment that actually ran the compiled
  tracker, not just in tests.

### Security

- Tenant isolation re-verified for the new `ClickResolver`
  (`TestClickResolverTenantIsolation`): org B resolving org A's click_id
  gets `ErrClickNotFound`, the same outcome as a genuinely nonexistent
  click — indistinguishable from outside, so the failure mode can't be used
  to confirm another tenant's click_id exists (mirrors
  `internal/attribution`'s own stated reasoning for `OutcomeUnknownClick`).

### Notes

- End-to-end smoke test against compiled `tracker`/`worker`/`api` binaries
  and real Postgres/ClickHouse: a real click through `/t/{trackingID}` →
  flushed to `click_events` → a real incoming postback for that exact
  click_id → `attribution_outcome = "attributed"`, `method = "click_id"` in
  the resulting `conversion_events` row — the first smoke test all session
  to NOT read "matched no click of this organization." Also verified: the
  postback's own audit trail in `postback_events` (both `incoming` and the
  `outgoing` delivery it triggered, including a real HTTP hit against a
  local receiver), and a from-scratch ClickHouse schema application
  (`DROP`s issued manually, `Migrate` re-run, all 5 tables + 3 views
  recreated cleanly).
- 69 tests total across the touched/new packages (up from 39 last phase):
  `chconn` 1, `chstore` 12 (incl. 7 new `ClickResolver` tests), `eventqueue`
  5 (unchanged), `postbacklog` 5 (new), `conversion` 24 (+1: every `Record`
  outcome logs an attempt), `postback` 12 (+1: every dispatch outcome logs
  an attempt), `event` 4 (+1: classification exhaustiveness), `analytics` 6
  (+2: the revenue endpoint).
- Full Postgres migration round-trip (`up`/`down`/`up`) verified through
  00016; a from-scratch ClickHouse schema application verified separately
  from the idempotency check `chstore_test.go` already had.

## [Phase 25] — Analytics Pipeline

### Added

- **Scope boundary with Phase 26 confirmed via user question before
  starting**: §47's own pipeline diagram names ClickHouse as one of its
  stages, but the actual table design (five tables, dimension-specific sort
  keys, per-campaign+day/per-GEO+day materialized views) is explicitly §48's
  content, in a phase titled "ClickHouse" on its own. Building both at once
  would blur a boundary the spec draws deliberately. Chosen: a minimal,
  disposable single-table schema now, proving the pipeline end to end;
  Phase 26 replaces it wholesale. Recorded in full in
  `docs/analytics-pipeline.md` so the coming rework reads as planned, not
  as backtracking.
- **`internal/eventqueue`**: the durable link in §43's "Tracker -> Event
  Queue -> Worker -> ClickHouse" pipeline. `event_queue` (migration 00015)
  is Postgres, the same `FOR UPDATE SKIP LOCKED` job-queue pattern
  `postback_deliveries` (Phase 24) already established — STACK has no
  message broker. One difference: a row here is **deleted**, not kept
  terminal, once its batch lands in ClickHouse — this queue is disposable
  transit, not an audit ledger. `payload` is the whole `event.Event`
  JSON-encoded (it already carries `json` tags for exactly this) rather than
  a wide explicit-column table that would just be Phase 26's real schema
  designed a second time.
- **`apps/tracker` drops `eventbuf.LogSink`** for `eventqueue.Sink` — the
  one-line swap Phase 21's design always promised, since everything
  upstream only ever saw the `eventbuf.Sink` interface.
- **`apps/worker` gains a second poll loop**, `internal/eventqueue.Flusher`:
  claims up to 500 due events, ONE ClickHouse batch insert per batch,
  deletes on success or requeues the whole batch on failure (fixed 10s
  delay). No dead-letter state, unlike outgoing postbacks — a delayed
  analytics batch has no per-item deadline the way a lost conversion does,
  so it retries indefinitely rather than giving up. A deliberately different
  tradeoff from `internal/postback`'s `MaxAttempts`, not an inconsistency.
- **`internal/chconn`**: the ClickHouse connection, always over HTTP —
  `infra/docker-compose.dev.yml` exposes only ClickHouse's HTTP interface
  (port 8123) to the host, unlike some reference implementations that
  default to the native protocol (port 9000, container-internal here to
  avoid colliding with MinIO).
- **`internal/chstore`**: the minimal schema (`schema/*.sql`, embedded,
  applied idempotently via `CREATE ... IF NOT EXISTS` — no migration
  framework, no version table, a deliberate simplification matched to one
  disposable table) plus `EventStore.InsertBatch` and the one aggregate
  query. `events`' `type` column covers the full ~20-value event model even
  in this disposable table (CLAUDE.md #2 isn't phase-scoped), and
  `organization_id` leads the sort key (CLAUDE.md #5), same as it will in
  Phase 26's real schema.
- **`events_daily_campaign`**: one `SummingMergeTree` aggregate fed by a
  `MATERIALIZED VIEW`, proving the "materialized/aggregate tables" pipeline
  stage with a single rollup rather than §48's full per-campaign+day/
  per-GEO+day set. Verified firing *synchronously* on `INSERT INTO events`
  — no polling needed in the test that checks it — though reading it still
  requires `SUM(event_count)`, not a plain read: `SummingMergeTree` merges
  same-key rows only in the background.
- **`internal/analytics`**: one query, one endpoint —
  `GET /analytics/campaigns/{campaignId}/daily?from=&to=` on `apps/api`,
  mounted behind the existing `tenant.Middleware`. Deliberately narrow:
  proves the pipeline's last two stages (analytics service -> REST API)
  without delivering the eventual rich analytics surface (per-GEO/source/
  offer breakdowns, the metrics registry) that later phases own. Date range
  capped at 366 days as a guard against an unbounded ClickHouse scan, not a
  spec requirement.
- **`httpserver.readyHandler` now checks ClickHouse too** (`ch Pinger`,
  nil-able) — the doc comment left in Phase 18 said this would happen
  "whenever a later phase actually wires it in, not before." `apps/api`'s
  ClickHouse connection is best-effort at startup: `/campaigns` doesn't need
  it, only `/analytics` does, so a down ClickHouse degrades one route group
  (and shows up on `/ready`) rather than the whole control-plane API.
- **`apps/worker/README.md`**, **`apps/tracker/README.md`**,
  **`docs/analytics-pipeline.md`** updated/added (§76).

### Fixed

- **`ARCHITECTURE.md`'s ClickHouse bullet** read as if the five-table design
  already existed; corrected to note the Phase 25 minimal table and point at
  `docs/analytics-pipeline.md`.

### Security

- Tenant isolation verified end-to-end through the real `/analytics` REST
  endpoint (not just at the repository layer): org B querying org A's
  campaign id via `X-Organization-Id` gets `{"counts":[]}`, not an error and
  not another tenant's data — `organization_id` scopes the ClickHouse query
  the same way it scopes every Postgres one.

### Notes

- 13 new tests across four new packages: `chconn` (1), `chstore` (3),
  `eventqueue` (5, including pure-logic `Flusher` tests against fakes —
  success deletes, failure requeues the whole batch, an empty claim never
  calls ClickHouse), `analytics` (4). Every Postgres/ClickHouse-gated test
  ran against real local instances, not just the fakes.
- Full end-to-end smoke test against compiled `tracker`/`worker`/`api`
  binaries and real Postgres/ClickHouse: events enqueued, drained by the
  worker within its poll interval, visible in ClickHouse's `events` table
  and the `events_daily_campaign` aggregate, and returned correctly by the
  real (not mocked) `/analytics` HTTP response — plus the cross-tenant check
  above.
- `apps/web` untouched. Wiring the frontend to real backend APIs is Phase
  27's explicit job ("Frontend/backend integration," ROADMAP.md), consistent
  with this project's frontend-first-on-mocks build order — `apps/web`
  keeps reading `apps/web/src/lib/mock/*` until then.

## [Phase 24] — Postback Engine (outgoing)

### Added

- **`apps/worker`** is a real binary for the first time — a health endpoint
  plus `internal/postback.Deliverer.PollLoop`. Its *other* eventual role,
  consuming the tracker's event queue into ClickHouse, is explicitly not
  this phase's job (§47/§48, Phases 25-26); `apps/worker/README.md` now says
  so instead of describing the full future scope as if it already existed.
- **`internal/postback`** (§46): the outgoing delivery engine.
  `postback_deliveries` (migration 00014) is the durable queue — §46's exact
  status vocabulary (`queued`/`processing`/`success`/`failed`/`retrying`/
  `dead`), claimed via the standard Postgres `FOR UPDATE SKIP LOCKED` job-
  queue pattern so concurrent worker replicas can never double-claim a row.
  Deliberately a **separate table** from `postbacks`, not more
  `direction='outgoing'` rows there as an earlier comment on that table once
  planned — `result`'s one-shot dedup vocabulary and a multi-attempt
  delivery's own state don't fit one column without forcing the dedup index
  to grow a predicate it has no other need for. `source_postback_id` keeps
  the two connected. Full reasoning: `docs/postback-delivery.md`.
- **Exponential backoff, 8 attempts, ~21 hour span, dead-letter after** —
  none of these numbers are in §46; chosen so a network's multi-hour outage
  still gets delivered without retrying forever. A dead-lettered delivery
  isn't lost — Phase 13's "Resend postback" UI action already exists as the
  recovery path.
- **`internal/conversion.Service` gets a `DeliveryEnqueuer` hook**, the same
  decoupled-narrow-interface pattern `EventSink` already established: every
  `ResultSuccess` with a configured `networks.postback_url` queues a
  delivery. `Enqueue` has no error return by contract — a conversion
  `internal/conversion` already durably recorded must never be reported back
  to the network as failed just because queuing its outgoing notification
  hit a database blip.
- **Trigger scope decided via user confirmation**: all five CPA statuses
  (not just HOLD/ACCEPT/REDEP) queue a delivery. §46 doesn't specify this;
  the Phase 13 frontend mock hints at "payable only" but that reads as
  mock-data flavor, and the URL template's `{status}` token exists
  specifically so a network can branch on it — restricting by status would
  have been inventing policy the spec never states.
- **`apps/internal/macro`**: the Go half of §27's shared macro resolver,
  porting `apps/web/src/lib/macros.ts`'s exact token contract rather than
  inventing a second one. Resolves `{click_id}`/`{status}`/`{revenue}`/
  `{currency}`/`{campaign_id}`/`{country}`/`{device}`/`{sub1..10}` — every
  field `internal/conversion.Service.Record` actually has in hand.
  `{payout}`/`{offer_id}`/`{source}` are part of the shared vocabulary but
  pass through literally: no Flow→Offer or click→TrafficSource lookup exists
  anywhere in the Go codebase yet, and wiring one in was out of scope for a
  queue/worker/retry phase — documented in `docs/postback-delivery.md`
  rather than silently narrowing the macro contract.
- **`apps/worker/README.md`**, **`docs/postback-delivery.md`** (§76).

### Fixed

- **Three docs still stated the pre-A1/A2 two-part dedup key**
  (`docs/event-model.md`, `docs/domain-model.md`,
  `apps/api/migrations/README.md`'s table list was also missing
  `event_mappings`/`postback_deliveries`) — found while touching this area
  for Phase 24. `ARCHITECTURE.md` and `CLAUDE.md` were corrected in Phase 23;
  these three were missed then. Corrected now, including
  `event-model.md`'s literal repetition of the exact "else a monotonic
  sequence" wording the A1 amendment identified as actively dangerous
  (disables deduplication entirely when a network sends no txn id).

### Notes

- End-to-end smoke test against the compiled `tracker` and `worker`
  binaries, a real Postgres/Redis, and a real local HTTP receiver: verified
  both the immediate-success path and the fail→`retrying`→backoff→succeed
  path with real wall-clock timing (a 500 response, `next_attempt_at` ~30s
  out, not due until then, due and delivered on the next poll).
- 39 new/updated tests: `internal/macro` (5), `internal/postback` (11, incl.
  Postgres-gated `ClaimDue`/backoff-timing/tenant-data-shape checks), plus 3
  new delivery-enqueue tests added to `internal/conversion`'s existing suite
  (now 23 total there).
- Migration 00014 round-tripped (`goose up` → `down` → `up`) against a real
  Postgres alongside 00001-00013.

## [Phase 23] — Conversion Engine

### Added

- **`internal/conversion`** (§45): turns an inbound postback into a recorded,
  deduplicated, correctly-attributed CPA event, or an honestly-logged reason
  it wasn't. Pure orchestration — no HTTP, no database driver — over `Store`,
  `Mapper`, `NetworkLookup`, `FXConverter`, and `internal/attribution`'s
  `AttributionService`, the same shape `internal/attribution` itself has over
  `ClickResolver`.
- **Dedup key is `(click_id, status, event_ref)`** (A1, §45), computed by
  `eventRefFor`: the network's transaction id for `CPA_REDEP` only, empty
  string for every other status even when one was sent. A network sending no
  txn id on a redeposit records exactly one — a missed redeposit (support
  ticket) is preferred over a double-counted one (bad invoice).
- **Status progression** (A2, §45): the only refused transition is back to
  `CPA_HOLD`. Independent of dedup — it exists specifically because
  `acceptDuplicates` networks bypass the dedup constraint but must not bypass
  this rule (nightly partner replays re-sending an already-approved `HOLD`
  would otherwise pull revenue out of a closed report).
- **`GET/POST /postback/{networkId}`** on `apps/tracker` (§45's endpoint,
  ARCHITECTURE.md explains why tracker and not worker). `{networkId}` — not a
  header, not the body — is where `OrganizationID` comes from
  (`PostgresNetworkLookup`), matching how attribution derives tenant scope.
- **Event Mapping is now real**: `event_mappings` table (migration 00012) plus
  `PostgresMapper`, replacing the Phase 13 frontend mock's documented "what
  the real Conversion Engine runs at ingest time." Matched case-insensitively
  — networks are inconsistent about casing across retries.
- **FX normalization at event time** (§50-FX, CLAUDE.md #7): `PostgresFX`
  reads `fx_rates` by `(currency, event date)`, never the current rate. No
  rate on file is `ok=false`, not an error — the conversion is stored with its
  original currency/amount and a nil USD value rather than inventing a rate or
  dropping the conversion.
- **Redis wired for real** (`github.com/redis/go-redis/v9`,
  `internal/rediscache`): `RedisStore` caches only the progression check
  (`LastStatus`), never the dedup insert itself. The cache is written only
  *after* Postgres confirms a success, so staleness can only cause an extra
  Postgres read, never a false "already seen" — see docs/conversion.md for why
  a Redis-first dedup pre-check was deliberately rejected. Best-effort at
  tracker startup: `PostgresStore` alone is already correct, so a
  down/unconfigured Redis logs a warning and falls back rather than failing
  the process.
- **`event.Event` grows its §45 fields** (`NetworkID`, `Revenue`, `Currency`,
  `USDValue`/`HasUSDValue`, `EventRef`, `NetworkTxnID`,
  `AttributionOutcome`/`AttributionMethod`, `TimeToConversionMS`) — extended,
  not redefined, per the package doc's own stated pattern.
- **`docs/conversion.md`** (§76); resolves docs/attribution.md's open
  question (no attribution window, decided this phase — see Notes).

### Fixed

- **`postbacks` migration 00008 predated the A1/A2 amendment**: its unique
  index was `(organization_id, click_id, status)`, which would have dropped
  every redeposit after the first. Migration 00013 replaces it with
  `(organization_id, click_id, status, event_ref) WHERE NOT
  network_accepts_duplicates AND result = 'success'` — the added `result =
  'success'` clause is what lets a duplicate/ignored/error attempt log its own
  row without colliding with an already-accepted one, making "log every
  postback... with replay ability" and "have we already processed this"
  answerable from one table.
- **`postbacks_status_check` rejected `status = ''`**: found via a full
  end-to-end smoke test against a running tracker binary — a postback missing
  `click_id`/`status` entirely, or with no Event Mapping configured for its
  raw status, has no canonical `CpaStatus` to record, and the original CHECK
  (inherited unmodified from 00008) required one on every row including
  `result = 'error'` ones. Migration 00013 exempts `status = ''` rows scoped
  to `result = 'error'` only — every other result still requires a real
  `CpaStatus`.
- **`ARCHITECTURE.md`'s non-negotiable #3** still stated the pre-amendment
  `(click_id, status)` key; corrected alongside the code per CLAUDE.md's own
  rule that a doc and its code must not read differently.

### Security

- `OrganizationID` never comes from the postback body — see "Added" above.
  `PostgresStore`'s dedup/progression queries filter on it directly (mirrors
  `internal/attribution`'s repository-layer enforcement), and
  `TestPostgresStoreTenantIsolation` confirms org B's `LastStatus` lookup
  cannot see org A's click, matching CLAUDE.md #5's DoD requirement for
  data-model phases.

### Notes

- **No attribution window**, decided in this phase rather than left as Phase
  22's open question: §45's own "never lose the conversion" stance (its
  Redis-unavailable fallback would rather accept a wrong report than drop
  revenue) argues against a policy that silently discards a late conversion.
  `Attribution.TimeToConversion` remains the observable an operator can alert
  on instead.
- **Known spec gap, not a bug**: a non-REDEP status recurring for the same
  click (e.g. `CPA_ACCEPT` reinstated after a `CPA_DECLINE` reversal) computes
  the same dedup key as its first occurrence and is recorded as a duplicate —
  §45 designates only `CPA_REDEP` as repeatable and explicitly forbids
  inventing a synthetic `event_ref` to work around this. Documented in
  docs/conversion.md and exercised (not "fixed") by
  `TestProgressionAllowsEverythingElse`. Left for a future spec amendment if a
  real network's reinstatement flow needs it.
- 23 tests: 13 pure-logic (fakes for `Store`/`Mapper`/`FXConverter`/
  `AttributionService`, mirroring `internal/attribution`'s test style) plus 10
  gated on `DATABASE_URL`/`REDIS_URL` proving the actual schema/cache
  implement the contract the fakes assert. Full migration round-trip (`goose
  up` → `down` → `up`) verified against a real Postgres, plus an end-to-end
  smoke test of the compiled tracker binary exercising every `ResultKind`
  through the real HTTP endpoint (this is what caught both Fixed items above).
- Click storage behind attribution is still `MemoryResolver` — unchanged by
  this phase, still arriving with the worker (Phase 24) and ClickHouse (Phase
  26).

## [Phase 22] — Attribution

### Added

- **`internal/attribution`** (§44): decides which click a conversion belongs
  to, implementing §44's `AttributionService` interface exactly. Pure — no
  HTTP, no database driver, no clock of its own — reading clicks through a
  `ClickResolver`, the same shape `internal/routing` has with `routingstore`.
- **Evidence is tried strongest-first**: `click_id` (which FLOX minted and
  handed to the network) before `external_click_id` (which the network
  supplied and which is not reliably unique). Two refusals carry the weight of
  §44's "do not invent attribution when there is insufficient evidence":
  - a `external_click_id` matching **several** clicks is `ambiguous`, not a
    tiebreak. The same `fbclid` recurs across a redirect chain, a prefetch and
    a genuine second visit; preferring "the most recent" would look sensible
    and credit the wrong click a fraction of the time, invisibly.
  - a present-but-unmatched `click_id` does **not** fall back to the external
    id. If the network echoed back an identifier we minted and it resolves to
    nothing, that claim is suspect, and re-matching it on a weaker field would
    hide exactly the case worth investigating.
- **Unattributed is a first-class answer, not an error.** Four closed outcomes
  (`attributed`, `no_identifier`, `unknown_click`,
  `ambiguous_external_click_id`), each carrying a human-readable reason for the
  postback log — a disputed payout gets re-argued from that record (§72's
  spirit). `no_identifier` is deliberately distinct from `unknown_click`: the
  first is a misconfigured postback template, the second is a real lookup miss,
  and the fixes differ.
- **`TimeToConversion`**, including negative values. A conversion timestamped
  before its click means clock skew or a replayed postback; clamping it to zero
  would erase the one signal that says so. It is a diagnostic, never grounds
  for refusing a click that matched.
- **`MemoryResolver`** — the honest stand-in, per-process and gone on restart,
  labelled as such. The tracker still writes to `eventbuf.LogSink`, which is
  explicitly not durable storage, so there is nothing to query yet; the real
  ClickHouse-backed resolver arrives with the worker (Phase 24) and the
  analytical schema (Phase 26). Adding a clicks table to Postgres instead would
  have contradicted §7 and §47.
- **`docs/attribution.md`** (§76).

### Security

- **Tenant isolation is enforced in the repository layer** (CLAUDE.md #5):
  every `ClickResolver` method takes `organizationID` and filters on it, rather
  than the service comparing ids after the fact — a filter that is part of the
  query cannot be forgotten at one call site. `Conversion.OrganizationID` comes
  from the authenticated credential, never the request body, and a missing one
  returns `ErrNoOrganization` rather than searching globally.
- A click belonging to a **different** organization is reported as
  `unknown_click`, indistinguishable from a nonexistent one, so the outcome
  cannot be used to confirm another tenant's click id exists.
- Four isolation tests, including one asserting that the same
  `external_click_id` present in two organizations resolves cleanly for each
  rather than turning ambiguous — the failure mode a dropped org filter would
  produce, and one that would otherwise read as a data-quality problem rather
  than a breach.

### Notes

- A resolver failure surfaces as an error, never as `unattributed`: recording a
  database blip as "no click found" would permanently write off real revenue.
- **No attribution window** is implemented. §44 specifies none, and a policy
  that silently discards late revenue is not this phase's decision to make
  alone; §45 already notes partners re-send deposits with hours-to-days delay.
  Recorded as an open question for Phase 23, where the postback timing rules
  live.
- Nothing is wired into a binary yet — Phase 23's postback handler is the
  consumer, exactly as `internal/routing` (Phase 19) waited for the tracker
  (Phase 21).

## [Between phases] — spec amendments (§38, §45, §58, §59)

Three amendments drafted in `docs/spec-amendments-phase22.md` after reviewing
a third-party tracker, then applied. All three are now spec **and** code: A3
landed with routing (Phase 22); A1 and A2 landed with the conversion engine
([Phase 23], above).

### Changed — weighted flow selection is deterministic (A3, §38)

- **`internal/routing.pickWeighted` now hashes a visit key instead of rolling
  a die.** §38 previously said routing must be deterministic "where
  configuration requires deterministic behavior", which reads as "when sticky
  is on" and permitted the injected RNG the engine actually used. The
  consequences were real: a replayed request landed in a different flow than
  the original before a sticky cookie existed, two replicas behind one load
  balancer disagreed about the same visit, and a restart re-bucketed everyone.
  The draw is now an unseeded FNV-1a/64 of `RequestContext.VisitKey` —
  uniform, so shares still converge to the configured weights, and pure, so a
  replay resolves identically. Explicitly **not** `hash/maphash`, whose
  per-process random seed would reintroduce exactly the cross-replica
  disagreement being fixed. Sticky is untouched: the cookie is still the
  source of truth and short-circuits before the draw.
- **`routing.Engine` lost its `Rand01` field and now has no fields at all** —
  no state, no entropy. A fresh `Engine{}` per call is indistinguishable from
  another replica or a restart, which is what makes the new determinism test
  expressible.
- **`RequestContext.VisitKey` (new, required for a real split).** The caller
  derives it, because "the same visit" is an HTTP-layer question the routing
  package is deliberately blind to: `apps/tracker` fingerprints
  `campaignID|clientIP|userAgent`, and the simulator mock derives it from its
  form. The campaign id is in the tracker's key so one visitor is bucketed
  independently per campaign rather than landing in the same relative
  position in every split on the platform.
- **A missing key is refused, not guessed** (`ErrNoVisitKey`). Hashing the
  empty string would route 100% of traffic to one arm of a split while every
  dashboard still reported the configured percentages — silent, and only
  discoverable after the experiment is over. A single eligible flow is not a
  draw and still needs no key, so the most common configuration is unaffected.
- **Eligibility is now decided before the draw, and zero/negative weights are
  skipped rather than clamped.** The previous implementation could select a
  zero-weight flow through its "float rounding fallback" branch, which meant a
  paused arm could still take traffic.
- **`lib/routing-simulate.ts` mirrors all of the above**, including FNV-1a in
  `BigInt` (hashing UTF-8 bytes, matching Go's byte-indexed strings — UTF-16
  code units would have diverged the two on exactly the non-ASCII user agents
  that are hardest to debug). Verified: Go and TS agree on `""`, `"a"`,
  `"hello"`, an ASCII key and a non-ASCII key, with the values checked against
  an independent third implementation rather than captured from either side.

### Changed — postback correctness (A1/A2, §45, §59 — spec only)

- **Dedup key is `(click_id, status, event_ref)`.** The old normative line said
  `(click_id, status)` while the rationale beneath it already described a txn
  id; the two-part form is what would have been coded, dropping every redeposit
  after the first. `event_ref` is scoped to `CPA_REDEP` and empty for every
  other status even when a network sends a txn id, since networks retry with a
  fresh one per attempt. Also drops "else a monotonic sequence", which would
  have disabled deduplication outright.
- **Status never moves back to `CPA_HOLD`.** No ordering rule existed anywhere
  in the spec, so a nightly partner replay re-sending the original hold after
  approval would have been recorded — it is not a duplicate under any dedup key
  — and would have taken revenue out of an already-published report. Only that
  one transition is refused; chargebacks and their reversals stay allowed.
- **§59** gained the cases that make both testable, and the `NEVER` lists in
  §80 and `CLAUDE.md` were widened: they banned only dedup on `click_id` alone,
  which after A1 left the newly-wrong two-part key permitted.

### Fixed

- `docs/routing.md` pointed at `apps/api/internal/routing`, a path that stopped
  existing when the module root moved in Phase 21.

## [Phase 21] — Tracking Engine

### Changed — module topology (the Phase 16 open question, resolved)

- **The Go module root moved from `apps/api` to `apps/`** (module
  `github.com/ismagilovnail/flox/apps`). Phase 16 flagged this as a
  decision for whoever started Phase 21: Go's internal-import rule means
  `.../internal/x` is only importable by code rooted at that directory's
  parent, so `apps/tracker` could never have imported
  `apps/api/internal/routing`. Both `ARCHITECTURE.md` ("separate binaries
  inside the same Go module") and §41 ("shares internal packages") state
  the answer outright, so there was nothing left to weigh — taken rather
  than re-litigated. `git mv` throughout, so history is preserved.
  - `apps/api/cmd/api/main.go` → `apps/api/main.go`; run with `go run ./api`.
  - `apps/api/internal/…` → `apps/internal/…`.
  - Migrations stay with the control plane: `go tool goose -dir api/migrations`.
  - The directory layout CLAUDE.md specifies is unchanged; only `go.mod` moved.
- **`apps/web/go.mod`** (new, 4 lines): the module root now sits above
  `apps/web`, so `go build ./...` was compiling and vetting a stray `.go`
  file shipped inside `web/node_modules` by an npm package — letting a JS
  dependency break the Go build. A nested module stub makes the toolchain
  skip that subtree. Found empirically right after the move, not
  theorised.

### Added

- **`apps/tracker`** (§41): the hot-path click/redirect binary.
  `GET /t/{trackingID}` runs parse → classify → route → record async →
  302 redirect, with no analytics query and no wait on persistence
  anywhere on the path. Measured on the dev stack over 200 requests:
  **p50 1.1ms, p95 1.4ms** against §56's p50 < 20ms / p95 < 50ms budget.
- **`internal/event`** (§43): the full ~20-type event model, present from
  day one and never truncated (CLAUDE.md non-negotiable #2), with the five
  CPA statuses as distinct enum members. A test asserts the model against
  §43's list so a later edit can't quietly drop a type, and asserts a
  generic `CONVERSION` type does *not* exist.
- **`internal/eventbuf`** (§41): buffered batch writer whose one hard
  guarantee is that `Enqueue` never blocks — a single non-blocking channel
  send, so a stalled sink drops events (counted, and logged, never
  silently) rather than making a user wait on a redirect. Proven by a test
  that wires a permanently-stalled sink to a 4-deep buffer and asserts
  1000 `Enqueue` calls still return promptly. Batches flush on size or
  interval, and `Close` drains rather than discarding accepted events.
  §43's durable queue + ClickHouse consumer arrive with `apps/worker`
  (Phase 24); until then the sink is an honest structured-log writer
  behind the same `Sink` interface.
- **`internal/routingstore`**: loads a campaign's routing configuration
  out of Postgres into `internal/routing`'s pure types — a separate
  package specifically so the routing engine keeps its no-database
  property. Rebuilds the recursive AND/OR filter tree from the flat
  `filter_groups`/`filter_conditions` rows in two queries rather than one
  per node, and resolves whether an offer destination's offer is still
  active (the check §58's "inactive offers" case needs).
- **`ConfigVersion`** is derived from the newest `updated_at` across the
  campaign and its routing objects. §39 asks for versioned configuration
  and no version column exists; this is a real monotonic version that
  changes exactly when the configuration changes, which is what
  cache-invalidation and "which config produced this decision" actually
  need.
- **Migration `00011`**: §39-STICKY's three flags (`sticky_flow`,
  `sticky_flow_keep_click_id`, `sticky_flow_skip_inactive`) on
  `campaigns`. Not in §35's table list, so Phase 17 correctly didn't
  invent them; the tracker is the first code that reads them, so they land
  now — schema following the code that needs it.
- **`config.LoadTracker`**: same shape and env vars as the API's loader,
  differing only in which URL variable supplies the port and the default
  OTel service name, so the two share one loader instead of growing
  near-identical copies that drift.

### Notable implementation decisions

- **Tracking links resolve by host + slug, never slug alone.**
  `tracking_links` is unique on `(domain_id, slug)`, so two organizations
  may each own the slug `summer` on their own domain — resolving by slug
  alone would be a cross-tenant data leak.
- **Two of Phase 19's three "caller-level" §58 cases are now implemented
  here**, where the data actually lives: an unresolvable tracking link and
  a non-`active` campaign both 404 without ever reaching the routing
  engine. The third (in-app WebView bounce) remains outstanding and
  belongs with the PWA install funnel.
- **`stickyFlowKeepClickId` still doesn't reach `internal/routing`.**
  Phase 19 documented that it affects attribution only, never flow
  selection. The tracker needs it, so `routingstore.CampaignRouting`
  returns it *alongside* the routing config rather than smuggling it in —
  and the tracker parses the full `setId:flowId[:clickId]` cookie itself,
  handing the engine only the two fields a routing decision depends on.
- **§42's unfilled-FB-subs rule is implemented by having no special case
  at all**: whatever subs arrive are persisted, whatever don't stay empty
  strings — no placeholder, no "unknown campaign", no inference.
  `event.Subs.SubCount()` makes subs-less traffic measurable, which is the
  point.
- **302, not 301.** A permanently-cached redirect would bypass the tracker
  on the next click, losing both the event and any routing change.
- **No per-request logging middleware on the tracker's router** — the path
  is latency-budgeted and every click already produces a structured event
  asynchronously; a second synchronous log line per click would be
  duplicate work on the hot path.

### Verified

- Live end-to-end against real Postgres with a seeded
  campaign → tracking link → 3 stream sets (bot block / nested
  `AND(device, OR(country US, CA))` / catch-all) → flows graph:
  - desktop → catch-all set → 302 to its flow destination;
  - unknown slug → 404;
  - Googlebot → classified `bot=1`, matched the block set, and routed to
    the campaign's **safe fallback** (§73's fallback model) rather than
    the real destination;
  - mobile with unknown geo → nested `OR(country…)` correctly failed, so
    the `AND` failed and it fell through to the catch-all — proving the
    filter tree really was rebuilt from Postgres and evaluated;
  - replayed sticky cookie → `sticky_applied=true` and the **original
    `click_id` reused**, a real attribution chain across a return visit.
- 205 events emitted, zero dropped, zero errors or warnings in the log.
- `go build`/`vet`/`gofmt`/`test -race` clean across the whole module.

### Fixed

- N/A this phase — no defects found in the implementation. Two mistakes
  were caught and corrected in the *test scaffolding* while validating:
  ULID literals in a seed script used the Crockford-excluded letters
  `O`/`L` (the `ulid` domain's CHECK constraint caught them, which is
  exactly what it is for), and a seed comment mispredicted that a blocked
  bot would end with no destination — it correctly received the campaign
  fallback instead.

### Known issues

- None new. Phase 10's unresolved crash-loop report carries over
  (unrelated). The in-app WebView bounce (§73) is still unimplemented, now
  tracked in `apps/tracker/README.md` rather than as an open architectural
  question.

### Files changed

- `apps/go.mod`, `apps/go.sum`, `apps/internal/**`, `apps/api/main.go` (moved)
- `apps/tracker/{main,handler,params,sticky}.go` + tests (new)
- `apps/internal/{event,eventbuf,routingstore}/**` (new)
- `apps/api/migrations/00011_campaign_sticky_flags.sql` (new)
- `apps/web/go.mod` (new — node_modules exclusion)
- `ARCHITECTURE.md`, `docs/architecture.md`, `apps/api/README.md`,
  `apps/api/migrations/README.md`, `apps/tracker/README.md` (modified)

## [Phase 20] — Traffic Classifier

### Added

- **`internal/classifier`** (§40): turns raw request signal (IP,
  User-Agent, `Accept-Language`) into `routing.Attributes`, keyed by the
  exact `routing.FilterField` constants stream-set filters already
  evaluate against — imported from Phase 19's package, not redeclared, so
  the two can never drift apart on a field's name. Feeds directly into
  `Router.Resolve` with no adapter layer, verified end-to-end in
  `integration_test.go`.
- **`GeoProvider`/`ASNProvider`/`BotDetector`** interfaces (§74/§75,
  non-negotiable #11 — no vendor lock-in), matching the spec's placeholder
  signatures with real method shapes. Defaults are honest about not being
  wired to a vendor: `NoopGeoProvider`/`NoopASNProvider` return empty
  results rather than fabricated geo/ASN data; `HeuristicBotDetector`
  flags well-known crawlers by a generic User-Agent substring list
  (Googlebot, curl, python-requests, …) — explicitly the provider-neutral
  technique §73 allows, not the ad-network moderator/reviewer detection it
  forbids — and always reports `IsProxy: false` rather than guess without
  a real IP-reputation vendor.
- **Local User-Agent parsing** (`useragent.go`, stdlib `regexp` only — RE2,
  non-negotiable #8): device/platform/os/os_version/browser/browser_version
  bucketed into the same small fixed vocabulary the frontend's
  `FIELD_VOCAB` already defines. No external UA-parsing dependency — the
  target vocabulary is already small and fixed, so hand-written
  substring/regex matching (with browser-detection order handled
  carefully: Edge and Samsung Internet checked before Chrome, Chrome
  before Safari, since all three embed each other's tokens) covers it
  completely.
- **`os_version`/`browser_version` populated even though §40's field list
  doesn't name them** — they're already real, filterable
  `routing.FilterField`s exposed since Phase 8's filter builder; leaving
  them dead would be a gap, not fidelity to the spec's (representative,
  not exhaustive) list.
- **`connection_type` always `"unknown"`** — no reliable
  wifi/cellular/ethernet signal exists server-side without a paid
  network-intelligence vendor or a client-side JS beacon, neither of which
  exist yet. Honest default, not a guess; matches the frontend's own
  vocabulary option of the same name.
- Device classification's one intentional non-"leave it empty" default:
  absent any mobile/tablet marker, `Device` defaults to `"desktop"` — "not
  mobile, not tablet" is itself a meaningful, safe-to-assume signal
  (the convention every real UA parser uses), unlike platform/os/browser
  where no such safe default exists.

### Fixed

- **A test expectation, not the implementation**: while writing
  `useragent_test.go`, an "unrecognized UA" case initially expected
  `Device: ""`, which failed against the (correct, intentional)
  `desktop`-default behavior described above. Fixed the test's
  expectation and tightened `UAResult`'s doc comment to state the
  device-defaulting rule explicitly, rather than changing working code to
  match a wrong assumption.

### Known issues

- None new. Phase 10's unresolved crash-loop report carries over
  (unrelated to this phase). `apps/tracker`/`apps/worker` module topology
  remains an open decision (documented Phase 16, still unresolved, not
  blocking) — `internal/classifier`'s eventual caller, alongside
  `internal/routing`.

### Files changed

- `apps/api/internal/classifier/*` (new)
- `docs/architecture.md` (modified — Phase 20 section)

## [Phase 19] — Routing Engine

### Added

- **`internal/routing`** (§38, §6-SHARED Strategy A): the single source of
  truth for routing decisions — stream-set priority, AND/OR nested filter
  evaluation, weighted flow selection, sticky assignment. Deliberately
  independent of `net/http` and any database driver (§38: "keep routing
  independent from HTTP handlers") — `Resolve` is a pure function of
  already-loaded config + already-classified request attributes →
  decision.
- **`Router.Resolve`** matches §38's exact spec'd interface and
  `RouteResult` shape byte for byte. **`Engine.Explain`** (a second method
  on the concrete engine, not part of the `Router` interface) runs the
  identical evaluation and additionally returns the full per-stream-set/
  per-flow `Explanation` §72 requires ("why did this match, why not that
  one, why this flow, why fallback, sticky from where") — the shape the
  frontend Routing Simulator already renders — without growing
  `RouteResult` beyond what §38 specifies. `Resolve`/`Explain` share one
  internal evaluation function, so they can't disagree.
- **Filter evaluation, weighted pick, and destination resolution ported
  faithfully from the existing frontend mock** (`lib/filters.ts`,
  `lib/routing-simulate.ts`) — same 31 `FilterField`s, same 16
  `FilterOperator`s (including the exact `norm()`/`compareValues()`
  semantics), same weighted-pick and fallback-cascade logic. Strategy A
  means the frontend's TypeScript version is a mock to be *replaced*
  (Phase 27), not a second implementation to keep in sync — porting the
  behavior faithfully now is what makes that swap invisible to users.
- **One deliberate correctness improvement over the frontend mock**: the
  Go engine checks `Destination.OfferActive` before using an offer
  destination, falling through to fallback if the offer is inactive — §58
  explicitly requires an "inactive offers" test case, and the current
  frontend mock never implemented this check at all.
- **`stickyFlowKeepClickId` doesn't reach this package at all** — it only
  affects whether the caller reuses an old `click_id` for attribution,
  which has zero effect on which flow gets selected. `RoutingConfig`
  doesn't carry it; documented in both the code and `docs/routing.md` so
  its absence reads as a decision, not an oversight.
- **Sticky is verifiably Redis-independent by construction**: this package
  has no cache dependency at all, so a sticky decision is a pure function
  of `(req.Sticky, req.Config)` on every call — there's nothing an "eviction"
  could invalidate on this side. Test proves identical results across
  repeated calls with the same sticky state.
- **Conformance fixture** (§6-SHARED, §58): `internal/routing/fixture_test.go`
  covers all 17 required cases — AND/OR/nested groups, priority
  (first-match wins), fallback (stream-set level before campaign level),
  weighted distribution (±2% over 10k picks, real seeded PRNG via an
  injectable `Engine.Rand01`, not a fixed value), sticky (+ skipInactive
  both branches), inactive flows/offers, missing-destination fallback
  cascade, and ISO-code-mismatch (proving no fuzzy UK/GB coercion — a
  literal, case-insensitive-only match). Three cases (`inactive campaigns`,
  `invalid tracking links`, `in-app WebView bounce`) are explicitly
  documented as out of this package's scope via `t.Skip` with reasoning
  inline, not silently absent — all three happen in the caller
  (`apps/tracker`, Phase 21) before `Resolve` is ever invoked.

### Fixed

- N/A this phase — no defects found. `go build`/`vet`/`gofmt`/`test`
  clean, full suite stable across 5 repeated runs and under `go test -race`.

### Known issues

- None new. Phase 10's unresolved crash-loop report carries over
  (unrelated to this phase). `apps/tracker`/`apps/worker` module topology
  remains an open decision (documented Phase 16, still unresolved, not
  blocking) — `internal/routing`'s eventual consumers.

### Files changed

- `apps/api/internal/routing/*` (new)
- `docs/routing.md` (rewritten — real conformance fixture table)
- `docs/architecture.md` (modified — Phase 19 section)

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
