-- The affiliate-platform template a network was set up from (audit
-- 2026-10-15, INT-3; apps/web lib/network-templates.ts): it decides which
-- of the platform's macros the incoming postback URL shown to the operator
-- carries, and the offer-link parameter for FLOX's click id.

-- +goose Up
ALTER TABLE networks ADD COLUMN template_key text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE networks DROP COLUMN template_key;
