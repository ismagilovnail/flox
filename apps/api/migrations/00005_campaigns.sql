-- +goose Up
-- +goose StatementBegin

-- No tracking_domain/tracking_id columns here even though the frontend mock
-- (Campaign.trackingDomain/trackingId) has them — that data is a
-- (campaign, domain, slug) tracking_links row instead (§35 lists domains
-- and tracking_links as separate tables from campaigns), so a campaign
-- with more than one tracking link never has to duplicate/desync a domain
-- string that's also stored elsewhere.
CREATE TABLE campaigns (
  id                ulid PRIMARY KEY,
  organization_id   ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  traffic_source_id ulid NOT NULL REFERENCES traffic_sources (id),
  name              text NOT NULL,
  status            text NOT NULL DEFAULT 'draft' CHECK (status IN ('active', 'paused', 'draft', 'archived')),
  fallback_url      text NOT NULL DEFAULT '',
  notes             text NOT NULL DEFAULT '',
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX campaigns_organization_id_idx ON campaigns (organization_id);
CREATE INDEX campaigns_traffic_source_id_idx ON campaigns (traffic_source_id);

CREATE TRIGGER campaigns_set_updated_at
  BEFORE UPDATE ON campaigns
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE campaigns;
-- +goose StatementEnd
