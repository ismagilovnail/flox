-- Postbacks whose status is not mapped yet wait for the mapping (audit
-- 2026-10-15, DATA-4).
--
-- A postback with a network status no event mapping knows is answered 200
-- "error" — the network takes it as delivered and never sends it again, so
-- the conversion was lost even after the operator mapped the status. Now
-- the ledger row is kept as awaiting_mapping, and apps/worker
-- (internal/mappingreplay) records it through the conversion engine once a
-- mapping for its status exists; replayed_at says when.
--
-- external_click_id and event_at make the replay the postback the network
-- sent: attribution by the network's click id, and the conversion's own
-- date for FX and the report day (insertEntry now writes event_at for
-- every row, not only successes).

-- +goose Up
ALTER TABLE postbacks
  ADD COLUMN awaiting_mapping boolean NOT NULL DEFAULT false,
  ADD COLUMN replayed_at timestamptz,
  ADD COLUMN external_click_id text NOT NULL DEFAULT '';

CREATE INDEX postbacks_awaiting_mapping_idx
  ON postbacks (organization_id, network_id, lower(raw_status), created_at)
  WHERE awaiting_mapping;

-- +goose Down
DROP INDEX postbacks_awaiting_mapping_idx;
ALTER TABLE postbacks
  DROP COLUMN external_click_id,
  DROP COLUMN replayed_at,
  DROP COLUMN awaiting_mapping;
