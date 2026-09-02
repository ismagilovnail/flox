# infra

Deployment configuration for web/api/tracker/worker/postgres/clickhouse/
redis/object storage (§61, Phase 33).

```
docker-compose.dev.yml   The whole dev stack, fully containerized:
                          postgres/clickhouse/redis/minio/mailhog/
                          prometheus plus api/tracker/worker/web
                          themselves (their Dockerfiles' `dev`/`deps`
                          targets — air hot-reload for the three Go
                          services, `next dev` for web — over a
                          bind-mounted apps/ tree). Everything talks to
                          everything else by compose service name on the
                          default bridge network; nothing here uses host
                          networking. Phase 16, extended through Phase 29
                          (prometheus), Phase 33 (alerts.yml wiring),
                          mailhog (dev SMTP catcher, staged ahead of any
                          real SMTP client — not part of §61's list, dev
                          convenience only), and later containerizing
                          api/tracker/worker/web themselves (dropping the
                          host.docker.internal/extra_hosts trick
                          prometheus used to need to reach them on the
                          host).
docker-compose.test.yml  A second, separate full containerized stack, all
                          eight §61 services built from this repo's own
                          Dockerfiles' *production* targets (no hot
                          reload — proves the deployment images
                          themselves actually build and boot together).
                          Not part of the day-to-day dev loop — a
                          pre-deploy sanity check and a deployment
                          template. Runs under its own `flox-test`
                          Compose project (the `name:` at the top of the
                          file), isolated from docker-compose.dev.yml's
                          containers/volumes — see that file's own
                          comment for why this matters.
prometheus.yml           Dev-stack Prometheus scrape config (§53,
                          Phase 29), now also loading alerts.yml.
alerts.yml               Prometheus alerting rules over the nine tracked
                          metrics (§61, Phase 33) — promtool-validated,
                          no Alertmanager wired to them (see the file's
                          own comment).
```

Each app's Dockerfile lives with its own app, not here:
`apps/api/Dockerfile` (also builds the `migrate` target — one-shot goose
runner, `docs/migrations.md` — and the `dev` target docker-compose.dev.yml
runs), `apps/tracker/Dockerfile` and `apps/worker/Dockerfile` (same `dev`
target pattern), `apps/web/Dockerfile`. The root `Makefile`'s `make up`/
`down`/`restart`/`*-logs` targets wrap `docker compose -f
infra/docker-compose.dev.yml` — see the Makefile's own header comment.

See `docs/deployment.md` for the full build/deploy/scale story,
`docs/environment.md` for what changes between dev/staging/production,
`docs/backup.md` for the backup strategy, and `docs/migrations.md` for
migration rollout/rollback strategy.
