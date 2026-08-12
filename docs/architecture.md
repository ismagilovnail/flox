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

## Phase 18 — Campaign API

`internal/campaign/` (`handler.go` → `service.go` → `repository.go` →
`model.go`, per CLAUDE.md's Go architecture): `GET/POST /campaigns`,
`GET/PATCH/DELETE /campaigns/:id`, `POST /campaigns/:id/{duplicate,pause,activate}`.

Two cross-cutting pieces landed alongside it, both reused by every domain
package from here on rather than being campaign-specific:

- **`internal/tenant`** — the `X-Organization-Id`-header stand-in for
  session-derived org scoping described in `apps/api/README.md`. §36-
  TENANCY's cross-tenant FK gap also got a real fix here: a campaign's
  `traffic_source_id` FK only proves that row *exists somewhere* — nothing
  stops it belonging to a different org unless the service layer checks
  `traffic_sources.organization_id` matches the caller's org before
  writing. `Repository.TrafficSourceBelongsToOrg` does that check; it's the
  pattern every future domain package with a cross-table reference needs
  to repeat, not just campaigns' problem.
- **`internal/apierror`** — one JSON error envelope (`{code, message,
  fields?}`) every handler renders, so a client gets the same shape from
  every endpoint instead of each domain package inventing its own.

`GET /ready` now genuinely pings Postgres (promised, not yet delivered, in
the Phase 16 section above) and returns `503` with the failing check named
if the ping fails.

Pause/activate aren't bare status setters — "validate using domain rules"
(§37) meant giving them real transition rules: idempotent from the target
state, rejected outright from `archived` (an archived campaign has to be
explicitly edited back via `PATCH`, not casually reactivated by a
pause/activate toggle).

**Cross-tenant isolation test** (CLAUDE.md DoD requirement for API phases):
`internal/campaign/repository_test.go`, gated on `DATABASE_URL` being set
(skips cleanly otherwise). Creates a campaign for org A, then proves org B's
`list`/`get`/`update`/`delete` all see nothing, and that org A can't attach
a campaign to org B's traffic source. Caught one real bug while writing
it: `defer pool.Close()` in the test body ran *before* `t.Cleanup`-registered
delete callbacks (Go runs deferred statements at function return, then
`t.Cleanup` callbacks after) — the pool was already closed by the time the
cleanup queries ran, so every seeded test org silently leaked. Fixed by
registering the pool's close via `t.Cleanup` too, ordered (via LIFO) after
the org-delete cleanups; confirmed fixed by checking `organizations`/
`campaigns` row counts before and after a test run.

## Phase 19 — Routing Engine

`internal/routing` — the §6-SHARED Strategy A single source of truth for
routing decisions. Full detail, including the conformance fixture table,
is in [`docs/routing.md`](routing.md); the short version: `Resolve` is a
pure function with no `net/http` or database dependency (§38), matches the
spec's exact `RouteResult` shape, and a second method on the concrete
engine (`Explain`) shares the same evaluation to additionally return the
full per-stream-set/per-flow trace §72 requires, without growing
`RouteResult` beyond what §38 specifies. All 17 of §58's required test
cases pass or are explicitly documented as out of this package's scope
(tracking-link resolution, campaign-active checks, and the WebView bounce
all happen in the caller, before `Resolve` is ever invoked).

One deliberate divergence from the current frontend mock
(`lib/routing-simulate.ts`): the Go engine checks a destination's offer is
still active before using it (`Destination.OfferActive`), which the
frontend mock never implemented. §58 explicitly requires an "inactive
offers" test case, and Strategy A means the Go engine — not the mock — is
what's actually correct here; the frontend catches up when Phase 27 swaps
it onto the real endpoint.
