-- +goose Up
-- +goose StatementBegin

-- What one organization may take of ClickHouse for its reports (audit
-- CH-1; docs/billing.md "Report limits"), per plan so a price or a limit
-- changes without a deploy. apps/api's chconn.TenantConn applies them to
-- every query made on an organization's behalf:
--   ch_max_concurrent        queries at once; more wait up to 10 s, then 429
--   ch_max_threads           max_threads of each query (its CPU share)
--   ch_max_execution_seconds max_execution_time
--   ch_max_memory_mb         max_memory_usage
--   ch_priority              ClickHouse's priority: lower runs first when
--                            queries compete (0 would mean "no priority")
-- Draft numbers, like the prices.
ALTER TABLE plans
  ADD COLUMN ch_max_concurrent        integer NOT NULL DEFAULT 2  CHECK (ch_max_concurrent > 0),
  ADD COLUMN ch_max_threads           integer NOT NULL DEFAULT 2  CHECK (ch_max_threads > 0),
  ADD COLUMN ch_max_execution_seconds integer NOT NULL DEFAULT 15 CHECK (ch_max_execution_seconds > 0),
  ADD COLUMN ch_max_memory_mb         integer NOT NULL DEFAULT 1024 CHECK (ch_max_memory_mb > 0),
  ADD COLUMN ch_priority              integer NOT NULL DEFAULT 4  CHECK (ch_priority > 0);

UPDATE plans SET ch_max_concurrent = 3, ch_max_threads = 4,  ch_max_execution_seconds = 30, ch_max_memory_mb = 2048, ch_priority = 3 WHERE code = 'start';
UPDATE plans SET ch_max_concurrent = 4, ch_max_threads = 8,  ch_max_execution_seconds = 30, ch_max_memory_mb = 4096, ch_priority = 2 WHERE code = 'pro';
UPDATE plans SET ch_max_concurrent = 6, ch_max_threads = 16, ch_max_execution_seconds = 30, ch_max_memory_mb = 4096, ch_priority = 1 WHERE code = 'business';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE plans
  DROP COLUMN ch_priority,
  DROP COLUMN ch_max_memory_mb,
  DROP COLUMN ch_max_execution_seconds,
  DROP COLUMN ch_max_threads,
  DROP COLUMN ch_max_concurrent;
-- +goose StatementEnd
