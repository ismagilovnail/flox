-- +goose Up
-- +goose StatementBegin

CREATE TABLE traffic_sources (
  id                ulid PRIMARY KEY,
  organization_id   ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  name              text NOT NULL,
  type              text NOT NULL, -- frontend SourceType: "Facebook" | "TikTok" | "Google" | ... | "Other"
  tracking_template text NOT NULL DEFAULT '',
  cost_integration  text NOT NULL DEFAULT 'none' CHECK (cost_integration IN ('none', 'manual', 'facebook_ads', 'tiktok_ads')),
  status            text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'archived')),
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);

-- §36-TENANCY: every tenant-scoped table's most common query is "list mine,"
-- so organization_id leads every such index in this schema.
CREATE INDEX traffic_sources_organization_id_idx ON traffic_sources (organization_id);

CREATE TRIGGER traffic_sources_set_updated_at
  BEFORE UPDATE ON traffic_sources
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE traffic_sources;
-- +goose StatementEnd
