-- offer_id and traffic_source_id on ltv_events — §26.5's LTV "source" and
-- "offer" filters and breakdowns (chstore.LTVFilter, ltv.Dimension).
-- conversion_events has carried both since 009/011 (internal/conversion
-- copies them from the attributed click).
--
-- The view is changed in place (MODIFY QUERY) rather than dropped and
-- recreated as 014 did, so no conversion inserted during the upgrade
-- misses its LTV row. Rows from before this get both from their click
-- (chstore.BackfillLTVAttribution, run by apps/worker).
ALTER TABLE ltv_events ADD COLUMN IF NOT EXISTS offer_id String;
ALTER TABLE ltv_events ADD COLUMN IF NOT EXISTS traffic_source_id String;

ALTER TABLE ltv_events_mv_v3 MODIFY QUERY
SELECT
    organization_id, click_id, campaign_id, network_id, country, event_at, type, usd_value, has_usd_value, event_id,
    offer_id, traffic_source_id
FROM conversion_events
WHERE type IN ('CPA_HOLD', 'CPA_ACCEPT', 'CPA_REDEP', 'CPA_DECLINE', 'CPA_TRASH');
