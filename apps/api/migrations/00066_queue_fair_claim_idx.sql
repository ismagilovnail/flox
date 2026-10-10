-- +goose NO TRANSACTION

-- Tech audit 2026-10-06, M2: the queues are claimed fairly across
-- organizations (internal/pgqueue fair claim) — each claim finds the
-- organizations with pending rows by a skip scan, one index probe per
-- organization, then takes each one's oldest due rows. These indexes are
-- what both steps read: organization first, then the due order.
--
-- event_queue / postback_attempt_queue: fresh rows only (batch_id IS NULL;
-- a retry batch is claimed whole, by batch_id). postback_deliveries: every
-- row still to attempt.
--
-- CONCURRENTLY, as 00061: no write lock on the queues while they build.

-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS event_queue_fair_idx
  ON event_queue (organization_id, next_attempt_at, id)
  WHERE status = 'queued' AND batch_id IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS postback_attempt_queue_fair_idx
  ON postback_attempt_queue (organization_id, next_attempt_at, id)
  WHERE status = 'queued' AND batch_id IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS postback_deliveries_fair_idx
  ON postback_deliveries (organization_id, next_attempt_at, id)
  WHERE delivery_status IN ('queued', 'retrying');

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS postback_deliveries_fair_idx;
DROP INDEX CONCURRENTLY IF EXISTS postback_attempt_queue_fair_idx;
DROP INDEX CONCURRENTLY IF EXISTS event_queue_fair_idx;
