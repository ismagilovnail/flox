-- +goose Up
-- +goose StatementBegin

-- Durable transit for postback_events (ClickHouse, §48) — the same role
-- event_queue (migration 00015) plays for click/tracking/conversion
-- events, applied to postback attempts (both directions: internal/
-- conversion's incoming outcomes, internal/postback's outgoing delivery
-- attempts) instead. A deliberate near-duplicate of event_queue's shape
-- rather than a shared/generic table or a Go generic queue type: the two
-- payload shapes (event.Event vs chstore.PostbackAttempt) are genuinely
-- different, and this table's own indexing/claim needs are identical to
-- event_queue's, so there is nothing to actually share beyond the SQL
-- pattern itself — see docs/analytics-pipeline.md.
CREATE TABLE postback_attempt_queue (
  id               ulid PRIMARY KEY,
  organization_id  ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  payload          jsonb NOT NULL,
  status           text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'processing')),
  next_attempt_at  timestamptz NOT NULL DEFAULT now(),
  created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX postback_attempt_queue_organization_id_idx ON postback_attempt_queue (organization_id);
CREATE INDEX postback_attempt_queue_due_idx ON postback_attempt_queue (next_attempt_at) WHERE status = 'queued';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE postback_attempt_queue;
-- +goose StatementEnd
