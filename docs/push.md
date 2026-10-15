# Push (Web Push from installed PWAs)

Gap 5 of `docs/spec-conformance-2026-10-11.md`: the "Push" section of the
sidebar (§17) and the NOTIFICATION_* events (§43). Scope chosen by the
owner on 2026-10-13: subscriptions and broadcasts sent now. Added
2026-10-14: scheduled broadcasts and push metrics (Push → Statistics,
custom metrics on the `push_stats` surface), then triggered broadcasts
("N hours after subscribing, if no deposit") and recurring broadcasts
("Mon/Wed/Fri at 19:00"), then push metrics as columns of the report
builder (2026-10-15, `docs/analytics-report-api.md` "Push metrics").
Not done: multi-step sequences in one trigger, an interval mode ("every
N hours").

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
| `POST /push/broadcasts/{id}/pause`, `/resume` | stop / restart a trigger or recurring broadcast (`push.write`); anything else, finished, or already so → 409 |
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

### Triggered broadcasts (migration 00074)

`trigger: {delayMinutes, condition, statuses}` (not with `sendAt`) makes a
broadcast `kind = 'triggered'`, `status = 'active'`. Owner's scope
(2026-10-14): a delay after subscription plus an optional condition; new
subscribers only; one message per trigger — a sequence is several triggers
with different delays.

- **Who:** active subscriptions matching the filters, created **after** the
  trigger was (current subscribers are never included — no surprise wave
  on start), once `created_at + delay` has passed.
- **Delay:** 5 minutes to 30 days. Measured from the subscription's first
  `created_at`; a re-subscribe does not restart it.
- **Condition:** `none`, or `with` / `without` one of the chosen CPA
  statuses, looked up at send time in `postbacks` (`result = 'success'`,
  by `click_id` **or** `attributed_click_id` = the subscription's click,
  same organization). "Without CPA_ACCEPT, CPA_REDEP" reaches who has not
  deposited yet.
- **Window:** a subscriber is sent to only within 24 h
  (`push.TriggerWindow`) of their due time. Resuming a trigger paused for
  a week, or a worker outage, does not send week-old "1 hour after"
  messages; due within the last 24 h still go. The window also keeps the
  once-per-subscriber check (the deliveries' unique (broadcast,
  subscription)) within the delivery rows' retention.
- **Sending:** the same `RunScheduler` tick (15 s) calls `SendTriggered`:
  per active trigger, one transaction holds the trigger row
  (`FOR UPDATE SKIP LOCKED`) and one statement picks the due subscribers
  (≤ 1000 per tick), queues their deliveries, adds to the trigger's
  `total` and the stats. Deliveries then go out like any other.
