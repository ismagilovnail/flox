-- +goose Up
-- +goose StatementBegin

-- Custom Metrics (§30.5, §50) — user-built formulas over the metrics
-- registry. Frontend has carried the real formula engine (tokenizer/
-- parser/evaluator, apps/web/src/lib/formula-engine.ts) and the full
-- builder UI (features/custom-metrics/*) against a zustand mock store
-- since its own frontend-first phase; this migration + apps/internal/
-- custommetrics is the real backend replacing it. Computation itself
-- stays client-side (features/analytics/report-table.tsx already
-- evaluates a published metric's formula against real /analytics/report
-- row data — that path needs no backend change); this table exists to
-- make the metric definitions themselves real, tenant-scoped, and
-- safely validated before they're ever saved.
--
-- `group_name`, not `group` — avoids quoting a SQL keyword at every call
-- site for no benefit. `targets` mirrors the frontend's ShowInTarget[]
-- (report_builder/campaigns_table/offers_table/sources_table) as a
-- plain text array — small, non-relational, never queried by value.
CREATE TABLE custom_metrics (
  id                 ulid PRIMARY KEY,
  organization_id    ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  name               text NOT NULL,
  group_name         text NOT NULL,
  formula            text NOT NULL,
  format             text NOT NULL CHECK (format IN ('int', 'currency', 'percent', 'ratio')),
  status             text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
  active             boolean NOT NULL DEFAULT false,
  targets            text[] NOT NULL DEFAULT '{}',
  created_by_user_id ulid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX custom_metrics_organization_id_idx ON custom_metrics (organization_id);

CREATE TRIGGER custom_metrics_set_updated_at
  BEFORE UPDATE ON custom_metrics
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- §30.5's own "Role access" section names Owner/Tech/Lead/Buyer — stale
-- v2-era role names that don't match this codebase's real six roles
-- (migration 00001). Mapped onto the real set using the exact tiering
-- the frontend's own mock UI (features/custom-metrics/custom-metrics-
-- list.tsx's canManageAny/canCreate/canManage) already implied before
-- any backend existed: Owner/Admin manage any metric; Manager creates
-- and manages only metrics it created; Buyer/Analyst/Viewer see
-- published+active metrics only, same as everyone else's read-only tier
-- elsewhere (analytics.read is already granted to all six roles).
INSERT INTO permissions (id, key, description) VALUES
  ('11HFE865V215DE1CTEWH0AVNH9', 'custom_metric.read', 'View published, active custom metrics.'),
  ('3N65R6PPG2X7R5JQC42EJ5E6RH', 'custom_metric.write', 'Create and manage custom metrics.');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.key = ANY (ARRAY['custom_metric.read', 'custom_metric.write'])
WHERE r.key IN ('Owner', 'Admin', 'Manager');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.key = 'custom_metric.read'
WHERE r.key IN ('Buyer', 'Analyst', 'Viewer');

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM role_permissions WHERE permission_id IN (
  SELECT id FROM permissions WHERE key IN ('custom_metric.read', 'custom_metric.write')
);
DELETE FROM permissions WHERE key IN ('custom_metric.read', 'custom_metric.write');
DROP TABLE custom_metrics;
-- +goose StatementEnd
