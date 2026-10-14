-- Triggered push broadcasts (docs/push.md).
--
-- push_broadcasts.kind: 'once' (sent now or scheduled — everything before
-- this migration) or 'triggered': sent to each subscriber trigger_delay_minutes
-- after they subscribed, for subscriptions made after the trigger was
-- created, while it is 'active' ('paused' sends nothing). trigger_condition
-- 'with'/'without' sends only if the subscriber's click has / has not a
-- recorded conversion (postbacks, result 'success') of one of
-- trigger_statuses at send time; 'none' does not look.
--
-- push_broadcast_stats.bucket: the 15-minute slot the deliveries were
-- queued in. A trigger sends a little every day, so its counts must fall on
-- the days it sent, not on the day it was created; 15 minutes cuts days
-- right in every real UTC offset (+05:45 included). Rows before this
-- migration take their broadcast's send time.
--
-- postbacks_attributed_success_idx: the trigger condition looks a
-- subscriber's click up as click_id and as attributed_click_id.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE push_broadcasts
  ADD COLUMN kind text NOT NULL DEFAULT 'once' CHECK (kind IN ('once', 'triggered')),
  ADD COLUMN trigger_delay_minutes integer,
  ADD COLUMN trigger_condition text NOT NULL DEFAULT 'none' CHECK (trigger_condition IN ('none', 'with', 'without')),
  ADD COLUMN trigger_statuses text[] NOT NULL DEFAULT '{}';
ALTER TABLE push_broadcasts DROP CONSTRAINT push_broadcasts_status_check;
ALTER TABLE push_broadcasts ADD CONSTRAINT push_broadcasts_status_check
  CHECK (status IN ('scheduled', 'sent', 'canceled', 'active', 'paused'));
ALTER TABLE push_broadcasts ADD CONSTRAINT push_broadcasts_trigger_check
  CHECK ((kind = 'triggered') = (status IN ('active', 'paused'))
     AND (kind = 'triggered') = (trigger_delay_minutes IS NOT NULL));

CREATE INDEX push_broadcasts_active_triggers_idx ON push_broadcasts (id) WHERE status = 'active';
CREATE INDEX push_subscriptions_org_created_idx ON push_subscriptions (organization_id, created_at) WHERE status = 'active';
CREATE INDEX postbacks_attributed_success_idx ON postbacks (organization_id, attributed_click_id)
  WHERE result = 'success' AND attributed_click_id <> '';

ALTER TABLE push_broadcast_stats ADD COLUMN bucket timestamptz;
UPDATE push_broadcast_stats st SET bucket = date_bin('15 minutes', coalesce(b.sent_at, b.created_at), timestamptz '2000-01-01 00:00:00+00')
FROM push_broadcasts b WHERE b.id = st.broadcast_id;
ALTER TABLE push_broadcast_stats ALTER COLUMN bucket SET NOT NULL;
ALTER TABLE push_broadcast_stats DROP CONSTRAINT push_broadcast_stats_pkey;
ALTER TABLE push_broadcast_stats ADD PRIMARY KEY (broadcast_id, bucket, pwa_id, campaign_id, country);
DROP INDEX push_broadcast_stats_org_idx;
CREATE INDEX push_broadcast_stats_org_bucket_idx ON push_broadcast_stats (organization_id, bucket);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX push_broadcast_stats_org_bucket_idx;
CREATE TEMP TABLE push_broadcast_stats_down AS
  SELECT organization_id, broadcast_id, pwa_id, campaign_id, country,
         sum(sent)::int AS sent, sum(delivered)::int AS delivered, sum(failed)::int AS failed,
         sum(unsubscribed)::int AS unsubscribed, sum(opened)::int AS opened
  FROM push_broadcast_stats GROUP BY 1, 2, 3, 4, 5;
DELETE FROM push_broadcast_stats;
ALTER TABLE push_broadcast_stats DROP CONSTRAINT push_broadcast_stats_pkey;
ALTER TABLE push_broadcast_stats DROP COLUMN bucket;
ALTER TABLE push_broadcast_stats ADD PRIMARY KEY (broadcast_id, pwa_id, campaign_id, country);
INSERT INTO push_broadcast_stats (organization_id, broadcast_id, pwa_id, campaign_id, country, sent, delivered, failed, unsubscribed, opened)
  SELECT * FROM push_broadcast_stats_down;
DROP TABLE push_broadcast_stats_down;
CREATE INDEX push_broadcast_stats_org_idx ON push_broadcast_stats (organization_id);

DROP INDEX postbacks_attributed_success_idx;
DROP INDEX push_subscriptions_org_created_idx;
DROP INDEX push_broadcasts_active_triggers_idx;
-- Triggers cannot exist without the columns that define them.
DELETE FROM push_broadcasts WHERE kind = 'triggered';
ALTER TABLE push_broadcasts DROP CONSTRAINT push_broadcasts_trigger_check;
ALTER TABLE push_broadcasts DROP CONSTRAINT push_broadcasts_status_check;
ALTER TABLE push_broadcasts ADD CONSTRAINT push_broadcasts_status_check
  CHECK (status IN ('scheduled', 'sent', 'canceled'));
ALTER TABLE push_broadcasts DROP COLUMN trigger_statuses, DROP COLUMN trigger_condition,
  DROP COLUMN trigger_delay_minutes, DROP COLUMN kind;
-- +goose StatementEnd
