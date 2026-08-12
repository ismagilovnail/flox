# apps/api

Go control-plane API (chi, pgx). Scaffolded in Phase 16 (Go Backend
Foundation).

This directory holds the API binary and the control-plane schema:

```
main.go        entrypoint — wires config, logging, telemetry, DB pool, HTTP server
migrations/    goose migrations — §35's core schema, landed Phase 17
```

Everything else lives in the module-wide `apps/internal/`, shared with
`apps/tracker` (see "Module topology" below):

```
config/     env-based configuration (Load for the API, LoadTracker for the tracker)
logging/    slog.Logger setup (JSON)
telemetry/  OpenTelemetry TracerProvider setup
postgres/   pgx pool constructor
idgen/      ULID generation/validation, matching the `ulid` Postgres domain
tenant/     organization_id request-context middleware (§36-TENANCY) — see below
apierror/   shared error envelope every domain package's handler renders
httpserver/ chi router: middleware, GET /health, GET /ready (pings Postgres)
campaign/   Campaign API (§37, Phase 18) — handler → service → repository
routing/    routing decision engine (§38, Phase 19)
classifier/ request → normalized attributes (§40, Phase 20)
event/      the full §43 event model
eventbuf/   buffered async event writer (§41)
routingstore/ loads routing config out of Postgres into routing's pure types
```

Run locally (needs Postgres — see below):

```
cd apps && go run ./api
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
go tool goose -dir api/migrations postgres "$DATABASE_URL" up
```

## Module topology — resolved in Phase 21

Phase 16 left this open: Go's internal-import rule means a package under
`.../internal/...` is only importable by code rooted at that directory's
parent, so `apps/tracker` could never have imported
`apps/api/internal/routing` while the module root sat at `apps/api`.

**Resolved by moving the module root up to `apps/`** (option 1 of the two
recorded then), because both `ARCHITECTURE.md` ("separate binaries inside
the same Go module") and §41 ("shares internal packages") state it
directly — there was nothing left to weigh.

```
apps/
  go.mod          module github.com/ismagilovnail/flox/apps
  internal/       shared by every binary — routing, classifier, config, …
  api/            this service (package main)
  tracker/        hot-path click/redirect service (package main)
  worker/         placeholder until Phase 24
  web/            Next.js; carries a stub go.mod purely to keep
                  node_modules out of the Go module (see that file)
```

The directory layout CLAUDE.md specifies is unchanged; only `go.mod` moved.
