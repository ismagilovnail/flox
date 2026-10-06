-- +goose Up
-- +goose StatementBegin

-- Audit 2026-10-06, D2: a ClickHouse insert that is retried (failed half way,
-- timed out after it actually landed, or reaped after a crash) used to
-- insert the same events again. apps/worker now sends each batch with an
-- insert_deduplication_token derived from batch_id, and ClickHouse skips a
-- block whose token it has already stored — in the materialized views too.
--
-- That only works if a retry carries exactly the same rows as the first
-- attempt: one token over different rows would make ClickHouse drop the new
-- ones. So the batch is fixed when it is first claimed (batch_id), kept by
-- Requeue and by the reaper, and a retry claims that whole batch again —
-- never re-mixed with fresh rows.
ALTER TABLE event_queue            ADD COLUMN batch_id text;
ALTER TABLE postback_attempt_queue ADD COLUMN batch_id text;

-- Rows already claimed by an old worker have no batch; give each its own so
-- a reaped retry is still a stable unit.
UPDATE event_queue            SET batch_id = id WHERE status = 'processing';
UPDATE postback_attempt_queue SET batch_id = id WHERE status = 'processing';

CREATE INDEX event_queue_retry_idx            ON event_queue (next_attempt_at, batch_id)            WHERE status = 'queued' AND batch_id IS NOT NULL;
CREATE INDEX postback_attempt_queue_retry_idx ON postback_attempt_queue (next_attempt_at, batch_id) WHERE status = 'queued' AND batch_id IS NOT NULL;
CREATE INDEX event_queue_batch_idx            ON event_queue (batch_id)            WHERE batch_id IS NOT NULL;
CREATE INDEX postback_attempt_queue_batch_idx ON postback_attempt_queue (batch_id) WHERE batch_id IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX postback_attempt_queue_batch_idx;
DROP INDEX event_queue_batch_idx;
DROP INDEX postback_attempt_queue_retry_idx;
DROP INDEX event_queue_retry_idx;
ALTER TABLE postback_attempt_queue DROP COLUMN batch_id;
ALTER TABLE event_queue            DROP COLUMN batch_id;
-- +goose StatementEnd
