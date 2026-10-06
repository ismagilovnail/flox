-- +goose Up
-- +goose StatementBegin

-- Audit 2026-10-06, T4/AN3 — §45: "chargeback (ACCEPT then DECLINE →
-- recorded, revenue reversed)". A CPA_DECLINE or CPA_TRASH recorded while
-- the click stands accepted reverses that CPA_ACCEPT's payout (the FTD only;
-- redeposits are separate deposits with their own txn ids). The reversal is
-- kept on the decline row itself — what was reversed, in the accept's
-- original currency and at the accept's USD value, so the two net to zero —
-- separate from revenue/currency, which stay exactly what the network sent.
-- NULL means the row reversed nothing.
ALTER TABLE postbacks
  ADD COLUMN reversal_revenue   numeric(14, 4),
  ADD COLUMN reversal_currency  text NOT NULL DEFAULT '',
  ADD COLUMN reversal_usd_value numeric(14, 4);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE postbacks
  DROP COLUMN reversal_usd_value,
  DROP COLUMN reversal_currency,
  DROP COLUMN reversal_revenue;
-- +goose StatementEnd
