# Analytics Pipeline (Phases 25-26, §47/§48)

`events -> ClickHouse -> materialized/aggregate tables -> analytics service
-> REST API -> frontend`. Phases 25-26 build every stage except the last —
wiring the actual Next.js UI to a real backend is Phase 27's explicit job
("Frontend/backend integration" per ROADMAP.md), consistent with this
project's frontend-first-on-mocks build order; `apps/web` stays on mocks
until then.

## Phase 25 → Phase 26: the schema rework, as planned

§47 (Phase 25)'s pipeline diagram names ClickHouse as one of its own
stages, but the *actual* table design is §48's stated content, in a phase
of its own titled "ClickHouse." Phase 25 built a minimal, disposable
single-table schema (`events` + one aggregate) just sufficient to prove the
pipeline end to end — confirmed with the user before starting, and recorded
at the time so the rework below would read as planned, not as backtracking.

Phase 26 replaces that schema wholesale with the real design below.
`schema/000_drop_phase25_schema.sql` drops Phase 25's `events` table and
its aggregate; everything from `001_click_events.sql` on is new. The
invariants that were never phase-scoped carried through unchanged:
`organization_id` leads every sort key (CLAUDE.md #5), and the type model
is never truncated (CLAUDE.md #2) — see the classification section below
for how that's now enforced by a test, not just a convention.

## The five tables

| Table | Event types | Sort key |
|---|---|---|
| `click_events` | `SOURCE_CLICK`, `SOURCE_FILTER` | `(organization_id, event_at, campaign_id, click_id)` |
| `tracking_events` | everything else non-CPA (`LAND_VIEW` .. `TG_START`) | `(organization_id, event_at, type, click_id)` |
| `conversion_events` | `CPA_HOLD` .. `CPA_TRASH` | `(organization_id, event_at, campaign_id, click_id)` |
| `cost_events` | n/a — mirrors Postgres `cost_entries` | `(organization_id, date, campaign_id, traffic_source_id)` |
| `postback_events` | n/a — postback attempt log, both directions | `(organization_id, event_at, network_id, direction)` |

**click_events vs tracking_events vs conversion_events** is decided by
`event.Type.IsClick()`/`IsCPA()` (`internal/event/event.go`) — exhaustive
and disjoint by construction, and
`TestEventClassificationIsExhaustiveAndDisjoint` guards that no future
event type silently lands in the wrong table or no table at all.
`internal/chstore.EventStore.InsertBatch` buckets one mixed batch (the
normal case — `apps/worker`'s flusher claims whatever's due, not grouped by
type) into up to three ClickHouse batch inserts, never one insert per row.

**Different sort keys for click_events vs tracking_events on purpose**:
click_events leads with `campaign_id` (the dominant query — "this
campaign's traffic") where tracking_events leads with `type` ("how many
PWA_INSTALLs this week," across campaigns) — funnel-stage dashboards slice
by stage first, campaign volume dashboards slice by campaign first. Neither
ordering is "more correct"; see each schema file's own comment for the
reasoning.

**`traffic_source_id` is NOT in any sort key** — `event.Event` has no such
field. Nothing threads a click's traffic source through the event pipeline
yet, the same pre-existing gap `internal/macro`'s `{source}` token has
(Phase 24). `campaign_id` is used as the practical proxy (a campaign has
exactly one `traffic_source_id` in Postgres) everywhere §48 asks to
"optimize for ... source ...". Real `traffic_source_id` propagation is
later work, not silently worked around here.

**No TTL** on any table — no data retention policy exists yet anywhere in
this project's docs (PRODUCT.md, ARCHITECTURE.md), and inventing one
silently would be a real product/compliance decision (e.g. GDPR-relevant),
confirmed with the user as out of scope for this phase. Add one later with
`ALTER TABLE ... MODIFY TTL ...` — no schema redesign needed.

## `cost_events`: schema only, no sync pipeline

Mirrors Postgres's `cost_entries` (migration 00009) for cross-database-free
JOINs against click/conversion volume — but the sync pipeline that
populates it from `cost_entries` is Phase 27-COST's job ("Cost ingestion,"
ROADMAP.md), not this phase's. `cost_entries` itself started manual-entry-
only in Phase 17 before any FB/TikTok import existed; an empty
`cost_events` table with the right shape follows the same pattern. It's a
`ReplacingMergeTree(updated_at)` rather than a plain `MergeTree`, anticipating
that a future sync will need to overwrite an edited cost entry, not just
append — one difference from every other table here, which are all
append-only event streams.

## `postback_events`: real ingestion, both directions

Migration 00008 (Phase 17) earmarked this table for the rich per-attempt
postback log ("does NOT duplicate the rich per-attempt log... that belongs
in ClickHouse's postback_events, outside this Postgres-only phase") — the
infrastructure to actually build it didn't exist until Phase 25. Phase 26
closes that gap for real, not just with a schema:

- **Incoming** (`internal/conversion.Service`): every exit point of
  `Record` — the main success/duplicate path, `logError`, `logIgnored` —
  now also calls `s.logAttempt(...)`, reporting every outcome (success,
  duplicate, ignored, error) via the new `AttemptLogger` interface, with a
  decoupled, no-error-return contract: an
  already-processed postback must never be reported differently to the
  network just because this secondary audit log's queue insert stumbled.
- **Outgoing** (`internal/postback.Deliverer`): every dispatch outcome
  (success, retrying, dead) now also calls `d.logAttempt(...)` via the same
  pattern.

Both feed `internal/postbacklog` (Postgres `FOR UPDATE SKIP LOCKED` queue,
`postback_attempt_queue`, migration 00016; a `Flusher` batching into
ClickHouse). Its consumer side and `Flusher` are `internal/pgqueue`'s
generic `Queue[T]` / `Flusher[T]`, shared with `internal/eventqueue`
(tech audit CQ2): claim, delete, requeue, reap, dead-letter, the detached
insert/cleanup contexts and the idempotent batch id exist once. Each queue
package keeps only its producer and its ClickHouse insert.
`postbacklog.ConversionAttemptLogger` and `.DeliveryAttemptLogger` adapt
`Producer` to each package's own `AttemptLogger` interface, so neither
`internal/conversion` nor `internal/postback` needs to import this package
or know ClickHouse exists — wired in `apps/worker/main.go` and
`apps/tracker/main.go`.

This table is explicitly **not** the dedup/delivery source of truth —
Postgres's `postbacks` and `postback_deliveries` still are, with real
unique constraints and `FOR UPDATE SKIP LOCKED` claims. `postback_events`
is the read-side replay/audit log §45 requires, fed asynchronously off the
same durable-queue pattern as every other table here — never on the
critical path of either direction. Verified end-to-end: a real incoming
postback produces both a `postbacks` row (Postgres) and a `postback_events`
row (`direction='incoming'`), and the outgoing delivery it triggers
produces its own `postback_events` row (`direction='outgoing'`) after
actually hitting the network's URL.

### Backoff and dead-letter (audit 2026-10-06, Q2)

A failed ClickHouse insert is retried after 10 s, doubling per attempt up to
10 min (`pgqueue.Fail`, columns `attempts`/`last_error`, migration 00052).
`chstore.Rejected` tells a server-side rejection (`Code: N.
DB::Exception`, other than overload/timeout codes) from everything else; a
batch rejected `pgqueue.MaxRejectedAttempts` (5) times moves to `dead` —
kept whole, with its `batch_id` and `last_error`, never claimed again on its
own. Network errors, timeouts and transient server errors never
dead-letter: an outage only backs off.

Dead rows are not lost, but they are not in the reports either
(`flox_queue_dead_lettered_total`, alert `QueueBatchesDeadLettered`). After
fixing the cause (read `last_error`), put them back whole — the same
`batch_id` keeps their deduplication tokens, so tables that already took the
batch skip it:

```sql
SELECT batch_id, count(*), max(attempts), min(last_error) FROM event_queue WHERE status = 'dead' GROUP BY batch_id;
UPDATE event_queue SET status = 'queued', attempts = 0, dead_at = NULL, next_attempt_at = now() WHERE status = 'dead';
-- same for postback_attempt_queue
```

The tokens are remembered for the last 100,000 blocks per table
(schema/012); a batch put back much later than that can insert duplicates
into the tables that had already accepted it.

## Reconciliation: Postgres vs ClickHouse (audit 2026-10-06)

`postbacks` is the record of every conversion; `conversion_events` is its
copy. `internal/reconcile` (run by `apps/worker` hourly over the last 3 days,
leaving out the newest 30 minutes still in flight) compares, per
(organization, UTC day, status), the number of events and the USD sum the
way reports count it — an accept/redeposit its `usd_value`, a
decline/trash its `reversal_usd_value` negated. USD is compared with a
tolerance of 0.01 + 0.0001 × rows (Postgres keeps 4 decimals, the event the
unrounded float).

Rows are bucketed on `postbacks.event_at` (migration 00054, indexed by
00055), written with the row from the analytics event's own time: a
conversion received at 23:59:59.998 is stamped by `created_at` a few
milliseconds later — the next day — and bucketing on that would report a
phantom difference on both days (the test fails exactly that way when the
query uses `created_at`).

Each differing bucket is logged as "conversions differ between postgres and
clickhouse" (at most 20 per run) and counted in
`flox_reconciliation_mismatched_buckets{kind=count|usd}` (as found, before
repair); alerts `ConversionsDiverge` (two runs in a row — i.e. repair is not
fixing it) and `ReconciliationNotRunning`. Usual causes: a dead-lettered
batch (`flox_queue_rows{status="dead"}`), a queue backlog older than 30
minutes, a retried event write whose commit had landed, an FX rate that
arrived after the conversion.

### Repair

Every event has a stable id (`event.Event.ID`; a conversion's is its
`postbacks` row id, stored as `event_id` in `conversion_events` and
`ltv_events`). For each differing bucket `reconcile.Repair` compares event by
event: a conversion missing from ClickHouse is re-emitted; one with several
copies or a different USD value has every copy deleted and is re-emitted
once; an event Postgres never recorded is deleted. The event is rebuilt by
`conversion.EventFor` from the `postbacks` row and the attributed click
(`click_lookup`) — the same builder the record path uses, verified field by
field (`TestEventForRebuildsTheQueuedEvent`). Deleted events take their
`ltv_events` rows with them; the affected rows of
`conversion_events_daily_campaign` are recomputed from the corrected
events (`insert_deduplicate = 0`, or the window from schema/012 could drop a
recomputed block identical to an earlier one). At most 1,000 events per run
(`MaxRepairsPerRun`); `flox_reconciliation_repaired_events_total{action}`.

Not repairable: rows written before event ids existed (`event_id = ''`) —
reported as unmatched, left alone. The daily rebuild is not atomic with live
traffic: a conversion of the same campaign and day inserted between its
delete and re-insert is counted twice there (repairs only touch days at
least 30 minutes old; the next run reports it).

Outside the 3-day window: `make reconcile FROM=YYYY-MM-DD TO=YYYY-MM-DD
[REPORT_ONLY=1]` (`apps/cmd/reconcile`, same env as the worker), one UTC day
at a time.

## `click_lookup`: clicks by id (audit 2026-10-06, CH1)

A `ReplacingMergeTree` copy of `SOURCE_CLICK` rows with only the columns
attribution needs, `ORDER BY (organization_id, click_id, event_at)`,
filled by `click_lookup_mv` (schema/013). Attribution reads it instead of
`click_events`. History that predates the table is copied once by
`apps/worker` at startup (`chstore.Backfill`, recorded in
`chstore_backfills`, in the background); exact duplicates of a racing
backfill collapse on merge, and queries take the earliest row anyway.
Schema files still run on every start — one-time data copies go through
`chstore.Backfill`, never into a schema file.

## Materialized views: per-campaign+day, per-GEO+day

§48 names both patterns explicitly. Three views exist:

- `click_events_daily_campaign` — click/filter volume, broken out by type.
- `click_events_daily_geo` — click volume per country.
- `conversion_events_daily_campaign` — conversion counts **and USD
  revenue** per campaign per day per status. Not named by §48 directly, but
  the same pattern applied to money instead of volume — added because
  CLAUDE.md #6 ("cost or it doesn't exist") and §27-COST's eventual ROI
  queries will need a revenue aggregate, and building it now costs nothing
  extra given the pattern already exists twice over.

All three are `SummingMergeTree` fed by a `MATERIALIZED VIEW` that fires
*synchronously* on `INSERT` (verified in `chstore_test.go` — no polling
needed to observe a fresh row). Querying any of them still requires
`SUM(...)`, never a plain read: `SummingMergeTree` merges same-key rows
only in the background, so trusting "one row per key already holds the
final total" would undercount whenever a merge hasn't run yet.
`conversion_events_daily_campaign_mv`'s revenue sum is
`sum(usd_value * has_usd_value)` — a conversion with no FX rate on file
(`has_usd_value = 0`) contributes exactly zero, not a fabricated value, and
the multiplication makes that exact rather than approximate.

## What a crash of the tracker loses (audit REL-1)

`apps/tracker` writes events asynchronously (CLAUDE.md #9): a click is
accepted into `internal/eventbuf`'s memory, and a background loop flushes
batches to the sink (Postgres `event_queue` or NATS). A graceful stop
drains the buffer. A hard crash (SIGKILL, OOM, node loss) loses whatever is
still only in memory.

- **Normally:** up to the flush interval, now **250 ms** (it was 1 s), plus
  the batch in flight — about a quarter of a second of clicks per crashed
  replica.
- **While the sink is failing:** the batch is retried and new events wait
  in the 10 000-event buffer, so the window grows with the outage. Once the
  buffer is full, new events are dropped even without a crash
  (`EventLossDetected`).

The window is measured, not guessed:

- `flox_events_buffer_pending` is how many events are only in memory;
- `flox_events_buffer_oldest_seconds` is how long the oldest of them has
  waited;
- `EventLossWindowGrowing` alerts when that passes 30 s, meaning do not
  restart trackers until the sink is fixed.

Tests: `TestLossWindowIsTheFlushIntervalWhenHealthy`,
`TestLossWindowGrowsWhileTheSinkIsStuck`, `TestPendingExcludesWhatWasGivenUp`.

**Decided against, for now:** a local disk spool that would carry events
through a sink outage and a crash. It needs a disk per tracker replica that
survives restarts. The accepted risk is a sink outage and a tracker crash at
the same time, which can lose up to about 10 000 clicks per replica.
Writing to disk inside the redirect would close even the 250 ms, but it
would break the redirect budget (#9).

## The queue: `event_queue` (Postgres) — unchanged since Phase 25

> Since 2026-10-10, with `NATS_URL` set, the tracker's events travel
> through NATS JetStream instead, and `event_queue` carries conversions,
> the tracker's fallback and batches the stream gave up on
> (`docs/event-stream.md`). Everything below still describes
> `event_queue` itself.

STACK has no message broker, so the queue in "Tracker -> Event Queue ->
Worker -> ClickHouse" is Postgres — the same `FOR UPDATE SKIP LOCKED`
job-queue pattern `postback_deliveries` (Phase 24) established. A row is
**deleted**, not marked terminal and kept, once its batch is durably in
ClickHouse — this queue is disposable transit, not an audit ledger.
`payload` is the whole `event.Event`, JSON-encoded (it already carries
`json` tags for exactly this) rather than a wide explicit-column table.

`internal/eventqueue.Flusher` claims a batch (up to 500,
`apps/worker/main.go`'s `eventPollBatchSize`), attempts ONE ClickHouse
batch insert (now routed across up to three tables — see above), and
either deletes the batch (success) or requeues the whole thing with a
fixed 10s delay (failure). No dead-letter state: a delayed analytics batch
has no per-item deadline the way a lost conversion does, unlike
`internal/postback`'s `MaxAttempts`/dead-letter — a deliberately different
tradeoff, not an inconsistency.

### Claims are never abandoned (audit 2026-10-06, Q1)

A claim flips rows to `processing` and stamps `claimed_at` (migration
00048). Two things keep a claim from outliving its worker:

- **Shutdown finishes the batch.** Once claimed, the ClickHouse insert
  (30s cap) and the delete/requeue (10s cap) run on a context SIGTERM
  cannot cancel; the poll loop just stops claiming new batches, and
  `apps/worker` waits up to 40s (`shutdownDrainTimeout`) for its loops.
  Before this, cancelling the insert also cancelled the requeue, and the
  batch stayed `processing` forever on every deploy.
- **The reaper is the backstop** (`internal/queuereaper`): every minute,
  and once at startup, each queue returns rows claimed more than 5 minutes
  ago to `queued`. It covers SIGKILL/OOM/a lost node. A reaped batch may
  already have reached ClickHouse just before the crash; its retry is a
  no-op thanks to the batch token below.

### Retries never duplicate (audit 2026-10-06, D2)

Before, a retried batch was inserted again: after a partial failure
(`click_events` landed, `tracking_events` failed → whole batch requeued),
after an insert that timed out but had committed, or after a reap. Raw
tables are plain `MergeTree` and the daily aggregates/LTV are
materialized views fired on every insert, so clicks, conversions and
revenue were counted twice.

Now every claim is one batch with a `batch_id` (`internal/pgqueue`,
migration 00049) that `Requeue` and the reaper keep, and a retry claims
exactly that batch again, never mixed with fresh rows. The worker inserts
it with `insert_deduplication_token = <batch_id>:<table>` and
`deduplicate_blocks_in_dependent_materialized_views = 1`
(`chstore.WithBatchID`); with `non_replicated_deduplication_window =
100000` on every written table and view target (schema/012), ClickHouse
skips a block whose token it has seen — in the views too. The same rows
under a different batch id are new data and are inserted.

The window holds the last 100,000 block tokens per table: ~25 minutes at
10k events/s, longer than the slowest retry (the 5-minute reaper). Above
that rate, raise it.

Caveat: with the window on, an insert **without** a token is deduplicated
by a hash of its content — a byte-identical second block is dropped from
the table while the views still count it. Every writer must pass a batch
id.

`postback_attempt_queue` follows the same rules. `flox_queue_reaped_total{queue}`
counts every reaped row — any increase means a worker died mid-batch.

### Claims are fair across organizations (audit 2026-10-06, M2/Q3)

A fresh batch used to be the oldest due rows of the whole queue, so one
organization's flood put every other organization behind its backlog. Now
(`internal/pgqueue`, fair.go) a claim first takes each organization's
share — up to ⌈limit / organizations⌉ of its oldest rows — and fills the
rest of the batch oldest-first from anyone, so a lone busy organization
still gets the whole capacity. The organizations with rows waiting are
found by a skip scan over `(organization_id, next_attempt_at, id)`
(migration 00066): one index probe each, never a scan of the backlog. One
claim looks at up to `limit` organizations; a rotation starts each claim
after the last one reached, so with more organizations than that, all of
them get their turn. Retry batches are unchanged (claimed whole, first).

Order: oldest-first within an organization; none across organizations,
and none was ever guaranteed across a click's events either (Q3) —
ClickHouse aggregates do not depend on insert order.

`postback_deliveries` is claimed the same way (`docs/postback-delivery.md`).

## ClickHouse over HTTP, not native protocol

`infra/docker-compose.dev.yml` exposes only ClickHouse's HTTP interface
(port 8123) to the host — the native protocol port stays container-internal
to avoid colliding with MinIO's API port. `internal/chconn` always dials
`clickhouse.HTTP`, unlike some reference implementations that default to
the native protocol; get this wrong and every connection attempt fails
with a connection-refused on 9000, not an auth error.

## Schema application

`internal/chstore.Migrate` applies `schema/*.sql` in lexical order, each
file once, recorded in `schema_migrations`; at startup api, worker and
tracker call `MigrateLocked`, which serializes them on a Postgres advisory
lock (tech audit CH5). Details and the rules for a new file:
`docs/migrations.md`.

## The analytics service and REST API

`internal/analytics` exposes two endpoints on `apps/api`, both behind the
existing `tenant.Middleware`:

- `GET /analytics/campaigns/{campaignId}/daily?from=&to=` — click_events_daily_campaign.
- `GET /analytics/campaigns/{campaignId}/daily-revenue?from=&to=` — conversion_events_daily_campaign.

Deliberately narrow — two queries, two endpoints — proving the pipeline's
last two stages, not delivering the eventual rich analytics surface
(per-GEO/source/offer breakdowns, the metrics registry, custom metrics)
that Phase 27's frontend integration and beyond will need. Date range is
capped at 366 days (`analytics.maxRangeDays`) as a guard against an
unbounded ClickHouse scan, not a spec requirement.

`apps/api`'s ClickHouse connection is best-effort at startup, same stance
as the tracker/worker's Redis wiring: `/campaigns` doesn't need it, only
`/analytics` does, so a down ClickHouse degrades one route group (and
shows up on `/ready`) rather than taking the whole control-plane API down.
