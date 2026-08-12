-- +goose Up
-- +goose StatementBegin

-- Only the prefix is ever displayed after creation (frontend ApiKey.prefix);
-- the full key is shown once at creation time and never persisted in
-- recoverable form — key_hash stores a one-way hash for verification.
CREATE TABLE api_keys (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  name            text NOT NULL,
  prefix          text NOT NULL,
  key_hash        text NOT NULL,
  scope           text NOT NULL CHECK (scope IN ('read', 'write', 'admin')),
  created_by_user_id ulid NOT NULL REFERENCES users (id),
  created_at      timestamptz NOT NULL DEFAULT now(),
  last_used_at    timestamptz,
  revoked_at      timestamptz
);

CREATE INDEX api_keys_organization_id_idx ON api_keys (organization_id);
CREATE UNIQUE INDEX api_keys_key_hash_key ON api_keys (key_hash);

-- Append-only (§54-style): who did what, when, to which entity. actor_user_id
-- is nullable for system/automated actions (e.g. a postback processor, a
-- scheduled job) that have no human behind them.
CREATE TABLE audit_logs (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  actor_user_id   ulid REFERENCES users (id),
  action          text NOT NULL, -- e.g. "campaign.created", "offer.archived"
  entity_type     text NOT NULL,
  entity_id       text NOT NULL,
  metadata        jsonb NOT NULL DEFAULT '{}',
  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_logs_organization_id_created_at_idx ON audit_logs (organization_id, created_at DESC);
CREATE INDEX audit_logs_entity_idx ON audit_logs (entity_type, entity_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE audit_logs;
DROP TABLE api_keys;
-- +goose StatementEnd
