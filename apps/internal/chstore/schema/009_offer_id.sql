-- offer_id on every event-derived table — the report builder's `offer`
-- dimension (chstore.DimOffer), same shape and same reasoning as
-- 008_landing_id.sql.
--
-- Set by apps/tracker from routing.RouteResult.OfferID: only when the
-- click was actually sent to its flow's offer — empty for a redirect flow
-- and for an inactive offer that fell through to a fallback URL. Carried
-- onto LAND_VIEW/LAND_CLICK (the offer the landing leads to) and onto
-- conversions through attribution. Rows from before this have ''.
--
-- Appended last, after 008's landing_id — chstore.EventStore's positional
-- batch.Append writes landing_id then offer_id; keep the order in sync.
ALTER TABLE click_events ADD COLUMN IF NOT EXISTS offer_id String;
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS offer_id String;
ALTER TABLE conversion_events ADD COLUMN IF NOT EXISTS offer_id String;
