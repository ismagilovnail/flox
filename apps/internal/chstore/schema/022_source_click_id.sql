-- external_click_id stops repeating fbclid/ttclid (audit DATA-7). The
-- tracker used to store fbclid or ttclid there when the link carried no
-- external_click_id, so each click id was stored twice, in click_events
-- and again in click_lookup: 286 of a click's 827 bytes. From this file on
-- it holds only what arrived as external_click_id, and readers take the
-- source's click id as external_click_id, else fb_click_id, else
-- tt_click_id (chstore.SourceClickIDExpr). Older rows still have the copy;
-- the same expression reads them the same way.
--
-- Attribution finds a click by that expression, so the bloom filter moves
-- from the column to it. The new index is built for existing parts before
-- the old one goes. Each statement is safe to re-run.
--
-- notEmpty(), not != '': the query analyzer rewrites x != '' into
-- notEmpty(x), and an index whose expression is spelled otherwise is
-- silently not used (ClickHouse 24.10).
ALTER TABLE click_events ADD INDEX IF NOT EXISTS idx_source_click_id
    if(notEmpty(external_click_id), external_click_id, if(notEmpty(fb_click_id), fb_click_id, tt_click_id))
    TYPE bloom_filter(0.01) GRANULARITY 4;
ALTER TABLE click_events MATERIALIZE INDEX idx_source_click_id SETTINGS mutations_sync = 1;
ALTER TABLE click_events DROP INDEX IF EXISTS idx_external_click_id;
