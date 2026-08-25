# Pixels (§29) — wired to a real Postgres-backed API

Fourth slice of the Landing/PWA/Postlanding/Pixels CRUD candidate list,
after [Postlanding](postlanding.md). `pixels` was already migrated
(migration 00008, alongside `postbacks` and the `stream_set_pixels` join
table), flat, no children — same shape/precedent as Postlanding.

## Backend: mirrors `apps/internal/postlanding` almost exactly

`apps/internal/pixel`: `model.go`/`handler.go`/`service.go`/
`repository.go`, wired at `/pixels` in `apps/api/main.go` the same way
`/postlandings` is. No URL at all, unlike Landing/PWA/Postlanding: a
pixel's `provider` + `pixelId` identify where a conversion gets reported,
not a page a visitor is sent to.

Validation in `pixel.Service`:
- `name`: 2-100 chars.
- `provider`: one of `facebook`/`tiktok`/`snapchat`/`twitter`/`generic`
  (`Provider.Valid()`).
- `pixelId`: free-form string, at most 80 chars — **not required**.
  Matches the frontend's own pre-existing zod schema
  (`z.string().max(80)`, no `.min(1)`): an operator can save a pixel
  before the provider has issued a real id yet.
- `events`: at least one event required, every value must be one of
  `ValidEventTypes` — a curated 6-entry subset of the full §43 event
  model (`PWA_INSTALL`, `CPA_HOLD`, `CPA_ACCEPT`, `CPA_REDEP`, `TG_JOIN`,
  `NOTIFICATION_SUBSCRIBE`), matching the frontend's
  `lib/api/pixels.ts`'s `PIXEL_EVENT_TYPES` exactly — checked for exact
  parity, same precedent as Postlanding's own curated subset.

`Create`/`Update`/`Delete`/`Duplicate` (copies fields verbatim, appends
" (Copy)", preserves non-active status via a follow-up `Update`)/`Pause`/
`Activate` (idempotent, `apierror.Conflict` from archived state) all
follow the same shape as `postlanding.Service`.

## Delete: no defensive FK catch needed, unlike Landing/PWA/Postlanding

`stream_set_pixels.pixel_id` (migration 00008) is the one place a pixel
gets referenced, and it `ON DELETE CASCADE`s from `pixels` — the
opposite of `flows.landing_id`/`pwa_id`/`postlanding_id`'s `RESTRICT`.
Deleting a pixel just drops its stream-set attachments along with it, so
`pixel.Repository.Delete` has no `23503` catch to add, unlike its three
siblings' `Delete` methods.

## Frontend

New `lib/api/pixels.ts` + `hooks/use-pixels.ts` (real API layer, mirrors
`lib/api/postlanding.ts`/`hooks/use-postlandings.ts` almost exactly).
`pixel-list.tsx`/`-form-sheet.tsx`/`-columns.tsx`/`-row-actions.tsx`
rewired off `stores/pixels.ts` (Zustand mock) onto the real hooks;
`LoadingState`/`ErrorState` added. No Content Gallery integration exists
for Pixels (unlike Landing/PWA/Postlanding) — the pre-existing mocked
`pixel-list.tsx` never had one either, so there was nothing to carry
forward.

`stores/pixels.ts` and `lib/mock/pixels.ts` deleted outright once
grepping confirmed zero remaining importers.

Provider display labels moved into `PIXEL_PROVIDER_I18N_KEY` (co-located
with `PixelProvider` in `lib/api/pixels.ts`), same pattern as
`trafficSources.ts`'s `SOURCE_TYPE_I18N_KEY` — the wire/stored value is
always the raw provider string; only the rendered label goes through
`t()`. Event codes (`PWA_INSTALL`, `CPA_HOLD`, ...) in the multi-select
stay deliberately untranslated — canonical §43 identifiers, not UI text,
same treatment as Postlanding's own event picker.

### i18n: added, not skipped

Same reasoning as every prior real-backend phase this session: Pixels
was still mocked (and, per its own mock file's doc comment, entirely
untranslated — hardcoded English strings) when the Frontend i18n phase
ran. Added a new `pixels` namespace (`en`+`ru`, both complete — key-set
parity checked directly) and registered it in `lib/i18n/config.ts`'s
`NAMESPACES`.

## Per-Stream-Set Pixel attachment: wired in a later phase

At the time this phase closed, `stream_set_pixels` (migration 00008,
the join table a Stream Set uses to fire zero or more of these Pixels)
had no CRUD wiring it to `stream_sets`. That landed in its own follow-on
phase — see `docs/stream-sets.md`'s "Stream Set ↔ Pixel attachment"
section. `docs/stream-sets.md`'s own past "per-flow Pixels" phrasing was
imprecise: the schema has always scoped pixels to the *Stream Set*, not
the Flow.

## Verified

- Backend: `go build/vet/gofmt/test ./...` all green — new `pixel_test.go`
  mirrors `postlanding_test.go`'s full test set (create/get/update/delete,
  an empty `pixelId` explicitly allowed, invalid-shape validation incl.
  bad provider/zero events/unrecognized event type/short name,
  pause/activate transitions incl. idempotency and archived-state
  conflict, duplicate keeps status, cross-tenant isolation across
  get/update/delete/list).
- Frontend: `tsc --noEmit`/`eslint`/`vitest run` (21 tests, unchanged —
  this phase's tests are backend-only, mirroring an existing CRUD shape
  with no new frontend logic to unit-test)/`next build` (production) all
  clean.
- Full manual browser pass against the real running `api`+`web` dev
  servers: confirmed the real empty state (no leftover mock seed data,
  unlike the old store's 3 hardcoded pixels); created a pixel (provider
  select showing real translated labels, one event selected via the
  multi-select); reloaded fresh and confirmed the row round-tripped
  through the real API (name/provider/pixelId/events/status all
  correct); paused it (status badge updated live); duplicated it (copy
  kept the paused status, provider, pixelId, and events verbatim);
  edited it and confirmed every field pre-filled correctly; archived it
  (confirmation dialog interpolated the name correctly, toast confirmed,
  status badge updated to "Archived", action menu correctly dropped to
  Edit/Duplicate only). Also spot-checked the Russian locale on this
  page (sidebar label, title, column headers, status badges, button —
  all correctly translated, no hydration errors). Test pixel rows
  deleted directly from Postgres afterward (no hard-delete in the UI for
  this entity, same as Landing/PWA/Postlanding).
