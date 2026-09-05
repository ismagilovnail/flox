-- +goose Up
-- +goose StatementBegin

-- §30 Settings > Integrations: org-level, non-ad-platform credential
-- storage (Namecheap, Cloudflare, Slack). Facebook/TikTok/Google Ads are
-- deliberately NOT here — those are per-traffic-source (ad_account_
-- connections, migration 00018), matching trafficsource.CostIntegration's
-- own singular design; the Settings panel only ever shows a real
-- read-only aggregate for those three, it doesn't duplicate their
-- connection flow.
--
-- One row per (org, provider) — connecting replaces the row in place,
-- same upsert-by-natural-key precedent ad_account_connections already
-- established. encrypted_payload is AES-256-GCM ciphertext of a small
-- JSON object holding that provider's own credential fields (e.g.
-- {"apiUser":"...","apiKey":"..."} for Namecheap) — encrypted, not
-- hashed, because a real API call to verify/use the credential needs it
-- back in plaintext, the same reasoning totp.EncryptSecret's own doc
-- comment gives for TOTP secrets (ad_account_connections predates this
-- encryption infrastructure and stores its token in plain text instead;
-- this table uses the real encryption apps/internal/totp already
-- established rather than repeating that now-avoidable gap).
--
-- last_error is what turns a row from "connected" into "error" without
-- deleting it — set by a later re-verification (POST .../verify)
-- against the currently-stored credential, distinct from Connect itself,
-- which always requires verification to succeed before a row is ever
-- written at all.
CREATE TABLE org_integrations (
  id                ulid PRIMARY KEY,
  organization_id   ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  provider          text NOT NULL CHECK (provider IN ('namecheap', 'cloudflare', 'slack')),
  encrypted_payload text NOT NULL,
  last_verified_at  timestamptz NOT NULL DEFAULT now(),
  last_error        text,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  UNIQUE (organization_id, provider)
);

CREATE INDEX org_integrations_organization_id_idx ON org_integrations (organization_id);

CREATE TRIGGER org_integrations_set_updated_at
  BEFORE UPDATE ON org_integrations
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE org_integrations;
-- +goose StatementEnd
