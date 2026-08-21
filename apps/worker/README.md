# apps/worker

Go async background processor — separate binary, same Go module as
`apps/api`/`apps/tracker`, standing up starting Phase 24.

```
main.go   wiring: config, logging, telemetry, DB pool, health endpoint,
          the postback delivery poll loop
```

## What's here (Phase 24, §46)

Outgoing postback delivery only: `internal/postback.Deliverer.PollLoop`
claims due rows from `postback_deliveries` (a network's status-change
notification, macro-resolved and enqueued by `internal/conversion` on every
successful incoming conversion — see `docs/postback-delivery.md`) and
dispatches them, with exponential backoff and a dead-letter state after
`postback.MaxAttempts` failures. Never blocks the tracker's incoming
postback response — see CLAUDE.md non-negotiable #9's spirit, which this
phase applies to outbound partner calls generally, not only the redirect
path.

## Run locally

```
docker compose -f ../../infra/docker-compose.dev.yml up -d postgres
cd apps && go tool goose -dir api/migrations postgres "$DATABASE_URL" up
go run ./worker      # listens on WORKER_URL's port (health only), default :8082
```

## Not yet this binary's job

Consuming the tracker's event queue and persisting to ClickHouse — §43's
pipeline (Tracker → Event Queue → Worker → ClickHouse) is real, but the
queue and ClickHouse's own schema arrive in later phases (§47 Analytics
Pipeline / §48 ClickHouse). `apps/tracker` still hands events to
`eventbuf.LogSink` until then. Per CLAUDE.md's "work strictly one phase at a
time," this binary's scope grows when those phases land, not ahead of them.
