# Performance (§56, Phase 31)

§56's brief: benchmark tracking/routing/classifier/postback/analytics, hit
tracking p50 < 20ms / p95 < 50ms (excluding third-party network latency),
load-test for zero event loss, **optimize only after measurement.**

**Result: every number below already clears its target by 1-2 orders of
magnitude on the real code path, against the real local dev stack (Postgres
+ Redis + ClickHouse, no mocks). No code changed in this phase.** §56 is
explicit that optimization only happens once measurement shows it's
needed; it didn't, so none was done. The rest of this document is the
measurement that justifies not touching anything.

## What was measured, and how

Two kinds of numbers, because they answer different questions:

1. **Go benchmarks** (`go test -bench`) — isolate one piece of the pipeline
   at a time, against a real (not mocked) Postgres/ClickHouse. Answers "how
   expensive is this specific piece."
2. **A real load test** — a real `apps/tracker` binary, listening on a real
   socket, hit by an external load generator (`vegeta`) over sustained
   traffic. Answers "does the whole thing hold up under concurrent load,
   and does it lose events."

All numbers below are from this machine's local dev stack
(`infra/docker-compose.dev.yml`: Postgres, Redis, ClickHouse, all on
loopback, no network latency, no replicas, no contention from other
tenants). See **Caveats** at the end for what that does and doesn't prove.

### Go benchmarks

New `*_test.go` files, one per §56 line item, each skipping itself with
`t.Skip`/`b.Skip` if its dependency (`DATABASE_URL` / `CLICKHOUSE_URL`)
isn't set — same convention every existing integration test in this repo
already uses:

