# Backups and restore (§61; tech audit 2026-10-06, DO2)

Three stores, three methods, one bucket. The backup bucket is **off-site**
in production — another provider or region than the databases and the
files bucket (`BACKUP_S3_*`, `docs/environment.md`). In dev it is
`flox-backups` in the same MinIO.

| Store | What is lost without it | Method | Kept | Loss on disaster (RPO) |
|---|---|---|---|---|
| Postgres | the control plane: organizations, campaigns, routing, postbacks, costs | WAL-G: every WAL segment archived continuously + nightly base backup | 14 base backups (`BACKUP_KEEP_POSTGRES`) = a 14-day PITR window | ≤ 60 s (`archive_timeout`) |
| ClickHouse | every event and every report built from them | `BACKUP DATABASE flox TO S3`, nightly, all tables incl. aggregates | 14 (`BACKUP_KEEP_CLICKHOUSE`) | up to a day; events newer than the backup that are still in Postgres's `event_queue` are not lost |
| Files (`S3_BUCKET`) | Content Gallery uploads | mirrored into `objects/`, nightly: new and changed files; deletions are not mirrored | everything ever mirrored | up to a day |

ClickHouse conversions can also be rebuilt from Postgres
(`make reconcile`, `docs/attribution.md`) — clicks cannot, which is why
ClickHouse has its own backup.

## How it runs

- **WAL archiving** is the postgres server's own job:
  `archive_mode=on`, `archive_command='wal-g wal-push %p'`,
  `archive_timeout=60`. The image is `postgres:16-alpine` plus WAL-G
  (`apps/cmd/backup/Dockerfile`, target `postgres`), WAL-G's settings in
  the server's environment (`WALG_S3_PREFIX`, `AWS_*`).
- **The backup sidecar** (`flox-backup serve`, same Dockerfile, target
  `backup`) runs at `BACKUP_AT` (UTC) every day: Postgres base backup +
  prune, ClickHouse backup + prune, files mirror. A store that fails does
  not stop the others. On `BACKUP_DRILL_DAY` the restore drill follows.
  It mounts the live data directory **read-only** (WAL-G reads it for the
  base backup) and a scratch volume for the drill — the drill restores
  the whole Postgres database there, so give it the database's size.
- The sidecar creates the bucket if it is missing (dev). In production
  create it beforehand with versioning or object lock, so a leaked
  credential cannot delete the backups.

Commands (dev; in production, `flox-backup <command>` in the sidecar):

```bash
make backup-now     # back up every store now
make backup-drill   # restore every store into a scratch copy and check it
make backup-list    # base backups, WAL archiving state, ClickHouse backups, mirror, last drill
```

## The restore drill

A backup nobody restored is a hope. `flox-backup drill` restores every
store into a scratch copy, checks it, and keeps the report in the bucket
(`drills/<name>.json`); it exits non-zero when a check fails.

| Store | What is restored | What is checked |
|---|---|---|
| Postgres | latest base backup + every archived WAL segment, into a private server in the sidecar (socket only, port 5433) | Before the restore a mark is written into `backup_drill_marks` (migration 00060) together with the list of tables, and its WAL segment is archived (`pg_switch_wal`). The restore must contain the mark — proof it reaches the present, not just last night's base backup — every table of the list, and the live schema version. |
| ClickHouse | latest backup, as `flox_drill` (`RESTORE ... AS` rewrites the materialized views to the copy — the live aggregates are never touched) | every table's rows between the counts taken right before and right after its backup (the manifest, `clickhouse-manifests/<name>.json`); then `flox_drill` is dropped |
| Files | — | every file the source had at the last mirror run is in the mirror with its size; 20 random files hash the same on both sides |

Verified 2026-10-09 against the dev stack, failing on purpose too: a file
removed from the mirror, a manifest count changed, and WAL archiving
broken (wrong secret on the postgres server) each fail their check.

## Monitoring

The sidecar reads its state back from the bucket every 5 minutes and on
every run, so a restarted sidecar reports the same (`/metrics` on :8083,
job `flox-backup`, `docs/observability.md`). Alerts (`infra/alerts.yml`,
group `flox.backup`): **BackupMissing** (a store's newest backup older
than 26 h), **BackupNeverTaken**, **WALArchivingFailing** (the last
archive attempt failed and none succeeded since, 10 min — PITR stops and
`pg_wal` grows), **RestoreDrillFailed**, **RestoreDrillOverdue** (35 days),
**BackupSidecarDown**.

## Restoring for real

Order: Postgres, then ClickHouse, then files; then the apps. Stop
`api`, `tracker`, `worker` first — nothing may write while you restore.

### Postgres — latest, or to a moment (PITR)

Into an **empty** data directory, with the server stopped, as the
postgres user, with WAL-G's environment (as on the postgres service):

```bash
# 1. fetch the base backup: LATEST, or the newest one before the target
#    moment (`wal-g backup-list --detail` shows finish times)
wal-g backup-fetch "$PGDATA" LATEST

# 2. replay the WAL archive
touch "$PGDATA/recovery.signal"
cat >> "$PGDATA/postgresql.auto.conf" <<'EOF'
restore_command = 'wal-g wal-fetch %f %p'
recovery_target_action = 'promote'
# to a moment instead of the end of the archive, e.g. just before a bad DELETE:
# recovery_target_time = '2026-10-09 14:05:00+00'
EOF

# 3. start; the server replays, then promotes itself and accepts writes
pg_ctl -D "$PGDATA" -w start
psql -c 'SELECT pg_is_in_recovery()'   # false once done
```

The restored server starts a new timeline. Take a base backup right away
(`flox-backup now`) so the next restore does not depend on the old one.
`goose` sees the restored schema version — no migration runs.

### ClickHouse

```sql
-- the newest backup: clickhouse-manifests/ in the bucket, or `make backup-list`
DROP DATABASE IF EXISTS flox SYNC;
RESTORE DATABASE flox FROM S3('<endpoint>/<bucket>/clickhouse/<name>', '<key>', '<secret>');
```

Then rebuild what Postgres has and the backup missed: `make reconcile
FROM=<backup day> TO=<today>` re-emits conversions recorded after the
backup. Clicks after the backup are lost, except those still in
`event_queue`, which the worker flushes when it starts.

### Files

Copy `objects/` back into the files bucket, e.g. with the MinIO client:
`mc mirror backup/<bucket>/objects/ files/<S3_BUCKET>/`.

## Not covered

- **Deletions are not mirrored** for files: the mirror keeps every file it
  ever copied. Prune it with a lifecycle rule on the bucket if needed.
- ClickHouse backups are full each night; incremental backups
  (`base_backup`) are the step once the data makes that slow.
- No Alertmanager receiver ships in this repo (see `infra/alerts.yml`).
