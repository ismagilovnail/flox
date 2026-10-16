-- Organization deletion (audit 2026-10-15, GDPR-1 part 2;
-- docs/organization-deletion.md).
--
-- An owner schedules it (deletion_scheduled_at, 7 days ahead) and can
-- cancel until then. When it is due, apps/worker (internal/orgdeletion)
-- deletes the organizations row — every tenant table cascades — and the
-- users who belonged to it and to no other organization, in one
-- transaction, and leaves a tombstone in organization_deletions. The
-- tombstone has no foreign key: it outlives the organization, and the
-- worker purges ClickHouse, object storage and Redis from it an hour later
-- (purged_at), once in-flight events have landed.

-- +goose Up
ALTER TABLE organizations
  ADD COLUMN deletion_scheduled_at timestamptz,
  ADD COLUMN deletion_requested_at timestamptz,
  ADD COLUMN deletion_requested_by ulid REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX organizations_deletion_due_idx
  ON organizations (deletion_scheduled_at) WHERE deletion_scheduled_at IS NOT NULL;

CREATE TABLE organization_deletions (
  organization_id ulid PRIMARY KEY,
  requested_at    timestamptz,
  deleted_at      timestamptz NOT NULL DEFAULT now(),
  users_deleted   integer NOT NULL DEFAULT 0,
  purged_at       timestamptz,
  last_error      text NOT NULL DEFAULT ''
);

CREATE INDEX organization_deletions_unpurged_idx
  ON organization_deletions (deleted_at) WHERE purged_at IS NULL;

-- +goose Down
DROP TABLE organization_deletions;
DROP INDEX organizations_deletion_due_idx;
ALTER TABLE organizations
  DROP COLUMN deletion_requested_by,
  DROP COLUMN deletion_requested_at,
  DROP COLUMN deletion_scheduled_at;
