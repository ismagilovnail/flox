-- landing_id on every event-derived table, so the report builder's
-- `landing` dimension (chstore.DimLanding) has a real column to group by
-- (docs/analytics-report-api.md).
--
-- Populated from the moment apps/tracker serves landings: SOURCE_CLICK
-- and LAND_VIEW carry the landing the visitor was shown (empty when the
-- flow has no landing stage, or it was skipped — paused landing, no
-- signing key), LAND_CLICK carries it from its signed click link, and
-- conversions inherit it from their click through attribution, exactly
-- like stream_set_id/flow_id. Rows written before this migration have ''
-- — the honest value: nothing served landings then.
--
-- ADD COLUMN appends at the end of each table, which is where
-- chstore.EventStore's positional batch.Append writes it — the insert
-- code and this file must change together. ADD COLUMN IF NOT EXISTS is
-- a metadata-only change in ClickHouse (no rewrite of existing parts),
-- and idempotent like every other file this package re-applies on boot.
ALTER TABLE click_events ADD COLUMN IF NOT EXISTS landing_id String;
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS landing_id String;
ALTER TABLE conversion_events ADD COLUMN IF NOT EXISTS landing_id String;
