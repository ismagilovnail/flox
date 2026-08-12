# Architecture

See [`/ARCHITECTURE.md`](../ARCHITECTURE.md) at the repo root for the
canonical, stable architecture summary (layers, repo layout, data stores,
shared-domain-logic decision, invariants).

This document accumulates deeper technical detail as each backend phase
lands: module boundaries inside `internal/`, request lifecycle through
`apps/tracker`, the event pipeline `apps/tracker → queue → apps/worker →
ClickHouse`, and how `apps/api`/`apps/tracker`/`apps/worker` share
`internal/routing` and `internal/classifier` without duplicating decision
logic.

## Phase 16 — Go Backend Foundation

`apps/api` is a Go module (`github.com/ismagilovnail/flox/apps/api`) with:

```
cmd/api/            entrypoint
internal/config/     env-based Config (loads, doesn't yet connect to, DATABASE_URL/CLICKHOUSE_*/REDIS_URL/S3_*)
internal/logging/    slog.Logger, JSON output
internal/telemetry/  OpenTelemetry TracerProvider (OTLP/HTTP exporter, no-op if OTEL_EXPORTER_OTLP_ENDPOINT is unset)
internal/httpserver/ chi router — request ID, real IP, structured request logging, panic recovery, 30s timeout, OTel instrumentation, GET /health, GET /ready
```

`GET /health` is liveness only (process is up). `GET /ready` returns `200`
unconditionally for now — it starts checking real dependencies (Postgres,
ClickHouse, Redis) once Phase 17+ actually wires them in; faking a
dependency check today would violate the "no fake APIs that look real" rule
for a check that doesn't check anything yet.

`infra/docker-compose.dev.yml` stands up Postgres, ClickHouse (HTTP
interface only — the native port stays container-internal to avoid a host
port clash with MinIO), Redis, and MinIO (S3-compatible), with credentials
matching `.env.example` exactly. `apps/api` doesn't connect to any of them
yet; the compose file exists now because every later phase needs it and
there's no reason to keep re-deriving it.

**Open decision, not yet made:** `apps/tracker` and `apps/worker` are
separate top-level directories from `apps/api`, but Go's internal-import
visibility rule means a sibling directory cannot import
`apps/api/internal/routing` regardless of module setup. Phase 21 (when
`apps/tracker` is first scaffolded) needs to either move this module's root
up to `apps/` (one `go.mod` for all three binaries, matching
`ARCHITECTURE.md`'s "same Go module" description literally) or keep three
modules and put routing/classifier under `pkg/` instead of `internal/`
(no cross-module visibility restriction there). See `apps/api/README.md`
for the fuller writeup.
