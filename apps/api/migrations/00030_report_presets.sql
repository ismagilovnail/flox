-- +goose Up
-- +goose StatementBegin

-- Report Presets (§27.5, Phase 14.7) — a saved, reusable report-builder
-- configuration: {dimensions, metrics, groupBy, period, timezone}. The
-- frontend has carried the full interaction (features/analytics/report-
-- preset-bar.tsx: apply/save/rename/delete) against a zustand mock store
-- since its own frontend-first phase; this migration + apps/internal/
-- reportpresets is the real backend replacing it.
--
-- Presets are team-scoped per §27.5 ("Presets are team-scoped (respect
-- tenant isolation §36-TENANCY)") — organization-wide, not per-user, so
-- there is deliberately no owner/creator restriction on who may edit or
-- delete a non-default preset (unlike Custom Metrics' Manager-owns-only
-- tier — that distinction doesn't exist here, the spec is explicit that
-- these are shared). created_by_user_id is still recorded (who made it,
-- for display/audit) but never gates access, and is nullable because the
-- system default preset (see below) has no human creator.
--
-- period_type/period_days/period_from/period_to mirror the frontend's
-- own tagged-union ReportPeriod ({type:"relative",days} |
-- {type:"custom",from,to}) as flat columns rather than JSONB — small,
-- fixed shape, always read/written whole, never queried by sub-field.
--
-- is_default: "a system default preset is available to everyone" (§27.5)
-- and, per the frontend's own existing rule, can't be renamed or deleted
-- — enforced server-side in Service.Update/Delete. The partial unique
-- index guarantees at most one default per org; apps/internal/
-- reportpresets.Service lazily creates it on first List() for an org
-- that has none yet (ON CONFLICT DO NOTHING against this same index
-- makes that race-safe), so no admin action or seed step is needed to
-- bootstrap it — same bar Content Gallery/Domains held themselves to.
CREATE TABLE report_presets (
  id                 ulid PRIMARY KEY,
  organization_id    ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  name               text NOT NULL,
  dimensions         text[] NOT NULL DEFAULT '{}',
  metrics            text[] NOT NULL DEFAULT '{}',
  group_by           text NOT NULL,
  period_type        text NOT NULL CHECK (period_type IN ('relative', 'custom')),
  period_days        integer,
  period_from        date,
  period_to          date,
  timezone           text NOT NULL DEFAULT 'UTC',
  is_default         boolean NOT NULL DEFAULT false,
  created_by_user_id ulid REFERENCES users (id) ON DELETE SET NULL,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX report_presets_organization_id_idx ON report_presets (organization_id);

-- Partial unique index, not a plain UNIQUE(organization_id) — an org may
-- have many non-default presets, but at most one default.
CREATE UNIQUE INDEX report_presets_one_default_per_org
  ON report_presets (organization_id) WHERE is_default;

CREATE TRIGGER report_presets_set_updated_at
  BEFORE UPDATE ON report_presets
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Read is granted to all six roles (matches "available to everyone" and
-- the report builder itself already being open to any role that can see
-- analytics); write follows the exact same Owner/Admin/Manager tiering
-- Custom Metrics' own migration (00029) established for a shared,
-- org-level configuration object.
INSERT INTO permissions (id, key, description) VALUES
  ('60KQCHVAQ7VGBBP5TA9D4CNM1X', 'report_preset.read', 'View saved report presets.'),
  ('51T00ABJ6T6BREN2E7ZDBGV82A', 'report_preset.write', 'Create, edit, and delete report presets.');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.key = ANY (ARRAY['report_preset.read', 'report_preset.write'])
WHERE r.key IN ('Owner', 'Admin', 'Manager');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.key = 'report_preset.read'
WHERE r.key IN ('Buyer', 'Analyst', 'Viewer');

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM role_permissions WHERE permission_id IN (
  SELECT id FROM permissions WHERE key IN ('report_preset.read', 'report_preset.write')
);
DELETE FROM permissions WHERE key IN ('report_preset.read', 'report_preset.write');
DROP TABLE report_presets;
-- +goose StatementEnd
