-- event_rollup_hourly: clicks and tracking events counted per UTC hour
-- and report dimension (audit DATA-6, docs/retention.md "Rollup"). The
-- report builder's click, landing-view, install and Telegram metrics read
-- it instead of the raw rows whenever every dimension and filter of the
-- query is here (chstore.rollupColumns), so they survive the raw rows'
-- retention. Not here: region, city, utm_*, sub1..10, click ids — those
-- reports read the raw rows and end where the rows do.
--
-- event_at is the start of the hour, so a report's own event_at range and
-- its toDate(event_at, tz) days apply unchanged: the same numbers for any
-- range that starts and ends on a whole hour, which every day in a
-- whole-hour time zone does. Unique clicks merge the visitors' set
-- (uniqExact over the same expression as chstore.uniqueVisitorExpr), so
-- they stay exact across hours.
CREATE TABLE IF NOT EXISTS event_rollup_hourly
(
    organization_id   String,
    event_at          DateTime('UTC'),
    type              LowCardinality(String),
    campaign_id       String,
    traffic_source_id String,
    stream_set_id     String,
    flow_id           String,
    landing_id        String,
    offer_id          String,
    pwa_id            String,
    postlanding_id    String,
    network_id        String,
    tg_channel_id     String,
    country           LowCardinality(String),
    device            LowCardinality(String),
    platform          LowCardinality(String),
    os                LowCardinality(String),
    browser           LowCardinality(String),
    language          LowCardinality(String),
    events            SimpleAggregateFunction(sum, UInt64),
    bots              SimpleAggregateFunction(sum, UInt64),
    empty_subs        SimpleAggregateFunction(sum, UInt64),
    visitors          AggregateFunction(uniqExact, String)
)
ENGINE = AggregatingMergeTree
PARTITION BY toYYYYMM(event_at)
ORDER BY (organization_id, event_at, type, campaign_id, traffic_source_id, stream_set_id, flow_id,
          landing_id, offer_id, pwa_id, postlanding_id, network_id, tg_channel_id,
          country, device, platform, os, browser, language);

-- The views below count events from the cutover on; chstore.BackfillRollup
-- copies everything before it, once, an hour after it, so no event is
-- counted twice. Set once: re-running this file keeps the first value.
CREATE TABLE IF NOT EXISTS event_rollup_cutover (cutover DateTime('UTC')) ENGINE = MergeTree ORDER BY tuple();
INSERT INTO event_rollup_cutover SELECT toStartOfHour(now()) + INTERVAL 1 HOUR
WHERE (SELECT count() FROM event_rollup_cutover) = 0;

CREATE MATERIALIZED VIEW IF NOT EXISTS event_rollup_clicks_mv TO event_rollup_hourly AS
SELECT organization_id, toStartOfHour(event_at) AS event_at, type,
       campaign_id, traffic_source_id, stream_set_id, flow_id, landing_id, offer_id, pwa_id, postlanding_id,
       network_id, tg_channel_id, country, device, platform, os, browser, language,
       count() AS events,
       countIf(is_bot = 1) AS bots,
       countIf(type = 'SOURCE_CLICK' AND sub1 = '' AND sub2 = '' AND sub3 = '' AND sub4 = '' AND sub5 = ''
               AND sub6 = '' AND sub7 = '' AND sub8 = '' AND sub9 = '' AND sub10 = '') AS empty_subs,
       uniqExactStateIf(multiIf(visitor_key != '', concat(campaign_id, '|', visitor_key),
                                ip = '' AND user_agent = '', click_id,
                                concat(campaign_id, '|', ip, '|', user_agent)),
                        type = 'SOURCE_CLICK') AS visitors
FROM click_events
WHERE type IN ('SOURCE_CLICK', 'SOURCE_FILTER')
  AND toStartOfHour(event_at) >= (SELECT max(cutover) FROM event_rollup_cutover)
GROUP BY organization_id, event_at, type, campaign_id, traffic_source_id, stream_set_id, flow_id, landing_id,
         offer_id, pwa_id, postlanding_id, network_id, tg_channel_id, country, device, platform, os, browser, language;

CREATE MATERIALIZED VIEW IF NOT EXISTS event_rollup_tracking_mv TO event_rollup_hourly AS
SELECT organization_id, toStartOfHour(event_at) AS event_at, type,
       campaign_id, traffic_source_id, stream_set_id, flow_id, landing_id, offer_id, pwa_id, postlanding_id,
       network_id, tg_channel_id, country, device, platform, os, browser, language,
       count() AS events,
       countIf(is_bot = 1) AS bots,
       toUInt64(0) AS empty_subs,
       uniqExactStateIf('', 0) AS visitors
FROM tracking_events
WHERE toStartOfHour(event_at) >= (SELECT max(cutover) FROM event_rollup_cutover)
GROUP BY organization_id, event_at, type, campaign_id, traffic_source_id, stream_set_id, flow_id, landing_id,
         offer_id, pwa_id, postlanding_id, network_id, tg_channel_id, country, device, platform, os, browser, language;
