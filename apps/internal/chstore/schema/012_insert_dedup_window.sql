-- Audit 2026-10-06, D2: apps/worker sends every flushed batch with an
-- insert_deduplication_token (chstore.WithBatchID). A non-replicated
-- MergeTree only remembers block tokens when this window is non-zero (its
-- default is 0 = no deduplication), and every materialized-view target
-- needs it too, or a retried batch is skipped in the source table but
-- counted again in the daily aggregates and LTV.
--
-- 100,000 blocks: each flushed batch is at most one block per table. At
-- 10k events/s (~20 batches/s across up to 3 tables) that still remembers
-- ~25 minutes — longer than the 5-minute reaper window, the slowest retry.
-- Hashes only, a few MB on disk. MODIFY SETTING is idempotent.
--
-- Side effect: an insert WITHOUT a token is now deduplicated by a hash of
-- its content. Every writer must send a token (chstore.WithBatchID).
ALTER TABLE click_events                     MODIFY SETTING non_replicated_deduplication_window = 100000;
ALTER TABLE tracking_events                  MODIFY SETTING non_replicated_deduplication_window = 100000;
ALTER TABLE conversion_events                MODIFY SETTING non_replicated_deduplication_window = 100000;
ALTER TABLE postback_events                  MODIFY SETTING non_replicated_deduplication_window = 100000;
ALTER TABLE click_events_daily_campaign      MODIFY SETTING non_replicated_deduplication_window = 100000;
ALTER TABLE click_events_daily_geo           MODIFY SETTING non_replicated_deduplication_window = 100000;
ALTER TABLE conversion_events_daily_campaign MODIFY SETTING non_replicated_deduplication_window = 100000;
ALTER TABLE ltv_events                       MODIFY SETTING non_replicated_deduplication_window = 100000;
