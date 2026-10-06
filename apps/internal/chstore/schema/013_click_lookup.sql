-- Audit 2026-10-06, CH1: attribution resolves every postback's click_id to
-- its click (chstore.ClickResolver.ByClickID). click_events is sorted by
-- (organization_id, event_at, ...), and a postback does not know when its
-- click happened, so that lookup read every granule of the organization —
-- linear in its whole click history, on every conversion.
--
-- click_lookup holds exactly the columns attribution needs, sorted by
-- (organization_id, click_id): the lookup is a primary-key range read of a
-- single granule per partition. Partitioned by month like the other tables,
-- so retention can drop old months the same way.
--
-- ReplacingMergeTree, keyed on the full sort key: the view and the one-time
-- backfill (chstore.Migrate) may both copy the same click, and those exact
-- duplicates collapse. A click_id that legitimately repeats (sticky
-- keepClickId journeys) keeps one row per event_at; ByClickID takes the
-- earliest, as before.
CREATE TABLE IF NOT EXISTS click_lookup
(
    organization_id   String,
    click_id          String,
    event_at          DateTime64(3, 'UTC'),

    campaign_id       String,
    stream_set_id     String,
    flow_id           String,
    external_click_id String,
    country           LowCardinality(String),
    region            String,
    city              String,
    device            LowCardinality(String),
    platform          LowCardinality(String),
    os                LowCardinality(String),
    utm_source        String,
    utm_medium        String,
    utm_campaign      String,
    utm_content       String,
    utm_term          String,
    sub1              String,
    sub2              String,
    sub3              String,
    sub4              String,
    sub5              String,
    sub6              String,
    sub7              String,
    sub8              String,
    sub9              String,
    sub10             String,
    landing_id        String,
    offer_id          String,
    pwa_id            String,
    traffic_source_id String
)
ENGINE = ReplacingMergeTree
PARTITION BY toYYYYMM(event_at)
ORDER BY (organization_id, click_id, event_at)
SETTINGS index_granularity = 8192, non_replicated_deduplication_window = 100000;

CREATE MATERIALIZED VIEW IF NOT EXISTS click_lookup_mv
TO click_lookup
AS
SELECT organization_id, click_id, event_at,
       campaign_id, stream_set_id, flow_id, external_click_id,
       country, region, city, device, platform, os,
       utm_source, utm_medium, utm_campaign, utm_content, utm_term,
       sub1, sub2, sub3, sub4, sub5, sub6, sub7, sub8, sub9, sub10,
       landing_id, offer_id, pwa_id, traffic_source_id
FROM click_events
WHERE type = 'SOURCE_CLICK';

-- One-time backfill marker table (chstore.Migrate's backfills): schema files
-- run on every start, so a copy of history must not.
CREATE TABLE IF NOT EXISTS chstore_backfills
(
    name       String,
    applied_at DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree
ORDER BY name;
