# Data retention (tech audit 2026-10-06, CH3/DB2/DB5)

Decided 2026-10-09: **only logs expire.** Everything a report, a
conversion or an invoice rests on is kept.

| Data | Where | Kept | Why |
|---|---|---|---|
| Postbacks recorded as conversions (`result = 'success'`) | Postgres `postbacks` | forever | Source of truth for revenue, dedup (CLAUDE.md #3) and the status progression |
| Postbacks not recorded (`duplicate`, `error`, `ignored`) | Postgres `postbacks` | `RETENTION_FAILED_POSTBACKS_DAYS`, default 90 | A log of what a network sent; nothing reads them back. Network replays make them most of the table |
| Postback attempt log (in and out) | ClickHouse `postback_events` | `RETENTION_POSTBACK_LOG_MONTHS`, default 13 | What the Postback Logs screen shows; a year plus a month covers a yearly dispute |
| Clicks, landing/PWA events, conversions, LTV, click lookup, costs | ClickHouse `click_events`, `tracking_events`, `conversion_events`, `ltv_events`, `click_lookup`, `cost_events` | forever | Every report reads them for any period; a TTL would empty old reports without a word, and a conversion on a click older than the TTL would become `unknown_click` |
| Daily aggregates | ClickHouse `*_daily_*` | forever | Small |
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

Raise or lower the variables and restart the worker. Lowering
`RETENTION_POSTBACK_LOG_MONTHS` or `RETENTION_FAILED_POSTBACKS_DAYS`
deletes the now-older rows within the hour (Postgres) or at the next
merges (ClickHouse) — irreversibly, except from backups.

Expiring the report tables is a separate decision: reports older than
the TTL would need to come from the daily aggregates, which today only
the campaign screen reads.
