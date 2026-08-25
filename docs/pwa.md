# PWA (§28) — wired to a real Postgres-backed API

Second slice of the Landing/PWA/Postlanding/Pixels CRUD candidate
CLAUDE.md's NEXT had been carrying, immediately after [Landings](landings.md).
`pwas` was already migrated (migration 00004, same file as `landings`/
`postlandings`), flat, no children — same shape, same precedent. Postlanding
(own frontend mock) and per-flow Pixels stay mocked; this phase is PWA only.

## Backend: mirrors `apps/internal/landing` almost exactly

`apps/internal/pwa`: `model.go`/`handler.go`/`service.go`/`repository.go`,
wired at `/pwas` in `apps/api/main.go` the same way `/landings` is. No
server-computed-URL business logic this time (that was Landing's one real
difference from the `network` template) — PWA's fields are closer to a
literal W3C manifest: `name`, `shortName`, `themeColor`, `backgroundColor`,
`iconUrl`, `startUrl`, `bounceInAppWebview`, `status`.

Validation in `pwa.Service`:
- `name`: 2-80 chars.
- `shortName`: required, ≤20 chars (the manifest's own `short_name` limit
  for home-screen icon labels).
- `themeColor`/`backgroundColor`: `^#[0-9a-fA-F]{6}$` hex only.
- `iconUrl`: absolute `http`/`https` URL (`isValidURL`).
- `startUrl`: non-empty only — **not** run through `isValidURL`. This is a
  relative install path (`/install/sweeps`), not an absolute URL; the one
  field where PWA's validation genuinely differs from Landing's, and
  covered by its own test (`TestUpdateAcceptsRelativeStartURL`) so a
  future refactor doesn't accidentally tighten it to `isValidURL` and
  break every real install link.

`Create`/`Update`/`Delete`/`Duplicate` (copies fields, appends " (Copy)",
preserves non-active status via a follow-up `Update`)/`Pause`/`Activate`
(idempotent, `apierror.Conflict` from archived state) all follow the same
shape as `landing.Service` and `network.Service`.

## Delete: same defensive-not-tested FK precedent as `landing`/`network`

