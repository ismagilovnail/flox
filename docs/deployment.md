# Production Deployment (§61, Phase 33)

No cloud vendor is specified anywhere in this project (`FLOX-master-
prompt-v3.md` never names one) — this doc is deliberately platform-neutral:
four container images plus four data stores, deployable anywhere that
runs containers (a single Docker host, Swarm, Kubernetes, ECS, Nomad,
whatever). `infra/docker-compose.test.yml` is the reference
implementation — every claim below was verified by actually building and
running it, not written from the Dockerfiles alone.

> Kubernetes: manifests and the ordered deploy script are in `infra/k8s`,
> described in `docs/kubernetes.md`.

## What gets deployed

| Service | Image | Dockerfile | Port |
|---|---|---|---|
| `apps/web` | Next.js, standalone output | `apps/web/Dockerfile` | 3000 |
| `apps/api` | Go control-plane API | `apps/api/Dockerfile` | 8080 |
| `apps/tracker` | Go hot-path redirect | `apps/tracker/Dockerfile` | 8081 |
| `apps/worker` | Go async processor | `apps/worker/Dockerfile` | 8082 |
| — (one-shot) | Postgres migrations | `apps/api/Dockerfile`, `--target migrate` | — |
| backup sidecar | nightly backups, monthly restore drill (`docs/backup.md`) | `apps/cmd/backup/Dockerfile`, `--target backup` | 8083 |
| Postgres (self-hosted) | `postgres:16-alpine` + WAL-G for WAL archiving | `apps/cmd/backup/Dockerfile`, `--target postgres` | 5432 |

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
docker build -f apps/cmd/backup/Dockerfile --target backup   -t flox/backup:$VERSION   apps/
docker build -f apps/cmd/backup/Dockerfile --target postgres -t flox/postgres:$VERSION apps/
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
   (they don't call each other directly); ClickHouse's schema is
   migrated by whichever starts first, the others wait on a Postgres
   advisory lock (apps/internal/chstore.MigrateLocked, versioned in
   schema_migrations; docs/migrations.md).
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

**User content gets its own host.** Object storage (`S3_PUBLIC_ENDPOINT`)
serves uploaded landing files and gallery media as they are, HTML and SVG
included, and the tracker serves landings with the operator's scripts.
Give both their own hosts (`cdn.`, `t.` — never a path under the panel's or
the API's host): on their own origin those scripts can neither read the
API (CORS allows only `APP_URL`) nor change anything (the `Origin` check).
Set `API_PUBLIC_URL` to the API's public origin and the binaries refuse a
configuration that breaks this (`docs/security.md` §9).

## HTTPS for tracking domains

Organizations add their own tracking domains, so the tracker's certificates
cannot be configured by hand. Put Caddy in front of `apps/tracker` with
`infra/Caddyfile.tracker`:

```
docker run -d -p 80:80 -p 443:443 \
  -e ACME_EMAIL=ops@yourdomain.example -e TRACKER=tracker:8081 \
  -v $PWD/infra/Caddyfile.tracker:/etc/caddy/Caddyfile:ro \
  -v caddy_data:/data caddy:2
```

- Caddy issues a certificate on the first HTTPS connection to a domain, after
  asking the tracker's `/internal/tls-ask` whether it is registered; the
  worker's SSL check (every 10 minutes for domains without one) makes that
  first connection, so nobody has to open the domain by hand.
- Ports 80 and 443 must reach Caddy from the internet (ACME HTTP-01 /
  TLS-ALPN-01), and each domain's DNS must point at it — see
  `TRACKER_DNS_TARGET` for the Cloudflare "Sync DNS" button.
- Keep `/data` persistent; with several Caddy replicas share it (or use a
  Caddy storage module), or each will request its own certificates and hit
  the CA's rate limits.
- Add Caddy's address to `TRUSTED_PROXIES` if it is not on a private
  network, or the tracker will see Caddy's IP instead of the visitor's.
- Behind Cloudflare's proxy (orange cloud) Cloudflare serves the visitor's
  certificate; the check then reports Cloudflare's and that is correct.

The worker's checker needs outbound access to the domains on port 443.

## Health checks

Every one of the four services answers `GET /health` (`apps/web`:
`GET /api/health`) with `200 {"status":"ok"}` — wired into each
Dockerfile's own `HEALTHCHECK` already, and reusable directly as a
Kubernetes `readinessProbe`/`livenessProbe` or equivalent. `apps/api` also
exposes `GET /ready` (`apps/internal/httpserver`), which additionally
pings Postgres and, when connected, ClickHouse — use that for readiness
specifically (traffic should wait for working store connections) and
plain `/health` for liveness (should the process itself be restarted).

`apps/tracker` has its own `GET /ready`: `200 ready` when Postgres answers,
`200 degraded` when it does not but routing is cached (the tracker keeps
serving every campaign it has seen — `internal/routingcache`), `503` when
it can serve nothing yet (Postgres down, cache cold) or while it is
shutting down. Point the load balancer's readiness at it; keep `/health`
for liveness.

## Scaling

- **`apps/tracker`** is the one service CLAUDE.md non-negotiable #9
  explicitly calls out to keep fast and stateless — horizontally scale
  this first and most aggressively under real click volume. Nothing about
  it holds in-process state that matters across replicas: routing config
  is an in-memory cache (`internal/routingcache`, flushed on every edit via
  Postgres LISTEN/NOTIFY — migration 00050, so each replica needs one extra
  Postgres connection for the listener); sticky truth is the visitor's own
  cookie, CLAUDE.md #4. A new replica starts cold and loads each campaign
  on its first click.
- **`apps/api`** is stateless per-request (sessions live in Postgres, not
  in-process) — scales horizontally with no special coordination.
- **`apps/worker`** runs poll loops that claim batches with `FOR UPDATE
  SKIP LOCKED` (`apps/internal/eventqueue`, `apps/internal/postback`) —
  already safe to run multiple replicas of; they naturally divide the
  queue rather than double-processing it.
  Give it a termination grace period of **at least 45s**: on SIGTERM it
  finishes the batch in hand and waits up to 40s for its loops before
  exiting. Anything cut off harder than that is returned to the queue by
  the reaper 5 minutes later (`docs/analytics-pipeline.md`).
- **`apps/web`** is a stateless Next.js server — scales horizontally with
  no special coordination either.

### Connection pool sizing

None of the four Go binaries set explicit pool limits in code —
`internal/postgres.NewPool` calls `pgxpool.New(ctx, databaseURL)` with no
`pgxpool.Config`, and `internal/chconn.NewConn` opens the ClickHouse HTTP
client with no `MaxOpenConns`/`MaxIdleConns`. Both client libraries fall
back to their own defaults (pgx: `max(4, runtime.NumCPU())` connections
per pool; clickhouse-go: unbounded HTTP client pooling), which is fine at
dev-stack scale but is a real knob to turn under production concurrency,
especially for `apps/tracker`, which opens one pool per replica and issues
several Postgres round trips per click (`routingstore.LoadRoutingConfig`).
Tune it without a code change via the DSN itself — pgx reads pool
settings straight out of the connection string:

```text
DATABASE_URL=postgres://user:pass@host:5432/flox?pool_max_conns=50&pool_min_conns=5
```

`internal/rediscache` is in the same position — no explicit `PoolSize` on
its `redis.Options`, so it also rides the go-redis client's default. The
one outbound client that *does* set explicit limits is unrelated to any
datastore: `apps/worker`'s postback-delivery HTTP transport
(`MaxIdleConns: 100`, `apps/worker/main.go`), sized for the partner
webhooks it calls, not for Postgres/ClickHouse/Redis.

### Redis memory (audit R2)

Every key FLOX writes expires, but the conversion `LastStatus` cache
(`flox:conversion:last-status:*`) lives 30 days: about 100 bytes per
converted click, ~1 GB at 10M conversions a month. Redis's default
`maxmemory-policy noeviction` fails **every** write once memory is full —
the cache then only falls through to Postgres, but the rate limiter fails
open and the `/auth` limits silently stop. Production Redis needs:

```text
maxmemory <bounded, e.g. 1gb>
maxmemory-policy allkeys-lru
```

With LRU the oldest cache entries go first; live rate-limit counters are
the most recently used keys and stay. Any `volatile-*` policy also works
(every key has a TTL). Dev and test compose run Redis with `256mb` and
`allkeys-lru`. `apps/api` and `apps/tracker` read the policy at start
(`rediscache.CheckEviction`) and log a WARN on `noeviction` or no
`maxmemory`; managed Redis that disables `CONFIG` logs it as unverified
instead — check the provider's parameter group by hand then.

### NATS JetStream (event stream)

With `NATS_URL` set the tracker publishes its events to the
`FLOX_EVENTS` stream and the worker writes them to ClickHouse
(`docs/event-stream.md`). Production: a JetStream cluster of 3 servers,
each with a persistent volume (the stream is file-backed, up to 10 GB),
and `NATS_STREAM_REPLICAS=3`. The stream and consumer are created by the
binaries themselves. A NATS outage loses nothing: the tracker falls back
to Postgres `event_queue`, the worker reads both. Monitor the server at
`:8222` (or prometheus-nats-exporter) besides FLOX's own
`flox_eventstream_*` metrics.

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
