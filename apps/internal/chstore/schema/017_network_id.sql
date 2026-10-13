-- network_id on click_events and tracking_events — the report builder's
-- `network` dimension (chstore.DimNetwork). conversion_events has carried
-- network_id since 003: the network that sent the postback.
--
-- Set by apps/tracker on a click routed to an offer: that offer's network
-- at click time (an offer moved to another network later keeps its old
-- clicks where they were). Carried onto LAND_*, PWA_* and POSTLANDING_*
-- through the signed link. Empty for redirect flows, fallbacks and
-- blocked clicks.
--
-- Rows from before this get it once from their offer_id and the offer's
-- network at the time of the backfill (chstore.BackfillNetworks, run by
-- apps/worker).
--
-- Appended last, after 016's postlanding_id — chstore.EventStore appends
-- positionally; keep the order in sync.
ALTER TABLE click_events    ADD COLUMN IF NOT EXISTS network_id String;
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS network_id String;
