# Production Deployment (§61, Phase 33)

No cloud vendor is specified anywhere in this project (`FLOX-master-
prompt-v3.md` never names one) — this doc is deliberately platform-neutral:
four container images plus four data stores, deployable anywhere that
runs containers (a single Docker host, Swarm, Kubernetes, ECS, Nomad,
whatever). `infra/docker-compose.test.yml` is the reference
implementation — every claim below was verified by actually building and
running it, not written from the Dockerfiles alone.

## What gets deployed

| Service | Image | Dockerfile | Port |
|---|---|---|---|
| `apps/web` | Next.js, standalone output | `apps/web/Dockerfile` | 3000 |
| `apps/api` | Go control-plane API | `apps/api/Dockerfile` | 8080 |
| `apps/tracker` | Go hot-path redirect | `apps/tracker/Dockerfile` | 8081 |
| `apps/worker` | Go async processor | `apps/worker/Dockerfile` | 8082 |
| — (one-shot) | Postgres migrations | `apps/api/Dockerfile`, `--target migrate` | — |

Plus four data stores this repo doesn't containerize a *production*
version of (dev/test both run throwaway single-node containers —
`infra/docker-compose.dev.yml`/`docker-compose.test.yml` — a real
deployment should point at managed or properly-clustered equivalents):
PostgreSQL, ClickHouse, Redis, S3-compatible object storage.

## Build

```bash
# from the repo root
docker build -f apps/api/Dockerfile     -t flox/api:$VERSION     apps/
docker build -f apps/tracker/Dockerfile -t flox/tracker:$VERSION apps/
docker build -f apps/worker/Dockerfile  -t flox/worker:$VERSION  apps/
docker build -f apps/api/Dockerfile --target migrate -t flox/api:$VERSION-migrate apps/
docker build -f apps/web/Dockerfile -t flox/web:$VERSION \
  --build-arg NEXT_PUBLIC_API_URL=https://api.yourdomain.example \
  apps/web
```

The three Go builds share `apps/` as their build context — they're one
Go module (`ARCHITECTURE.md`'s "modular monolith... separate binaries
inside the same Go module"), not three independent projects — so each
`docker build` is fast after the first (Go module cache layer is shared).
`apps/web` builds from `apps/web/` alone; it has no dependency outside its
own directory. `--build-arg NEXT_PUBLIC_API_URL` **must** be the API's
real public URL — it's inlined into the browser bundle at build time
(`apps/web/Dockerfile`'s own comment), so a wrong value here means
rebuilding the image, not an env var fix at runtime.

## Startup order

```
1. postgres, clickhouse, redis, object-storage — up and healthy.
2. migrate (one-shot: `docker run flox/api:$VERSION-migrate "$DATABASE_URL" up`)
   — must complete successfully before step 3. See docs/migrations.md
   for why this is a separate step, never automatic inside apps/api's
   own startup.
3. apps/worker, apps/api, apps/tracker — any order between these three
   (they don't call each other directly); ClickHouse's own schema is
   created idempotently by whichever of apps/api/apps/worker starts
   first (apps/internal/chstore.Migrate, CREATE ... IF NOT EXISTS).
4. apps/web last — it only ever talks to apps/api's HTTP contract.
```

`infra/docker-compose.test.yml`'s `depends_on: condition:
service_completed_successfully`/`service_healthy` chains encode exactly
this ordering — copy its shape for a Kubernetes init container / ECS task
dependency / whatever the target platform's equivalent is.

## TLS and cookies — read before setting `NODE_ENV=production`

`docs/environment.md` has the full explanation: `NODE_ENV=production` (or
anything other than `development`) marks the session cookie `Secure`,
which a browser silently drops over plain HTTP. **A real deployment needs
TLS terminated in front of `apps/api` (and `apps/web`) before flipping
this** — a reverse proxy or load balancer doing TLS termination is
expected in front of both; neither Go service terminates TLS itself.
`APP_URL` must also be the exact scheme+host+port a browser's `Origin`
header will show (§54 CSRF check, `apps/internal/tenant.
RequireSameOrigin`) — `https://app.yourdomain.example`, not an internal
hostname.

## Health checks

Every one of the four services answers `GET /health` (`apps/web`:
`GET /api/health`) with `200 {"status":"ok"}` — wired into each
Dockerfile's own `HEALTHCHECK` already, and reusable directly as a
Kubernetes `readinessProbe`/`livenessProbe` or equivalent. `apps/api` also
exposes `GET /ready` (`apps/internal/httpserver`), which additionally
pings Postgres and, when connected, ClickHouse — use that for readiness
specifically (traffic should wait for working store connections) and
plain `/health` for liveness (should the process itself be restarted).

## Scaling

- **`apps/tracker`** is the one service CLAUDE.md non-negotiable #9
  explicitly calls out to keep fast and stateless — horizontally scale
  this first and most aggressively under real click volume. Nothing about
  it holds in-process state that matters across replicas (routing config
  loads fresh per request via `routingstore`; sticky truth is the visitor's
  own cookie, CLAUDE.md #4).
- **`apps/api`** is stateless per-request (sessions live in Postgres, not
  in-process) — scales horizontally with no special coordination.
- **`apps/worker`** runs poll loops that claim batches with `FOR UPDATE
  SKIP LOCKED` (`apps/internal/eventqueue`, `apps/internal/postback`) —
  already safe to run multiple replicas of; they naturally divide the
  queue rather than double-processing it.
- **`apps/web`** is a stateless Next.js server — scales horizontally with
  no special coordination either.

## Monitoring and alerting

`docs/observability.md` (Phase 29) already covers the nine tracked
Prometheus metrics and how to scrape them — point a real Prometheus at
each service's `/metrics` the same way `infra/prometheus.yml` does for
dev, just with real hostnames instead of `host.docker.internal`.
`infra/alerts.yml` (this phase) adds real, `promtool`-validated alerting
rules over those same metrics — load them via `rule_files:`, exactly as
`infra/prometheus.yml` now does.

No Alertmanager ships anywhere in this repo (`infra/alerts.yml`'s own
comment explains why: no real notification receiver has ever existed in
this project, and standing one up with a fake destination would be
exactly the kind of "fake API that looks real" CLAUDE.md forbids). A real
deployment wires one in — this is a documented example, never run, for
whoever configures the real receiver:

```yaml
# alertmanager.yml — EXAMPLE ONLY, not deployed anywhere in this repo.
route:
  receiver: default
  group_by: [alertname]
receivers:
  - name: default
    # slack_configs / email_configs / pagerduty_configs / webhook_configs
    # — pick whatever this deployment's real on-call tooling is.
```

## Reference: running the whole thing locally

```bash
docker compose -f infra/docker-compose.test.yml up --build
```

Brings up all eight services (§61's full list) on the same ports
`.env.example` documents for local dev (`3000`/`8080`/`8081`/`8082`) plus
offset ports for the data stores so it can run alongside
`docker-compose.dev.yml` without colliding (`infra/docker-compose.test.
yml`'s own top comment has the full port list and explains the `name:
flox-test` project isolation — found necessary after an early version of
this file, run without it, briefly recreated the dev stack's own
containers under the test config). This is not part of the day-to-day dev
loop (`go run`/`npm run dev` on the host stays the fast path,
`docker-compose.dev.yml` unchanged) — it exists to prove the images this
phase's Dockerfiles produce actually build and boot together, and as a
template for a real deployment.
