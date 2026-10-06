-- traffic_source_id on every event-derived table — the report builder's
-- `source` dimension (chstore.DimSource), same shape as 008/009/010.
--
-- Set by apps/tracker from the campaign's traffic source at click time and
-- carried onto LAND_*/PWA_* events through the signed link claims and onto
-- conversions through attribution. Until now `source` was a proxy that
-- grouped by campaign_id, so two campaigns on one source never merged.
-- Rows from before this have '' — backfill them from Postgres
-- (campaigns.traffic_source_id) with docs/analytics-report-api.md's
-- "Backfilling traffic_source_id" recipe.
--
-- Appended last, after 010's pwa_id — chstore.EventStore's positional
-- batch.Append writes it after pwa_id; keep the order in sync.
ALTER TABLE click_events ADD COLUMN IF NOT EXISTS traffic_source_id String;
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS traffic_source_id String;
ALTER TABLE conversion_events ADD COLUMN IF NOT EXISTS traffic_source_id String;
