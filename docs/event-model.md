# Event Model

Authoritative list — §43 of the master spec. Do not truncate: the ClickHouse
schema (Phase 26) must accommodate all of these from day one, since adding
event types later is a migration on live analytics data. Modeled as one
typed enum shared across tracker, worker, analytics, and pixels.

```
TRAFFIC
  SOURCE_CLICK            click on the tracking link
  SOURCE_FILTER           click blocked by campaign filters (bot/geo/device/…)

LANDING
  LAND_VIEW
  LAND_CLICK
  POSTLANDING_VIEW
  POSTLANDING_CLICK

PWA
  PWA_VIEW
  PWA_OPEN
  PWA_INSTALL
  IOS_INSTALL

PUSH
  NOTIFICATION_REQUEST
  NOTIFICATION_SUBSCRIBE
  NOTIFICATION_DECLINE
  NOTIFICATION_UNSUBSCRIBE
  NOTIFICATION_CLICK

TELEGRAM
  TG_JOIN
  TG_START

CPA CONVERSIONS (status is an enum, NOT a single "conversion" type)
  CPA_HOLD      registration
  CPA_ACCEPT    first deposit / FTD (key conversion)
  CPA_REDEP     re-deposit (drives LTV)
  CPA_DECLINE   rejected
  CPA_TRASH     junk / duplicate
```

## Canonical funnel (PWA + gambling)

```
SOURCE_CLICK → LAND_VIEW → LAND_CLICK → PWA_VIEW → PWA_INSTALL
            → CPA_HOLD → CPA_ACCEPT → CPA_REDEP
```

All events for a user chain are linked by a single `click_id`, preserved
across returns when `stickyFlowKeepClickId` is enabled (see
[`routing.md`](routing.md)).

## Deduplication (postback / conversion events)

`DEDUP KEY = (click_id, status)`, not `click_id` alone — the same click
legitimately produces `CPA_HOLD`, then `CPA_ACCEPT`, then multiple
`CPA_REDEP`. Redeposits are distinguished by an additional event identifier
(network txn id if provided, else a monotonic sequence): N distinct
redeposits are N events, a re-sent identical one is dropped. Dedup key held
in Redis with a long TTL (fast path); a durable unique constraint in
ClickHouse/PG is the backstop. Per-network `acceptDuplicates` override
exists for partners whose semantics require accepting duplicates
intentionally. Full detail: master spec §45.

## Currency

Revenue is stored in both original currency and USD, normalized using the FX
rate at the **event date**, never the current rate (§50-FX).

## Unfilled subs (FB/IG)

A portion of Facebook/Instagram clicks arrive with empty sub parameters
depending on how the link was opened (in-app WebView vs external browser,
prefetch, redirect chains). Missing subs are persisted as empty, never
inferred as "unknown campaign" — exposed in analytics as a diagnostic (e.g.
"empty subs %"), not silently miscounted. Full detail: master spec §42 note.
