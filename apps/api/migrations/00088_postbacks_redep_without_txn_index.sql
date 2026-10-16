-- +goose NO TRANSACTION

-- The network page says when redeposits arrive without txn_id and repeat
-- deposits are refused as duplicates (audit 2026-10-15, DATA-2): it counts
-- a network's REDEP rows with an empty event_ref over 30 days. Without
-- this it would scan every postback of the network. CONCURRENTLY, as in
-- 00055.

-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS postbacks_redep_without_txn_idx
  ON postbacks (organization_id, network_id, created_at)
  WHERE status = 'CPA_REDEP' AND event_ref = '' AND direction = 'incoming';

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS postbacks_redep_without_txn_idx;
