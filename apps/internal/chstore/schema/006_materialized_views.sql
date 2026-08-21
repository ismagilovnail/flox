-- §48's named aggregation patterns: "per campaign+day, per GEO+day."
-- SummingMergeTree fed by a MATERIALIZED VIEW, same shape Phase 25
-- established (verified there to fire synchronously on INSERT, not a
-- periodic recompute) — reading any of these still requires SUM(...) at
-- query time, never a plain read: SummingMergeTree merges same-key rows
-- only in the background.

-- Per-campaign+day click volume, broken out by type (SOURCE_CLICK vs
-- SOURCE_FILTER) so "how much of this campaign's traffic got filtered" is
-- one query, not a scan.
CREATE TABLE IF NOT EXISTS click_events_daily_campaign
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

CREATE MATERIALIZED VIEW IF NOT EXISTS click_events_daily_campaign_mv
TO click_events_daily_campaign
AS
SELECT organization_id, campaign_id, type, toDate(event_at) AS day, count() AS event_count
FROM click_events
GROUP BY organization_id, campaign_id, type, day;

-- Per-GEO+day click volume — the other pattern §48 names explicitly.
CREATE TABLE IF NOT EXISTS click_events_daily_geo
(
    organization_id String,
    country         LowCardinality(String),
    day             Date,
    event_count     UInt64
)
ENGINE = SummingMergeTree(event_count)
PARTITION BY toYYYYMM(day)
ORDER BY (organization_id, country, day);

CREATE MATERIALIZED VIEW IF NOT EXISTS click_events_daily_geo_mv
TO click_events_daily_geo
AS
SELECT organization_id, country, toDate(event_at) AS day, count() AS event_count
FROM click_events
GROUP BY organization_id, country, day;

-- Per-campaign+day conversion revenue — the same pattern applied to money
-- rather than volume, which non-negotiable #6 ("cost or it doesn't exist")
-- and §27-COST's eventual ROI queries will need an aggregate for; adding
-- it now costs nothing extra given the pattern already exists, and
-- building the real cost-vs-revenue query without it would mean a raw
-- scan every time.
CREATE TABLE IF NOT EXISTS conversion_events_daily_campaign
(
    organization_id String,
    campaign_id     String,
    type            LowCardinality(String),
    day             Date,
    event_count     UInt64,
    revenue_usd     Float64
)
ENGINE = SummingMergeTree((event_count, revenue_usd))
PARTITION BY toYYYYMM(day)
ORDER BY (organization_id, campaign_id, type, day);

CREATE MATERIALIZED VIEW IF NOT EXISTS conversion_events_daily_campaign_mv
TO conversion_events_daily_campaign
AS
SELECT
    organization_id,
    campaign_id,
    type,
    toDate(event_at) AS day,
    count() AS event_count,
    -- has_usd_value = 0 rows contribute 0 here, not a fabricated
    -- conversion — sum() over unconditionally-zero values is exact, not
    -- an approximation of "unknown."
    sum(usd_value * has_usd_value) AS revenue_usd
FROM conversion_events
GROUP BY organization_id, campaign_id, type, day;
