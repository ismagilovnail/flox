-- Web Push (docs/push.md; gap 5 of docs/spec-conformance-2026-10-11.md).
--
-- pwas.push_enabled: the installed app asks its visitor for notifications
-- on launch.
--
-- push_subscriptions: one browser's subscription to one PWA, made from an
-- installed app's launch page (apps/tracker). Bound to the click the app
-- was installed from: link_token is that click's signed link (it builds the
-- notification's click-through URL), context the click's attribution as
-- JSON (landinglink.Claims) for the events a send records, origin the
-- tracker scheme+host it subscribed on (the click-through URL's). endpoint is the
-- push service URL the worker posts to — validated (https, known push
-- services) when stored, dialed through urlsafety when sent. Unsubscribed
-- (the push service answered 404/410) rows stay as a record and are never
-- sent to again.
--
-- push_broadcasts: a message sent now to the subscribers matching its
-- filters (empty array = no filter). The counters are updated as
-- deliveries finish and as notifications are clicked, so they outlive the
-- delivery rows (deleted after RETENTION_PUSH_DELIVERIES_DAYS).
--
-- push_deliveries: the send queue, one row per (broadcast, subscription);
-- same shape and fair claim as pixel_deliveries (00069).

-- +goose Up
-- +goose StatementBegin
ALTER TABLE pwas ADD COLUMN push_enabled boolean NOT NULL DEFAULT false;

CREATE TABLE push_subscriptions (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  pwa_id          ulid NOT NULL REFERENCES pwas (id) ON DELETE CASCADE,
  campaign_id     text NOT NULL DEFAULT '',
  click_id        text NOT NULL DEFAULT '',
  endpoint        text NOT NULL UNIQUE,
  p256dh          text NOT NULL,
  auth            text NOT NULL,
  link_token      text NOT NULL,
  origin          text NOT NULL,
  context         jsonb NOT NULL DEFAULT '{}',
  country         text NOT NULL DEFAULT '',
  device          text NOT NULL DEFAULT '',
  language        text NOT NULL DEFAULT '',
  status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'unsubscribed')),
  unsubscribed_at timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

-- Audience selection and the subscriber summary.
CREATE INDEX push_subscriptions_org_status_idx ON push_subscriptions (organization_id, status, pwa_id);

CREATE TRIGGER push_subscriptions_set_updated_at
  BEFORE UPDATE ON push_subscriptions
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE push_broadcasts (
  id                 ulid PRIMARY KEY,
  organization_id    ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  name               text NOT NULL,
  title              text NOT NULL,
  body               text NOT NULL DEFAULT '',
  icon_url           text NOT NULL DEFAULT '',
  image_url          text NOT NULL DEFAULT '',
  target             text NOT NULL CHECK (target IN ('destination', 'url')),
  url                text NOT NULL DEFAULT '',
  pwa_ids            text[] NOT NULL DEFAULT '{}',
  campaign_ids       text[] NOT NULL DEFAULT '{}',
  countries          text[] NOT NULL DEFAULT '{}',
  total              integer NOT NULL DEFAULT 0,
  delivered          integer NOT NULL DEFAULT 0,
  failed             integer NOT NULL DEFAULT 0,
  unsubscribed       integer NOT NULL DEFAULT 0,
  opened             integer NOT NULL DEFAULT 0,
  created_by_user_id text NOT NULL DEFAULT '',
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX push_broadcasts_org_created_idx ON push_broadcasts (organization_id, created_at DESC);

CREATE TRIGGER push_broadcasts_set_updated_at
  BEFORE UPDATE ON push_broadcasts
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE push_deliveries (
  id                   ulid PRIMARY KEY,
  organization_id      ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  broadcast_id         ulid NOT NULL REFERENCES push_broadcasts (id) ON DELETE CASCADE,
  subscription_id      ulid NOT NULL REFERENCES push_subscriptions (id) ON DELETE CASCADE,

  delivery_status      text NOT NULL DEFAULT 'queued'
                         CHECK (delivery_status IN ('queued', 'processing', 'success', 'retrying', 'dead')),
  attempt_count        integer NOT NULL DEFAULT 0,
  next_attempt_at      timestamptz NOT NULL DEFAULT now(),
  claimed_at           timestamptz,
  response_status_code integer,
  message              text NOT NULL DEFAULT '',
  opened_at            timestamptz,

  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now(),

  UNIQUE (broadcast_id, subscription_id)
);

CREATE INDEX push_deliveries_organization_id_idx ON push_deliveries (organization_id);
CREATE INDEX push_deliveries_subscription_idx ON push_deliveries (subscription_id);
CREATE INDEX push_deliveries_fair_idx ON push_deliveries (organization_id, next_attempt_at, id)
  WHERE delivery_status IN ('queued', 'retrying');
CREATE INDEX push_deliveries_due_idx ON push_deliveries (next_attempt_at)
  WHERE delivery_status IN ('queued', 'retrying');
CREATE INDEX push_deliveries_finished_idx ON push_deliveries (created_at)
  WHERE delivery_status IN ('success', 'dead');

CREATE TRIGGER push_deliveries_set_updated_at
  BEFORE UPDATE ON push_deliveries
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE push_deliveries SET (
  autovacuum_vacuum_scale_factor = 0.01,
  autovacuum_vacuum_threshold = 1000,
  autovacuum_analyze_scale_factor = 0.02
);

-- Same tiering as content.* (00035): Owner/Admin/Manager write, everyone
-- reads.
INSERT INTO permissions (id, key, description) VALUES
  (flox_new_ulid(), 'push.read',  'View push subscribers and broadcasts.'),
  (flox_new_ulid(), 'push.write', 'Send push broadcasts.');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.key = ANY (ARRAY['push.read', 'push.write'])
WHERE r.key IN ('Owner', 'Admin', 'Manager');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.key = 'push.read'
WHERE r.key IN ('Buyer', 'Analyst', 'Viewer');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM role_permissions WHERE permission_id IN (
  SELECT id FROM permissions WHERE key IN ('push.read', 'push.write')
);
DELETE FROM permissions WHERE key IN ('push.read', 'push.write');
DROP TABLE push_deliveries;
DROP TABLE push_broadcasts;
DROP TABLE push_subscriptions;
ALTER TABLE pwas DROP COLUMN push_enabled;
-- +goose StatementEnd
