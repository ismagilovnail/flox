# apps/tracker

Go hot-path click/redirect service (§41, Phase 21). A separate binary from
`apps/api` so it can be deployed and scaled independently, but part of the
same Go module (rooted at `apps/`) so it imports the *same*
`apps/internal/routing` and `apps/internal/classifier` as the API — there
is exactly one implementation of routing and classification in the system
(CLAUDE.md non-negotiable #1).

```
main.go      wiring: config, logging, telemetry, DB pool, event writer, HTTP server
handler.go   GET /t/{trackingID} — the §41 critical path
params.go    pass-through attribution params (utm_*, sub1..10, fbclid/ttclid)
sticky.go    sf_{campaignId} cookie parse/write (§39-STICKY)
```

## Critical path

```
HTTP request → parse → classify → route → record async → 302 redirect
```

Nothing on this path runs an analytics query or waits on event
persistence. Events go through `internal/eventbuf`, whose `Enqueue` is a
single non-blocking channel send — when the buffer is full events are
dropped (counted and logged, never silent) rather than making a user wait
on a redirect.

Measured on the dev stack: **p50 1.1ms, p95 1.4ms** against the §56 budget
of p50 < 20ms / p95 < 50ms.

## Run locally

```
docker compose -f ../../infra/docker-compose.dev.yml up -d postgres
cd apps && go tool goose -dir api/migrations postgres "$DATABASE_URL" up
go run ./tracker      # listens on TRACKER_URL's port, default :8081
```

A tracking link resolves by **host + slug**, not slug alone —
`tracking_links` is unique on `(domain_id, slug)`, so two organizations may
each own the slug `summer` on their own domain. Looking up by slug alone
would be a cross-tenant data leak. For local testing, seed a `domains` row
with `domain = 'localhost'`.

## What this service deliberately does not do

- **Persist durably.** §43's pipeline is Tracker → Event Queue → Worker →
  ClickHouse. The queue and its consumer arrive with `apps/worker`
  (Phase 24). Until then the event sink is an honest structured-log writer
  — swapping it is a one-line change in `main.go`, because everything
  upstream only sees the `eventbuf.Sink` interface.
- **In-app WebView bounce (§73).** A pre-routing redirect based on
  User-Agent, and a required, provider-neutral capability — but it belongs
  with the PWA install funnel, not the first cut of the redirect path.
- **Serve landing/PWA/postlanding stages.** The routing engine already
  selects a flow that names them; actually serving them is later work.
