# Observability (§53, Phase 29)

Structured logging (`apps/internal/logging`, `slog` JSON) and OpenTelemetry
tracing (`apps/internal/telemetry`) already existed before this phase —
this phase's real gap was Prometheus metrics: no client library, no
`/metrics` endpoint, none of §53's nine tracked metrics existed in any
form. This phase closes that gap, adds a request ID to `apps/tracker`
(the one binary that didn't have one), and instruments `apps/worker`'s
one outbound HTTP call (postback delivery) for tracing.

## Scope decisions

- **`prometheus/client_golang`, not the OTel metrics SDK** (confirmed via
  `AskUserQuestion`). A plain `/metrics` endpoint any Prometheus scrapes
  directly — simpler than routing through `otel/sdk/metric` just to
  export as Prometheus anyway, and this project's existing OTel tracing
  setup is unaffected either way (tracing and metrics are separate OTel
  signals; nothing here touches `apps/internal/telemetry`).
- **Prometheus added to `infra/docker-compose.dev.yml`** (confirmed via
  `AskUserQuestion`), so the new `/metrics` endpoints can be verified with
  a real scrape rather than curl alone — matching this project's own
  pattern of verifying against real infrastructure. No Grafana; a running
  Prometheus server with working scrape targets was the goal, not a
  dashboard.
- **Deeper custom tracing spans are an explicit, separate follow-up, not
  done here.** `apps/api` already gets a span per inbound request via
  `otelhttp.NewHandler` (pre-existing); this phase adds one more —
  `apps/worker`'s postback delivery client (`otelhttp.NewTransport`) — but
  does not add manual child spans inside `routing`, `conversion`, or
  other packages. §53 asked for "trace IDs" as a capability, which
  already existed on `apps/api` and is now closed everywhere that matters
  for this pass; hand-instrumenting individual decision points inside
  business logic is real, valuable work but a distinctly larger, open-
  ended task than "give every binary a `/metrics` endpoint and wire up
  the nine named metrics."
- **`apps/tracker` still has no per-request logging middleware, and still
  isn't wrapped in `otelhttp`.** Both were deliberate decisions from
  Phase 21 (§41, CLAUDE.md non-negotiable #9: p50 < 20ms / p95 < 50ms) and
  this phase doesn't relitigate them — see "What `apps/tracker` still
  doesn't have, on purpose" below.

## The nine tracked metrics

All defined once in `apps/internal/metrics/metrics.go`, `flox_` namespace,
`promauto`-registered against each process's own default registry (three
separate OS processes — `apps/api`, `apps/tracker`, `apps/worker` — so a
metric never mixes across binaries; Prometheus's own `job` scrape label
is what tells them apart on a dashboard).

| §53 name | Metric(s) | Recorded in |
|---|---|---|
| `tracking_requests` | `flox_tracking_requests_total{outcome}` | `apps/tracker/handler.go`'s `track()` — outcome is `redirected`/`blocked`/`not_found`/`error` |
| `tracking_latency` | `flox_tracking_latency_seconds` | same, whole-handler wall time |
| `routing_latency` | `flox_routing_latency_seconds` | `internal/routing.Engine.Resolve` — recorded wherever it's called (the hot path, and `apps/api`'s `/routing/simulate`, each in their own process) |
| `event_processing_latency` | `flox_event_processing_latency_seconds` | `eventqueue.Flusher.RunOnce` — the ClickHouse `InsertBatch` call only |
| `event_queue_depth` | `flox_event_queue_depth` | `eventqueue.PollDepth`, a new goroutine in `apps/worker/main.go`, polling `PostgresQueue.Depth` (new method) every 15s |
| `event_loss` (enqueued vs persisted) | `flox_events_enqueued_total`, `flox_events_buffer_dropped_total`, `flox_events_queue_written_total`, `flox_events_queue_write_failed_total` (all `apps/tracker`, from `eventbuf.Writer.Stats()`) **+** `flox_events_persisted_total`, `flox_events_requeued_total` (`apps/worker`, from `eventqueue.Flusher`) | See "Why event_loss is two counters, not one" below |
| `postback_success` / `postback_failure` | `flox_postback_deliveries_total{outcome}` | `postback.Deliverer.logAttempt` — outcome is `success`/`retrying`/`dead` |
| `analytics_latency` | `flox_analytics_query_latency_seconds{endpoint}` | `analytics.Service.CampaignDaily`/`CampaignDailyRevenue` — the ClickHouse round trip only, `endpoint` is `campaign_daily`/`campaign_daily_revenue` |

### Why `event_loss` is two counters, not one

