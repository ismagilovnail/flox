-- Table 2 of 5: every non-click, non-conversion event in the §43 funnel —
-- LAND_VIEW/LAND_CLICK/POSTLANDING_VIEW/POSTLANDING_CLICK, PWA_VIEW/
-- PWA_OPEN/PWA_INSTALL/IOS_INSTALL, the NOTIFICATION_* family, TG_JOIN,
-- TG_START. These share click_events' dimension columns (they're all
-- carried on the same event.Event struct) but are split into their own
-- table because they answer a different question — funnel progression per
-- click_id, not "how much traffic did this campaign get" — and mixing
-- funnel-stage rows into click_events' sort order (organization_id,
-- event_at, campaign_id, click_id) would make "every stage for this
-- click_id" a scan instead of a seek.
--
-- ORDER BY (organization_id, event_at, type, click_id): type leads over
-- campaign_id here, unlike click_events — "how many PWA_INSTALLs this
-- week" (across campaigns) is as common a query shape as "this campaign's
-- funnel," and campaign_id is still reachable via a secondary index below
-- rather than the sort key, since funnel dashboards slice by stage first.
--
-- No TTL — see click_events' comment; the same reasoning applies to every
-- table in this schema.
CREATE TABLE IF NOT EXISTS tracking_events
(
    organization_id String,
    event_at DateTime64(3, 'UTC'),
    type LowCardinality(String),

    campaign_id String,
    click_id String,
    stream_set_id String,
    flow_id String,
    destination String,

    country LowCardinality(String),
    region String,
    city String,
    device LowCardinality(String),
    platform LowCardinality(String),
    os LowCardinality(String),
    os_version LowCardinality(String),
    browser LowCardinality(String),
    browser_version LowCardinality(String),
    language LowCardinality(String),
    is_bot UInt8,
    is_proxy UInt8,
    ip String,
    user_agent String,
    referrer String,

    utm_source String,
    utm_medium String,
    utm_campaign String,
    utm_content String,
    utm_term String,
    sub1 String, sub2 String, sub3 String, sub4 String, sub5 String,
    sub6 String, sub7 String, sub8 String, sub9 String, sub10 String,

    external_click_id String,

    INDEX idx_campaign_id campaign_id TYPE bloom_filter(0.01) GRANULARITY 4
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(event_at)
ORDER BY (organization_id, event_at, type, click_id)
SETTINGS index_granularity = 8192;
