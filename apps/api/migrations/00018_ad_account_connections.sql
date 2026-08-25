-- +goose Up
-- +goose StatementBegin

-- §74/§27-COST: the storage half of the ad-network cost import phase —
-- credentials letting a later sync (a separate phase, once real Facebook/
-- TikTok app credentials exist) pull spend for a traffic source's
-- CostIntegration ('facebook_ads'/'tiktok_ads', migration 00002). One
-- connection per traffic source (UNIQUE), matching cost_integration's own
-- singular "this source uses at most one integration" design — no
-- separate status column: the row's existence IS "connected," a
-- disconnect is a plain DELETE, a reconnect just inserts a fresh row.
--
-- access_token is stored in plain text: no KMS/envelope-encryption
-- infrastructure exists yet anywhere in this codebase (api_keys, the only
-- prior credential-shaped table, gets to use a one-way hash instead,
-- since it only ever needs to *verify* a key — this table's token must be
-- read back out in full to call the ad platform's API in a later phase,
-- which a hash can never allow). The Go API layer never serializes this
-- column back out after write (see adaccount.Connection's own doc
-- comment) — that boundary is the real control until encryption-at-rest
-- lands as its own hardening phase.
CREATE TABLE ad_account_connections (
  id                ulid PRIMARY KEY,
  organization_id   ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  traffic_source_id ulid NOT NULL UNIQUE REFERENCES traffic_sources (id) ON DELETE CASCADE,
  ad_account_id     text NOT NULL,
  access_token      text NOT NULL,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ad_account_connections_organization_id_idx ON ad_account_connections (organization_id);

CREATE TRIGGER ad_account_connections_set_updated_at
  BEFORE UPDATE ON ad_account_connections
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE ad_account_connections;
-- +goose StatementEnd
