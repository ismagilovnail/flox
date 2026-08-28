# infra

Deployment configuration for web/api/tracker/worker/postgres/clickhouse/
redis/object storage (§61, Phase 33).

```
docker-compose.dev.yml   Local dev data stores only (postgres/clickhouse/
                          redis/minio/prometheus) — the four app binaries
                          run on the host via `go run`/`npm run dev`, not
                          here. Phase 16, extended through Phase 29
                          (prometheus) and Phase 33 (alerts.yml wiring).
docker-compose.test.yml  The FULL containerized stack, all eight §61
                          services built from this repo's own Dockerfiles.
                          Not part of the day-to-day dev loop — proves the
                          images actually build and boot together, and
                          doubles as a deployment template. Runs under its
                          own `flox-test` Compose project (the `name:` at
                          the top of the file), isolated from
                          docker-compose.dev.yml's containers/volumes —
                          see that file's own comment for why this matters.
prometheus.yml           Dev-stack Prometheus scrape config (§53,
                          Phase 29), now also loading alerts.yml.
alerts.yml               Prometheus alerting rules over the nine tracked
                          metrics (§61, Phase 33) — promtool-validated,
                          no Alertmanager wired to them (see the file's
                          own comment).
```

Each app's Dockerfile lives with its own app, not here:
`apps/api/Dockerfile` (also builds the `migrate` target — one-shot goose
runner, `docs/migrations.md`), `apps/tracker/Dockerfile`,
`apps/worker/Dockerfile`, `apps/web/Dockerfile`.

See `docs/deployment.md` for the full build/deploy/scale story,
`docs/environment.md` for what changes between dev/staging/production,
`docs/backup.md` for the backup strategy, and `docs/migrations.md` for
migration rollout/rollback strategy.
