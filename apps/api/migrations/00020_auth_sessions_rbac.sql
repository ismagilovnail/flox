-- +goose Up
-- +goose StatementBegin

-- §52/Phase 28: real password auth. Migration 00001 deliberately left this
-- off ("Auth fields (password hash, sessions, MFA) are deliberately
-- absent — that's Phase 28's job"). DEFAULT '' rather than NULL: an empty
-- string can never bcrypt-match a real password, so it doubles as the
-- sentinel for "no password set yet" (a shell user created by an invite,
-- before they've accepted it) without a separate nullability check at
-- every read site.
ALTER TABLE users ADD COLUMN password_hash text NOT NULL DEFAULT '';

-- Server-side sessions, not JWTs (confirmed via AskUserQuestion): trivially
-- revocable (DELETE the row) with no denylist/rotation machinery. Scoped to
-- exactly one organization_id, chosen at login/signup/invite-accept time —
-- tenant.OrgID(ctx) resolves to a single org per request (§36-TENANCY), so
-- a session that could span multiple orgs would need an explicit "switch
-- organization" concept nothing in this codebase's frontend exposes yet
-- (no org switcher UI exists). A user who belongs to more than one org
-- simply holds a separate session per org.
--
-- token_hash, never the raw bearer token — same one-way-hash-at-rest
-- precedent as api_keys.key_hash (migration 00010): the raw token is a
-- session cookie value, shown/used once, never persisted in recoverable
-- form.
CREATE TABLE sessions (
  id              ulid PRIMARY KEY,
  user_id         ulid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  token_hash      text NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  expires_at      timestamptz NOT NULL
);

CREATE UNIQUE INDEX sessions_token_hash_key ON sessions (token_hash);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);

-- Invite tokens live on the membership row itself rather than a separate
-- table: migration 00001's memberships.status already models 'invited' as
-- a first-class state, and a membership IS the invite once it exists (an
-- accepted invite is just that same row transitioning to status='active').
-- Only ever set while status = 'invited'; cleared on accept. Same one-way-
-- hash-at-rest precedent as sessions.token_hash above.
ALTER TABLE memberships ADD COLUMN invite_token_hash text;
ALTER TABLE memberships ADD COLUMN invite_token_expires_at timestamptz;

-- Partial (not plain UNIQUE): every already-accepted/never-invited
-- membership has invite_token_hash = NULL, and Postgres treats every NULL
-- as distinct for a plain unique index anyway — this index exists purely
-- so two *pending* invites can never collide on the same token by
-- construction, not to enforce anything about NULLs.
CREATE UNIQUE INDEX memberships_invite_token_hash_key ON memberships (invite_token_hash) WHERE invite_token_hash IS NOT NULL;

-- The role -> permission mapping migration 00001 explicitly deferred
-- ("not yet wired to `roles`... belongs to Phase 28, not this schema-only
-- phase"). Seeded here to match apps/web's src/lib/mock/team.ts
-- ROLE_PERMISSIONS table exactly, cell for cell — that mock has been this
-- project's reference vocabulary for §52 since Phase 14, not a
-- placeholder being replaced now.
CREATE TABLE role_permissions (
  role_id       ulid NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
  permission_id ulid NOT NULL REFERENCES permissions (id) ON DELETE CASCADE,
  PRIMARY KEY (role_id, permission_id)
);

-- Owner and Admin: every permission.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE r.key IN ('Owner', 'Admin');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.key = ANY (ARRAY[
  'campaign.read', 'campaign.write', 'analytics.read', 'offer.read',
  'offer.write', 'source.read', 'source.write', 'team.read'
])
WHERE r.key = 'Manager';

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.key = ANY (ARRAY[
  'campaign.read', 'campaign.write', 'analytics.read', 'offer.read',
  'source.read', 'source.write'
])
WHERE r.key = 'Buyer';

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.key = ANY (ARRAY[
  'campaign.read', 'analytics.read', 'offer.read', 'source.read'
])
WHERE r.key = 'Analyst';

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.key = ANY (ARRAY[
  'campaign.read', 'analytics.read'
])
WHERE r.key = 'Viewer';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE role_permissions;
DROP INDEX memberships_invite_token_hash_key;
ALTER TABLE memberships DROP COLUMN invite_token_expires_at;
ALTER TABLE memberships DROP COLUMN invite_token_hash;
DROP TABLE sessions;
ALTER TABLE users DROP COLUMN password_hash;
-- +goose StatementEnd
