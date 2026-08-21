# Traffic Sources CRUD (§27/Phase 11)

Full CRUD for the `traffic_sources` entity, growing `apps/internal/trafficsource`
from Phase 27's deliberately read-only `List` (a campaign form's source
picker needed one, nothing else did yet) into create/edit/pause/activate/
archive/duplicate — mirroring `internal/campaign`'s own shape closely,
since the two packages solve the same "list of team-managed entities a
campaign references" problem.

## Why this domain, first

Chosen (confirmed with the user via `AskUserQuestion`, among Offers/
Networks/Stream Sets) as the smallest next slice: `GET /traffic-sources`
already existed, the frontend already had a complete mock CRUD UI
(`source-list.tsx`, `source-form-sheet.tsx`, `source-row-actions.tsx`,
`source-columns.tsx`) to wire up unchanged in shape, and the entity has no
nested children (unlike Stream Sets, which depend on Offers existing
first for flow targets).

## `type` stays free text, deliberately

`traffic_sources.type` has no CHECK constraint (migration 00002's own
comment: `"frontend SourceType"`) — the backend accepts any non-empty
string. `SOURCE_TYPES` (`lib/api/traffic-sources.ts`) is a client-side UX
vocabulary for the dropdown, not a contract the server enforces. This was
already true before this phase; it's unchanged, not a new decision.

## `costIntegration` records intent, not data

`CostIntegration` (`none`/`manual`/`facebook_ads`/`tiktok_ads`) is
independent of `internal/cost`'s actual per-day amounts (Phase 27-COST): a
source with `costIntegration: "manual"` still needs real entries logged
through a campaign's Cost tab — this field doesn't create them, and
setting it to `facebook_ads` doesn't pull anything yet (that's the FB/
TikTok import phase, still unbuilt). The frontend form's helper text says
this explicitly.

## `Duplicate` keeps status, unlike `campaign.Service.Duplicate`

`campaign.Service.Duplicate` forces a fresh copy back to `"draft"` —
traffic sources have no draft-equivalent status (`active`/`paused`/
`archived` only), and the mock store this replaced
(`stores/traffic-sources.ts`) never reset it either. `Service.Duplicate`
copies the source's status as-is; `TestDuplicateKeepsStatus` pauses a
source, duplicates it, and asserts the copy is paused too.

## `Delete`: FK-conflict turned into a clean 409, not a raw 500

`campaigns.traffic_source_id` is `NOT NULL REFERENCES traffic_sources(id)`
with no `ON DELETE` clause (00005) — Postgres defaults to `RESTRICT`,
deliberately: a source with campaigns still pointing at it shouldn't
silently vanish out from under them. `Repository.Delete` catches Postgres
error code `23503` and returns the same `apierror.Conflict` shape every
other domain error in this API uses, rather than letting a raw
`foreign_key_violation` surface as an opaque 500.
`TestDeleteConflictsWhenReferencedByACampaign` seeds a referencing
campaign and asserts the clean error. (The frontend's row-actions menu has
no hard-delete option at all — only Archive — matching campaigns'
precedent; `Delete` exists on the backend for admin/test cleanup, same as
`campaign.Handler`'s.)

## `created_by_user_id`-shaped gap: none here

Unlike `cost_entries` (Phase 27-COST), `traffic_sources` has no
"created by" column — nothing in this migration needed the Phase 28 auth
workaround `cost_entries.created_by_user_id` required.

## Verified

Backend: `go build/vet/gofmt/test ./...` all green, including 7 new
integration tests (create/get/update/delete round-trip, invalid tracking
template rejected, pause/activate transitions incl. idempotency and the
archived-state rejection, duplicate keeps status, delete conflicts when
referenced, full cross-tenant isolation across get/update/delete/list).

Frontend: `tsc --noEmit`/`eslint` clean. `lib/mock/traffic-sources.ts` and
`stores/traffic-sources.ts` deleted outright (unlike
`lib/mock/campaigns.ts`/`stores/campaigns.ts`, which Phase 27 left in
place because other still-mocked features — conversions, tag assignments
— import them transitively; nothing outside the traffic-sources feature
ever imported the traffic-sources mock/store, so there was nothing to
preserve).

Manual browser pass against the real running `api`+`web` dev servers:
created a source through the UI (`Push Test Source`), paused it (status
flipped live), duplicated it (copy correctly kept `paused`, not reset),
opened Edit and confirmed the form pre-filled from the real record with
the Status field present (edit-only, matching `campaign-form.tsx`'s own
`showStatus` pattern). Verified the FK-conflict path directly via `curl`:
deleting a source referenced by a real campaign returned a clean `409`
with a helpful message, not a raw Postgres error. Test rows removed via
the real `DELETE` endpoint afterward; the dev org's original two seed
sources ("Facebook Ads", "TikTok Ads") were restored to their pre-test
state (one was deleted by mistake mid-verification, before the
FK-referencing test that would have caught it — recreated with the same
name/type once noticed).

## Deliberately unchanged

Tags stay local-only mock (`stores/tags.ts`) for traffic sources, same as
campaigns — Tags is its own later phase (14.5) regardless of which domain
gets real CRUD first. "View statistics" still links into the mocked
`/analytics` report builder, untouched.
