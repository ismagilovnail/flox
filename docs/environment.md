# Environment Documentation (§61, Phase 33)

`.env.example` at the repo root tracks the full variable surface — this
doc is what actually changes between dev, staging, and production, and
why. Every variable below is read by `apps/internal/config.Load`/
`LoadTracker`/`LoadWorker` (Go) or Next.js directly (web); nothing here is
invented for this doc.

## What actually changes behavior per environment

Grepping the Go codebase for `cfg.Env`/`NODE_ENV` turns up exactly **one**
behavior gate, in `apps/api/main.go`:

```go
authHandler := auth.NewHandler(authSvc, logger, cfg.Env != "development")
```

`cfg.Env != "development"` becomes `secureCookies` — whether the session
cookie (`apps/internal/auth/handler.go`'s `setSessionCookie`) is marked
`Secure`. That's the entire runtime behavior difference `NODE_ENV`
controls in this codebase. Everything else that differs between dev and
production is which *values* the other variables hold, not code paths.

**This has one hard operational consequence**: set `NODE_ENV=production`
(or anything other than `development`) only once TLS is actually
terminated in front of `apps/api` — a browser drops a `Secure` cookie
sent over plain HTTP outright, which breaks every authenticated request.
`docker-compose.test.yml` deliberately stays `NODE_ENV=development` (unset
→ default) for exactly this reason: it has no TLS termination in front of
it. A real deployment needs a reverse proxy/load balancer doing TLS
(`docs/deployment.md`) before flipping this.

`apps/web`'s own `NODE_ENV=production` (set inside `apps/web/Dockerfile`,
unconditionally) is standard Next.js build behavior — React production
mode, minification — not a FLOX-specific gate; it's unrelated to the
`secureCookies` question above and always correct in a built image.

## Variable reference

### App

| Variable | Dev value | Staging/production |
|---|---|---|
| `NODE_ENV` | `development` (default) | `production` — **only after** TLS is live in front of `apps/api` (see above) |
| `APP_URL` | `http://localhost:3000` | The real public URL of `apps/web`, exactly as a browser's `Origin` header will read it (`apps/internal/tenant.RequireSameOrigin`, §54 CSRF, compares this literally — scheme/host/port must match exactly, not just the hostname) |
| `API_URL` / `TRACKER_URL` / `WORKER_URL` | `http://localhost:8080/8081/8082` | Only the **port digits** are read (`config.parsePort`) — these decide each binary's bind port, nothing else. Safe to leave as the container's internal port even behind a reverse proxy that exposes a different public URL. |
| `LOG_LEVEL` | `info` | `info` in production too; `debug` only for active incident investigation — every log line is JSON (`apps/internal/logging`), fine for either volume in most log pipelines |

### PostgreSQL

`DATABASE_URL` — dev: the throwaway `flox`/`flox`/`flox` credentials in
`infra/docker-compose.dev.yml`. **Staging/production: a real managed
Postgres instance, real generated credentials, `sslmode=require` (or
stricter) instead of dev's implicit disable.** Never reuse the dev/test
compose passwords anywhere real — they're published in this repo.

### ClickHouse

`CLICKHOUSE_URL`/`_DATABASE`/`_USER`/`_PASSWORD` — same story: dev runs
with an empty password (`infra/docker-compose.dev.yml`'s
`CLICKHOUSE_PASSWORD: ""`) and `CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT: 1`
(lets the default user manage other users — fine for a single-developer
local container, wrong for anything real). Production needs a real
password and TLS (`https://` in `CLICKHOUSE_URL`) — `apps/internal/chconn`
already just passes the URL through to the official driver, no code
change needed to point it at a TLS endpoint.

### Redis

`REDIS_URL` — dev: no auth (`infra/docker-compose.dev.yml`'s `redis`
service sets none). Production: a real password (`redis://:password@host:
6379/0`) — CLAUDE.md's own stance on Redis (cache/rate-limit/sticky-cache/
postback-dedup only, never sticky's source of truth) already means a
Redis outage degrades rather than corrupts (`apps/internal/conversion`,
`apps/tracker/sticky.go` both documented fallback paths), but that's not
a reason to leave it unauthenticated on a real network.

### S3-compatible object storage

Not consumed by any code path yet (`apps/internal/config.S3Config` exists,
nothing reads it — see `docs/backup.md`'s own note). The dev/test values
(MinIO's `flox-minio-admin`/`flox-minio-secret`) are throwaway
compose-internal credentials; replace both before this is ever wired to a
real feature, same as every other credential on this list.

### Auth / sessions

No secret to configure — session and invite tokens are random
(`crypto/rand`), never signed or encrypted, and stored hashed (SHA-256).
There is no JWT signing key anywhere in this system to rotate or protect.
See `docs/auth.md`.

### OpenTelemetry

`OTEL_EXPORTER_OTLP_ENDPOINT` — empty/unset is a valid, intentional value
(`apps/internal/telemetry.Setup`'s own doc: a no-op when unset, not an
error). Dev runs with no OTel collector; point this at a real OTLP
endpoint (Jaeger, Tempo, a vendor) for production tracing. Prometheus
metrics (`docs/observability.md`) are a completely separate signal, always
on regardless of this variable — `GET /metrics` on all three Go binaries
needs no configuration at all.

### FX rate provider, domain/DNS providers, Facebook/TikTok ad accounts

All blank in `.env.example` on purpose — every one of these is a real
external vendor credential CLAUDE.md non-negotiable #11 requires stay
behind an interface (no vendor lock-in in core logic), and none has ever
been wired to a live account in this project (`internal/adaccount/
facebookads`/`tiktokads` are structurally real but only ever benchmarked
against fixtures — `docs/performance.md`). Fill in real values only when
actually connecting a real vendor account; the code path degrades
gracefully with them empty — confirmed directly against a real
`apps/worker` log line with no ad accounts connected: `"scheduled ad
spend sync run finished" connections_attempted=0`, not an error.

## Secrets handling

Nothing in this list is a signing key or encryption key (see "Auth /
sessions" above) — every secret here is a plain credential (a database
password, a vendor API key). Standard practice applies: never commit
`.env`, inject real values via your deployment platform's secret store
(Kubernetes Secret, a cloud provider's parameter store, Docker Swarm
secret, etc.) rather than baking them into an image or compose file.
`docker-compose.test.yml`'s hardcoded values are acceptable *only* because
they're throwaway, disposable, local-only credentials — the same
`docker-compose.dev.yml` has always used.
