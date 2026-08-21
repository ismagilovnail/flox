-- +goose Up
-- +goose StatementBegin

-- §50-FX / CLAUDE.md #7: "store both original and USD" — 00009 gave
-- cost_entries an original amount+currency but never the USD side.
-- Nullable, same reasoning as conversion_entries.usd_value (00013): a
-- missing FX rate for (currency, entry_date) is stored as NULL, never as
-- 0 — CLAUDE.md #6's "no cost shows as '—', never zero" already breaks if
-- "no USD rate yet" and "confirmed zero spend" share one representation.
ALTER TABLE cost_entries ADD COLUMN amount_usd numeric(14, 4);

-- There is no auth yet (Phase 28) — no session, no real user id to put
-- here for a cost entry created through the dev X-Organization-Id
-- stand-in. NOT NULL would force every dev-created row to reference a
-- fabricated user, which is worse than an honest NULL. Phase 28 should
-- backfill/re-tighten this once real sessions exist.
ALTER TABLE cost_entries ALTER COLUMN created_by_user_id DROP NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE cost_entries ALTER COLUMN created_by_user_id SET NOT NULL;
ALTER TABLE cost_entries DROP COLUMN amount_usd;
-- +goose StatementEnd
