-- Recurring push broadcasts (docs/push.md).
--
-- kind 'recurring' is a template: on recur_weekdays (ISO 1 = Monday … 7)
-- at recur_time in recur_timezone (the organization's when it was made),
-- until recur_until (a date in that zone, inclusive) or for good. The
-- worker turns each due slot (next_send_at) into an ordinary sent
-- broadcast (kind 'once', parent_id = the template), so deliveries, opens
-- and push metrics need nothing new. A template is active, paused, or
-- finished once past recur_until; its own counters stay 0 — the API sums
-- its sends.
--
-- push_broadcasts_trigger_check (00074) becomes push_broadcasts_kind_shape_check:
-- statuses by kind, and each kind's own columns.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE push_broadcasts DROP CONSTRAINT push_broadcasts_kind_check;
ALTER TABLE push_broadcasts ADD CONSTRAINT push_broadcasts_kind_check CHECK (kind IN ('once', 'triggered', 'recurring'));
ALTER TABLE push_broadcasts DROP CONSTRAINT push_broadcasts_status_check;
ALTER TABLE push_broadcasts ADD CONSTRAINT push_broadcasts_status_check
  CHECK (status IN ('scheduled', 'sent', 'canceled', 'active', 'paused', 'finished'));
ALTER TABLE push_broadcasts DROP CONSTRAINT push_broadcasts_trigger_check;

ALTER TABLE push_broadcasts
  ADD COLUMN recur_weekdays smallint[] NOT NULL DEFAULT '{}',
  ADD COLUMN recur_time time,
  ADD COLUMN recur_timezone text NOT NULL DEFAULT '',
  ADD COLUMN recur_until date,
  ADD COLUMN next_send_at timestamptz,
  ADD COLUMN parent_id ulid REFERENCES push_broadcasts (id) ON DELETE SET NULL;

ALTER TABLE push_broadcasts ADD CONSTRAINT push_broadcasts_kind_shape_check CHECK (
  (kind = 'once') = (status IN ('scheduled', 'sent', 'canceled'))
  AND (kind <> 'triggered' OR status IN ('active', 'paused'))
  AND (kind = 'triggered') = (trigger_delay_minutes IS NOT NULL)
  AND (kind = 'recurring') = (recur_time IS NOT NULL)
  AND (kind <> 'recurring' OR (cardinality(recur_weekdays) > 0 AND recur_timezone <> ''))
  AND (kind <> 'recurring' OR status = 'finished' OR next_send_at IS NOT NULL)
  AND (parent_id IS NULL OR kind = 'once'));

CREATE INDEX push_broadcasts_recurring_due_idx ON push_broadcasts (next_send_at)
  WHERE kind = 'recurring' AND status = 'active';
CREATE INDEX push_broadcasts_parent_idx ON push_broadcasts (parent_id) WHERE parent_id IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX push_broadcasts_parent_idx;
DROP INDEX push_broadcasts_recurring_due_idx;
-- Templates cannot exist without their columns; their sends stay as
-- ordinary broadcasts.
DELETE FROM push_broadcasts WHERE kind = 'recurring';
ALTER TABLE push_broadcasts DROP CONSTRAINT push_broadcasts_kind_shape_check;
ALTER TABLE push_broadcasts DROP COLUMN parent_id, DROP COLUMN next_send_at, DROP COLUMN recur_until,
  DROP COLUMN recur_timezone, DROP COLUMN recur_time, DROP COLUMN recur_weekdays;
ALTER TABLE push_broadcasts ADD CONSTRAINT push_broadcasts_trigger_check
  CHECK ((kind = 'triggered') = (status IN ('active', 'paused'))
     AND (kind = 'triggered') = (trigger_delay_minutes IS NOT NULL));
ALTER TABLE push_broadcasts DROP CONSTRAINT push_broadcasts_status_check;
ALTER TABLE push_broadcasts ADD CONSTRAINT push_broadcasts_status_check
  CHECK (status IN ('scheduled', 'sent', 'canceled', 'active', 'paused'));
ALTER TABLE push_broadcasts DROP CONSTRAINT push_broadcasts_kind_check;
ALTER TABLE push_broadcasts ADD CONSTRAINT push_broadcasts_kind_check CHECK (kind IN ('once', 'triggered'));
-- +goose StatementEnd
