-- §48's five-table ClickHouse schema, table 1 of 5. Holds SOURCE_CLICK and
-- SOURCE_FILTER only — the tracker's own redirect-time decision, the
-- highest-volume event type and the one every dashboard's first query hits
-- ("how much traffic, how much of it filtered").
--
-- ORDER BY (organization_id, event_at, campaign_id, click_id):
--   organization_id leads per CLAUDE.md #5 (tenant isolation, non-
--   negotiable). event_at next gives every "this campaign, this date
--   range" dashboard query a contiguous range scan — the single most
--   common query shape. campaign_id before event_at would also satisfy
--   §48's "optimize for organization/date/campaign", but date range
--   scans matter for every query including cross-campaign ones, so it
--   leads over campaign here (both orderings are defensible; this one
--   favors the more universal access pattern). click_id last keeps the
--   key unique without forcing a wider key, and is the join column
--   attribution/conversion queries actually use.
--
-- No TTL: no data retention policy exists yet anywhere in this project's
-- docs (PRODUCT.md, ARCHITECTURE.md) — inventing one here, silently, would
-- be a real product/compliance decision (e.g. GDPR-relevant) made by
-- default rather than on purpose. Add one later with `ALTER TABLE ...
-- MODIFY TTL` when a real policy exists; no schema redesign needed.
--
-- "source" from §48's "organization/date/campaign/source/country/flow/
-- offer" optimization list is NOT in the sort key: apps/internal/event.Event
-- has no traffic_source_id field — nothing threads a click's traffic
-- source through the event pipeline yet (the same pre-existing gap
-- apps/internal/macro's {source} token has, documented in
-- docs/postback-delivery.md). campaign_id is used as the practical proxy
-- (a campaign has exactly one traffic_source_id in Postgres); adding real
-- traffic_source_id here is later work, not silently worked around.
CREATE TABLE IF NOT EXISTS click_events
(
    organization_id      String,
    event_at             DateTime64(3, 'UTC'),
    type                 LowCardinality(String), -- SOURCE_CLICK | SOURCE_FILTER

    campaign_id          String,
    click_id             String,
    stream_set_id        String,
    flow_id              String,
    destination          String,
    sticky_applied       UInt8,
    config_version       Int64,

    country              LowCardinality(String),
    region               String,
    city                 String,
    device               LowCardinality(String),
    platform             LowCardinality(String),
    os                   LowCardinality(String),
    os_version           LowCardinality(String),
    browser              LowCardinality(String),
    browser_version      LowCardinality(String),
    language             LowCardinality(String),
    is_bot               UInt8,
    is_proxy             UInt8,
    asn                  String,
    connection_type      LowCardinality(String),
    ip                   String,
    user_agent           String,
    referrer             String,

    utm_source           String,
    utm_medium           String,
    utm_campaign         String,
    utm_content          String,
    utm_term             String,
    sub1                 String,
    sub2                 String,
    sub3                 String,
    sub4                 String,
    sub5                 String,
    sub6                 String,
    sub7                 String,
    sub8                 String,
    sub9                 String,
    sub10                String,

    external_click_id    String,
    fb_click_id          String,
    tt_click_id          String,
    filter_reason        String,

    INDEX idx_external_click_id external_click_id TYPE bloom_filter(0.01) GRANULARITY 4,
    INDEX idx_country country TYPE bloom_filter(0.01) GRANULARITY 4
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(event_at)
ORDER BY (organization_id, event_at, campaign_id, click_id)
SETTINGS index_granularity = 8192;
