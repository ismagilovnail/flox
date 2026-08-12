-- +goose Up
-- +goose StatementBegin

CREATE TABLE domains (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  domain          text NOT NULL,
  status          text NOT NULL DEFAULT 'pending' CHECK (status IN ('active', 'pending', 'error', 'expired')),
  ssl             text NOT NULL DEFAULT 'none' CHECK (ssl IN ('issued', 'pending', 'none', 'error')),
  purpose         text[] NOT NULL DEFAULT '{}', -- subset of "tracking" | "pwa" | "fallback"
  registrar       text NOT NULL DEFAULT 'unmanaged' CHECK (registrar IN ('namecheap', 'godaddy', 'cloudflare_registrar', 'unmanaged')),
  dns_provider    text NOT NULL DEFAULT 'unmanaged' CHECK (dns_provider IN ('cloudflare', 'route53', 'unmanaged')),
  expires_at      timestamptz,
  verified_at     timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX domains_organization_id_idx ON domains (organization_id);
CREATE UNIQUE INDEX domains_domain_key ON domains (lower(domain));

CREATE TRIGGER domains_set_updated_at
  BEFORE UPDATE ON domains
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- The concrete (domain, slug) → campaign mapping the tracker's hot path
-- resolves on every incoming click (§41/§56). A campaign can have more than
-- one tracking link (e.g. across domains); a domain's slugs must be unique
-- to it so the tracker's lookup is a single indexed equality match.
CREATE TABLE tracking_links (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  campaign_id     ulid NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
  domain_id       ulid NOT NULL REFERENCES domains (id) ON DELETE CASCADE,
  slug            text NOT NULL,
  status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'archived')),
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX tracking_links_organization_id_idx ON tracking_links (organization_id);
CREATE INDEX tracking_links_campaign_id_idx ON tracking_links (campaign_id);
CREATE UNIQUE INDEX tracking_links_domain_id_slug_key ON tracking_links (domain_id, slug);

CREATE TRIGGER tracking_links_set_updated_at
  BEFORE UPDATE ON tracking_links
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE tracking_links;
DROP TABLE domains;
-- +goose StatementEnd
