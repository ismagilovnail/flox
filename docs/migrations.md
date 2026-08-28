# Migration Strategy (§61, Phase 33)

Two stores, two different strategies — deliberately, not an oversight.
Schema *conventions* (ULID keys, tenant scoping, naming) already live in
`apps/api/migrations/README.md`; this doc is the rollout/rollback story
for a real deployment.

## Postgres: goose, forward *and* backward, run once before rollout

Every schema change is a numbered `goose` migration under
`apps/api/migrations/` (`apps/api/migrations/README.md` has the full
command reference). For a real deployment:

1. **Migrations run once, before any new binary starts** — never
   automatically inside `apps/api`/`apps/tracker`/`apps/worker` at boot.
   `infra/docker-compose.test.yml`'s `migrate` one-shot service is the
   reference shape: build the `migrate` target of `apps/api/Dockerfile`,
   run it to completion, only then start the three app services
   (`depends_on: migrate: condition: service_completed_successfully`).
   Running it automatically on every container boot would let N replicas
   of `apps/api` race to apply the same migration concurrently — goose has
   no built-in distributed lock for that.
2. **Backward-compatible migrations only, until the old binary is fully
   retired.** Adding a column, a new table, a new index: safe to run
   before the new code that uses it ships. Renaming or dropping a column
   a *currently running* binary still reads/writes is not — that binary
   errors mid-request the instant the migration commits. The safe
   sequence for a genuinely breaking schema change is expand → deploy code
   that writes both old and new shapes → backfill → deploy code that only
   uses the new shape → contract (drop the old column in a later
   migration). This project hasn't needed that sequence yet (no migration
   has renamed or dropped a live column), but the moment one does, this is
   the order.
3. **Rollback is `goose down` — a real, tested command
   (`apps/api/migrations/README.md`), not just a forward-only promise —
   but it only ever undoes schema, never restores data a forward migration
   destructively changed** (a dropped column's data is gone the moment
   that migration commits, `down` or not). This is exactly why step 2's
   expand/contract sequence matters: a migration that never destroys data
   in the first place needs no data-recovery step.
4. **A migration failing partway through is a `goose` transaction per
   file** (each file's `-- +goose StatementBegin`/`StatementEnd` blocks
   run inside one transaction by default) — it rolls back cleanly, the
   version table stays at the last successful migration, and rerunning
   `up` is safe. Nothing here needs a manual "what state is the DB in"
   investigation after a failed migration.

## ClickHouse: idempotent, automatic, forward-only, no rollback

`apps/internal/chstore.Migrate` runs automatically at startup inside both
`apps/api` (best-effort — a down ClickHouse degrades `/analytics`, not the
whole control plane) and `apps/worker` (required — worker cannot do its
job without it). Every statement is `CREATE TABLE IF NOT EXISTS` /
`CREATE MATERIALIZED VIEW IF NOT EXISTS` (`chstore.Migrate`'s own doc
comment) — safe to run concurrently from multiple processes, in any
order, any number of times. This is why ClickHouse gets no goose-style
one-shot `migrate` service in `docker-compose.test.yml`: there's nothing
for a separate step to coordinate.

**This means ClickHouse schema changes are additive-only in practice** —
there is no rollback mechanism and no destructive `ALTER` anywhere in
`chstore.Migrate`. A genuine ClickHouse schema change (a new column on an
existing table, a new materialized view) needs its own `ALTER TABLE ... IF
NOT EXISTS`-style statement added to `chstore.Migrate`, following the same
idempotent pattern — never a manual one-off `ALTER` run by hand against
production, which `chstore.Migrate` wouldn't know about and couldn't
reconcile on the next deploy.

## Zero-downtime deploy ordering

For a rolling deploy (old and new binary versions briefly coexisting):

```
1. Run Postgres migrations (goose up) — must be backward-compatible
   with the OLD binary still running, per the expand/contract rule above.
2. Roll apps/worker (it's the only binary ClickHouse schema changes are
   required for, and it has no inbound traffic to drain).
3. Roll apps/tracker and apps/api (any order — they don't call each
   other; both need ClickHouse's schema live, which step 2 guaranteed).
4. Roll apps/web last (it only ever talks to apps/api's stable HTTP
   contract, never the database directly).
```

## Reference implementation

`infra/docker-compose.test.yml`'s `migrate` service + `depends_on:
condition: service_completed_successfully` chain is a working example of
step 1 above, validated by actually running the full containerized stack
(`docker compose -f infra/docker-compose.test.yml up --build`) — see
`docs/deployment.md`.
