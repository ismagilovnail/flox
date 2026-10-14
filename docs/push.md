# Push (Web Push from installed PWAs)

Gap 5 of `docs/spec-conformance-2026-10-11.md`: the "Push" section of the
sidebar (§17) and the NOTIFICATION_* events (§43). Scope chosen by the
owner on 2026-10-13: subscriptions and broadcasts sent now. Added
2026-10-14: scheduled broadcasts and push metrics (Push → Statistics,
custom metrics on the `push_stats` surface). Not done: recurring or
triggered broadcasts ("N hours after install"), push metrics in the
report builder (one formula = one data source, §30.5: push metrics are not
on the regular rows).

## Subscribing (apps/tracker)

A PWA with **Ask for notifications** on (`pwas.push_enabled`, migration
00070) asks on launch. The installed app's `start_url` (`/pwa/{t}/open`,
`docs/pwa-serving.md`) records PWA_OPEN and, instead of redirecting at once,
serves a short launch page (`apps/tracker/templates/launch.html`):

- permission already **granted** → subscribe silently (or re-send the
  existing subscription if this browser has not stored it yet), continue;
- **denied**, or no Push API → continue at once;
- **default** → a card: "Allow notifications" / "Not now". The browser's
  own prompt needs a tap (Safari requires one, Chrome quiets prompts
  without one), so the card asks first. "Allow" records
  **NOTIFICATION_REQUEST**, then either subscribes or records
  **NOTIFICATION_DECLINE**; "Not now" records nothing and asks again on the
  next launch.

"Continue" is `/pwa/{t}/go`: the next stage of the click's journey
(postlanding or destination, `continueOnward`). A 15-second guard moves on
if a browser never settles the permission or the subscription — only after
the visitor chose; the card itself waits for them (found in the live test:
an unconditional timer took visitors away mid-read).

`POST /pwa/{t}/push/subscribe` stores the subscription
(`push_subscriptions`) bound to the token's click: organization, PWA,
campaign, click id, the click's attribution as JSON (`context`), the signed
link (`link_token`) and the tracker origin it subscribed on. A new one
records **NOTIFICATION_SUBSCRIBE**; the same endpoint again refreshes it.

The endpoint is a URL from the browser that the worker will POST to, so it
is checked when stored — https, no port or credentials, a known push
service host (FCM, Mozilla, Apple, Windows; `push.ValidEndpoint`) — and
dialed through `urlsafety.SafeDialContext` when sent (§54). Keys must be a
65-byte P-256 point and a 16-byte secret. Constructor test links never ask
or subscribe.

Needs `PUSH_VAPID_PUBLIC_KEY` on the tracker; unset, no app asks
(`docs/environment.md`).

## Broadcasts (apps/api, internal/push)

`/push` (permission `push.read` for everyone; `push.write` for Owner,
Admin, Manager — migration 00070):

| Route | Does |
|---|---|
| `GET /push/summary` | active/unsubscribed counts, active by PWA/campaign/country; `configured` (VAPID keys present) |
| `GET /push/subscriptions` | the subscriber list (filters `status`, `pwaId`, `campaignId`, `country`); the endpoint itself is never returned, only its host |
| `POST /push/audience` | how many active subscribers filters match now |
| `GET /push/broadcasts`, `/push/broadcasts/{id}` | broadcasts with their counters |
| `POST /push/broadcasts` | send now, or schedule with `sendAt` (`push.write`) |
| `POST /push/broadcasts/{id}/cancel` | call off a scheduled broadcast (`push.write`); sent or canceled → 409 |
| `GET /push/stats?from=&to=&timezone=&groupBy=` | push metrics, see below |

A broadcast: name, title (≤ 80), text (≤ 240), optional https icon (empty:
the PWA's) and image, a click target, and filters (PWAs, campaigns,
countries; empty = everyone active). The click target is either the
**subscriber's offer** — the destination their install click was routed
to, through `continueOnward`, so conversions stay on that click — or **a
URL** with the click's macros resolved (`{click_id}`, `{sub1}`…). The
audience is fixed when it is sent: one `push_deliveries` row per matching
subscriber, in the same transaction as the broadcast. No subscriber
matching is a validation error; without VAPID keys, 503.

The UI (Push → Broadcasts → New broadcast) counts the audience live, shows
a notification preview and asks to confirm "send to N subscribers"; the
list refreshes every 15 s.

### Scheduled broadcasts (migration 00073)

`sendAt` (RFC 3339) a minute to 30 days ahead stores the broadcast as
`status = 'scheduled'` with no deliveries. The UI takes a wall-clock time
in the organization's timezone (`zonedLocalToUtc`, DST-aware). The audience
is **not** fixed at scheduling: it may be empty today, and subscribers who
join before the send time get it too. `apps/worker` runs
`push.RunScheduler` every 15 s: `SendDue` claims each due broadcast with
`FOR UPDATE SKIP LOCKED` (safe with several workers), queues its deliveries
exactly as a send-now does and marks it `sent` with `sent_at`, in one
transaction — sent once, however many workers or ticks see it. It runs
without VAPID keys too; the deliveries then wait in the queue. A scheduled
broadcast can be canceled until then (`canceled`, never sent).