- **UI:** New broadcast → When: Trigger (delay + unit, condition,
  statuses; confirm "each new subscriber gets it 1 h after subscribing ·
  if no CPA_ACCEPT"). The list shows Active/Paused with that description
  and a pause/resume button; Audience is what it has sent so far.

Deploy note: 00074 changes `push_broadcast_stats`' key. Code from before
it cannot record finished deliveries or opens against it, so roll the
worker, api and tracker together with the migration.

### Recurring broadcasts (migration 00075)

`recurrence: {weekdays, time, until}` (not with `sendAt` or a trigger)
makes a **template**, `kind = 'recurring'`. Owner's scope (2026-10-14):
weekdays + a time of day, an optional end date, the template plus each
send shown.

- **Schedule:** ISO weekdays (1 = Monday … 7, all seven = every day) at
  `HH:MM` in the organization's timezone **as it was when the template was
  made** (`recur_timezone`; the request's own value is ignored), until
  `until` (a date in that zone, inclusive) or for good.
  `push.NextOccurrence` computes slots: a time in the spring DST gap moves
  forward, one in the autumn overlap is its first occurrence.
- **Sending:** `RunScheduler` calls `SendRecurring` each tick: one due
  template at a time, in a transaction holding its row
  (`FOR UPDATE SKIP LOCKED`) that creates the send — an ordinary sent
  broadcast `"<name> · YYYY-MM-DD"` with `parent_id` = the template, its
  audience fixed then exactly like a send-now — and moves `next_send_at` to
  the next slot after now. A slot goes out once however many workers run.
- **Skipped, not late:** a slot reached more than an hour late
  (`push.RecurringLateness`, a worker outage) is skipped. A slot whose
  filters match no one creates nothing.
- **End:** past `until` the template is `finished` (`next_send_at` null)
  and cannot be resumed. Pause stops it; resume continues from the next
  slot after now (finished if there is none).
- **Counters:** a template's own are 0; the API returns its sends' sums
  and `occurrences`. Each send is an ordinary row (repeat icon in the list)
  with its own numbers, so Statistics by broadcast compares the days.
- **UI:** New broadcast → When: Repeat (weekday toggles, time in the
  organization's zone, optional end date; confirm "goes out Mon, Wed, Fri ·
  19:00 (UTC)…"). The list shows Active/Paused/Finished, the schedule, the
  next send and the number of sends, with pause/resume.

## Push metrics (Push → Statistics)

`push_broadcast_stats` holds each broadcast's counters per 15-minute
bucket (when the deliveries were queued, migration 00074) × PWA × campaign ×
country: `sent` at queue time, `delivered`/`failed`/`unsubscribed` with each
finished delivery (`pushsend`), `opened` with the first open of a delivery.
Each delivery row carries its subscriber's PWA, campaign and country as they
were when sent. The table outlives the delivery rows (retention) and always
adds up to the broadcast's own counters. Broadcasts sent before migration
00073 have one row of their totals with empty dimensions ("Not split").

`GET /push/stats` groups by `day` (in `timezone`), `broadcast`, `pwa`,
`campaign` or `country`, over deliveries **queued** in the range: a
one-off on its send day, a trigger spread over the days it sent; later
opens stay with their delivery; ≤ 366 days. 15-minute buckets cut days
right in every real UTC offset (+05:45 included). Each row's metrics are keyed as the custom-metric catalog names them
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

## Verified (2026-10-14, recurring)

- Tests: `TestNextOccurrence` (later today, at the slot, weekday sets,
  Sunday = 7, until inclusive / past, autumn DST in Berlin, the spring
  gap); `recurring_test.go` — not before the slot; a template held by
  another worker is skipped at once; two workers send a slot once; the
  send's name, audience and parent; template sums and next slot; a slot two
  hours late skipped; empty audience creates nothing; pause / resume (next
  slot from now), cross-tenant 404, a send cannot be paused; until →
  finished, not resumable; the organization's zone wins over the request's;
  validation. Mutation-checked: dropping the lateness check, the end-date
  finish or `SKIP LOCKED` each fails a test (the held-row case was added
  because the two-goroutine race alone did not catch the missing lock).
  The isolation guard required `parentId` to be declared response-only.
- Live: a template due two minutes ahead, filtered to one test
  subscription → the running worker sent it at 11:50:14, the template moved
  to the next day and showed 1 send. In Chrome: the list (template row,
  send with the repeat icon, pause → "На паузе"), the form (weekday
  toggles, "pick a day" error, confirm text). Test rows removed.

## Verified (2026-10-14, triggers)

- Tests: `internal/push` `triggers_test.go` — not before due; old
  subscribers never; once per subscriber; the 24 h window; pause (nothing
  sent) / resume (due meanwhile sent), 409s, cross-tenant 404; stats by
  day split by when deliveries went out; the condition by click and by
  attributed click, success only, other organization's conversion
  ignored; validation. Mutation-checked: dropping the window, the
  "created after the trigger" rule or the attributed-click lookup each
  fails a test. `trigger-format.test.ts`. The worker sends
  (`SendDue`, `SendTriggered`) are scoped to the test's organization
  (`export_test.go`) — before, a `SendDue` test with a "later now" could
  have sent the shared dev database's due broadcasts into its private
  table.
- Live: a trigger "5 min, without CPA_ACCEPT" through the API, two
  backdated test subscriptions (one on a click with a CPA_ACCEPT) → the
  running worker queued only the other one on its next tick; pause, pause
  again (409), resume; stats in Asia/Kathmandu. In Chrome: the form (inline
  "pick a status" error, confirm text), the list (Active → Paused), and
  Push → Statistics (the 30.20 browser pass). Test rows removed.

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
