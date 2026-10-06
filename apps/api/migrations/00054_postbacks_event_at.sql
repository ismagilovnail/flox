-- +goose Up
-- +goose StatementBegin

-- Audit 2026-10-06, Postgres↔ClickHouse reconciliation (internal/reconcile):
-- a recorded conversion's ClickHouse event is dated by the moment the
-- postback was received (conversion.Postback.OccurredAt), while
-- postbacks.created_at is stamped a little later, at the write. Near
-- midnight the two fall on different days and a per-day comparison would
-- report phantom differences. event_at is the analytics event's own time,
-- written with the row (conversion.PostgresStore); older rows take
-- created_at.
ALTER TABLE postbacks ADD COLUMN event_at timestamptz;
UPDATE postbacks SET event_at = created_at;
ALTER TABLE postbacks ALTER COLUMN event_at SET DEFAULT now(), ALTER COLUMN event_at SET NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE postbacks DROP COLUMN event_at;
-- +goose StatementEnd
