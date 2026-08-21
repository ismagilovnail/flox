-- §26.5's data model: "A dedicated ltv_events table (or materialized
-- view) driving cohort math" — a materialized view over conversion_events,
-- same pattern as 006's aggregates, narrowed to exactly the three CPA
-- statuses cohort/LTV math needs:
--
--   CPA_HOLD   — anchors Reg cohorts (registration date)
--   CPA_ACCEPT — anchors FTD cohorts (first-deposit date)
--   CPA_REDEP  — every subsequent deposit, the thing LTV windows sum
--
-- CPA_DECLINE/CPA_TRASH are excluded: no formula in §26.5 references them,
-- and a query scanning them for cohort math would pay for rows the
-- computation never uses. internal/ltv's Go layer reads this table, not
-- conversion_events directly, keeping the (already large) raw money table
-- out of every cohort query.
--
-- ORDER BY (organization_id, click_id, event_at): every cohort computation
-- starts from "give me this click's whole deposit history," not a date
-- range scan — internal/ltv.Repository fetches by click_id set (itself
-- derived from a first-query into this same table), never a raw
-- organization-wide date scan.
--
-- No TTL — see click_events' comment (Phase 26); the same reasoning
-- applies here.
CREATE TABLE IF NOT EXISTS ltv_events
(
    organization_id String,
    click_id        String,
    campaign_id     String,
    network_id      String,
    country         LowCardinality(String),
    event_at        DateTime64(3, 'UTC'),
    type            LowCardinality(String), -- CPA_HOLD | CPA_ACCEPT | CPA_REDEP
    usd_value       Float64,
    has_usd_value   UInt8
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(event_at)
ORDER BY (organization_id, click_id, event_at)
SETTINGS index_granularity = 8192;

CREATE MATERIALIZED VIEW IF NOT EXISTS ltv_events_mv
TO ltv_events
AS
SELECT
    organization_id, click_id, campaign_id, network_id, country, event_at, type, usd_value, has_usd_value
FROM conversion_events
WHERE type IN ('CPA_HOLD', 'CPA_ACCEPT', 'CPA_REDEP');
