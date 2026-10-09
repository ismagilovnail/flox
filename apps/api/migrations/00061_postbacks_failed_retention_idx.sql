-- +goose NO TRANSACTION

-- Tech audit 2026-10-06, DB2: internal/retention deletes postbacks rows
-- that are not successes once they are RETENTION_FAILED_POSTBACKS_DAYS old.
-- This partial index is what its batched DELETE finds them by — without
-- it, every batch would scan the whole table, successes included.
--
-- CONCURRENTLY, outside a transaction, as 00051: no write lock on
-- postbacks while it builds. An interrupted build leaves an INVALID index;
-- drop it and run again.

-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS postbacks_unsuccessful_created_idx
  ON postbacks (created_at)
  WHERE result <> 'success';

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS postbacks_unsuccessful_created_idx;
