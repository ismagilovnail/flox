-- Raw clicks expire by plan (audit DATA-6 part 2, docs/retention.md "Raw
-- clicks by plan"). retention_months is how long the row's organization
-- keeps raw events: written on insert from its plan (apps/worker,
-- EventStore.WithRetention), rewritten on the organization's rows when
-- the plan changes (internal/retention.Plans). 0 — rows from before this
-- file, or an insert path that does not know the plan — counts as 24, the
-- longest plan, so no row is kept forever by omission.
--
-- Reports outlive the rows through event_rollup_hourly (schema/023).
-- click_lookup keeps a click at least 13 months whatever the plan: a late
-- postback (a re-deposit months later) must still find its click.
--
-- Appended last — chstore.EventStore appends positionally (after 021's
-- ip_visitor_key on click_events, 019's tg_channel_id on tracking_events).
ALTER TABLE click_events    ADD COLUMN IF NOT EXISTS retention_months UInt8;
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS retention_months UInt8;
ALTER TABLE click_lookup    ADD COLUMN IF NOT EXISTS retention_months UInt8;

ALTER TABLE click_events MODIFY TTL
    toDateTime(event_at) + toIntervalMonth(if(retention_months = 0, 24, retention_months));
ALTER TABLE tracking_events MODIFY TTL
    toDateTime(event_at) + toIntervalMonth(if(retention_months = 0, 24, retention_months));
ALTER TABLE click_lookup MODIFY TTL
    toDateTime(event_at) + toIntervalMonth(greatest(13, if(retention_months = 0, 24, retention_months)));

-- click_lookup's view, with retention_months. Same swap as 015 and 016.
CREATE MATERIALIZED VIEW IF NOT EXISTS click_lookup_mv_v4
TO click_lookup
AS
SELECT organization_id, click_id, event_at,
       campaign_id, stream_set_id, flow_id, external_click_id,
       country, region, city, device, platform, os,
       utm_source, utm_medium, utm_campaign, utm_content, utm_term,
       sub1, sub2, sub3, sub4, sub5, sub6, sub7, sub8, sub9, sub10,
       landing_id, offer_id, pwa_id, traffic_source_id,
       ip, user_agent, fb_click_id, tt_click_id,
       postlanding_id, retention_months
FROM click_events
WHERE type = 'SOURCE_CLICK';

DROP VIEW IF EXISTS click_lookup_mv_v3;
