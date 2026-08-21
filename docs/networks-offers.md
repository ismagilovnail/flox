# Networks & Offers CRUD (§27/Phase 11)

Full CRUD for `networks` and `offers` (+ nested `offer_links`), landed
together in one phase after a scope discovery: Traffic Sources CRUD had
established the "give a mocked domain a real backend, wire its existing
frontend mock" pattern, and Offers was picked as the next slice on the
(wrong) assumption that its network reference could stay a free-text field
until Networks existed separately. It can't — `offers.network_id` is `NOT
NULL REFERENCES networks (id)` (migration 00003). Surfaced to the user via
`AskUserQuestion` rather than silently expanding scope or silently
weakening the schema; the user chose to build both together, plus
`offer_links` as a small nested piece — completing the spec's own stated
hierarchy, Network → Offer → Offer Link.

## `internal/network`: a second copy of the trafficsource pattern

Flat entity, no children, mirrors `internal/trafficsource`'s
handler→service→repository split closely enough that most of it reads as
a find-and-replace (`Network` for `TrafficSource`, `postbackUrl`/
`acceptDuplicates` for `trackingTemplate`/`costIntegration`). `Duplicate`
keeps status as-is, same reasoning as every other domain's Duplicate in
this session — no draft-equivalent status exists to reset to.

### Delete: the opposite of trafficsource's own conflict story

`offers.network_id` is `ON DELETE CASCADE` — deleting a network silently
deletes its offers too, a deliberate schema choice this phase inherits
rather than makes. `TestDeleteCascadesToOffers` proves it directly (seeds
an offer, deletes its network, confirms the offer row is gone). This is
the opposite of `campaigns.traffic_source_id`'s `RESTRICT` — two sibling
domains, two different cascade policies, both intentional in the original
schema (00003 vs 00009), not something this phase reconciles or changes.

`flows.destination_network_id` (00006) has no `ON DELETE` clause
(`RESTRICT`) — no Flow CRUD exists yet to populate it, but
`Repository.Delete` catches Postgres `23503` defensively anyway, same
shape as `trafficsource`'s.

## `internal/offer`: the one non-flat domain in this session so far

### The hierarchy is enforced, not just modeled

`CreateInput.NetworkID` is validated against `NetworkBelongsToOrg` before
insert — the same §36-TENANCY cross-reference guard
`campaign.Repository.TrafficSourceBelongsToOrg` established. An offer
literally cannot be created against another org's network, or against no
network at all (empty/invalid id fails `idgen.IsValid`).

### Links: whole-array replace, not per-link CRUD

`offer-form-sheet.tsx`'s `useFieldArray` submits every link on every save
— not a diff against what existed before. `Repository.Update`/`Create`
match that exactly: on any write where `Links` is present, the offer's
entire `offer_links` row set is deleted and reinserted inside the same
transaction as the scalar-field update, generating fresh ids every time.
There is no standalone `/offers/{id}/links` endpoint — a link is never
addressed independently of its parent offer, matching how the frontend
never lets you edit "just one link" outside the full form.
`TestUpdateReplacesLinksWholesale` proves old link ids don't survive a
replace and the row count matches exactly.

### `Cap`: the one field needing three PATCH states

`cap` is nullable (`uncapped`) *and* optional-in-a-partial-PATCH (the
Archive convenience endpoint sends `{"status":"archived"}` alone and must
leave `cap` untouched, same pattern as every other domain's archive
action). A plain `*int` can't tell "key absent" apart from `"cap": null`
— `encoding/json` collapses both to a nil pointer. `OptionalCap` (with its
own `UnmarshalJSON`) is the one place in this phase that needed a custom
JSON type rather than a plain pointer:

```go
type OptionalCap struct {
    Set   bool
    Value *int
}
```

`Set == false` (the whole `*OptionalCap` is nil): not sent, leave
unchanged. `Set == true, Value == nil`: sent as `null`, clear to uncapped.
`Set == true, Value != nil`: sent as a number, set that cap.
`TestUpdateCapThreeStates` exercises exactly these three paths against a
real Postgres round-trip. The frontend side is simpler: `UpdateOfferInput`
types `cap` as `number | null | undefined` and relies on `JSON.stringify`
already dropping `undefined` keys — no custom serialization needed there.

### A real bug caught mid-verification: `values` + `useFieldArray` = infinite loop

The offer form crashed with React's "Maximum update depth exceeded" the
first time it was opened in the browser — not a validation bug, a render
loop. Every other rewritten form in this session (`campaign-form.tsx`,
`source-form-sheet.tsx`, `network-form-sheet.tsx`) uses React Hook Form's
`values` option (not `defaultValues`) so the form re-syncs if its
`defaultValues` prop changes identity across renders without a full
remount. `offer-form-sheet.tsx` combines that with `useFieldArray` (for
the links list) and a `MultiSelect` (for countries) — together, a fresh
`values` object literal on every render caused RHF to keep re-syncing the
field array, which triggered a state update in a Radix Popper ref
callback, which caused another render, which built another fresh
`values` object, forever.

Fixed by reverting `offer-form-sheet.tsx` to plain `defaultValues` (read
once per mount) and restoring `key={target?.id ?? "new"}` on all three
list components' form-dialog wrappers
(`OfferFormDialog`/`NetworkFormDialog`/`SourceFormDialog`) — the mount-key
pattern the original mock-backed components already used and this
session's earlier rewrites had quietly dropped when extracting the dialog
into its own component. `values` still works fine for the two simple
forms (no array fields), left unchanged; `defaultValues` + remount-via-key
is the safer default whenever a form has a `useFieldArray`.

## Frontend: a third mock/store pair left untouched

Same situation `lib/mock/campaigns.ts`/`stores/campaigns.ts` was in after
Phase 27: `lib/mock/networks.ts`/`stores/networks.ts` and
`lib/mock/offers.ts`/`stores/offers.ts` stay exactly as they were —
stream-sets, postbacks, and conversions (still fully mocked features)
import them transitively for their own UI. `lib/api/networks.ts` and
`lib/api/offers.ts` are new, parallel files used only by the Networks/
Offers pages themselves; nothing shared changed.

## Verified

Backend: `go build/vet/gofmt/test ./...` all green, including 6 new
`network` tests (CRUD round-trip, invalid-URL rejection, pause/activate
transitions, duplicate-keeps-status, cascade-delete-to-offers, cross-tenant
isolation) and 6 new `offer` tests (CRUD + link/country/currency
normalization, no-links/no-countries/non-positive-payout/cross-org-network
rejected, whole-array link replace, the three-state Cap PATCH, duplicate
keeps status and copies links with fresh ids, cross-tenant isolation).

Frontend: `tsc --noEmit`/`eslint` clean. Full manual browser pass against
the real running `api`+`web` dev servers: created a network, then an
offer against it through the full form (name, network picker, GEO
multi-select, payout, currency, daily cap, one tracking link) — all real
fields landed correctly in the list. Edit pre-filled every field including
the link URL and showed the Status field (edit-only). Pause flipped status
live. Duplicate copied every field including GEOs/cap and correctly kept
`paused` rather than resetting it. Test offers and the test network
removed via the real `DELETE` endpoints afterward (both were newly created
for this test — unlike Traffic Sources, no pre-existing dev seed data
existed for Networks/Offers to accidentally disturb).

## Deliberately deferred

Stream Sets/Flows (the next natural step — offers need somewhere to be
routed to) remain fully mocked, same reasoning as always: no backend
exists yet, and completing that is its own, larger phase.