`flows.pwa_id` (migration 00006) has no `ON DELETE` clause (defaults to
RESTRICT). A later phase (see `docs/stream-sets.md`'s "Landing/PWA/
Postlanding stages: restored" section) wired Flow CRUD to actually
populate this column, so the RESTRICT is real and reachable now, not
just defensive scaffolding. `pwa.Repository.Delete` catches Postgres
`23503` into `apierror.Conflict`
defensively, matching `landing.Repository.Delete`'s own comment — and,
deliberately matching that same precedent, isn't given a dedicated
integration test, since seeding a real `flows` row would mean seeding a
`stream_sets` row and a campaign purely to exercise a path nothing in the
product can reach yet.

## Frontend

New `lib/api/pwa.ts` + `hooks/use-pwas.ts` (real API layer, mirrors
`lib/api/landings.ts`/`hooks/use-landings.ts`). `pwa-list.tsx`/
`pwa-form-sheet.tsx`/`pwa-columns.tsx`/`pwa-row-actions.tsx` rewired off
`stores/pwas.ts` (Zustand mock) onto the real hooks; `LoadingState`/
`ErrorState` added (the old mock was synchronous and had none). The
Content Gallery integration (`?gallery=<id>` prefilling the create form
from a gallery item's `pwaPayload`) and Tags integration stay exactly as
they were — same shared, already-real infrastructure Landings/Offers/
Networks use unchanged. The manifest-preview JSON block in the form sheet
(`name`/`short_name`/`theme_color`/... ) is deliberately left
untranslated — those are real W3C manifest field names, not UI text.

`stores/pwas.ts` and `lib/mock/pwas.ts` deleted outright once grepping
confirmed zero remaining importers — same "drop it, don't fake it"
precedent as every prior real-backend phase.

### i18n: added, not skipped

Same reasoning as Landings: PWA was still mocked when the Frontend i18n
phase ran, so it was correctly out of scope then. Added a `pwa` namespace
(`en`+`ru`, both complete — key-set parity checked directly, not just
assumed) and registered it in `lib/i18n/config.ts`.

## Cross-cutting fix found during this phase: i18n hydration race

Manual browser verification of `/pwa` surfaced a genuine, reproducible
React hydration error (visible via the Next.js dev-overlay "issue" badge,
not just a screenshot) — `t("list.title")` rendered the server's English
default (`"New PWA"`) but hydrated against the just-switched Russian
value (`"Новое PWA"`), and React discarded/re-rendered the subtree.

Root cause: `I18nProvider` (`components/i18n-provider.tsx`, from the
earlier Frontend i18n phase) calls `i18n.changeLanguage()` from a plain
post-mount `useEffect`. Any page whose list component calls
`useSearchParams()` (required for the Content Gallery `?gallery=<id>`
integration — Landings, PWA, Postlanding) must sit inside a `<Suspense>`
boundary (a Next.js requirement), and a Suspense boundary can hydrate on
its own deferred commit, separate from the rest of the tree. When
`changeLanguage()` fires and notifies every `useTranslation()` subscriber
before that deferred commit has happened, the boundary hydrates against
the new language while the server sent HTML for the default one — a real
defect, not cosmetic (auto-recovered by React, but still a genuine
mismatch and wasted re-render on every load).

This bug pre-dated this phase: it was introduced with `I18nProvider`
itself and was already live on `/landings` (which also uses
`useSearchParams()`) but went unnoticed because the prior phase's manual
testing didn't check the dev-overlay badge. It was absent on pages
without `useSearchParams()`/`Suspense` (`/networks`, `/offers`), which is
what made it visible only now.

Fix (`components/i18n-provider.tsx`): defer the `changeLanguage()` call
with `requestIdleCallback` (Safari fallback: `setTimeout(fn, 0)`, since
Safari also lacks the concurrent-rendering machinery that causes the
race in the first place) instead of calling it synchronously in the
effect. Tried and rejected first: `React.startTransition()` (still
raced), a fixed short `setTimeout` (still raced) — a fixed long
`setTimeout` did avoid it in testing, confirming the race hypothesis, but
was rejected as a fragile magic-number production fix with no duration
that's both provably safe and not a user-visible stall.
`requestIdleCallback` waits for the browser's main thread to actually go
idle, which in practice means any deferred Suspense hydration commit has
already drained, without guessing a constant.

Verified via repeated fresh navigations to both `/landings` and `/pwa`
with the browser console read directly (not just a screenshot check):
zero hydration errors on either page. This retroactively fixes the same
latent defect on the already-shipped Landings phase — no Landings code
changed, only the shared provider.

## Verified

- Backend: `go build/vet/gofmt/test ./...` all green — new `pwa_test.go`
  mirrors `landing_test.go`'s test set (create/get/update/delete,
  invalid-shape validation incl. bad hex color/bad icon URL/empty short
  name/blank start URL, pause/activate transitions, duplicate keeps
  status, cross-tenant isolation) plus
  `TestUpdateAcceptsRelativeStartURL` (`startUrl` explicitly NOT run
  through `isValidURL`).
- Frontend: `tsc --noEmit`/`eslint`/`vitest run`/`next build` (production)
  all clean.
- Full manual browser pass against the real running `api`+`web` dev
  servers, in both locales: created a PWA (manifest preview matched form
  values live), edited it, paused/resumed, duplicated (copy kept
  non-active status when applicable), archived (action menu correctly
  dropped to Edit/Duplicate only, matching Landings/Networks), confirmed
  full Russian rendering throughout (list, form, row actions, archive
  confirmation dialog, toasts). Also confirmed the hydration fix via
  repeated `/landings` ↔ `/pwa` navigation with console reads. Test PWA
  row deleted directly from Postgres afterward (no hard-delete in the UI,
  same as every other archive-only domain).
