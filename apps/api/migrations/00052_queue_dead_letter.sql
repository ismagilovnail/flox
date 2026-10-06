-- +goose Up
-- +goose StatementBegin

-- Audit 2026-10-06, Q2: a batch ClickHouse rejects (bad data, a schema it
-- cannot take) was retried every 10 seconds forever, and an outage was
-- hammered at the same pace.
--
-- attempts drives exponential backoff (10 s doubling, capped at 10 min).
-- A batch the ClickHouse server explicitly rejected moves to 'dead' after a
-- few attempts — kept whole, with its batch_id and last_error, so after the
-- cause is fixed it can be put back as is (docs/analytics-pipeline.md) and
-- its deduplication tokens still skip the tables that already took it.
-- Network errors and timeouts never dead-letter: an outage must not bury
-- every batch.
ALTER TABLE event_queue
  ADD COLUMN attempts   integer NOT NULL DEFAULT 0,
  ADD COLUMN last_error text NOT NULL DEFAULT '',
  ADD COLUMN dead_at    timestamptz,
  DROP CONSTRAINT event_queue_status_check,
  ADD CONSTRAINT event_queue_status_check CHECK (status IN ('queued', 'processing', 'dead'));

ALTER TABLE postback_attempt_queue
  ADD COLUMN attempts   integer NOT NULL DEFAULT 0,
  ADD COLUMN last_error text NOT NULL DEFAULT '',
  ADD COLUMN dead_at    timestamptz,
  DROP CONSTRAINT postback_attempt_queue_status_check,
  ADD CONSTRAINT postback_attempt_queue_status_check CHECK (status IN ('queued', 'processing', 'dead'));

CREATE INDEX event_queue_dead_idx            ON event_queue (dead_at)            WHERE status = 'dead';
CREATE INDEX postback_attempt_queue_dead_idx ON postback_attempt_queue (dead_at) WHERE status = 'dead';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX postback_attempt_queue_dead_idx;
DROP INDEX event_queue_dead_idx;
-- Dead rows go back to the queue rather than violating the old constraint.
UPDATE postback_attempt_queue SET status = 'queued' WHERE status = 'dead';
UPDATE event_queue            SET status = 'queued' WHERE status = 'dead';
ALTER TABLE postback_attempt_queue
  DROP CONSTRAINT postback_attempt_queue_status_check,
  ADD CONSTRAINT postback_attempt_queue_status_check CHECK (status IN ('queued', 'processing')),
  DROP COLUMN dead_at, DROP COLUMN last_error, DROP COLUMN attempts;
ALTER TABLE event_queue
  DROP CONSTRAINT event_queue_status_check,
  ADD CONSTRAINT event_queue_status_check CHECK (status IN ('queued', 'processing')),
  DROP COLUMN dead_at, DROP COLUMN last_error, DROP COLUMN attempts;
-- +goose StatementEnd
