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

## Macros in offer links (audit 2026-10-06, T7)

An offer link, a redirect flow's URL and an external landing's URL are
resolved by `apps/tracker` at click time with `macro.ResolveURL`:
`{click_id}`, `{campaign_id}`, `{offer_id}`, `{country}`, `{device}`,
`{sub1}`…`{sub10}`. `{status}`/`{revenue}`/`{currency}` resolve empty at
click time; `{payout}` and `{source}` are not known to the resolver and
stay literal. Values are query-escaped — `sub1..10` come from the click's
own URL and must not be able to add parameters. The resolved URL is what
the click event records and what the signed landing/PWA links carry, so
the click-through after a landing reaches the same URL with the same id.

Before this the tracker redirected with a literal `{click_id}`: a network
never received the id it has to echo back in its postback. `apps/e2e` now
takes the click id from the redirect itself, not from ClickHouse.

## Incoming and outgoing postback URLs (audit 2026-10-15, UX-1, UX-2)

**Incoming** — what the network calls. The API returns
`incomingPostbackUrl` on every network: `TRACKER_PUBLIC_URL` +
`/postback/{id}`, no query (`network.Service.WithTrackerURL`, set in
`apps/api/main.go`). It is empty when `TRACKER_PUBLIC_URL` is not set; the
UI then says so instead of guessing a host. `lib/postback-url.ts` adds the
query `apps/tracker/postback.go` reads:
`?secret={secret}&click_id={click_id}&status={status}&revenue={revenue}&currency={currency}&txn_id={txn_id}`
— braces are for the operator to swap for the network's macros.

- Network page: an "Incoming postback" card — the URL with a copy button,
  what each parameter means (the currency row names the network's payout
  currency), the statuses FLOX maps for this network (a warning with a link
  to the mapping when there are none), and "Get a new secret" behind a
  confirmation.
- Right after a create or a regenerate (the only time the secret is
  known) the dialog shows the whole URL with the secret already in it.
- Postbacks → Incoming lists the same URL per network. It used to show a
  made-up `https://api.floxlink.io/postback/{id}` — the API's host, not
  the tracker's, and no secret: every postback sent to it would have been
  refused.
- Regenerating from the row menu now asks first too: the old secret stops
  working at once.

**Outgoing** — what FLOX calls when a conversion of this network changes
status (`postback_url`, §46). Optional now: empty means no outgoing
postback (`conversion` already skipped delivery for an empty URL); a
non-empty value is still validated. Saving `" "` clears it.

Verified live: a network created without an outgoing URL; the URL copied
from the dialog accepted (secret, then "no event mapping" until one was
added, then `success`).

## Repeat purchases without txn_id (audit 2026-10-15, DATA-2; migration 00088)

A REDEP's dedup key is `(click_id, CPA_REDEP, txn_id)` (§45, CLAUDE.md #3).
A network that does not send `txn_id` gives every repeat purchase of a click the
same empty `event_ref`, so each repeat after the first is refused as a
duplicate and is missing from revenue and LTV. The key stays as it is: two
replays of one purchase and two purchases are indistinguishable without the
id, and counting replays would be worse. The fix is on the network's side,
so FLOX now says so:

- `GET /networks/{id}/diagnostics` (`offer.read`, org-scoped, 404 for
  another organization's network): over 30 days, REDEPs with an empty
  `event_ref` received (success + duplicate), how many were duplicates,
  the last one. A partial index (00088) keeps it off the rest of the
  network's postbacks.
- The network page shows "N repeat purchases lost: repeat purchases come without
  txn_id" when any were refused, and a softer note when REDEPs arrive
  without the id but none was lost yet; both say to add
  `txn_id={transaction_id}` with the network's macro.
- The duplicate's message (ledger, the tracker's answer, the postback
  check) says the repeat purchase came without `txn_id`. A duplicate's outcome
  now carries the store's message rather than the attribution note.

## Affiliate-platform templates (audit 2026-10-15, INT-3; migration 00093)

`networks.template_key`; the catalog is `apps/web/src/lib/network-templates.ts`.
When a network is created on a known platform:
- **Incoming URL.** The URL the operator copies carries the platform's own
  macros, and only the parameters the platform has macros for. A FLOX
  placeholder left in would reach FLOX literally; a missing currency
  falls back to the network's payout currency.
