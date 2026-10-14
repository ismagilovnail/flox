-- API key rotation with a grace period (§54; gap 14 of
-- docs/spec-conformance-2026-10-11.md).
--
-- Rotating a key mints a new row (same name and scope, a new secret) and
-- gives the old one expires_at = now() + 24h: integrations keep working on
-- the old secret until they switch. An expired key authenticates nothing
-- and leaves the list, like a revoked one; the row stays for the audit
-- trail. replaced_by_id points from the old key to its successor, and a
-- key that already has one cannot be rotated again (rotate the new one).

-- +goose Up
-- +goose StatementBegin
ALTER TABLE api_keys
  ADD COLUMN expires_at timestamptz,
  ADD COLUMN replaced_by_id ulid REFERENCES api_keys (id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE api_keys
  DROP COLUMN replaced_by_id,
  DROP COLUMN expires_at;
-- +goose StatementEnd
