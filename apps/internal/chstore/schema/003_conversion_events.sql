-- Table 3 of 5: CPA_HOLD/CPA_ACCEPT/CPA_REDEP/CPA_DECLINE/CPA_TRASH. The
-- money table — every revenue/currency/attribution field §45 and §50-FX
-- require, plus the dimensions a revenue report slices by.
--
-- ORDER BY (organization_id, event_at, campaign_id, click_id): campaign_id
-- leads over type here (unlike tracking_events) — "this campaign's
-- revenue over time" is the dominant query shape for money data, and
-- status is low-cardinality enough (5 values) that a bloom filter index
-- serves the rarer "just CPA_REDEP" queries well enough without earning a
-- sort-key slot.
--
-- revenue/usd_value/has_usd_value mirror event.Event exactly — see
-- Phase 23's conversion.go doc for why "no FX rate on file" (has_usd_value
-- = 0) must stay distinguishable from "converts to zero" rather than
-- being collapsed into one Float64.
--
-- No TTL — see click_events' comment.
CREATE TABLE IF NOT EXISTS conversion_events
(
    organization_id String,
    event_at DateTime64(3, 'UTC'),
    type LowCardinality(String), -- CPA_HOLD | CPA_ACCEPT | CPA_REDEP | CPA_DECLINE | CPA_TRASH

    campaign_id String,
    click_id String,
    stream_set_id String,
    flow_id String,

    country LowCardinality(String),
    region String,
    city String,
    device LowCardinality(String),
    platform LowCardinality(String),

    utm_source String,
    utm_medium String,
    utm_campaign String,
    utm_content String,
    utm_term String,
    sub1 String, sub2 String, sub3 String, sub4 String, sub5 String,
    sub6 String, sub7 String, sub8 String, sub9 String, sub10 String,
    external_click_id String,

    network_id String,
    revenue Float64,
    currency LowCardinality(String),
    usd_value Float64,
    has_usd_value UInt8,
    event_ref String,
    network_txn_id String,
    attribution_outcome LowCardinality(String),
    attribution_method LowCardinality(String),
    time_to_conversion_ms Int64,

    INDEX idx_click_id click_id TYPE bloom_filter(0.01) GRANULARITY 4,
    INDEX idx_type type TYPE bloom_filter(0.01) GRANULARITY 4
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(event_at)
ORDER BY (organization_id, event_at, campaign_id, click_id)
SETTINGS index_granularity = 8192;
