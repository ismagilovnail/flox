# migrations

PostgreSQL schema migrations, managed with [goose](https://github.com/pressly/goose)
(vendored as a Go tool dependency — run via `go tool goose`, no separate
install needed).

Run against local dev Postgres (`docker compose -f infra/docker-compose.dev.yml up -d postgres`):

```
cd apps/api
go tool goose -dir migrations postgres "$DATABASE_URL" up
go tool goose -dir migrations postgres "$DATABASE_URL" status
go tool goose -dir migrations postgres "$DATABASE_URL" down   # one step back
```

`$DATABASE_URL` defaults to `postgres://flox:flox@localhost:5432/flox?sslmode=disable`
per `.env.example`.

## Conventions (§35, §36-TENANCY)

- **ULID primary keys everywhere** — the `ulid` domain (a `text` column with
  a Crockford-base32 format check) is created in `00001` and used by every
  table after it. One standard, not "UUID or ULID."
- **`organization_id NOT NULL` on every tenant-scoped table**, including
  child tables (`offer_links`, `filter_conditions`, …) — denormalized onto
  the child rather than left implicit via a join through the parent, so a
  repository query can filter on it directly and a forgotten join can't
  leak data across tenants.
- **`created_at`/`updated_at`** on every mutable table; `updated_at` is
  maintained by a trigger (`set_updated_at()`, defined in `00001`), not
  application code, so it can't go stale from a forgotten `SET updated_at = now()`.
- `roles`/`permissions`/`fx_rates` are the only non-tenant-scoped tables —
  roles are a fixed platform-wide set (not customized per org today),
  permissions are a catalog, fx_rates are an objective market fact shared
  by every tenant.

Global lookup tables (`organizations`, `users`, `roles`, `permissions`,
`traffic_sources`, `campaigns`, `stream_sets`, `filter_groups`,
`filter_conditions`, `flows`, `landings`, `pwas`, `postlandings`,
`networks`, `offers`, `offer_links`, `domains`, `tracking_links`, `pixels`,
`stream_set_pixels`, `postbacks`, `cost_entries`, `fx_rates`, `api_keys`,
`audit_logs`, `memberships`) match §35's table list exactly. No tables for
Tags/Custom Metrics/Report Presets/Referral/Content Gallery yet — those are
the v3 "secondary" frontend phases (14.5–14.9), not in §35's core list;
their backend tables land whenever the spec actually schedules them, not
guessed at here.
