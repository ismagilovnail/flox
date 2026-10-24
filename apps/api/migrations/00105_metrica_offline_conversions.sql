-- Offline conversions to Yandex Metrica (audit UNI-2 part 3;
-- docs/offline-conversions.md). A traffic source whose clicks come from
-- Yandex Direct with yclid can report its leads, purchases and repeat
-- purchases to the Metrica counter on the landing, which passes them on to
-- Direct.
--
-- metrica_connections: one per traffic source, whatever its cost
-- integration (Direct's cost import is UNI-3):
--   counter_id — the Metrica counter's number;
--   access_token — an OAuth token with metrika:offline_data (or
--     metrika:write), AES-256-GCM ciphertext under ADACCOUNT_ENCRYPTION_KEY;
--     token_preview its last 4 characters, the only part ever shown;
--   currency — the counter's currency: Metrica zeroes a price in any other,
--     so values are converted to it at the event's date;
--   enabled / since — on/off, and when it was last turned on (no backfill);
--   lead_goal / purchase_goal / repurchase_goal — the identifier of the
--     JavaScript-event goal each status is reported as, '' for none.
--
-- On offline_conversion_uploads (00104), a row is now for Google
-- (connection_id) or for Metrica (metrica_connection_id):
--   remote_status — Metrica's status of the upload the row went in
--     (UPLOADED … PROCESSED / LINKAGE_FAILURE), '' for Google rows;
--     request_id holds the upload's id.

-- +goose Up
CREATE TABLE metrica_connections (
  id                ulid PRIMARY KEY,
  organization_id   ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  traffic_source_id ulid NOT NULL UNIQUE REFERENCES traffic_sources (id) ON DELETE CASCADE,
  counter_id        text NOT NULL,
  access_token      text NOT NULL,
  token_preview     text NOT NULL DEFAULT '',
  currency          text NOT NULL DEFAULT 'RUB',
  enabled           boolean NOT NULL DEFAULT false,
  since             timestamptz,
  lead_goal         text NOT NULL DEFAULT '',
  purchase_goal     text NOT NULL DEFAULT '',
  repurchase_goal   text NOT NULL DEFAULT '',
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX metrica_connections_organization_id_idx ON metrica_connections (organization_id);

CREATE TRIGGER metrica_connections_set_updated_at
  BEFORE UPDATE ON metrica_connections
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE offline_conversion_uploads
  ADD COLUMN metrica_connection_id ulid REFERENCES metrica_connections (id) ON DELETE SET NULL,
  ADD COLUMN remote_status text NOT NULL DEFAULT '';

CREATE INDEX offline_conversion_uploads_metrica_idx ON offline_conversion_uploads (organization_id, metrica_connection_id, status);
CREATE INDEX offline_conversion_uploads_unchecked_idx ON offline_conversion_uploads (sent_at)
  WHERE metrica_connection_id IS NOT NULL AND status = 'sent' AND remote_status NOT IN ('PROCESSED', 'LINKAGE_FAILURE');

-- +goose Down
ALTER TABLE offline_conversion_uploads
  DROP COLUMN metrica_connection_id,
  DROP COLUMN remote_status;
DROP TABLE metrica_connections;
