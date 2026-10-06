-- +goose Up
-- +goose StatementBegin

-- Audit 2026-10-06, Q1: a row claimed by a worker that then dies (SIGKILL,
-- OOM, a deploy that cancels the batch mid-flight) stayed 'processing'
-- forever — nothing ever looked at it again. claimed_at lets apps/worker's
-- reaper find claims older than any real batch could take and put them back
-- in the queue (internal/queuereaper).
ALTER TABLE event_queue            ADD COLUMN claimed_at timestamptz;
ALTER TABLE postback_attempt_queue ADD COLUMN claimed_at timestamptz;
ALTER TABLE postback_deliveries    ADD COLUMN claimed_at timestamptz;

-- Rows already stuck before this migration have no claim time. Stamping them
-- now (rather than treating NULL as "infinitely old") gives a worker that is
-- still mid-batch on the old binary during a rolling deploy the full reap
-- window to finish before its rows are handed out again.
UPDATE event_queue            SET claimed_at = now() WHERE status = 'processing';
UPDATE postback_attempt_queue SET claimed_at = now() WHERE status = 'processing';
UPDATE postback_deliveries    SET claimed_at = now() WHERE delivery_status = 'processing';

CREATE INDEX event_queue_processing_idx            ON event_queue (claimed_at)            WHERE status = 'processing';
CREATE INDEX postback_attempt_queue_processing_idx ON postback_attempt_queue (claimed_at) WHERE status = 'processing';
CREATE INDEX postback_deliveries_processing_idx    ON postback_deliveries (claimed_at)    WHERE delivery_status = 'processing';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX postback_deliveries_processing_idx;
DROP INDEX postback_attempt_queue_processing_idx;
DROP INDEX event_queue_processing_idx;
ALTER TABLE postback_deliveries    DROP COLUMN claimed_at;
ALTER TABLE postback_attempt_queue DROP COLUMN claimed_at;
ALTER TABLE event_queue            DROP COLUMN claimed_at;
-- +goose StatementEnd
