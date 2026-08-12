# apps/api

Go control-plane API (chi, pgx). Scaffolded in Phase 16 (Go Backend
Foundation).

```
cmd/api/          entrypoint — wires config, logging, telemetry, HTTP server
internal/config/   env-based configuration (Config.Load)
internal/logging/  slog.Logger setup (JSON)
internal/telemetry/ OpenTelemetry TracerProvider setup
internal/httpserver/ chi router: middleware, GET /health, GET /ready
migrations/        goose migrations — §35's core schema, landed Phase 17
pkg/                code meant for import by other services — empty until something needs it
```

Run locally:

```
cd apps/api && go run ./cmd/api
curl localhost:8080/health
curl localhost:8080/ready
```

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
