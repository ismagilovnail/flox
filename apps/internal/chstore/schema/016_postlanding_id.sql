-- postlanding_id on every event-derived table — the report builder's
-- `postlanding` dimension (chstore.DimPostlanding), same shape as 010's
-- pwa_id.
--
-- Set by apps/tracker when a click's routed path includes the flow's
-- postlanding stage (not paused, reachable — no external landing before
-- it): on SOURCE_CLICK, LAND_*, PWA_* and the POSTLANDING_VIEW/
-- POSTLANDING_CLICK events of the stage itself. Conversions inherit it
-- through attribution, which reads click_lookup. Rows from before this
-- have '' — no postlanding was ever served before it.
--
-- Appended last, after 014's event_id — chstore.EventStore appends
-- positionally; keep the order in sync.
ALTER TABLE click_events      ADD COLUMN IF NOT EXISTS postlanding_id String;
ALTER TABLE tracking_events   ADD COLUMN IF NOT EXISTS postlanding_id String;
ALTER TABLE conversion_events ADD COLUMN IF NOT EXISTS postlanding_id String;
ALTER TABLE click_lookup      ADD COLUMN IF NOT EXISTS postlanding_id String;

-- click_lookup's view, with postlanding_id. Same swap as 015: the new view
-- is created before the old one is dropped, and a click copied by both
-- collapses to one row (ReplacingMergeTree, same sort key).
CREATE MATERIALIZED VIEW IF NOT EXISTS click_lookup_mv_v3
TO click_lookup
AS
SELECT organization_id, click_id, event_at,
       campaign_id, stream_set_id, flow_id, external_click_id,
       country, region, city, device, platform, os,
       utm_source, utm_medium, utm_campaign, utm_content, utm_term,
       sub1, sub2, sub3, sub4, sub5, sub6, sub7, sub8, sub9, sub10,
       landing_id, offer_id, pwa_id, traffic_source_id,
       ip, user_agent, fb_click_id, tt_click_id,
       postlanding_id
FROM click_events
WHERE type = 'SOURCE_CLICK';

DROP VIEW IF EXISTS click_lookup_mv_v2;
