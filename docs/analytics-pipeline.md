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
  duplicate, ignored, error) via the new `AttemptLogger` interface. Same
  decoupled, no-error-return contract as `EventSink`/`DeliveryEnqueuer`: an
  already-processed postback must never be reported differently to the
  network just because this secondary audit log's queue insert stumbled.
- **Outgoing** (`internal/postback.Deliverer`): every dispatch outcome
  (success, retrying, dead) now also calls `d.logAttempt(...)` via the same
  pattern.

Both feed `internal/postbacklog` — a near-duplicate of `internal/eventqueue`
(Postgres `FOR UPDATE SKIP LOCKED` queue, `postback_attempt_queue`,
migration 00016; a `Flusher` batching into ClickHouse) rather than a
generalized/shared implementation. The payload shapes
(`chstore.PostbackAttempt` vs `event.Event`) are genuinely different and
there is nothing else to share beyond the SQL pattern itself — duplicating
~150 lines of proven, tested code was judged lower-risk than a generics
refactor touching Phase 25's already-shipped `eventqueue` mid-project.
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

## The queue: `event_queue` (Postgres) — unchanged since Phase 25

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

## ClickHouse over HTTP, not native protocol

`infra/docker-compose.dev.yml` exposes only ClickHouse's HTTP interface
(port 8123) to the host — the native protocol port stays container-internal
to avoid colliding with MinIO's API port. `internal/chconn` always dials
`clickhouse.HTTP`, unlike some reference implementations that default to
the native protocol; get this wrong and every connection attempt fails
with a connection-refused on 9000, not an auth error.

## Schema application: idempotent DDL, not a migration framework — unchanged

`internal/chstore.Migrate` applies `schema/*.sql` in lexical order, every
statement `CREATE ... IF NOT EXISTS` (or `DROP ... IF EXISTS` for
`000_drop_phase25_schema.sql`), with no version-tracking table. Both
`apps/api` and `apps/worker` call it at startup — safe because it's
idempotent, necessary because there's no ordering guarantee about which one
starts first in local dev. Still a deliberate simplification, not upgraded
in this phase: a real migration tool (or at minimum a version table) is
warranted once this schema is meant to stop changing wholesale between
phases — it has now been replaced once (Phase 25 → 26) and may not be the
last time before it earns that investment.

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
