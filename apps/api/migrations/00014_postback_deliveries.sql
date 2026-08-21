-- +goose Up
-- +goose StatementBegin

-- Phase 24 (§46) implements outgoing postbacks: FLOX notifying a network's
-- configured postback_url of a status change. Deliberately a SEPARATE table
-- from `postbacks` (migration 00008/00013) rather than more direction=
-- 'outgoing' rows there — that table's `result` column and its unique dedup
-- index are a one-shot "have we already accepted this" ledger; a delivery is
-- a genuinely different shape (multiple attempts, backoff timing, a queue to
-- poll) that would either overload `result`'s CHECK vocabulary or force the
-- dedup index to grow a `direction` predicate it has no other use for.
-- source_postback_id keeps the two connected instead: every delivery traces
-- back to exactly the accepted incoming row that triggered it.
CREATE TABLE postback_deliveries (
  id                  ulid PRIMARY KEY,
  organization_id     ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  network_id          ulid NOT NULL REFERENCES networks (id) ON DELETE CASCADE,
  source_postback_id  ulid NOT NULL REFERENCES postbacks (id) ON DELETE CASCADE,

  click_id            text NOT NULL,
  status              text NOT NULL CHECK (status IN ('CPA_HOLD', 'CPA_ACCEPT', 'CPA_REDEP', 'CPA_DECLINE', 'CPA_TRASH')),
  -- The fully macro-resolved URL actually dispatched, so a disputed
  -- delivery can be re-argued from exactly what was sent, not the template.
  url                 text NOT NULL,

  -- §46's exact status vocabulary.
  delivery_status     text NOT NULL DEFAULT 'queued'
                         CHECK (delivery_status IN ('queued', 'processing', 'success', 'failed', 'retrying', 'dead')),
  attempt_count       integer NOT NULL DEFAULT 0,
  -- When the worker's poll loop should next consider this row. Set to now()
  -- on enqueue so a fresh delivery is immediately due; pushed forward on
  -- each failed attempt (exponential backoff, apps/internal/postback).
  next_attempt_at     timestamptz NOT NULL DEFAULT now(),
  response_status_code integer,
  message             text NOT NULL DEFAULT '',

  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX postback_deliveries_organization_id_idx ON postback_deliveries (organization_id);
CREATE INDEX postback_deliveries_source_postback_id_idx ON postback_deliveries (source_postback_id);
-- The worker's poll query: "due rows, oldest first" — queued and retrying
-- are the only delivery_status values a poll ever picks up.
CREATE INDEX postback_deliveries_due_idx ON postback_deliveries (next_attempt_at)
  WHERE delivery_status IN ('queued', 'retrying');

CREATE TRIGGER postback_deliveries_set_updated_at
  BEFORE UPDATE ON postback_deliveries
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE postback_deliveries;
-- +goose StatementEnd
