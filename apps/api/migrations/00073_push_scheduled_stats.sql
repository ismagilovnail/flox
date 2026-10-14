-- Scheduled push broadcasts and push metrics (docs/push.md).
--
-- push_broadcasts.status: 'sent' (queued for delivery, the only state
-- before this migration), 'scheduled' (send_at in the future, no
-- deliveries yet: the audience is fixed when the worker sends it) or
-- 'canceled' (a scheduled one called off). sent_at is when it was queued;
-- its default keeps a broadcast inserted by code from before this migration
-- valid during a rolling deploy.
--
-- push_deliveries.pwa_id/campaign_id/country: the subscriber's, as they
-- were when the broadcast was sent — a subscription re-made later from
-- another click must not move the counts already made.
--
-- push_broadcast_stats: a broadcast's counters per PWA × campaign ×
-- country, kept up to date with push_broadcasts' own (sent = total). They
-- outlive the delivery rows, which retention deletes, and back the push
-- metrics (push_sent, push_delivered, push_opened, push_ctr). Broadcasts
-- sent before this migration get one row of their totals, with no
-- breakdown ('' dimensions).

-- +goose Up
-- +goose StatementBegin
ALTER TABLE push_broadcasts
  ADD COLUMN status text NOT NULL DEFAULT 'sent' CHECK (status IN ('scheduled', 'sent', 'canceled')),
  ADD COLUMN send_at timestamptz,
  ADD COLUMN sent_at timestamptz DEFAULT now();
UPDATE push_broadcasts SET sent_at = created_at;
ALTER TABLE push_broadcasts ADD CONSTRAINT push_broadcasts_schedule_check
  CHECK ((status <> 'scheduled' OR (send_at IS NOT NULL AND sent_at IS NULL))
     AND (status <> 'sent' OR sent_at IS NOT NULL));

CREATE INDEX push_broadcasts_due_idx ON push_broadcasts (send_at) WHERE status = 'scheduled';

ALTER TABLE push_deliveries
  ADD COLUMN pwa_id text NOT NULL DEFAULT '',
  ADD COLUMN campaign_id text NOT NULL DEFAULT '',
  ADD COLUMN country text NOT NULL DEFAULT '';

CREATE TABLE push_broadcast_stats (
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  broadcast_id    ulid NOT NULL REFERENCES push_broadcasts (id) ON DELETE CASCADE,
  pwa_id          text NOT NULL DEFAULT '',
  campaign_id     text NOT NULL DEFAULT '',
  country         text NOT NULL DEFAULT '',
  sent            integer NOT NULL DEFAULT 0,
  delivered       integer NOT NULL DEFAULT 0,
  failed          integer NOT NULL DEFAULT 0,
  unsubscribed    integer NOT NULL DEFAULT 0,
  opened          integer NOT NULL DEFAULT 0,
  PRIMARY KEY (broadcast_id, pwa_id, campaign_id, country)
);

CREATE INDEX push_broadcast_stats_org_idx ON push_broadcast_stats (organization_id);

INSERT INTO push_broadcast_stats (organization_id, broadcast_id, sent, delivered, failed, unsubscribed, opened)
SELECT organization_id, id, total, delivered, failed, unsubscribed, opened FROM push_broadcasts;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE push_broadcast_stats;
ALTER TABLE push_deliveries DROP COLUMN country, DROP COLUMN campaign_id, DROP COLUMN pwa_id;
DROP INDEX push_broadcasts_due_idx;
ALTER TABLE push_broadcasts DROP CONSTRAINT push_broadcasts_schedule_check;
ALTER TABLE push_broadcasts DROP COLUMN sent_at, DROP COLUMN send_at, DROP COLUMN status;
-- +goose StatementEnd
