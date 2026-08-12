-- +goose Up
-- +goose StatementBegin

CREATE TABLE networks (
  id                ulid PRIMARY KEY,
  organization_id   ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  name              text NOT NULL,
  postback_url      text NOT NULL DEFAULT '', -- macro template, e.g. "...?click_id={click_id}&status={status}"
  -- §45 per-network dedup override: accept postbacks FLOX would otherwise
  -- drop as duplicates on (click_id, status).
  accept_duplicates boolean NOT NULL DEFAULT false,
  status            text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'archived')),
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX networks_organization_id_idx ON networks (organization_id);

CREATE TRIGGER networks_set_updated_at
  BEFORE UPDATE ON networks
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE offers (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  network_id      ulid NOT NULL REFERENCES networks (id) ON DELETE CASCADE,
  name            text NOT NULL,
  countries       text[] NOT NULL DEFAULT '{}', -- ISO-3166 alpha-2 codes
  payout          numeric(14, 4) NOT NULL,
  currency        text NOT NULL DEFAULT 'USD',
  cap             integer, -- daily conversion cap; NULL = uncapped
  status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'archived')),
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX offers_organization_id_idx ON offers (organization_id);
CREATE INDEX offers_network_id_idx ON offers (network_id);

CREATE TRIGGER offers_set_updated_at
  BEFORE UPDATE ON offers
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- organization_id is denormalized onto this (and every other) child table
-- rather than left implicit via a join through offer_id → offers. §36-
-- TENANCY calls cross-tenant isolation a hard invariant, not a convention:
-- a repository query that filters offer_links directly by organization_id
-- can't leak data even if a future join condition is written wrong.
CREATE TABLE offer_links (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  offer_id        ulid NOT NULL REFERENCES offers (id) ON DELETE CASCADE,
  label           text NOT NULL,
  url             text NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX offer_links_organization_id_idx ON offer_links (organization_id);
CREATE INDEX offer_links_offer_id_idx ON offer_links (offer_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE offer_links;
DROP TABLE offers;
DROP TABLE networks;
-- +goose StatementEnd
