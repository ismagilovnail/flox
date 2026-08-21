-- Phase 25's schema (a single `events` table + one aggregate) was
-- explicitly disposable — see docs/analytics-pipeline.md, confirmed with
-- the user before Phase 25 started. Phase 26 replaces it wholesale with
-- the real §48 five-table design below. DROP ... IF EXISTS is idempotent,
-- same as every CREATE in this directory, so this is safe to run on an
-- environment that never had the Phase 25 schema at all.
DROP VIEW IF EXISTS events_daily_campaign_mv;
DROP TABLE IF EXISTS events_daily_campaign;
DROP TABLE IF EXISTS events;
