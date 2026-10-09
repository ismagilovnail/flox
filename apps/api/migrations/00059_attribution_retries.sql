-- +goose Up
-- +goose StatementBegin

-- Audit 2026-10-06, T1. A conversion whose click ClickHouse does not have
-- yet (the click is still in event_queue, ClickHouse is behind or down) is
-- recorded at once as unknown_click — its CPA event and outgoing postback
-- leave immediately — and gets one row here, in the same transaction. The
-- worker (internal/reattribute) looks the click up again on a backoff; once
-- found, it rewrites the postbacks row and the conversion's event in
-- ClickHouse. Without this, unknown_click was final.
--
-- attributed_at set = Postgres already carries the attribution and only the
-- ClickHouse copy of the event is left to rewrite.
CREATE TABLE attribution_retries (
  postback_id     ulid PRIMARY KEY REFERENCES postbacks (id) ON DELETE CASCADE,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  attempt         int NOT NULL DEFAULT 0,
  next_attempt_at timestamptz NOT NULL,
  attributed_at   timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX attribution_retries_due ON attribution_retries (next_attempt_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE attribution_retries;
-- +goose StatementEnd
