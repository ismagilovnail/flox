# Analytics Pipeline (Phase 25, §47)

`events -> ClickHouse -> materialized/aggregate tables -> analytics service
-> REST API -> frontend`. This phase builds every stage except the last —
wiring the actual Next.js UI to a real backend is Phase 27's explicit job
("Frontend/backend integration" per ROADMAP.md), consistent with this
project's frontend-first-on-mocks build order; `apps/web` stays on mocks
until then.

## Scope boundary with Phase 26, decided up front

§47's pipeline diagram names ClickHouse as one of its own stages, but the
*actual* table design (five tables — click_events/tracking_events/
conversion_events/cost_events/postback_events — with dimension-specific sort
keys, partitioning, and per-campaign+day/per-GEO+day materialized views) is
§48's stated content, in a phase of its own titled "ClickHouse." Building
both at once would blur a phase boundary the spec itself draws; confirmed
with the user before starting: **Phase 25 builds a minimal, single-table
schema just sufficient to prove the pipeline end to end, and Phase 26
replaces it wholesale** with the real design. This is a deliberate, known
rework — not an oversight — recorded here so it isn't mistaken for one when
Phase 26 drops this table.

The minimal schema still honors the invariants that aren't phase-scoped:
`events`' `type` column covers the full ~20-value event model (CLAUDE.md
#2 — the type enum is never truncated, even in a table meant to be
replaced), and `organization_id` leads every sort key (CLAUDE.md #5).

## The queue: `event_queue` (Postgres)

STACK has no message broker, so the queue in "Tracker -> Event Queue ->
Worker -> ClickHouse" is Postgres — the same `FOR UPDATE SKIP LOCKED`
job-queue pattern `postback_deliveries` (Phase 24) already established,
applied here to click/tracking events instead of outgoing notifications.
One difference from that queue: a row here is **deleted**, not marked
terminal and kept, once its batch is durably in ClickHouse — `event_queue`
is disposable transit, not an audit ledger; the ClickHouse row is the record
from that point on.

`payload` is the whole `event.Event`, JSON-encoded, rather than one column
per field. `event.Event` already carries `json` tags on every field for
exactly this, the worker only ever deserializes the whole thing (never
queries into individual fields), and a wide explicit-column table here would
just be ClickHouse's real schema (Phase 26) designed a second time for a
table that gets emptied continuously.

`apps/tracker`'s `eventbuf.Writer` now uses `eventqueue.Sink` in place of
the `eventbuf.LogSink` stand-in it ran with through Phase 24 — the one-line
swap that design always promised, since everything upstream only ever saw
the `eventbuf.Sink` interface.

## The worker: `internal/eventqueue.Flusher`

Claims a batch (up to 500 events, `apps/worker/main.go`'s
`eventPollBatchSize`), attempts ONE ClickHouse batch insert for the whole
claimed set, and either deletes the batch (success) or requeues the whole
thing with a fixed 10s delay (failure). Batching the insert, not just the
claim, is what makes this viable at click volume — clickhouse-go's batch API
sends one block per `Send()` call.

No dead-letter state here, unlike outgoing postbacks: a delayed analytics
batch has no per-item deadline the way a lost conversion does, so
`Flusher` retries the same batch every `flushRetryDelay` until ClickHouse
recovers rather than giving up. This is a deliberately different tradeoff
from `internal/postback`'s `MaxAttempts`/dead-letter, not an inconsistency —
see that package's own doc for why postbacks DO need a give-up point.

## ClickHouse over HTTP, not native protocol

`infra/docker-compose.dev.yml` exposes only ClickHouse's HTTP interface
(port 8123) to the host — the native protocol port stays container-internal
to avoid colliding with MinIO's API port. `internal/chconn` always dials
`clickhouse.HTTP`, unlike some reference implementations that default to the
native protocol; get this wrong and every connection attempt fails with a
connection-refused on 9000, not an auth error.

## Schema application: idempotent DDL, not a migration framework

`internal/chstore.Migrate` applies `schema/*.sql` in lexical order, every
statement `CREATE ... IF NOT EXISTS`, with no version-tracking table. Both
`apps/api` and `apps/worker` call it at startup — safe because it's
idempotent, and necessary because there's no ordering guarantee about which
one starts first in local dev. This is a deliberate simplification matched
to one disposable table, not a pattern to keep: once Phase 26's schema is
meant to last, it gets a real migration tool (or at minimum a version table)
the way Postgres already has with goose.

## The one aggregate: `events_daily_campaign`

A `SummingMergeTree` fed by a `MATERIALIZED VIEW` that fires synchronously
on every `INSERT INTO events` (verified in `chstore_test.go` — no polling
needed to observe it). Proves the "materialized/aggregate tables" pipeline
stage with one rollup (`organization_id, campaign_id, type, day`) rather
than the full per-campaign+day/per-GEO+day set §48 specifies — that
dimension coverage is Phase 26's job, once the real five-table schema exists
to aggregate from.

Querying it requires `SUM(event_count)`, not a plain read: `SummingMergeTree`
merges same-key rows only in the background, so a query that trusted "one
row per key already holds the final total" would undercount whenever a
merge hasn't run yet.

## The analytics service and REST API

`internal/analytics` is deliberately one query, one endpoint —
`GET /analytics/campaigns/{campaignId}/daily?from=&to=` on `apps/api` —
proving the pipeline's last two stages, not delivering the eventual rich
analytics surface (per-GEO/source/offer breakdowns, the metrics registry,
custom metrics). `organization_id` comes from `tenant.Middleware`, same as
every other `apps/api` route; date range is capped at 366 days
(`analytics.maxRangeDays`) as a guard against an unbounded ClickHouse scan,
not a spec requirement.

`apps/api`'s ClickHouse connection is best-effort at startup, same stance as
the tracker/worker's Redis wiring: `/campaigns` doesn't need it, only
`/analytics` does, so a down ClickHouse degrades one route group (and shows
up on `/ready`) rather than taking the whole control-plane API down.
