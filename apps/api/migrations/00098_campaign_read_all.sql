-- +goose Up
-- +goose StatementBegin

-- campaign.read_all: read every campaign of the organization and every
-- report, conversion and log of them (audit TEAM-1b; docs/team.md).
-- Without it campaign.read covers only the campaigns the member owns — a
-- Buyer sees their own campaigns, nobody else's, and no unowned ones.
INSERT INTO permissions (id, key, description) VALUES
  ('01M5J73HQKEWKKS9PZHB82013T', 'campaign.read_all', 'See every campaign and its data, not only own ones.');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.key = 'campaign.read_all'
WHERE r.key IN ('Owner', 'Admin', 'Manager', 'Analyst', 'Viewer');

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE key = 'campaign.read_all');
DELETE FROM permissions WHERE key = 'campaign.read_all';
-- +goose StatementEnd