## Push metrics (Push → Statistics)

`push_broadcast_stats` holds each broadcast's counters per PWA × campaign ×
country: `sent` at queue time, `delivered`/`failed`/`unsubscribed` with each
finished delivery (`pushsend`), `opened` with the first open of a delivery.
Each delivery row carries its subscriber's PWA, campaign and country as they
were when sent. The table outlives the delivery rows (retention) and always
adds up to the broadcast's own counters. Broadcasts sent before migration
00073 have one row of their totals with empty dimensions ("Not split").

`GET /push/stats` groups by `day` (send day in `timezone`), `broadcast`,
`pwa`, `campaign` or `country`, over broadcasts **sent** in the range (a
broadcast counts on its send day, its later opens stay with it; ≤ 366
days). Each row's metrics are keyed as the custom-metric catalog names them
— `push_sent`, `push_delivered`, `push_opened`, `push_ctr` (opened ÷
delivered × 100, null with nothing delivered) — plus `push_failed`,
`push_unsubscribed`. So the registry's Push category is live: a custom
metric built from push ids can target **Push statistics** (`push_stats`)
and is evaluated per row on that tab; the two registries' surface lists
are kept equal by `TestSurfacesMatchTheFrontend`.

## Sending (apps/worker, internal/pushsend)

Same queue mechanics as pixel deliveries: fair claim across organizations
(`internal/pgqueue`), 20 sends at a time, the reaper returns abandoned
claims, `queuestats` reports depth and age. Sender: `pushsend.WebPush`
(github.com/SherClockHolmes/webpush-go — RFC 8291 encryption, RFC 8292
VAPID; §74 behind the `Sender` interface), TTL 24 h. The payload is
`{title, body, icon, image, url}` with `url` =
`{origin}/pwa/{link_token}/push/click/{delivery id}`; the PWA's service
worker (`sw.js`) shows it and opens the URL on click.

| Push service answer | Outcome |
|---|---|
| 2xx | delivered |
| 404 / 410 | the subscription is gone: unsubscribed, **NOTIFICATION_UNSUBSCRIBE** on its click (once), its other pending deliveries dropped |
| no answer, 429, 5xx | retried on the postback schedule, at most 4 attempts, then failed |
| other 4xx | failed at once |

The broadcast's counters (delivered, failed, unsubscribed, opened) are
updated with each finished delivery, so they outlive the delivery rows,
which are deleted after `RETENTION_PUSH_DELIVERIES_DAYS` (default 30).
`flox_push_deliveries_total{outcome}`.

## Opens (apps/tracker)

`GET /pwa/{t}/push/click/{delivery}` (any token age — the app may have been
installed long ago): records **NOTIFICATION_CLICK** on every click, counts
the broadcast's open once per delivery — only for a delivery sent to this
click's subscription in this organization — and redirects to the target.
A delivery that is not this click's, or no longer kept, still leads on.

## Verified (2026-10-14, scheduling and metrics)

- Tests: `internal/push` `schedule_stats_test.go` (nothing queued before
  the send time; sent at the due time to the audience then, with
  dimensions and group counts; sent once; cancel incl. cross-tenant 404 and
  409 twice / after send; sendAt bounds; stats by country/broadcast/day,
  other organization and scheduled ones left out, opens counted once,
  validation), `pushsend` (group counts add up to the broadcast's),
  `custommetrics` (surface lists equal on both sides), `timezones.test.ts`.
- Live on the dev stack (API, no browser — the Chrome extension was not
  connected): schedule → cancel → 409; a broadcast due in 70 s sent by the
  running worker on its next tick to 1 subscriber; `/push/stats` by country
  over the pre-migration broadcast ("Not split" row, CTR 75%); bad
  `groupBy`/range → 422.

## Verified (2026-10-13)

- Tests: `internal/push` (endpoint and key checks, subscriptions incl.
  cross-tenant, audience filters, broadcast queueing, opens counted once and
  only for the right click and organization, validation, 503 unconfigured),
  `internal/pushsend` (classification; the real WebPush sender against a
  local server — aes128gcm, VAPID header, TTL, payload not in the clear;
  the deliverer: delivered/gone/refused, counters, unsubscribe event,
  retries then failure), `tracker/push_test.go` (launch page, events,
  subscribe once, bad endpoints refused, not asked without key or switch,
  click → URL with macros, open counted once), the route RBAC and tenant
  isolation walks, the seeded role policy.
- Live, in Chrome on the dev stack: the PWA switch saved; a click through
  the demo campaign → install page → launch page → "Allow" → Chrome's
  prompt accepted by the owner → a real FCM subscription stored; a
  broadcast sent from the UI → FCM answered 201 → the notification showed
  on the desktop → clicked → the open counted (CTR 100%), and ClickHouse
  holds PWA_VIEW, PWA_OPEN, NOTIFICATION_REQUEST, NOTIFICATION_SUBSCRIBE,
  NOTIFICATION_CLICK on that one click.
