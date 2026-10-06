-- +goose NO TRANSACTION

-- The reconciliation reads recent successful incoming postbacks by event_at
-- every hour; without this it would scan all of postbacks. CONCURRENTLY, as
-- in 00051, so a live database keeps taking postbacks while it builds.

-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS postbacks_success_event_at_idx
  ON postbacks (event_at)
  WHERE result = 'success' AND direction = 'incoming';

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS postbacks_success_event_at_idx;
