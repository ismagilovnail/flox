# apps/api

Go control-plane API (chi, pgx). Scaffolded in Phase 16 (Go Backend
Foundation).

```
cmd/api/          entrypoint — wires config, logging, telemetry, DB pool, HTTP server
internal/config/   env-based configuration (Config.Load)
internal/logging/  slog.Logger setup (JSON)
internal/telemetry/ OpenTelemetry TracerProvider setup
internal/postgres/ pgx pool constructor
internal/idgen/     ULID generation/validation, matching the `ulid` Postgres domain
internal/tenant/    organization_id request-context middleware (§36-TENANCY) — see below
internal/apierror/  shared error envelope every domain package's handler renders
internal/httpserver/ chi router: middleware, GET /health, GET /ready (now pings Postgres)
internal/campaign/  Campaign API (§37, Phase 18) — handler → service → repository
migrations/        goose migrations — §35's core schema, landed Phase 17
pkg/                code meant for import by other services — empty until something needs it
```

Run locally (needs Postgres — see below):

```
cd apps/api && go run ./cmd/api
curl localhost:8080/health
curl localhost:8080/ready
curl -H "X-Organization-Id: <ulid>" localhost:8080/campaigns/
```

## Tenant context: no auth yet

There's no session/API-key auth until Phase 28. Every tenant-scoped route
requires an `X-Organization-Id` header set to a real, already-existing
organization id — `internal/tenant`'s middleware validates it's present and
ULID-shaped, then every handler reads `organization_id` from request
context, never from the body or a query param. This satisfies §36-TENANCY's
letter (a handler *cannot* pull org scope from anywhere else) while being
honest that it isn't real auth. Phase 28 replaces the middleware's header
lookup with a session/API-key lookup; nothing downstream of it changes.

Database migrations — see `migrations/README.md` for the full command
reference and schema conventions:

```
docker compose -f ../../infra/docker-compose.dev.yml up -d postgres
go tool goose -dir migrations postgres "$DATABASE_URL" up
```

## Open question: module topology once tracker/worker exist

`ARCHITECTURE.md` states `apps/tracker` and `apps/worker` are "separate
binaries inside the same Go module" as `apps/api`, sharing
`internal/routing` and `internal/classifier`. This module's `go.mod` lives
at `apps/api` per §33's literal Phase 16 instruction, and Go's
internal-import visibility rule means a package under `.../internal/...` is
only importable by source rooted at that internal directory's parent — so
`apps/tracker` and `apps/worker`, as sibling directories to `apps/api`,
structurally cannot import `apps/api/internal/routing` no matter what
module they're in.

Not a Phase 16 problem to solve — routing/classifier logic doesn't exist
until Phase 19/20 — but whoever starts Phase 21 (`apps/tracker`) needs to
pick one:

1. Move this module's root up to `apps/`, with `cmd/api`, `cmd/tracker`,
   `cmd/worker` as siblings under one `go.mod` — matches the "same module"
   claim exactly, at the cost of a one-time directory shuffle.
2. Keep three separate modules/binaries, and put anything that genuinely
   needs cross-service reuse (the routing/classifier decision logic itself)
   under `pkg/` instead of `internal/` — `pkg/` has no cross-module
   visibility restriction, so this satisfies "no duplicated routing logic"
   (CLAUDE.md's non-negotiable #1) without an `internal/` rename.

Either is fine; don't guess now — decide it in Phase 21 with the actual
tracker code in front of you.