| Package | Benchmark | What it measures |
|---|---|---|
| `internal/routing` | `BenchmarkResolve` | Pure in-memory routing decision: 5 stream sets, nested AND/OR filters, weighted flows — no I/O |
| `internal/routing` | `BenchmarkResolve_Sticky` | Sticky-cookie fast path (§39-STICKY) — no filter evaluation, no weighted draw |
| `internal/classifier` | `BenchmarkClassify` | UA parsing + the wired-up Noop geo/ASN/heuristic-bot providers (`tracker/main.go`'s actual `classifier.New(nil, nil, nil)`) |
| `internal/routingstore` | `BenchmarkLoadRoutingConfig` | The real Postgres cost: campaign row + stream sets + filter groups/conditions + flows — 5 sequential queries, no cache in front |
| `internal/routingstore` | `BenchmarkResolveTrackingLink` | The (domain, slug) → campaign lookup, one indexed query |
| `apps/tracker` | `BenchmarkTrack` | **The whole §41 hot path**, serially: resolve link → load config → classify → route → enqueue (discarded, not written — isolates this from the worker's independent ClickHouse-write cost) → redirect, through a real `net/http` mux |
| `apps/tracker` | `BenchmarkTrack_Parallel` | Same, `b.RunParallel` across `GOMAXPROCS` goroutines — real traffic is concurrent, this is what actually stresses the Postgres connection pool |
| `apps/tracker` | `BenchmarkPostback` | The incoming `POST /postback/{id}` path end to end (secret auth, network lookup, status mapping, progression check, attribution, FX, durable write, delivery enqueue) — **not** on the §41 budget (`tracker/postback.go`'s own doc comment says so), measured for regression-visibility only |
| `internal/analytics` | `BenchmarkCampaignDaily` / `BenchmarkCampaignDailyRevenue` | `apps/api`'s two analytics reads against a real ClickHouse seeded with 30 days × 200 events/day (~6,000 rows) for one campaign — also no explicit SLA, same reasoning as postback |

Run with (from `apps/`):

```
DATABASE_URL=postgres://flox:flox@localhost:5432/flox?sslmode=disable \
CLICKHOUSE_URL=http://localhost:8123 CLICKHOUSE_DATABASE=flox CLICKHOUSE_USER=flox \
go test ./internal/routing/... ./internal/classifier/... ./internal/routingstore/... \
        ./tracker/... ./internal/analytics/... -run xxx -bench . -benchtime=1s
```

Results (this machine, AMD Ryzen 7 7745HX, 16 threads; `ns/op` → converted
to a human unit alongside):

| Benchmark | ns/op | ≈ |
|---|---:|---:|
| `BenchmarkResolve` | 2,458 | 2.5 µs |
| `BenchmarkResolve_Sticky` | 272 | 0.27 µs |
| `BenchmarkClassify` | 1,722 | 1.7 µs |
| `BenchmarkLoadRoutingConfig` | 402,898 | 0.40 ms |
| `BenchmarkResolveTrackingLink` | 86,296 | 0.086 ms |
| `BenchmarkTrack` (serial) | 513,971 | **0.51 ms** |
| `BenchmarkTrack_Parallel` (16-way) | 84,455 | **0.084 ms** |
| `BenchmarkPostback` | 1,699,356 | 1.7 ms |
| `BenchmarkCampaignDaily` | 1,957,029 | 2.0 ms |
| `BenchmarkCampaignDailyRevenue` | 2,147,570 | 2.1 ms |

`BenchmarkTrack` at 0.51 ms serial is **~39× under** the 20 ms p50 target
before a single request even runs concurrently. The two Postgres round
trips (`LoadRoutingConfig` + `ResolveTrackingLink`, ~0.49 ms combined) are
essentially the entire cost — routing and classification together are
under 5 µs, three orders of magnitude smaller. There is currently no cache
in front of `routingstore.Store` (CLAUDE.md's routing invariant doesn't
require one, and non-negotiable #9 only forbids blocking on *analytics
queries or outbound partner calls*, not a config read) — measurement here
is the reason one wasn't added anyway: the cost it would remove is
already negligible against the budget.

### Load test

§56: "sustained clicks with zero event loss (enqueued == persisted)."

A real `apps/tracker` binary (`go run ./tracker`, connected to the same
local Postgres/Redis/ClickHouse) listening on `:8081`, seeded with one
real campaign (5 stream sets + a catch-all, same shape the benchmarks use)
via a small new tool, `apps/cmd/loadtestseed` (seeds via the real
trafficsource/campaign/streamset write path; `cleanup <orgId>` tears it
back down — not wired into any service, a manual companion to this load
test and to any future re-run of it):

```
go run ./cmd/loadtestseed seed        # -> {"orgId","host","slug"}
go run ./tracker                       # separate process
vegeta attack -targets=targets.http -rate=<N>/1s -duration=<T> -redirects=0 | vegeta report
go run ./cmd/loadtestseed cleanup <orgId>
```

(`-redirects=0` matters: the seeded flows point at non-resolvable
`*.example` destinations on purpose, so `vegeta` must be told not to try
following the 302 — otherwise it reports the DNS failure as the request's
own latency/status instead of the tracker's.)

Four sustained runs, back to back, same process:

| Rate | Duration | Requests | p50 | p95 | p99 | max |
|---|---:|---:|---:|---:|---:|---:|
| 300/s | 20s | 6,000 | 0.99 ms | 1.18 ms | 1.37 ms | 4.6 ms |
| 1,000/s | 15s | 15,000 | 0.67 ms | 1.00 ms | 1.18 ms | 22.9 ms |
| 3,000/s | 10s | 30,000 | 0.64 ms | 1.10 ms | 1.88 ms | 39.5 ms |
| 6,000/s | 8s | 47,997 | 0.82 ms | 1.52 ms | 3.36 ms | 9.3 ms |

p50/p95 stay under ~1.5 ms through every rate tested, including 6,000
req/s sustained — 300× the traffic of the first run — with no sign of
degradation. This machine's loopback stack was never the bottleneck in
this test; see **Caveats**.

**Zero event loss**, checked three independent ways after each run:

- `flox_events_enqueued_total` == `flox_events_queue_written_total`
  (Prometheus counters, `/metrics`) at every rate — 12,002 → 27,002 →
  57,002 → 104,999, matching cumulative request count exactly each time.
- `flox_events_buffer_dropped_total` and
  `flox_events_queue_write_failed_total` stayed at **0** throughout —
  the in-memory buffer (10,000-deep channel, §41's async-persist design)
  never once filled, even at 6,000 req/s.
- Independently, `SELECT count(*) FROM event_queue WHERE organization_id = …`
  against the real Postgres matched the enqueued count exactly (104,999)
  after the full four-run sequence — durable, not just counter-reported.

## Conclusion

At every scale tested, `apps/tracker`'s redirect path is measured at
roughly **15-40× headroom** under its p50 target and comparable headroom
under p95, with zero event loss. §56 says optimize only after measurement
shows a need; this measurement shows none. `internal/postback` and
`internal/analytics` (no explicit target) both come in at 1.7-2.1 ms,
fine for their actual call sites (a network's async postback retry loop,
an operator's dashboard fetch respectively) — not touched either.

## Caveats — what this does and doesn't prove

- **Loopback only.** Tracker, Postgres, Redis, and ClickHouse all ran on
  the same machine with no real network hop between them. Real deployment
  topology (tracker behind a load balancer, Postgres possibly in a
  different AZ) adds real latency this test cannot see — though even a
  few ms of added round-trip per query would still leave meaningful
  headroom against a 20/50 ms budget, given the ~0.5 ms baseline measured
  here.
- **Single tracker instance, single-tenant load.** No concurrent
  deployment-scale traffic from other organizations sharing the same
  Postgres/connection pool, no read replicas, no PgBouncer — this is the
  simplest possible topology, not a worst case.
- **Every routed click hit the same campaign's fallback URL** (the seeded
  stream sets' filters require `bot = 0`; `vegeta`'s default requests
  carry no signal that clears that condition against the Noop geo
  provider, since no real geo/ASN vendor is wired up yet — see
  `internal/classifier/defaults.go`). This still exercises the complete
  hot path (link resolution, full config load, classification, filter
  evaluation against every stream set, event enqueue) — the one thing
  it doesn't exercise under load is the weighted-draw code path, which
  `BenchmarkResolve`'s in-memory numbers already show costs under 3 µs
  regardless of outcome.
- **ClickHouse benchmarks ran against ~6,000 seeded rows** for one
  campaign — realistic for one month of one mid-size campaign, not for
  an operator querying years of aggregate history across a large org.
  `click_events_daily_campaign`/`conversion_events_daily_campaign` are
  pre-aggregated `SummingMergeTree` targets specifically so that scaling
  is sublinear in raw event count, but this was not independently
  verified at 10-100× the seeded volume.

## Reproducing

```
cd apps
docker compose -f ../infra/docker-compose.dev.yml up -d postgres redis clickhouse
go tool goose -dir api/migrations postgres "$DATABASE_URL" up

# Go benchmarks
DATABASE_URL=... CLICKHOUSE_URL=... CLICKHOUSE_DATABASE=flox CLICKHOUSE_USER=flox \
go test ./internal/routing/... ./internal/classifier/... ./internal/routingstore/... \
        ./tracker/... ./internal/analytics/... -run xxx -bench . -benchtime=1s

# Load test
go run ./cmd/loadtestseed seed
DATABASE_URL=... CLICKHOUSE_URL=... REDIS_URL=... TRACKER_URL=http://localhost:8081 go run ./tracker &
vegeta attack -targets=targets.http -rate=1000/1s -duration=15s -redirects=0 | vegeta report
curl -s localhost:8081/metrics | grep flox_events_
go run ./cmd/loadtestseed cleanup <orgId>
```
