# Conversions list + detail/timeline (§29, §43) — wired to a real backend

The first of three separable pieces the "Conversions/Postbacks" domain
turned out to be on inspection (the other two — Postback Logs, Event
Mappings CRUD — are still mocked, deliberately deferred). The hard backend
work was already done: `apps/internal/conversion` (singular) is Phase 23's
conversion engine — dedup, status progression, attribution — wired into
`apps/tracker`'s hot path, and ClickHouse's `click_events`/
`tracking_events`/`conversion_events` (§48) were already being written.
What was missing was purely a browser-facing read API. This phase adds
exactly that: `apps/internal/conversions` (plural — do not confuse with
the singular engine package), a thin read layer mirroring
`apps/internal/analytics`'s own "no writes, no dedup, just query
ClickHouse" shape.

## Why click_id, not a synthetic id, addresses a conversion

The old mock modeled "a conversion" as one row with a fabricated uuid. The
real data model doesn't have that: `conversion_events` records one row per
CPA status event, and a single click_id legitimately has several over time
(HOLD, then ACCEPT, then REDEP, ...) — that's real status history, not
duplicate rows to dedupe away. So the List page shows one row per
(click_id, status) event, and the Detail page is addressed by **click_id**
— `GET /conversions/{clickId}` returns every event ever recorded for that
click, conversion and pre-conversion alike, not just one.

## The timeline is real, variable-length data, not a fixed six-stage funnel

The old mock's `ConversionTimeline` always rendered exactly six stages
(Click → Landing → PWA → Offer → Conversion → Postback), four of which
were fabricated sentences ("Landing page rendered", "Install prompt
accepted", ...) with no backing data. The real backend can do better:
`click_events` and `tracking_events` already record every SOURCE_CLICK/
SOURCE_FILTER and funnel-stage event (§43's full ~20-type model) with a
real `event_at` and `click_id`. `conversions.Service.Timeline` reads both
(`chstore.FunnelByClickID`) plus every conversion event for that click_id
(`chstore.ConversionsByClickID`) and merges them into one chronological
list — whatever actually happened, in whatever order, however many items
long. A click that never converted still gets a real (shorter) timeline;
a click with three status changes gets a real five-plus-item one. Nothing
is invented to fill a fixed shape.

The frontend's `ConversionTimeline` groups the ~20 real event types into
the same six icon buckets the old mock used (Click/Landing/PWA/
Notification/Telegram/Conversion) by string-prefix matching on
`event.Type`, but the label under each icon is the real event type
(`"PWA Install"`, not a synthesized sentence) — except for CPA_* types,
which keep the old mock's more editorial labels ("Registration held",
"First deposit accepted", ...) since those carry real, useful meaning.

## What's dropped, not faked

- **Offer**: `conversion_events` carries `flow_id`, not `offer_id` —
  resolving an offer would mean a separate Postgres join through
  `flows.destination_offer_id`, out of this phase's scope. The list's
  old Offer column is gone; Campaign and Network (both directly on the
  ClickHouse row) took its place.
- **Postback status / "Resend postback"**: outgoing delivery status
  (`postback_deliveries`) is the deferred Postback Logs domain. The old
  mock's `postbackStatus` column, its StatCard, and the "Resend postback"
  button are all gone — not shown as "—", not disabled-but-present, gone
  entirely, matching this session's standing "drop it, don't fake it"
  precedent.

Campaign/Network **names** are resolved client-side from the already-real
`useCampaigns()`/`useNetworks()` hooks (id→name maps), the same pattern
the old mock UI used against its own mock stores — `apps/internal/
conversions` itself returns raw ids only, no cross-database joins.

## `stores/conversions.ts` deleted, `lib/mock/conversions.ts` kept (partially)

`stores/conversions.ts` had no remaining importers once `ConversionList`/
`ConversionDetailView` switched to the real hooks — deleted outright.
`lib/mock/conversions.ts` stays, but trimmed: `CpaStatus`/`CPA_STATUSES`
moved to `lib/api/conversions.ts` (a real domain enum, not mock-specific)
and every consumer — including this file itself — now imports it from
there. What remains in the mock file (`generateConversions`,
`Conversion`, `PostbackDeliveryStatus`, the timeline generator) exists
only because the still-mocked Postback Logs feature
(`lib/mock/postback-logs.ts`) cross-references a fake conversion feed to
synthesize fake postback attempts.

## A real bug caught during manual browser verification: date-only `to` excluded same-day events

`GET /conversions?to=YYYY-MM-DD` parses `to` as midnight UTC of that date.
`internal/analytics`'s daily endpoints do the same thing safely, because
they query pre-aggregated `day`-granularity materialized views where a
date-only comparison is exactly right. This package queries raw
`event_at` timestamps — with `to` left at midnight, every event later that
same day was silently excluded from the query (`event_at <= to`
evaluating false for anything after 00:00:00). Caught live: seeding test
conversions "today" and passing an explicit `?to=<today>` returned zero
rows even though the default (no explicit range) correctly showed all
three. Fixed by pushing an explicit date-only `to` to the end of that day
(`+24h - 1ns`) in the handler before it reaches the service/repository.

## Verified

Backend: `go build/vet/gofmt/test ./...` all green — new
`chstore.ListConversions`/`CountConversions`/`ConversionsByClickID`/
`FunnelByClickID` integration tests against real ClickHouse, plus
`conversions.Service` unit tests (date-range/limit validation, funnel+
conversion chronological merge, the no-events-found 404) against a fake
in-memory repository.

Frontend: `tsc --noEmit`/`eslint` clean.

Full manual browser pass against the real running `api`+`web` dev
servers: seeded two click_ids' worth of real ClickHouse events (one full
funnel — SOURCE_CLICK → LAND_VIEW → PWA_INSTALL → CPA_HOLD → CPA_ACCEPT —
one bare SOURCE_CLICK → CPA_DECLINE) via a throwaway `chstore.EventStore
.InsertBatch` call (never committed) against a real test campaign/network
created through the real API. Confirmed: the list shows both click_ids
with real campaign/network names and the multi-row HOLD→ACCEPT status
history for click 1; the detail page for click 1 renders the full real
five-event timeline with correct icons/labels/revenue; the detail page
for click 2 renders its genuinely shorter two-event timeline (no
fabricated Landing/PWA steps); an unknown click_id renders a clean 404
error state, not a crash; the still-mocked Postback Logs and Event
Mapping panels (both of which depend on files this phase touched)
continued to render correctly. The date-range bug above was caught and
fixed during this same pass. Test campaign, network, and their ClickHouse
rows (via `ALTER TABLE ... DELETE`) removed afterward.

## Deliberately deferred

Postback Logs (incoming/outgoing delivery log, ClickHouse
`postback_events`, replay-capable per §45) and Event Mappings CRUD
(per-network raw-status → CPA-status config, Postgres `event_mappings`,
already migrated) — both scoped out when this phase's exact slice was
negotiated, both still fully mocked. See `docs/frontend-integration.md`.
