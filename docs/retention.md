# Data retention (tech audit 2026-10-06, CH3/DB2/DB5)

Decided 2026-10-09: **only logs expire.** Everything a report, a
conversion or an invoice rests on is kept. Since 2026-10-16 one exception
inside the kept tables: visitors' **IP address and user agent** are
personal data and expire (audit GDPR-1, below); the rows stay. Since
2026-10-23 **raw clicks expire by plan** (audit DATA-6, below); the reports
on them are kept in an hourly rollup.

| Data | Where | Kept | Why |
|---|---|---|---|
| Postbacks recorded as conversions (`result = 'success'`) | Postgres `postbacks` | forever | Source of truth for revenue, dedup (CLAUDE.md #3) and the status progression |
| Postbacks not recorded (`duplicate`, `error`, `ignored`) | Postgres `postbacks` | `RETENTION_FAILED_POSTBACKS_DAYS`, default 90 | A log of what a network sent; nothing reads them back. Network replays make them most of the table. One waiting for its status mapping (`awaiting_mapping`) stays for the 30-day replay window whatever the setting (`docs/event-mappings.md`) |
| Pixel deliveries, sent or dead-lettered | Postgres `pixel_deliveries` | `RETENTION_PIXEL_DELIVERIES_DAYS`, default 30 | A send log carrying the visitor's IP and user agent the platform matched on (§54); the pixel page shows the last 100 |
| Postback attempt log (in and out) | ClickHouse `postback_events` | `RETENTION_POSTBACK_LOG_MONTHS`, default 13 | What the Postback Logs screen shows; a year plus a month covers a yearly dispute |
| Raw clicks and landing/PWA/Telegram events | ClickHouse `click_events`, `tracking_events` | the plan's `raw_retention_months`: Free 3, Start 6, Pro 12, Business 24 months (below) | Most of the stored bytes (docs/unit-economics.md); reports by the main dimensions outlive them in `event_rollup_hourly` |
| Click lookup | ClickHouse `click_lookup` | the plan's months, but at least 13 | A late postback (a repeat purchase months later) must still find its click, or it becomes `unknown_click` |
| Conversions, LTV, costs | ClickHouse `conversion_events`, `ltv_events`, `cost_events` | forever | Revenue and cohorts for any period; one row per conversion, small |
| Visitors' IP address, user agent and referrer (columns, not rows) | ClickHouse `click_events`, `tracking_events`, `click_lookup` | `RETENTION_IP_DAYS`, default 90 | Personal data (GDPR-1). Unique clicks count `visitor_key`, a keyed pseudonym that stays, so reports do not change. Pixels match a conversion to its ad click by IP + user agent: a conversion more than 90 days after its click goes to the ad platform without them (the platforms take events of the last 7 days anyway) |
| Daily aggregates | ClickHouse `*_daily_*` | forever | Small |
| Hourly rollup of clicks and tracking events | ClickHouse `event_rollup_hourly` | forever | What the report builder reads for old periods once raw clicks expire (audit DATA-6, below) |
| Expired sign-in state: sessions, pending second-factor logins | Postgres `sessions`, `pending_totp_logins` | until `expires_at` | Unusable once expired — every lookup requires `expires_at > now()` (DB5) |
| Expired invite and email-verification tokens | Postgres `memberships.invite_token_*`, `users.email_verification_token_*` | until expiry; then the token is cleared, the row kept | The invite still shows on the Team page and can be resent; the user is still the account |
| Failed sign-in counts | Postgres `login_failures` | 15 minutes (one window) | Only the current window counts (`docs/security.md` §3) |

Backups follow their own retention (`docs/backup.md`): what is deleted
here stays restorable for the backup window.

## How

- **Postgres** — `internal/retention`, in `apps/worker`, hourly: `DELETE`
  in batches of 5000 over the partial index
  `postbacks_unsuccessful_created_idx` (migration 00061), so each batch is
  a short transaction. `flox_retention_deleted_total{table="postbacks"}`.
  The table is **not partitioned** for this, although the audit suggested
  it: a unique index on a partitioned table must include the partition
  key, and the dedup key `(organization_id, click_id, status, event_ref)`
  must stay one global constraint.
- **Postgres, sign-in state** — `retention.ExpiredAuth`, in `apps/worker`,
  hourly, always on: batches of 5000 per table; expired sessions found by
  `sessions_expires_at_idx` (migration 00064).
  `flox_retention_deleted_total{table="sessions"|"pending_totp_logins"|
  "invite_tokens"|"email_verification_tokens"|"login_failures"}`.
- **ClickHouse** — `chstore.SetPostbackLogTTL`, run by `apps/worker` at
  start: `ALTER TABLE postback_events MODIFY TTL toDateTime(event_at) +
  INTERVAL n MONTH` when the configured period differs from the table's
  (`0` removes the TTL). A change rewrites the table's parts once; rows
  past the TTL go at the next merges.

All of them run across all organizations, like the queue reapers: maintenance,
not tenant data access.

## Changing it

A plan's raw-click period is a row: `UPDATE plans SET raw_retention_months`,
then the hourly pass applies it (shorter after the wait).

Raise or lower the variables and restart the worker. Lowering
`RETENTION_POSTBACK_LOG_MONTHS` or `RETENTION_FAILED_POSTBACKS_DAYS`
deletes the now-older rows within the hour (Postgres) or at the next
merges (ClickHouse) — irreversibly, except from backups.

## Raw clicks by plan (audit DATA-6, part 2)

- **Periods.** `plans.raw_retention_months` (migration 00099): Free 3,
  Start 6, Pro 12, Business 24 months.
- **The rows carry it.** `click_events`, `tracking_events` and `click_lookup`
  have `retention_months` (schema/024).
  - `apps/worker` writes it on every insert from the organization's months
    (`EventStore.WithRetention`, `retention.Plans.Of`, reloaded every
    minute).
  - Each table's row TTL deletes a row `retention_months` after its
    `event_at`, at the next merges.
  - `0` counts as 24 months: rows from before schema/024, or an insert path
    that does not know the plan. No row is kept forever by omission.
  - `click_lookup` keeps at least 13 months.
- **A plan change.** `retention.Plans` runs hourly in `apps/worker`
  (`organization_retention`: the months the rows carry, the plan's target,
  since when).
  - **Longer:** applies within the hour.
  - **Shorter (a downgrade, or a lapsed payment that moved the organization
    to Free):** waits `RETENTION_SHORTEN_AFTER_DAYS` (30) on the plan first,
    so a late renewal or a short downgrade deletes nothing.
  - Then new rows carry the new months, and once every worker's cache has
    them (5 minutes), the organization's existing rows are rewritten
    (`chstore.SetRawRetention`, an `ALTER TABLE … UPDATE` per table, waited
    for).
  - On the first run every organization starts at 24 months, what its rows
    meant before; a shorter plan waits the 30 days too.
- **Irreversible.** Rows past a shortened retention go at the next merges.
  Only backups bring them back.
- **What the operator sees.**
  - The Plan tab: the click history of every plan; the organization's months
    now; and, while a shorter one waits, the date it applies.
  - The report builder and the click log: a note when the range starts
    before the cutoff (`RawRetentionNotice`).
- **Tests:**
  - `TestRawRetentionFollowsThePlan`: the wait, the immediate upgrade, the
    rewrite of all three tables, and new rows carrying the months;
  - `TestRawRetentionTTLDeletesOldRows`: a 4-month-old click of a 3-month
    plan is gone after a merge, one without retention stays, and
    `click_lookup` keeps it.

## Rollup (audit DATA-6, part 1)

`event_rollup_hourly` (schema/023) counts every click (`SOURCE_CLICK`,
`SOURCE_FILTER`) and every tracking event per organization, UTC hour, type
and report dimension.

- **Dimensions:** campaign, traffic source, stream set, flow, landing, offer,
  PWA, postlanding, network, Telegram channel, country, device, platform, OS,
  browser, language.
- **Measures:** events, bots, clicks with no subs, and the set of unique
  visitors (`uniqExactState` of `chstore.uniqueVisitorExpr`, so unique clicks
  merge exactly across hours).
- **How it fills.** Two materialized views count events from
  `event_rollup_cutover` on (the hour after schema/023 first ran).
  `chstore.BackfillRollup`, in `apps/worker`, copies everything before the
  cutover once, an hour after it, so nothing is counted twice. It copies
  only if the rollup has nothing from before the cutover yet, and records
  itself in `chstore_backfills`. An event dated before the cutover that
  arrives after the copy is in no rollup. In production that can only be an
  event still on its way at the switch (the hour's wait covers it): the
  tracker dates events when they happen, and retention changes rows with
  mutations, which the views do not see.
- **Tests (BUG-9).** Tests seed events at fixed past dates, before the
  shared dev database's cutover, so once its backfill is done a plain
  `NewEventStore` would read the rollup and find nothing. chstore's tests
  use `NewRawEventStore` (export_test.go); only `rollup_test.go` reads the
  rollup, with `ForceRollup` and its own `RunRollupBackfill`. A test outside
  chstore that seeds past events and reads a report by rollup dimensions
  over whole hours has the same trap: date its events after the cutover or
  give it a raw store.
- **Who reads it.** The report builder's clicks, unique clicks, blocked,
  bots, empty subs, landing views, PWA installs and Telegram starts/joins,
  and the filter-value list. They read it when every dimension and filter of
  the query is in it, the range starts and ends on a whole UTC hour, and the
  backfill is done; otherwise they read the raw rows (`EventStore.useRollup`).
  `TestRollupReportsMatchRawRows` checks the two answer the same, history and
  live, by day in Moscow and New York time.
- **Not in it, so these end with the raw rows:**
  - region and city;
  - utm and subs as dimensions;
  - click ids and the click log;
  - the funnel and the flow summary, which count distinct click ids per
    stage (a landing reloaded three times is one view there);
  - conversions by OS, browser or language, which join their click;
  - a day in a half-hour time zone (India, Nepal), whose bounds are not whole
    UTC hours.
- **Deletion.** Organization deletion empties the rollup with every other
  table that has `organization_id` (`orgdeletion.purgeClickHouse`). A
  visitor's data request (`docs/visitor-data.md`) removes the visitor's raw
  rows only. The rollup keeps the counts, and the visitor's keyed pseudonym
  survives inside the unique-visitor state as a hash, linked to no other
  data.

## Visitors' IP and user agent (audit 2026-10-15, GDPR-1)

- **Pseudonym.** `apps/tracker` puts `visitor_key` on every click
  (schema/020): HMAC-SHA256 of IP + user agent with `VISITOR_KEY`
  (`internal/visitorkey`), 16 bytes in hex. Unique clicks count
  `campaign|visitor_key` (`chstore.uniqueVisitorExpr`); a click without one
  falls back to raw IP + user agent, and one with neither counts as its own
  visitor. Keyed, because the IPv4 space times the common user agents is
  small enough to reverse a plain hash. The key is the same on every
  tracker replica and in the worker, and is never rotated: a new key makes
  every returning visitor a new one.
- **Backfill.** Clicks from before schema/020 get their pseudonym once,
  from `apps/worker` (`chstore.BackfillVisitorKeys`): distinct IP + user
  agent pairs, keyed in Go, written by `transform()` mutations of 2000 pairs.
  Without `VISITOR_KEY` it is skipped and not recorded, so it runs once a
  key is set.
- **TTL.** After the backfill, the worker sets a column TTL on `ip` and
  `user_agent` in the three tables (`chstore.SetPersonalDataTTL`): an
  expired value becomes `''` when ClickHouse merges the part, so it can
  outlive its day by hours. The order matters: a click whose IP went
  before it got its pseudonym would count as its own unique visitor.
  `0` removes the TTL.
- **`referrer`** expires with them (since 2026-10-16): it can carry the
  referring page's query string. `click_lookup` has none.
- **Elsewhere:** organization deletion (`docs/organization-deletion.md`),
  a visitor's data request (`docs/visitor-data.md`). Not covered: the
  operator audit trail's IPs. Pixel deliveries already expire
  (`RETENTION_PIXEL_DELIVERIES_DAYS`).

