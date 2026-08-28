# E2E Testing (Phase 32, §57)

`apps/e2e/scenario_test.go` is one continuous automated test that walks
§57's full funnel against real running services — no mocks, no in-process
`httptest` server standing in for the real binaries:

```
Create organization -> Create source -> Create network -> Create offer ->
Create landing -> Create campaign -> Create Stream Set (+ nested flow +
filter) -> Enter cost -> Generate tracking URL -> Click -> Route ->
Record event -> Receive conversion (HOLD -> ACCEPT -> REDEP) ->
Attribute conversion -> Send postback -> Analytics + LTV
```

Every step is verified against real state: real HTTP responses from
`apps/api`/`apps/tracker`, a real 302 redirect, real rows in Postgres and
ClickHouse, real attribution and revenue numbers.

## Running it

The test expects `apps/api`, `apps/tracker`, and `apps/worker` to already
be running against real Postgres/ClickHouse/Redis — the same local dev
workflow used for every other manual/browser validation pass in this
project, not something the test spawns itself:

```
# infra
docker compose -f infra/docker-compose.dev.yml up -d postgres clickhouse redis

# from apps/, one terminal each
DATABASE_URL=... CLICKHOUSE_URL=http://localhost:8123 CLICKHOUSE_DATABASE=flox \
  CLICKHOUSE_USER=flox CLICKHOUSE_PASSWORD= REDIS_URL=redis://localhost:6379 \
  APP_URL=http://localhost:3000 API_URL=http://localhost:8080 go run ./api
TRACKER_URL=http://localhost:8081 ... go run ./tracker
WORKER_URL=http://localhost:8082 ... go run ./worker

# then, from apps/
FLOX_E2E_API_URL=http://localhost:8080 \
  FLOX_E2E_TRACKER_URL=http://localhost:8081 \
  FLOX_E2E_APP_URL=http://localhost:3000 \
  DATABASE_URL=... CLICKHOUSE_URL=http://localhost:8123 \
  CLICKHOUSE_DATABASE=flox CLICKHOUSE_USER=flox CLICKHOUSE_PASSWORD= \
  go test ./e2e/... -run TestFullFunnel -v
```

If `DATABASE_URL`/`CLICKHOUSE_URL` are unset, or `apps/api`/`apps/tracker`
aren't answering `/health`, the test `t.Skip`s with a clear message —
`go test ./...` stays green in an environment that never started them
(exactly like every other `CLICKHOUSE_URL`-gated benchmark added in
Phase 31).

Fixtures clean up in `t.Cleanup`, on both pass and failure: one
`DELETE FROM organizations WHERE id = ...` (every table this scenario
touches cascades from `organizations`) plus explicit `ALTER TABLE ...
DELETE WHERE organization_id = ...` against the ClickHouse tables (no
FK/cascade there — same pattern `internal/analytics/bench_test.go`
already established in Phase 31).

## Known product gap: no tracking-URL API

There is no HTTP endpoint anywhere in `apps/api` for "generate a tracking
URL" — no `/domains` or `/tracking-links` routes exist. The only place
that has ever written the `domains`/`tracking_links` tables (migration
`00007`) is `apps/cmd/loadtestseed`'s raw SQL, built for Phase 31's load
test. `apps/web` has no UI for this yet either.

The scenario test works around this the same way loadtestseed does —
seeding the two rows directly over its own Postgres connection
(`scenario.generateTrackingURL`) — rather than expanding this testing
phase into building the missing endpoint. Building a real
`POST /campaigns/{id}/tracking-links` (or similar) is real, scoped
backend work for its own phase.

## Design notes / gotchas this test had to work around

- **`event_queue` is a work queue, not a log.** `apps/worker`'s flusher
  claims rows (`status='processing'`) and deletes them once ClickHouse
  accepts them. An earlier version of this test polled Postgres
  `event_queue` for the click's id as a "fast path" before falling back to
  ClickHouse — that raced the worker's own poll loop in practice (a row
  visible at one instant could be gone, already flushed, by the next) and
  produced a real intermittent failure. The test now polls **ClickHouse
  only** for the click, which is also the only state that matters
  downstream (`apps/api`'s attribution resolver reads clicks from
  ClickHouse, never Postgres).
- **A stream set's `rootFilter` must be a `group`, not a bare
  `condition`** — `apps/internal/streamset`'s create validation rejects a
  top-level condition (`"root filter must be a group, not a condition"`).
  Found by actually running the test against the real handler, not by
  reading the code.
- **The filter matches on `bot IS "0"`, not `country`.** This dev
  environment has no real geo/ASN vendor wired up
  (`classifier.New(nil, nil, nil)`), so a country-based filter never
  matches real requests here — the same gap Phase 31's load test
  documented (every load-test click fell through to the campaign
  fallback). `bot` is reliably `"0"` under the heuristic detector for any
  UA that isn't a known crawler signature, so it's what this scenario (and
  `loadtestseed`) use to get a deterministic, non-fallback route.
- **LTV cohort `to=<date>` bug, found and fixed this phase.**
  `apps/internal/chstore`'s `LTVFilter` query is a half-open
  `event_at >= from AND event_at < to` range. The LTV handler parsed a
  bare date-only `to` as that date's midnight, so `?to=<today>` silently
  excluded every same-day cohort anchor — including the one this scenario
  itself creates. `apps/internal/conversions`' handler already carries the
  correct fix (add `23:59:59.999` to a same-day `to`) for the identical
  reason; `apps/internal/ltv/handler.go`'s `parseParams` now does the
  same. The scenario test's `?from=today&to=today` LTV assertions are this
  fix's regression coverage.
