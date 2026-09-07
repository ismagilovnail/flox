-- +goose Up
-- +goose StatementBegin

-- Audit finding #1 (2026-09-06): the role -> permission policy seeded by
-- migrations 00001/00020 was never enforced on most of the API. Six route
-- groups had no permission key that could name them at all — landings,
-- PWAs, postlandings, pixels, tags and the content gallery — so there was
-- nothing for apps/api/main.go to gate them with even in principle.
--
-- One key pair rather than six, deliberately: all six are campaign
-- creative/tracking assets, and every one of them lands on the identical
-- role tiering below (Owner/Admin/Manager write, everyone reads). Six
-- separate pairs would be six identical grants — vocabulary with no
-- expressive gain today. Splitting content.write into per-entity keys
-- later is a cheap additive migration; collapsing six keys back into one
-- afterwards is not.
--
-- Same shape as 00029/00030/00031's own key-minting migrations: insert
-- the permissions, grant write to the top three roles, grant read to the
-- other three.
INSERT INTO permissions (id, key, description) VALUES
  ('01M1V8PF4XA04AGCYPEXKXSC6E', 'content.read',  'View landings, PWAs, postlandings, pixels, tags and the content gallery.'),
  ('01M1V8PF4XA04AGCYPEY8R376J', 'content.write', 'Create, edit, archive landings, PWAs, postlandings, pixels, tags and content gallery files.');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.key = ANY (ARRAY['content.read', 'content.write'])
WHERE r.key IN ('Owner', 'Admin', 'Manager');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.key = 'content.read'
WHERE r.key IN ('Buyer', 'Analyst', 'Viewer');

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DELETE FROM role_permissions WHERE permission_id IN (
  SELECT id FROM permissions WHERE key IN ('content.read', 'content.write')
);
DELETE FROM permissions WHERE key IN ('content.read', 'content.write');

-- +goose StatementEnd
