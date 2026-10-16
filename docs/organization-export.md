# Organization data export (audit 2026-10-15, GDPR-1 part 4)

An owner or admin downloads everything of the organization as a ZIP.
Settings → Organization → Export data. `apps/internal/orgexport`,
migration 00091.

## Request and download

- `POST /organization/exports {from, to}` (`settings.write`): queues an
  export. The events of the period are included, up to 366 days and not
  in the future. One at a time per organization (409).
- `GET /organization/exports`: the latest 20 with status, size and expiry.
  The card refetches every 5 s while one is being built.
- `GET /organization/exports/{id}/download` (`settings.write`): streams
  the archive. Another organization's export, one not finished, or one
  expired: 404.

## The archive (apps/worker, every minute)

- `postgres/<table>.jsonl`:
  - every tenant table (every table with `organization_id`, read from the
    schema, so a new table is included without a change), one JSON object
    per line;
  - **minus** the columns `SecretColumn` matches — `secret`, `token`,
    `password`, `hash`, `encrypt`, `p256dh`, `auth`, `endpoint`,
    `private_key`: credentials, tokens and their hashes, encrypted
    payloads, push keys and endpoints;
  - **minus** `ExcludedTables`: sessions and pending sign-ins, internal
    queues, and `organization_exports` itself (its object keys keep the
    archives private).
- `team.jsonl`: members — name, email, role, status. No password hashes.
- `clickhouse/<table>.csv`: `click_events`, `tracking_events`,
  `conversion_events`, `postback_events`, `cost_events` of the period,
  every column as ClickHouse prints it.
- `README.txt`: what is inside, and per table the columns left out.

Uploaded files (landings, gallery) are not included: they are downloaded
where they are. Visitors' IP, user agent and referrer are empty on events
older than `RETENTION_IP_DAYS`.

## Keeping it private

The bucket serves any object whose key is known (`storage`'s public-read
policy, for gallery files embedded in pages). So an archive's key is
`exports/{org}/{export id}-{32 random bytes, hex}.zip`. It never leaves
the server: not in API responses, not in the email. The download streams
through the API (`storage.Client.Open`) after the permission check.

After `Retention` (7 days) the worker deletes the object and marks the
export expired. An export stuck in "running" for 2 hours (a worker that
stopped) is failed so the organization can ask again. Organization
deletion purges `exports/{org}/` too.

The requester is emailed when the archive is ready. The worker needs
`SMTP_URL` / `MAIL_FROM` (in the dev compose since 2026-10-16); without
them there is no email.

## Tests

`orgexport_test.go` (Postgres, ClickHouse, MinIO):
- validation (reversed period, over a year), one at a time, no download
  before it is built or by another organization;
- the archive has records, team, the period's clicks and not older ones,
  the README;
- **no planted secret value** (network secret hash, API key hash, pixel
  token, password hash) appears anywhere, no exported column matches
  `SecretColumn`, nothing of another organization, no excluded table;
- expiry deletes it.

`TestSecretColumnCoversKnownSecrets` pins the secret columns the schema
has today. Mutation-checked: without the column exclusion the planted
secrets show up and the test fails. The cross-tenant route test downloads
another organization's export (404).
