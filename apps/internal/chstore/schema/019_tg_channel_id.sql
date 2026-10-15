-- tg_channel_id on click_events and tracking_events — the report builder's
-- `telegram_channel` dimension (chstore.DimTelegramChannel).
--
-- Set by apps/tracker's Telegram webhook on TG_JOIN only: the FLOX id of
-- the channel joined (telegram_channels.id). Every other event, and
-- TG_JOIN rows from before this, have '' — which channel an older join
-- was in was never recorded. click_events never carries one (a click is
-- not a join); it gets the column so both tables keep one dimension set.
--
-- Appended last, after 017's network_id — chstore.EventStore appends
-- positionally; keep the order in sync.
ALTER TABLE click_events    ADD COLUMN IF NOT EXISTS tg_channel_id String;
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS tg_channel_id String;