Standard Prometheus practice: store raw monotonic counters, let a
dashboard/alert derive rates and differences via PromQL — never
pre-subtract and store a single "loss" value, since that requires one
process to know both sides' counts, and here the two sides live in
*different binaries* (`apps/tracker` enqueues; `apps/worker` persists).
`rate(flox_events_enqueued_total[5m]) - rate(flox_events_persisted_total[5m])`
(summed across instances) is the query; nothing here computes it directly.

`apps/tracker`'s four counters aren't incremented at extra call sites —
`RegisterEventBufStats` (`apps/internal/metrics/eventbuf.go`) exposes
`eventbuf.Writer`'s own pre-existing atomic `Stats()` counters as
Prometheus `CounterFunc`s, read at scrape time. `eventbuf`'s internals are
untouched; there's exactly one source of truth for these four numbers.
Two of the four are genuine, permanent loss (`events_buffer_dropped_total`
— the in-memory channel was full; `events_queue_write_failed_total` — the
batch failed writing into Postgres `event_queue`, and `eventbuf.Writer`
does not retry a failed sink write, confirmed by reading `write()`
directly). `apps/worker`'s `events_requeued_total` is *not* loss — a
failed ClickHouse insert requeues the whole batch (`eventqueue.Flusher`
has no dead-letter state; it retries forever until ClickHouse recovers).

### Why `postback_deliveries_total` has three outcome values, not two

§53 names exactly two metrics, `postback_success`/`postback_failure`.
`success` maps directly. The single "failure" bucket is split into
`retrying` (will be attempted again — `postback.Deliverer` hasn't given
up) and `dead` (exhausted `MaxAttempts` — genuinely, permanently lost)
because that distinction already exists in `postback.DeliveryStatus` and
collapsing it at the metric layer would throw away information a
dashboard can always re-derive (`sum by (outcome) (... {outcome!="success"})`
for "true failure rate") but never recover once the finer label is gone.

## What `apps/tracker` gained

- **`middleware.RequestID` + an `X-Request-Id` response header** (a
  6-line `echoRequestID`, duplicated from `apps/internal/httpserver`
  rather than imported — that package is documented as apps/api's own
  router builder, and importing it into `apps/tracker` for one helper
  would be a heavier coupling than repeating six lines). This is *not*
  the per-request logging middleware `apps/api` uses — `RequestID` is a
  `context.WithValue` and one header write, categorically different cost
  from a synchronous `slog.Logger.Info` call per click, which is exactly
  what Phase 21's own comment on this router already ruled out ("a second
  synchronous log line per click would be duplicate work on the hot
  path"). Verified live: `curl`ing a real tracking link returns a real
  `X-Request-Id: rage/H0PCA9Zp0w-000007`-shaped header (see Verified,
  below).
- **`GET /metrics`**, alongside the existing bare `/health`.
- **`tracking_requests_total`/`tracking_latency_seconds`**, measured
  around the entire `track()` handler via a `defer`, with `outcome` set
  at each of the handler's four exit points.

## What `apps/tracker` still doesn't have, on purpose

No per-request logging middleware, and no `otelhttp.NewHandler` wrap on
its router. Both would add real synchronous cost to every click — a
tracing span (attribute recording, eventual batch export) and a log line
are not free the way a `prometheus.Counter.Inc()` (an atomic add, no
allocation, no I/O) is. §41's latency budget (CLAUDE.md non-negotiable
#9: tracking p50 < 20ms / p95 < 50ms) is the reason Phase 21 skipped
per-request logging here in the first place; this phase's metrics
additions respect that same budget by construction (every hot-path
metric call here is an in-memory counter/histogram observation, nothing
that blocks on a network write or synchronous log flush) rather than by
exception.

## `apps/worker`: queue depth + traced postback delivery

`apps/worker/main.go` gained a fifth background goroutine,
`eventqueue.PollDepth`, on the same `time.Ticker` shape as
`costsync.Scheduler.RunLoop` — a plain `SELECT count(*) FROM event_queue
WHERE status IN ('queued', 'processing')` every 15s (a new
`PostgresQueue.Depth` method; counting `'processing'` too, not just
`'queued'`, so the gauge doesn't dip to zero mid-flush during a slow
ClickHouse insert).

`postback.NewDeliverer`'s `http.Client` is now `&http.Client{Transport:
otelhttp.NewTransport(http.DefaultTransport)}` — each outgoing postback
delivery attempt becomes its own trace (a root span; there's no inbound
HTTP request to inherit a trace context from, since this fires from a
poll loop, not a handler). A no-op when no OTel endpoint is configured,
same as every other use of `apps/internal/telemetry` in this codebase.

`apps/worker`'s bare `http.HandlerFunc` for `/health` became a small
`http.ServeMux` with `/health` and `/metrics`, since a `HandlerFunc`
alone can only serve one path.

## Dev-stack: Prometheus in `infra/docker-compose.dev.yml`

A new `prometheus` service (image `prom/prometheus:v3.1.0`), config at
`infra/prometheus.yml`, scraping all three binaries every 15s.
`apps/api`/`apps/tracker`/`apps/worker` run on the **host** via `go run`
(this repo's whole dev workflow — `docker-compose.dev.yml` has only ever
run the data stores), not as containers in this compose network, so the
scrape targets are `host.docker.internal:8080`/`:8081`/`:8082` —
matching `API_URL`/`TRACKER_URL`/`WORKER_URL`'s defaults in
`.env.example`. `extra_hosts: ["host.docker.internal:host-gateway"]` on
the `prometheus` service is what makes that hostname resolve on Linux
Docker Engine (Docker Desktop already provides it and ignores the extra
entry harmlessly). Prometheus's own UI is at `http://localhost:9090`.

