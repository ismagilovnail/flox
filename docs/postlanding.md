# Postlanding (§28) — wired to a real Postgres-backed API

Third slice of the Landing/PWA/Postlanding/Pixels CRUD candidate, right
after [PWA](pwa.md). `postlandings` was already migrated (migration 00004,
same file as `landings`/`pwas`), flat, no children — same shape, same
precedent. Per-flow Pixels stays mocked; this phase is Postlanding only.

## Backend: mirrors `apps/internal/landing`/`apps/internal/pwa` almost exactly

`apps/internal/postlanding`: `model.go`/`handler.go`/`service.go`/
`repository.go`, wired at `/postlandings` in `apps/api/main.go` the same
way `/landings` and `/pwas` are. No internal/external split and no
server-computed URL, unlike Landing: a postlanding's URL is always an
advertiser/team-owned page the caller supplies directly, so it's closer to
PWA's shape than Landing's.

Validation in `postlanding.Service`:
- `name`: 2-100 chars.
- `url`: absolute `http`/`https` URL (`isValidURL`, same helper shape as
  Landing/PWA).
- `events`: at least one event required, every value must be one of
  `ValidEventTypes` — a curated 6-entry subset of the full §43 event model
  (`PWA_INSTALL`, `NOTIFICATION_REQUEST`, `NOTIFICATION_SUBSCRIBE`,
  `NOTIFICATION_DECLINE`, `TG_JOIN`, `TG_START`) a postlanding can
  plausibly fire on. Not a duplicate of the canonical event enum — the
  frontend (`lib/api/postlanding.ts`'s `POSTLANDING_EVENT_TYPES`)
  references the same string values, checked for exact parity.

`Create`/`Update`/`Delete`/`Duplicate` (copies fields verbatim, appends
" (Copy)", preserves non-active status via a follow-up `Update`)/`Pause`/
`Activate` (idempotent, `apierror.Conflict` from archived state) all
follow the same shape as `landing.Service`/`pwa.Service`.

## Delete: same defensive-not-tested FK precedent as `landing`/`pwa`/`network`

`flows.postlanding_id` (migration 00006) has no `ON DELETE` clause
(defaults to RESTRICT), but no Flow CRUD exists yet to populate that
column. `postlanding.Repository.Delete` catches Postgres `23503` into
`apierror.Conflict` defensively, matching the same comment/precedent as
`landing.Repository.Delete`/`pwa.Repository.Delete` — not given a
dedicated integration test for the same reason: seeding a real `flows`
row would mean seeding `stream_sets` and a campaign purely to exercise a
path nothing in the product can reach yet.

## Frontend

New `lib/api/postlanding.ts` + `hooks/use-postlandings.ts` (real API
layer, mirrors `lib/api/pwa.ts`/`hooks/use-pwas.ts` exactly).
`postlanding-list.tsx`/`-form-sheet.tsx`/`-columns.tsx`/
`-row-actions.tsx` rewired off `stores/postlandings.ts` (Zustand mock)
onto the real hooks; `LoadingState`/`ErrorState` added. The Content
Gallery integration (`?gallery=<id>` prefilling the create form from a
gallery item's `postlandingPayload`) stays exactly as it was — same
shared, already-real infrastructure Landings/PWA use unchanged; only its
type import moved from the deleted `lib/mock/postlandings.ts` to
`lib/api/postlanding.ts`. Event codes (`PWA_INSTALL`, `TG_START`, ...) in
the multi-select are deliberately left untranslated — canonical §43
identifiers, not UI text, same treatment as the Event Mappings panel.

`stores/postlandings.ts` and `lib/mock/postlandings.ts` deleted outright
once grepping confirmed zero remaining importers.

### i18n: added, not skipped

Same reasoning as Landings/PWA: Postlanding was still mocked when the
Frontend i18n phase ran. Added a `postlanding` namespace (`en`+`ru`, both
complete — key-set parity checked directly).

## Known issue (resolved in a later phase): i18n hydration race was mitigated, not eliminated

**Update:** closed deterministically in a later phase via server-side
locale resolution — see `docs/i18n-hydration-fix.md`. The description
below is kept as the original finding for historical context.

The `requestIdleCallback`-based fix landed in the PWA phase
(`components/i18n-provider.tsx`) reduces, but does not fully eliminate,
the hydration race on any page whose list component calls
`useSearchParams()` inside a `<Suspense>` boundary (Landings, PWA,
Postlanding). During this phase's manual verification, the same
"Hydration failed" error the PWA phase fixed was still observed
intermittently — roughly 2 of ~6 fresh `/postlanding` navigations logged
it, and the identical race reproduced on `/landings` in the same session
once specifically tested for. `requestIdleCallback` fires once the main
thread is idle, which in practice (not by guarantee) means any deferred
Suspense hydration commit has drained — under some timing/load
conditions it can still fire first. Not a new regression from this
phase or specific to Postlanding: same underlying architectural
trade-off already documented in `i18n-provider.tsx`'s comment, now
confirmed non-deterministic rather than fully closed. React
auto-recovers (discards and re-renders the mismatched subtree), so it's
not user-visible beyond a rare, instant re-render — left as a known,
non-blocking issue rather than expanding this phase into a deeper
architectural fix (e.g. server-side locale detection via cookie).

## Verified

- Backend: `go build/vet/gofmt/test ./...` all green — new
  `postlanding_test.go` mirrors `landing_test.go`'s/`pwa_test.go`'s full
  test set (create/get/update/delete, invalid-shape validation incl. bad
  URL/zero events/unrecognized event type/short name, pause/activate
  transitions incl. idempotency and archived-state conflict, duplicate
  keeps status, cross-tenant isolation across get/update/delete/list).
- Frontend: `tsc --noEmit`/`eslint`/`vitest run`/`next build` (production)
  all clean.
- Full manual browser pass against the real running `api`+`web` dev
  servers, in Russian locale: created a postlanding (multi-select events,
  URL validation error surfaced correctly then cleared), edited it
  (all fields incl. events prefilled correctly), paused/resumed,
  duplicated (copy kept paused status, URL, and both events verbatim),
  archived (confirmation dialog interpolated the name correctly; action
  menu correctly dropped to Edit/Duplicate only, matching Landings/PWA).
  Test postlanding rows deleted directly from Postgres afterward (no
  hard-delete in the UI for this entity).
