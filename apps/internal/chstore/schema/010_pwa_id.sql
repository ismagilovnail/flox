-- pwa_id on every event-derived table — the report builder's `pwa`
-- dimension (chstore.DimPwa), same shape as 008_landing_id/009_offer_id.
--
-- Set by apps/tracker when a click's routed path includes the flow's PWA
-- stage (internal type, not paused, reachable — i.e. no landing before
-- it, or an internal landing whose click-through leads to it): on
-- SOURCE_CLICK, LAND_*, and the PWA_VIEW/PWA_INSTALL/PWA_OPEN events
-- the install page itself produces. Conversions inherit it through
-- attribution. Rows from before this have ''.
--
-- Appended last, after 009's offer_id — chstore.EventStore's positional
-- batch.Append writes landing_id, offer_id, pwa_id; keep the order in sync.
ALTER TABLE click_events ADD COLUMN IF NOT EXISTS pwa_id String;
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS pwa_id String;
ALTER TABLE conversion_events ADD COLUMN IF NOT EXISTS pwa_id String;
