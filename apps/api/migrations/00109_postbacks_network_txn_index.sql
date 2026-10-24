-- +goose NO TRANSACTION

-- The tracker asks whether a store's order was recorded already (a
-- replayed webhook) and which purchase a refund is of, by the order id
-- kept as the postback's network_txn_id (audit UNI-4 part 3,
-- internal/shopify). CONCURRENTLY, as in 00089.

-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS postbacks_network_txn_idx
  ON postbacks (organization_id, network_id, network_txn_id)
  WHERE network_txn_id <> '' AND result = 'success' AND direction = 'incoming';

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS postbacks_network_txn_idx;
