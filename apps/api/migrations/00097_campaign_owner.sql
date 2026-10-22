-- +goose Up
-- +goose StatementBegin

-- Campaigns belong to a buyer (audit TEAM-1; docs/team.md). The Buyer
-- role was always described as "create and manage own campaigns", but
-- nothing recorded whose a campaign was, so a Buyer could change anyone's.
--
-- owner_user_id: the member the campaign is assigned to — its creator
-- unless reassigned. NULL: no owner (created by an API key, or its owner
-- was deleted); only members with campaign.manage_all change those.
ALTER TABLE campaigns ADD COLUMN owner_user_id ulid REFERENCES users (id) ON DELETE SET NULL;
CREATE INDEX campaigns_owner_idx ON campaigns (organization_id, owner_user_id);

-- Existing campaigns: their creator per the audit trail, when that member
-- is still active in the organization.
UPDATE campaigns c SET owner_user_id = a.actor_user_id
FROM audit_logs a
JOIN memberships m ON m.user_id = a.actor_user_id AND m.status = 'active'
WHERE a.action = 'POST /campaigns' AND a.entity_id = c.id
  AND a.organization_id = c.organization_id AND m.organization_id = c.organization_id
  AND a.actor_user_id IS NOT NULL;

-- campaign.manage_all: change and reassign any campaign. Without it,
-- campaign.write covers only campaigns the member owns (and creating new
-- ones, which they then own).
INSERT INTO permissions (id, key, description) VALUES
  ('01M5J5RPYTSHEH0FY701GZZ1ZP', 'campaign.manage_all', 'Change and reassign every campaign, not only own ones.');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.key = 'campaign.manage_all'
WHERE r.key IN ('Owner', 'Admin', 'Manager');

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE key = 'campaign.manage_all');
DELETE FROM permissions WHERE key = 'campaign.manage_all';
DROP INDEX campaigns_owner_idx;
ALTER TABLE campaigns DROP COLUMN owner_user_id;
-- +goose StatementEnd
