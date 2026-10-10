-- Tech audit 2026-10-06, DB3: the Postgres queues churn. Every event_queue
-- and postback_attempt_queue row is inserted, updated on claim and deleted
-- within seconds; postback_deliveries rows are updated on every attempt.
-- With the default autovacuum_vacuum_scale_factor (0.2) a queue table is
-- vacuumed only after a fifth of it is dead tuples, and between runs every
-- claim (ORDER BY next_attempt_at ... FOR UPDATE SKIP LOCKED) wades
-- through them. Vacuum these after 1% (plus a floor of 1000 rows so an
-- empty queue is not vacuumed constantly); inserts count too (PG 13+).
--
-- Storage parameters only: SHARE UPDATE EXCLUSIVE, does not block reads or
-- writes. A broker replaces these queues past ~1–2k events/s (audit §19).

-- +goose Up
-- +goose StatementBegin
ALTER TABLE event_queue SET (
  autovacuum_vacuum_scale_factor = 0.01,
  autovacuum_vacuum_threshold = 1000,
  autovacuum_vacuum_insert_scale_factor = 0.01,
  autovacuum_analyze_scale_factor = 0.02
);
ALTER TABLE postback_attempt_queue SET (
  autovacuum_vacuum_scale_factor = 0.01,
  autovacuum_vacuum_threshold = 1000,
  autovacuum_vacuum_insert_scale_factor = 0.01,
  autovacuum_analyze_scale_factor = 0.02
);
ALTER TABLE postback_deliveries SET (
  autovacuum_vacuum_scale_factor = 0.01,
  autovacuum_vacuum_threshold = 1000,
  autovacuum_analyze_scale_factor = 0.02
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE event_queue RESET (
  autovacuum_vacuum_scale_factor, autovacuum_vacuum_threshold,
  autovacuum_vacuum_insert_scale_factor, autovacuum_analyze_scale_factor
);
ALTER TABLE postback_attempt_queue RESET (
  autovacuum_vacuum_scale_factor, autovacuum_vacuum_threshold,
  autovacuum_vacuum_insert_scale_factor, autovacuum_analyze_scale_factor
);
ALTER TABLE postback_deliveries RESET (
  autovacuum_vacuum_scale_factor, autovacuum_vacuum_threshold,
  autovacuum_analyze_scale_factor
);
-- +goose StatementEnd
