-- The "materialized/aggregate tables" pipeline stage (§47), proven with one
-- rollup rather than the full per-campaign+day / per-GEO+day set §48
-- specifies — that dimension coverage is Phase 26's job, once the real
-- five-table schema exists to aggregate from.
--
-- SummingMergeTree merges same-key rows by summing event_count across
-- background merges, so a query must still SUM(event_count) itself (merges
-- are not guaranteed complete at query time) rather than trusting one row
-- per key already holds the final total.
CREATE TABLE IF NOT EXISTS events_daily_campaign
(
    organization_id String,
    campaign_id     String,
    type            LowCardinality(String),
    day             Date,
    event_count     UInt64
)
ENGINE = SummingMergeTree(event_count)
PARTITION BY toYYYYMM(day)
ORDER BY (organization_id, campaign_id, type, day);

-- Fires on every INSERT into `events`, computing the SELECT over just the
-- newly-inserted block and pushing the result into events_daily_campaign —
-- not a periodic recompute over the whole table.
CREATE MATERIALIZED VIEW IF NOT EXISTS events_daily_campaign_mv
TO events_daily_campaign
AS
SELECT
    organization_id,
    campaign_id,
    type,
    toDate(event_at) AS day,
    count() AS event_count
FROM events
GROUP BY organization_id, campaign_id, type, day;
