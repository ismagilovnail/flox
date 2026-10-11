-- Pixels (docs/pixels.md "Sending"): a conversion sent to an ad platform is
-- matched to the ad click by what the visitor's click carried — IP, user
-- agent and the platform's own click id (fbclid, ttclid). Attribution reads
-- the click from click_lookup, so the lookup carries them too.
--
-- The new view is created before the old one is dropped: a click inserted
-- in between is copied by both, and the two rows share the sort key, so
-- ReplacingMergeTree keeps one. Clicks from before this file have the new
-- columns empty.
ALTER TABLE click_lookup ADD COLUMN IF NOT EXISTS ip          String;
ALTER TABLE click_lookup ADD COLUMN IF NOT EXISTS user_agent  String;
ALTER TABLE click_lookup ADD COLUMN IF NOT EXISTS fb_click_id String;
ALTER TABLE click_lookup ADD COLUMN IF NOT EXISTS tt_click_id String;

CREATE MATERIALIZED VIEW IF NOT EXISTS click_lookup_mv_v2
TO click_lookup
AS
SELECT organization_id, click_id, event_at,
       campaign_id, stream_set_id, flow_id, external_click_id,
       country, region, city, device, platform, os,
       utm_source, utm_medium, utm_campaign, utm_content, utm_term,
       sub1, sub2, sub3, sub4, sub5, sub6, sub7, sub8, sub9, sub10,
       landing_id, offer_id, pwa_id, traffic_source_id,
       ip, user_agent, fb_click_id, tt_click_id
FROM click_events
WHERE type = 'SOURCE_CLICK';

DROP VIEW IF EXISTS click_lookup_mv;
