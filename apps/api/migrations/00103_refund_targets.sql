-- Refunds by order and partial refunds (audit UNI-4, §45 amended;
-- docs/conversion.md "Refunds by order").
--
-- A CPA_DECLINE/CPA_TRASH used to reverse the click's accept and nothing
-- else. Now it may reverse a repeat purchase (its txn_id is the repeat
-- purchase's) and may reverse part of a purchase (refund_amount):
--   reverses_postback_id — the success row it reversed, so what is left of
--     that purchase is its revenue minus the reversals pointing at it;
--   refund_kind — '' a decision on the click (the reversal of its accept,
--     as before); 'repeat' the refund of a repeat purchase; 'partial' a
--     partial refund of the accept. Only '' rows count as the click's last
--     decision (re-approval, which accept to reverse), so refunding one
--     order never makes the whole click look declined.
-- Rows from before this migration have neither: no production data
-- existed then.

-- +goose Up
ALTER TABLE postbacks
  ADD COLUMN reverses_postback_id ulid,
  ADD COLUMN refund_kind text NOT NULL DEFAULT ''
    CHECK (refund_kind IN ('', 'repeat', 'partial'));

CREATE INDEX postbacks_reverses_idx ON postbacks (reverses_postback_id)
  WHERE reverses_postback_id IS NOT NULL;

-- +goose Down
DROP INDEX postbacks_reverses_idx;
ALTER TABLE postbacks DROP COLUMN reverses_postback_id, DROP COLUMN refund_kind;