- **Offer link.** The card says which offer-link parameter passes FLOX's
  click id to the platform.
- **Status mappings** come with the network (created by the form right
  after the network; a failure is reported, the network stands).

Only platforms with public postback docs are in the catalog:
- **Affise:** `click_id={sub1}` ("there is no clickid macro for affiliate
  postback"), `status={status}` with 1 Approved → CPA_ACCEPT, 2 Pending →
  CPA_HOLD, 3 Declined → CPA_DECLINE, 5 approved-and-on-Hold → CPA_HOLD (a
  hold after an accept is refused by the progression rule anyway),
  `revenue={sum}`, `currency={currency}`, `txn_id={transactionid}`.
- **Scaleo:** `click_id={aff_click_id}`, `revenue={payout}`. It sends one
  postback per goal and has no status macro, so the operator puts the
  goal's name in place of `REPLACE_GOAL` and maps those names.

Affilka, Cellxpert, MyAffiliates, Income Access, NetRefer and IREV keep
their docs behind login (checked 2026-10-16). A template is one catalog
entry once their macros are known.

## Own site — conversions without a partner (audit 2026-10-24, UNI-1 part 1)

A shop, a service or an app that sells itself has no affiliate network; its
own server reports leads, purchases and refunds. In FLOX it is a network
with `templateKey = "own_site"` — the same entity, postback endpoint, secret
and dedup rules, no schema change; what changes is the words and the
statuses.

- **Creating:** the form first asks "Where conversions come from": a
  partner network or own site (`NetworkForm` `chooseKind`, only when
  creating). Own site hides the platform picker; its statuses are mapped
  with it (`OWN_SITE_TEMPLATE`, `lib/network-templates.ts`): `lead` →
  CPA_HOLD, `purchase` → CPA_ACCEPT, `repeat_purchase` → CPA_REDEP,
  `refund` and `cancel` → CPA_DECLINE, `spam` → CPA_TRASH — the site can
  send `status=purchase` at once.
- **Its page:** "Conversion URL — for your site", three steps (the site's
  URL with `click_id={click_id}` in the offer link; keep the click id until
  the order; the server calls the URL with status, amount, currency and the
  order number as `txn_id`), "call it from your server only" (the URL holds
  the secret), and what each status means. The secret dialog speaks to the
  site's developer. The list marks such sources "Own site".
- **Every source:** statuses show by name (ACCEPT, HOLD…), the enum in the
  tooltip, instead of `999 → CPA_ACCEPT`.
- **Words:** the section is "Conversion sources" (was "Networks"), and the
  onboarding asks for "a conversion source" — a partner network or your own
  site.
- **Known limit, found in the live check:** a refund always reverses the
  click's first purchase (§45: DECLINE reverses the CPA_ACCEPT payout). A
  refund of a repeat purchase therefore takes the first purchase's amount
  out — said in the status's explanation; per-order refunds are UNI-4.
- **Not in part 1:** a script for pages with no server (Tilda and other
  builders) — the secret cannot be put in a page; it needs its own public
  endpoint with fraud limits (UNI-1 part 2).

Live (dev stack): an own-site source created through the API with its six
mappings; on one click `lead` → CPA_HOLD, `purchase` 1 990 RUB → CPA_ACCEPT
$24.71, `repeat_purchase` 990 RUB (`txn_id` A-1002) → CPA_REDEP $12.29,
`refund` → CPA_DECLINE −1 990 RUB (the limit above). Tests:
`network-templates.test.ts` (the mappings cover every status, the URL),
`network-form.test.tsx` (mutation-checked), `incoming-postback-card.test.tsx`.

## Reports by network (2026-10-13)

A click routed to an offer carries that offer's network (`network_id`,
stamped by `apps/tracker` at click time; ClickHouse `schema/017`), and a
conversion carries the network that sent its postback. Together they back
the `network` report dimension and the networks list's "View statistics"
(`docs/analytics-report-api.md`). Moving an offer to another network
changes only clicks from then on; clicks from before the column existed
were filled once from their offer.

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
