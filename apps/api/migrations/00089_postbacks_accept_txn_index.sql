-- +goose NO TRANSACTION

-- An accept after a decline counts as a re-approval only with a txn_id no
-- earlier accept of the click carried, recorded or refused (audit
-- 2026-10-15, DATA-3; internal/conversion redecisionFor). The lookup reads
-- a click's accepts of every result; postbacks_click_success_idx covers
-- only successes. CONCURRENTLY, as in 00055.

-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS postbacks_accept_txn_idx
  ON postbacks (organization_id, click_id, network_txn_id)
  WHERE status = 'CPA_ACCEPT' AND network_txn_id <> '' AND direction = 'incoming';

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS postbacks_accept_txn_idx;
