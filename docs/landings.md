# Landings (§28) — wired to a real Postgres-backed API

The smallest slice of the Landing/PWA/Postlanding/Pixels CRUD candidate
CLAUDE.md's NEXT had been carrying: `landings` was already migrated
(migration 00004), flat, no children — the exact shape
`apps/internal/network` already establishes a real precedent for. PWA and
Postlanding (own migrations, own frontend mocks) and per-flow Pixels stay
mocked; this phase is Landings only.

## Backend: mirrors `apps/internal/network` almost exactly

`apps/internal/landing`: `model.go`/`handler.go`/`service.go`/
`repository.go`, wired at `/landings` in `apps/api/main.go` the same way
`/networks` is. One real difference from the template, not a structural
one: Landing has a `type` (`internal`/`external`) that changes what's
required and what's derived.

## The one piece of real business logic: internal URLs are server-computed

The old mock (and, transcribed faithfully from it, the frontend form
submit handler in `landing-list.tsx`) computed an internal landing's
`url` client-side: `https://cdn.floxlink.io/lnd/{slugify(name)}`. That's
exactly the kind of derived value CLAUDE.md's own layering rule says
shouldn't live in a React component. `landing.Service.Create`/`Update`
now compute it server-side from `Name` — a client-supplied `url` for
`type: internal` is accepted in the request shape (so the JSON contract
stays uniform with `external`) but always ignored, never persisted.

- `Create`: `type: internal` always derives `url` from `name`, regardless
  of what the request sent.
- `Update`: `url` is only recomputed when `name` or `type` actually
  changed in that call — a status-only PATCH (pause/activate/duplicate's
  preserve-status follow-up) leaves `url`/`content` untouched. Verified
  directly (`TestInternalLandingURLIsServerComputed`'s third case).
- `Duplicate` goes through `Service.Create` (not `Repository.Create`
  directly), so a `(Copy)`'s URL is recomputed for its new name rather
  than copied verbatim from the source — two landings never silently
  share a "hosted" path.
- `type: external → internal` on Update clears `content` server-side (an
  advertiser-owned page has none); `internal → external` is the mirror,
  handled by the existing "recompute only when needed" branch simply not
  applying to external.

`slugify` is reimplemented in Go (`regexp`-based) to match
`apps/web/src/lib/utils.ts`'s `slugify` exactly, so the form's
client-side "Hosted URL" preview and the value the server actually
persists never disagree. No real CDN backs this today — same MVP
realism as every other resolved-but-not-actually-served URL in this
codebase (e.g. postback delivery URLs before a real network exists to
receive them).

## Delete: same defensive-not-tested FK precedent as `network`

`flows.landing_id` (migration 00006) has no `ON DELETE` clause (defaults
to RESTRICT). A later phase (see `docs/stream-sets.md`'s "Landing/PWA/
Postlanding stages: restored" section) wired Flow CRUD to actually
populate this column, so the RESTRICT is real and reachable now, not
just defensive scaffolding. `landing.Repository.Delete` catches Postgres
`23503` into
`apierror.Conflict` defensively, matching `network.Repository.Delete`'s
own comment and — deliberately, matching that same precedent — isn't
given a dedicated integration test, since seeding a real `flows` row
would mean seeding a `stream_sets` row and a campaign purely to exercise
a path nothing in the product can reach yet.

## Frontend

New `lib/api/landings.ts` + `hooks/use-landings.ts` (real API layer,
mirrors `lib/api/networks.ts`/`hooks/use-networks.ts` exactly).
`landing-list.tsx`/`landing-form-sheet.tsx`/`landing-columns.tsx`/
`landing-row-actions.tsx` rewired off `stores/landings.ts` (Zustand
mock) onto the real hooks; loading/empty/error states added (DoD §79
requires them; the old mock was synchronous and had none). The Content
Gallery integration (`?gallery=<id>` pre-filling the create form from a
gallery item's `landingPayload`) and Tags integration both stay exactly
as they were — neither is Landing-specific, both are already shared,
already-local infrastructure other real domains (Offers, Networks) use
unchanged too.

`stores/landings.ts` and `lib/mock/landings.ts` deleted outright once
grepping confirmed zero remaining importers — the same "drop it, don't
fake it" precedent every prior real-backend phase this session has
followed.

### i18n: added, not skipped

Landings was still mocked when the Frontend i18n phase ran, so it was
correctly out of that phase's scope. Now that it's real, leaving it as
the one hardcoded-English holdout next to every other now-real domain
page would be a visible regression, not a scope discipline win — DoD's
loading/error states need *some* text regardless, and this domain's
component set is small and closely mirrors `networks.json`'s already-
established shape. Added a `landings` namespace (`en`+`ru`, both
complete) and registered it in `lib/i18n/config.ts`.

## Verified

- Backend: `go build/vet/gofmt/test ./...` all green — new
  `landing_test.go` mirrors `network_test.go`'s test set (create/get/
  update/delete, invalid-shape validation, pause/activate transitions,
  duplicate keeps status, cross-tenant isolation) plus
  `TestInternalLandingURLIsServerComputed` (client-supplied URL ignored
  on create, URL follows a rename, a status-only update leaves URL/
  content untouched).
- Frontend: `tsc --noEmit`/`eslint`/`next build` (production) all clean.
- Full manual browser pass against the real running `api`+`web` dev
  servers, in both locales: created an internal landing (confirmed the
  server-computed URL matches the client preview exactly), renamed it
  (confirmed the URL followed, via a real round-trip, not just client
  state), duplicated it (confirmed the copy's URL was recomputed for
  its new name, not copied), created an external landing (confirmed its
  client-supplied URL persisted untouched, and that submitting with an
  empty URL is rejected client-side), archived a landing (confirmed the
  archived row's action menu correctly drops Pause/Archive, keeping
  only Edit/Duplicate — matching Networks' identical pattern), and
  confirmed full Russian rendering throughout (list, form, row actions,
  archive confirmation dialog, toasts). Test landings deleted afterward
  (no hard-delete exists in the UI for this entity, same as every other
  archive-only domain — deleted directly, matching the Postback Replay
  phase's precedent for rows with no UI path to remove them).
