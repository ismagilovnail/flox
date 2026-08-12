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

## Phase 17 — Database

`apps/api/migrations/` — 10 goose migrations, §35's full core table list
(24 tables + one join table, `stream_set_pixels`, for `StreamSet.pixels:
string[]`). No tables for Tags/Custom Metrics/Report Presets/Referral/
Content Gallery — those v3 "secondary" phases (14.5–14.9) aren't in §35's
list, so their schema isn't guessed at here.

Conventions applied uniformly (detail in `apps/api/migrations/README.md`):
ULID PKs via a `ulid` domain type with a format CHECK; `organization_id NOT
NULL` denormalized onto every tenant-scoped table *including child tables*
(`offer_links`, `filter_conditions`, `flows`, …) rather than left implicit
via a join to the parent; `updated_at` maintained by a trigger, not
application code.

A few schema decisions worth calling out because they resolve real
ambiguity in the spec rather than restating it:

- **`postbacks` is the durable dedup ledger for §45's invariant #3**
  (`UNIQUE (organization_id, click_id, status)`, with a partial-index
  exemption for networks with `accept_duplicates`), not a full delivery
  log — the rich per-attempt log (message, payload, retries) belongs in
  ClickHouse's `postback_events` (high-volume, analytics-shaped), outside
  this Postgres-only phase's scope.
- **`campaigns` has no `tracking_domain`/`tracking_id` columns** even
  though the frontend mock has them on `Campaign` — that data is a
  `(campaign, domain, slug)` row in `tracking_links` instead, so one
  campaign with links on multiple domains never duplicates/desyncs a
  domain string that's also stored elsewhere.
- **`roles`/`permissions`/`fx_rates` are the only non-tenant-scoped
  tables** — roles are a fixed platform-wide set today (not customized per
  org), permissions are a catalog `roles` doesn't reference yet (real RBAC
  enforcement is Phase 28's job), fx_rates are an objective market fact
  shared by every tenant.

Verified against a real Postgres (`infra/docker-compose.dev.yml`): full
`goose up`/`down-to 0`/`up` roundtrip is clean (no orphaned objects), and a
smoke test inserted a complete campaign→stream_set→filter_tree→flow graph
for one org, confirmed a second org's queries return zero rows against it,
and confirmed every hand-written constraint (postback dedup +
`accept_duplicates` override, `flows` destination shape CHECK, cost_entries
per-day dedup) fires exactly as intended.
