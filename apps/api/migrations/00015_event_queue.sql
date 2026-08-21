-- +goose Up
-- +goose StatementBegin

-- The durable half of §43's pipeline: "Tracker -> Event Queue -> Worker ->
-- ClickHouse." STACK has no message broker, so this is the queue — the
-- same "Postgres as a job queue, FOR UPDATE SKIP LOCKED" pattern
-- postback_deliveries (migration 00014) already established, applied here
-- to the tracker's own click/tracking events instead of outgoing
-- notifications.
--
-- payload is the whole event.Event, JSON-encoded, rather than one column
-- per field: apps/internal/event.Event already has json tags on every
-- field for exactly this, the worker only ever needs to deserialize the
-- whole thing (never query into individual fields), and a wide explicit-
-- column table here would just be ClickHouse's real schema (Phase 26)
-- designed a second time for a table that gets emptied continuously, not
-- queried directly by anyone.
CREATE TABLE event_queue (
  id               ulid PRIMARY KEY,
  organization_id  ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  type             text NOT NULL,
  payload          jsonb NOT NULL,
  status           text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'processing')),
  next_attempt_at  timestamptz NOT NULL DEFAULT now(),
  created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX event_queue_organization_id_idx ON event_queue (organization_id);
-- The worker's poll query: "due rows, oldest first." Unlike
-- postback_deliveries, a row is DELETED once its batch is durably in
-- ClickHouse rather than marked 'success' — this queue is disposable
-- transit, not an audit ledger (the ClickHouse row IS the durable record
-- from that point on), so there is nothing to keep once delivered.
CREATE INDEX event_queue_due_idx ON event_queue (next_attempt_at) WHERE status = 'queued';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE event_queue;
-- +goose StatementEnd