## Verified

`gofmt`/`go build ./...`/`go vet ./...`/`go test ./...` green across the
whole repo, including two new test files: `apps/internal/metrics/metrics_test.go`
(the `/metrics` handler serves all nine metric families; `RegisterEventBufStats`
against a real `eventbuf.Writer` produces exactly the counter values its
`Stats()` reports) and a new `TestDepthReflectsQueuedAndProcessingNotDeleted`
in `apps/internal/eventqueue/postgres_test.go` (enqueue/claim/delete each
move `Depth()` by the expected amount, and a claimed-but-undeleted
`'processing'` row still counts).

Full manual pass: started `apps/api`+`apps/tracker`+`apps/worker` on
their default ports against the real dev Postgres/ClickHouse/Redis stack,
plus `docker compose -f infra/docker-compose.dev.yml up -d prometheus`.
Confirmed via `GET /api/v1/targets` that Prometheus saw all three
`host.docker.internal` scrape targets as `"health":"up"` — the real risk
in this phase's docker-compose change, and it worked on the first try.

Then generated genuine traffic for every one of the nine metrics and
confirmed each one via Prometheus's own query API (`/api/v1/query`), not
just a local `curl /metrics`:

- Seeded one real org/traffic source/campaign/domain/tracking link
  directly in Postgres, then hit the real tracking link three times and
  one unknown slug once: `flox_tracking_requests_total{outcome="redirected"}
  = 3`, `{outcome="not_found"} = 1`, `flox_tracking_latency_seconds_count
  = 4`, `flox_routing_latency_seconds_count = 3` (correctly excludes the
  not-found request, which never reaches the routing engine) — each real
  redirect response also carried a genuine `X-Request-Id` header.
- The same three clicks flowed through the real pipeline end to end:
  `apps/tracker`'s `flox_events_enqueued_total = 3` and
  `flox_events_queue_written_total = 3`, then (after the worker's poll
  loop picked them up) `apps/worker`'s `flox_events_persisted_total = 3`
  and `flox_event_processing_latency_seconds_count = 1` (one batch), with
  `flox_event_queue_depth` back to `0` once drained — verified as three
  *separately job-labeled* series in one Prometheus query, proving the
  "two counters in two processes" design in `docs/observability.md`'s own
  section above actually works, not just compiles.
- Inserted one real `postback_deliveries` row pointing at an
  intentionally unreachable URL (`http://127.0.0.1:1/...`, connection
  refused). The worker's `Deliverer` picked it up, and
  `flox_postback_deliveries_total{outcome="retrying"} = 1` matched the
  row's own `delivery_status = 'retrying'` in Postgres exactly.
- Signed up a real user (Phase 28's real auth) and called
  `GET /analytics/campaigns/{id}/daily` with the session cookie:
  `flox_analytics_query_latency_seconds_count{endpoint="campaign_daily"} = 1`.
- No unexpected errors in any of the three binaries' logs throughout
  (the postback delivery's own "connection refused" was the intended,
  induced failure).
- Cleanup: deleted every fixture this pass created (the org, its
  cascaded traffic source/campaign/domain/tracking link/network/postback
  rows, and the signed-up user/org), confirmed a zero count on every
  `LIKE 'Metrics%'` pattern afterward, then stopped all three manually-
  started binaries.

## Alerting (§61, Phase 33)

`infra/alerts.yml` adds real Prometheus alerting rules over these same
nine metrics — `TrackingLatencyBudgetExceeded` (`histogram_quantile`
against CLAUDE.md non-negotiable #9's own 50ms p95 budget),
`EventLossDetected`/`EventQueueBacklogGrowing`,
`PostbackDeliveryDeadLetterRateHigh`, `AnalyticsQueryLatencyHigh`, and
three `up == 0` service-down rules. `infra/prometheus.yml` loads them via
`rule_files:` — validated with `promtool check rules infra/alerts.yml`
and confirmed loaded via a real dev-stack Prometheus's own
`/api/v1/rules`. See `docs/deployment.md`'s monitoring section for the
production story, including why no Alertmanager ships in this repo.
