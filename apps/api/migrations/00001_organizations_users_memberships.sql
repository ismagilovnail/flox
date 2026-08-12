-- +goose Up
-- +goose StatementBegin

-- ULID, consistently, everywhere (§35: "choose ULID, not 'UUID or ULID' —
-- one standard"). Stored as text with a format check rather than a native
-- type/extension, so every service connecting to this database (including
-- ones outside this Go module) can generate/read them without a Postgres
-- extension dependency. Crockford base32, 26 chars, first char in [0-7]
-- (time component high bits).
CREATE DOMAIN ulid AS text CHECK (VALUE ~ '^[0-7][0-9A-HJKMNP-TV-Z]{25}$');

-- Attached to every table with updated_at (§35) so it can never go stale
-- because an application code path forgot to set it.
CREATE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Not tenant-scoped by definition — this IS the tenant table (§36-TENANCY).
CREATE TABLE organizations (
  id         ulid PRIMARY KEY,
  name       text NOT NULL,
  timezone   text NOT NULL DEFAULT 'UTC',
  currency   text NOT NULL DEFAULT 'USD',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER organizations_set_updated_at
  BEFORE UPDATE ON organizations
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- A user can belong to multiple organizations (via memberships), so users
-- itself is not org-scoped. Auth fields (password hash, sessions, MFA) are
-- deliberately absent — that's Phase 28's job; this table only needs to
-- exist now so memberships/audit_logs/etc. have something to reference.
CREATE TABLE users (
  id         ulid PRIMARY KEY,
  email      text NOT NULL,
  name       text NOT NULL,
  avatar_url text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX users_email_key ON users (lower(email));

CREATE TRIGGER users_set_updated_at
  BEFORE UPDATE ON users
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- The six roles are a fixed, platform-wide set today (CLAUDE.md: "reusing
-- Team's canonical roles" — the frontend never lets a user invent a new
-- role), so this is a small global lookup table, not tenant-scoped, seeded
-- once below rather than created per organization.
CREATE TABLE roles (
  id          ulid PRIMARY KEY,
  key         text NOT NULL UNIQUE, -- "Owner" | "Admin" | "Manager" | "Buyer" | "Analyst" | "Viewer"
  description text NOT NULL
);

-- Catalog of permission keys the frontend's mock/team.ts Permission union
-- already enumerates. Not yet wired to `roles` (no role_permissions join
-- table) — that mapping is real RBAC-enforcement business logic and
-- belongs to Phase 28, not this schema-only phase. Existing today only so
-- Phase 28 has a stable set of permission keys to build on instead of
-- inventing its own.
CREATE TABLE permissions (
  id          ulid PRIMARY KEY,
  key         text NOT NULL UNIQUE, -- e.g. "campaign.write"
  description text NOT NULL
);

INSERT INTO roles (id, key, description) VALUES
  ('3JDXP6ABBFEXQ6AWHG60R30C1G', 'Owner',   'Full access, including billing and org deletion.'),
  ('3JDXP6ABB1CHPPJVHG60R30C1G', 'Admin',   'Full access except billing and org deletion.'),
  ('3JDXP6ABBDC5Q62SV5E8R30C1G', 'Manager', 'Manage campaigns, offers, sources; no team/settings changes.'),
  ('3JDXP6ABB2ENWPAWHG60R30C1G', 'Buyer',   'Create and manage own campaigns; read-only elsewhere.'),
  ('3JDXP6ABB1DSGPRYBKEGR30C1G', 'Analyst', 'Read-only across campaigns and analytics.'),
  ('3JDXP6ABBPD5JQESBJ60R30C1G', 'Viewer',  'Read-only, no analytics export.');

INSERT INTO permissions (id, key, description) VALUES
  ('3GCNS6TBB3C5PQ0RB9CXQ2TWK5', 'campaign.read',  'View campaigns and their configuration.'),
  ('3GCNS6TBB3C5PQ0RB9CXQ2TXVJ', 'campaign.write', 'Create, edit, archive campaigns.'),
  ('3GCNS6TBB1DSGPRYBMD5HQ6BBJ', 'analytics.read', 'View analytics and reports.'),
  ('3GCNS6TBBFCSK6AWHDE9JP2S1G', 'offer.read',     'View offers and networks.'),
  ('3GCNS6TBBFCSK6AWHDEXS6JX35', 'offer.write',    'Create, edit, archive offers and networks.'),
  ('3GCNS6TBBKDXTQ4RV55NS6ARB4', 'source.read',    'View traffic sources.'),
  ('3GCNS6TBBKDXTQ4RV55NVQ4TBM', 'source.write',   'Create, edit, archive traffic sources.'),
  ('3GCNS6TBBMCNGPTBBJCNGP8C1G', 'team.read',      'View team members.'),
  ('3GCNS6TBBMCNGPTBBQE9MQ8S9G', 'team.write',     'Invite, edit, remove team members.'),
  ('3GCNS6TBBKCNT78TBECXSJTXVJ', 'settings.write', 'Change organization settings.');

CREATE TABLE memberships (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  user_id         ulid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  role_id         ulid NOT NULL REFERENCES roles (id),
  status          text NOT NULL DEFAULT 'invited' CHECK (status IN ('active', 'invited', 'suspended')),
  invited_at      timestamptz NOT NULL DEFAULT now(),
  last_active_at  timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (organization_id, user_id)
);

-- The membership list for an org, and "which orgs is this user in" — the
-- two access patterns every session/membership lookup needs.
CREATE INDEX memberships_organization_id_idx ON memberships (organization_id);
CREATE INDEX memberships_user_id_idx ON memberships (user_id);

CREATE TRIGGER memberships_set_updated_at
  BEFORE UPDATE ON memberships
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE memberships;
DROP TABLE permissions;
DROP TABLE roles;
DROP TABLE users;
DROP TABLE organizations;
DROP FUNCTION set_updated_at();
DROP DOMAIN ulid;
-- +goose StatementEnd
