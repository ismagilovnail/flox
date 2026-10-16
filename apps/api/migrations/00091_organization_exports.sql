-- An organization's data export (audit 2026-10-15, GDPR-1 part 4;
-- docs/organization-export.md). Requested through the API, built by
-- apps/worker (internal/orgexport) into a ZIP in object storage under an
-- unguessable key, downloaded only through the API, deleted at expires_at.

-- +goose Up
CREATE TABLE organization_exports (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  requested_by    ulid REFERENCES users(id) ON DELETE SET NULL,
  status          text NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending', 'running', 'done', 'failed', 'expired')),
  from_date       date NOT NULL,
  to_date         date NOT NULL,
  object_key      text NOT NULL DEFAULT '',
  size_bytes      bigint NOT NULL DEFAULT 0,
  error           text NOT NULL DEFAULT '',
  created_at      timestamptz NOT NULL DEFAULT now(),
  started_at      timestamptz,
  finished_at     timestamptz,
  expires_at      timestamptz
);

CREATE INDEX organization_exports_org_idx ON organization_exports (organization_id, created_at DESC);
CREATE INDEX organization_exports_open_idx ON organization_exports (status, created_at)
  WHERE status IN ('pending', 'running', 'done');

-- +goose Down
DROP TABLE organization_exports;
