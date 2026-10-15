-- A network's payout currency (audit 2026-10-15, DATA-1).
--
-- A postback with revenue but no currency used to be stored without a USD
-- value, and so dropped out of every revenue/ROI figure without a word.
-- Now apps/internal/conversion takes such an amount to be in the network's
-- currency. Existing networks get USD: affiliate networks pay in USD unless
-- the operator says otherwise, and the column's whole point is that an
-- empty value no longer exists.
--
-- Numbered 00086: 00085 is the gallery-transcoding work still in progress
-- in the working tree.

-- +goose Up
ALTER TABLE networks
  ADD COLUMN currency text NOT NULL DEFAULT 'USD'
    CHECK (currency ~ '^[A-Z]{3}$');

-- +goose Down
ALTER TABLE networks DROP COLUMN currency;
