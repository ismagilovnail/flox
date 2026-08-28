# Backup Strategy (§61, Phase 33)

Three stores, three different loss profiles, so three different backup
approaches — not a single "back up everything the same way" script.

| Store | What's lost if it's gone | Approach |
|---|---|---|
| Postgres | Every org, campaign, routing config, session, cost entry — the entire control plane. Nothing else has this data. | Full logical dumps + WAL archiving for point-in-time recovery |
| ClickHouse | Every click/conversion/postback event and every analytics/LTV number derived from them. Large, append-mostly, individually less critical per row than a control-plane row. | Periodic table snapshots |
| Object storage (S3/MinIO) | Nothing today — `apps/internal/config.S3Config` exists but no code path writes to it yet (`.env.example`'s own comment: "not consumed yet"). | Bucket versioning, once something actually stores files there |

Every command below was run against the real local dev stack
(`infra/docker-compose.dev.yml`) while writing this doc, not copied from
memory.

## Postgres

**Logical backup** (works from any Postgres client, restorable into any
compatible version — the safe default until a specific deployment's scale
demands `pg_basebackup`):

```bash
docker exec infra-postgres-1 pg_dump -U flox -d flox --format=custom \
  --file=/tmp/flox_$(date +%Y%m%d_%H%M%S).dump
docker cp infra-postgres-1:/tmp/flox_<timestamp>.dump ./backups/
```

(Outside Docker: `pg_dump "$DATABASE_URL" --format=custom --file=...` —
identical, `--format=custom` is what makes it `pg_restore`-only and
compressed, not a plain-text `.sql` you'd want for a large database.)

**Restore**, into an empty database (schema included — no need to run
migrations first):

```bash
pg_restore --clean --if-exists --no-owner -d "$DATABASE_URL" flox_<timestamp>.dump
```

**Point-in-time recovery**: a nightly `pg_dump` alone loses everything
since the last dump if Postgres crashes mid-day. A real production
deployment should also archive WAL segments (`archive_mode = on`,
`archive_command` pointed at object storage) so a restore can replay up to
the moment of failure, not just the last snapshot. This project's dev
Postgres doesn't run with WAL archiving on (no production traffic to lose)
— enabling it is a deployment-time Postgres config change, not application
code.

**Cadence**: nightly full `pg_dump`, retained 30 days; WAL archived
continuously if enabled. Postgres holds the control plane, not
high-volume event data — a daily cadence is cheap here specifically
because this database stays small (campaigns/routing config/sessions, not
per-click rows).

## ClickHouse

Native `Native` format round-trips exactly (schema included per row, no
separate `CREATE TABLE` needed on restore into a matching schema) and
compresses well:

```bash
for table in click_events tracking_events conversion_events cost_events postback_events; do
  docker exec infra-clickhouse-1 sh -c \
    "clickhouse-client --user flox --query \"SELECT * FROM flox.$table FORMAT Native\" | gzip" \
    > "backups/${table}_$(date +%Y%m%d).native.gz"
done
```

**Restore** (target table must already exist — run `apps/internal/chstore.
Migrate` first, e.g. by starting `apps/worker` once, which is idempotent
and safe to run before a restore):

```bash
gunzip -c backups/click_events_20260101.native.gz | \
  docker exec -i infra-clickhouse-1 clickhouse-client --user flox \
  --query "INSERT INTO flox.click_events FORMAT Native"
```

The five base tables above (`internal/chstore/schema/001-005`) are what
actually needs backing up; the materialized views
(`click_events_daily_campaign` etc., `schema/006`) rebuild themselves from
the base tables' data as new inserts land — they don't need their own
backup, only their `CREATE MATERIALIZED VIEW IF NOT EXISTS` definition,
which `chstore.Migrate` already recreates on any fresh instance.

**Cadence**: nightly, retained 14 days (shorter than Postgres — this data
is high-volume and, per CLAUDE.md #2, an intentionally full event history
rather than a compact control-plane snapshot; losing a day of raw click
events to a disaster is a materially smaller loss than losing the
campaign/routing configuration that decided where those clicks went).

## Object storage (S3/MinIO)

No backup procedure is defined yet because nothing writes here yet
(`apps/internal/config.S3Config`, unused — see the table above). When a
real feature starts storing files here (content gallery assets,
per §14.9, is the most likely first consumer), the standard approach is
bucket versioning (S3 native, MinIO supports it too) plus cross-region/
cross-bucket replication — not a separate dump-and-restore script, since
object storage is already durable by design; versioning protects against
accidental overwrite/delete, which is the actual risk a backup addresses
here. Revisit this section when that phase lands.

## Restore order for a full disaster recovery

```
1. Restore Postgres first (pg_restore) — every other store's rows
   reference organization_id/campaign_id/etc. that only make sense
   relative to Postgres's control-plane state.
2. Start apps/worker once (idempotently creates ClickHouse's schema via
   chstore.Migrate) before restoring ClickHouse data into it.
3. Restore ClickHouse table backups (INSERT ... FORMAT Native, above).
4. Start apps/api, apps/tracker, apps/worker, apps/web (docs/deployment.md's
   own startup order — migrations already applied by step 1, so the
   `migrate` one-shot service is a no-op here, matching goose's own
   idempotent "no migrations to run" behavior).
```

## What this doc deliberately does not do

No backup automation (cron job, scheduled Lambda, etc.) ships in this
repo — every command above is meant to be wired into whatever a real
deployment's own scheduler is (systemd timer, Kubernetes CronJob, a
managed database provider's built-in backup feature), which varies by
where FLOX actually gets deployed and isn't chosen anywhere in this
project (`docs/deployment.md`'s own note: no cloud vendor was ever
specified). Automating a schedule against infrastructure that doesn't
exist yet would be exactly the kind of speculative build-ahead CLAUDE.md's
"§80 NEVER" list warns against.
