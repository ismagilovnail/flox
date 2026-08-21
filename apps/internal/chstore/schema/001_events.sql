-- Phase 25's minimal, single-table ClickHouse schema (§47). Deliberately
-- NOT the final design: §48 (Phase 26) splits this into five tables
-- (click_events/tracking_events/conversion_events/cost_events/
-- postback_events) with dimension-specific sort keys and TTLs. This table
-- exists to prove the pipeline end-to-end — tracker -> Postgres queue ->
-- worker -> ClickHouse -> materialized aggregate -> analytics API — with
-- one disposable table rather than designing the real schema twice.
--
-- Column set mirrors apps/internal/event.Event field-for-field (not a
-- subset) since the mapping is mechanical and a narrower table would just
-- mean re-adding fields later for no benefit — this table is replaced
-- wholesale in Phase 26, not incrementally migrated.
--
-- organization_id leads ORDER BY (CLAUDE.md #5, non-negotiable: ClickHouse
-- sort keys lead with tenant id) even in this minimal table. `type` is
-- LowCardinality(String), not an Enum — an Enum's fixed value list would
-- duplicate (and risk drifting from) apps/internal/event.go's authoritative
-- ~20-type list; LowCardinality gets the same dictionary-encoding benefit
-- without that duplication.
CREATE TABLE IF NOT EXISTS events
(
    organization_id      String,
    event_at             DateTime64(3, 'UTC'),
    type                 LowCardinality(String),

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

    network_id           String,
    revenue              Float64,
    currency             LowCardinality(String),
    usd_value            Float64,
    has_usd_value        UInt8,
    event_ref            String,
    network_txn_id       String,
    attribution_outcome  LowCardinality(String),
    attribution_method   LowCardinality(String),
    time_to_conversion_ms Int64
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(event_at)
ORDER BY (organization_id, event_at, type, click_id)
SETTINGS index_granularity = 8192;
