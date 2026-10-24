-- The search and ad networks' own click id of a click (audit UNI-2):
-- ad_click_param is the parameter it came in — gclid, gbraid, wbraid,
-- yclid or msclkid (internal/adclick) — and ad_click_id its value. One per
-- click, the first present in that order. Offline conversions report a
-- purchase back to the network by it, so click_lookup carries it too.
-- Clicks from before this file have both empty.
--
-- Appended last, after 024's retention_months — chstore.EventStore appends
-- positionally; keep the order in sync.
ALTER TABLE click_events ADD COLUMN IF NOT EXISTS ad_click_param LowCardinality(String);
ALTER TABLE click_events ADD COLUMN IF NOT EXISTS ad_click_id    String;
ALTER TABLE click_lookup ADD COLUMN IF NOT EXISTS ad_click_param LowCardinality(String);
ALTER TABLE click_lookup ADD COLUMN IF NOT EXISTS ad_click_id    String;

-- click_lookup's view, with them. Same swap as 015, 016 and 024.
CREATE MATERIALIZED VIEW IF NOT EXISTS click_lookup_mv_v5
TO click_lookup
AS
SELECT organization_id, click_id, event_at,
       campaign_id, stream_set_id, flow_id, external_click_id,
       country, region, city, device, platform, os,
       utm_source, utm_medium, utm_campaign, utm_content, utm_term,
       sub1, sub2, sub3, sub4, sub5, sub6, sub7, sub8, sub9, sub10,
       landing_id, offer_id, pwa_id, traffic_source_id,
       ip, user_agent, fb_click_id, tt_click_id,
       postlanding_id, retention_months,
       ad_click_param, ad_click_id
FROM click_events
WHERE type = 'SOURCE_CLICK';

DROP VIEW IF EXISTS click_lookup_mv_v4;
