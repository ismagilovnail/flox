# Organization deletion (audit 2026-10-15, GDPR-1 part 2)

An owner can delete their organization and everything in it.
`apps/internal/orgdeletion`, migration 00090.

## Request and grace period

- `POST /organization/deletion {confirmName, password}`: only an active
  **Owner**. The organization's exact name and the owner's own password
  confirm it. The deletion is scheduled `GracePeriod` (7 days) ahead.
  Every active owner is emailed.
- `DELETE /organization/deletion`: any owner cancels until then. The
  owners are emailed.
- `GET /organization/deletion`: any member. `{scheduledAt, requestedAt,
  requestedBy}`, `scheduledAt` null when none.
- An API key cannot schedule or cancel (403): deleting an organization is
  a person's decision. Requests pass the usual `Origin` check.
- The organization keeps working during the grace period. Every page
  shows a banner with the date (owners get a cancel button), and the
  Settings → Organization tab has a "Delete organization" card with the
  confirmation dialog.

## Deletion (apps/worker, every 5 minutes)

1. **Postgres, when due.** One transaction per organization, row locked
   `FOR UPDATE SKIP LOCKED`:
   - writes the tombstone `organization_deletions`;
   - deletes the `organizations` row, from which all 55 tenant tables
     cascade;
   - deletes the users who belonged to it and to no other organization.

   A user still referenced from another organization's rows by a
   no-action foreign key (audit log, API key, cost entry — a member who
   left an organization that keeps those) stays as an account with no
   memberships, rather than failing the transaction.
2. **Everything else, `PurgeDelay` (1 hour) later.** The tracker serves a
   deleted organization's links until its routing cache drops them
   (NOTIFY, at most 30 s), and events already queued land after that, so
   the purge waits:
   - **ClickHouse:** lightweight `DELETE` from every MergeTree table with
     an `organization_id` column, read from `system.columns`, so a new table
     is covered without a code change;
   - **object storage:** `landings/{org}/`, `exports/{org}/` and `{org}/`
     (gallery);
   - **Redis:** `flox:conversion:last-status:{org}:*` (rate-limit counters
     expire in minutes on their own).

   A failing store leaves the tombstone unpurged with `last_error`, and
   the next run retries; every step is idempotent. A store the worker has
   no configuration for is skipped and named in `last_error` on the
   purged tombstone.

The tombstone keeps only the organization id, dates and the number of
users deleted — no name, no data.

## Not covered

- **Backups** keep deleted data for their own retention window
  (`docs/backup.md`); a restore brings it back.
- **External accounts** the organization connected (ad accounts,
  registrar, Cloudflare, Telegram bots): their stored tokens are deleted
  with the rows, but nothing is revoked at the provider.
- Data export: `docs/organization-export.md`; a visitor's request:
  `docs/visitor-data.md`.

## Tests

`internal/orgdeletion` (Postgres, ClickHouse, MinIO, Redis):
- scheduling: only an owner, exact name, right password, twice is a
  conflict, cancel;
- nothing happens before the date;
- when due: no Postgres table with `organization_id` keeps a row of the
  organization (the list comes from `information_schema`); its users go,
  a user also in another organization stays;
- the other stores wait for the delay, then are purged;
- a second organization's rows, objects and keys stay.

Mutation-checked: without the ClickHouse purge, or without the
other-membership check, it fails. Card and banner:
`organization-deletion-card.test.tsx`.
