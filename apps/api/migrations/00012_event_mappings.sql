-- +goose Up
-- +goose StatementBegin

-- Per-network translation from a network's own raw status string to the
-- canonical §43 CpaStatus. Not in §35's original table list — same
-- reasoning as 00011's sticky flags: Phase 13's UI shipped this as a
-- Zustand mock (apps/web/src/stores/event-mappings.ts) documented as "what
-- the real Conversion Engine (Phase 23) runs at ingest time," and Phase 23
-- is the code that actually needs the table, so it lands here rather than
-- being guessed at during Phase 17.
CREATE TABLE event_mappings (
  id               ulid PRIMARY KEY,
  organization_id  ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  network_id       ulid NOT NULL REFERENCES networks (id) ON DELETE CASCADE,
  -- Matched case-insensitively at lookup time (lower()) — networks are
  -- inconsistent about casing on retries/redeploys of their own postback
  -- code, and requiring exact-case configuration is a support ticket
  -- waiting to happen for no correctness benefit.
  network_status   text NOT NULL,
  flox_status      text NOT NULL CHECK (flox_status IN ('CPA_HOLD', 'CPA_ACCEPT', 'CPA_REDEP', 'CPA_DECLINE', 'CPA_TRASH')),
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX event_mappings_organization_id_idx ON event_mappings (organization_id);
-- One mapping per (network, raw status) — a network cannot claim "sale"
-- means two different things at once. lower() so the uniqueness check
-- honors the same case-insensitive matching the lookup uses.
CREATE UNIQUE INDEX event_mappings_network_status_idx ON event_mappings (network_id, lower(network_status));

CREATE TRIGGER event_mappings_set_updated_at
  BEFORE UPDATE ON event_mappings
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE event_mappings;
-- +goose StatementEnd
