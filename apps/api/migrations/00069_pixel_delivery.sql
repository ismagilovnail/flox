-- Pixels send conversion events to the ad platform (§29, §74
-- PixelProvider; docs/pixels.md "Sending"). Until now a pixel was only a
-- name, a provider and the provider's pixel id: nothing was ever sent.
--
-- pixels gains what sending needs:
--   access_token     the platform's server-side API token, AES-256-GCM
--                    ciphertext under PIXEL_ENCRYPTION_KEY ('' = not set);
--                    never returned by the API
--   access_token_last4  what the API shows of it instead
--   test_event_code  the platform's test mode code ('' = live events)
--   url              the generic provider's GET URL template (macros)
--   event_names      per-event override of the platform event name,
--                    {"CPA_ACCEPT": "Purchase", ...}; absent = the default
--
-- pixel_deliveries is the send queue and its log: one row per (pixel,
-- source event). source_event_id is the event's own id — the postbacks row
-- of a conversion, the tracker's event id of a PWA_INSTALL — and also the
-- event_id the platform deduplicates on, so a retried batch can neither
-- queue nor count an event twice. payload is what the send needs, captured
-- when the event happened (visitor IP and user agent among it: rows are
-- deleted after RETENTION_PIXEL_DELIVERIES_DAYS, internal/retention).

-- +goose Up
-- +goose StatementBegin
ALTER TABLE pixels
  ADD COLUMN access_token    text  NOT NULL DEFAULT '',
  ADD COLUMN access_token_last4 text NOT NULL DEFAULT '',
  ADD COLUMN test_event_code text  NOT NULL DEFAULT '',
  ADD COLUMN url             text  NOT NULL DEFAULT '',
  ADD COLUMN event_names     jsonb NOT NULL DEFAULT '{}';

CREATE TABLE pixel_deliveries (
  id                   ulid PRIMARY KEY,
  organization_id      ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  pixel_id             ulid NOT NULL REFERENCES pixels (id) ON DELETE CASCADE,
  source_event_id      text NOT NULL,
  event_type           text NOT NULL,
  click_id             text NOT NULL DEFAULT '',
  payload              jsonb NOT NULL,

  delivery_status      text NOT NULL DEFAULT 'queued'
                         CHECK (delivery_status IN ('queued', 'processing', 'success', 'retrying', 'dead')),
  attempt_count        integer NOT NULL DEFAULT 0,
  next_attempt_at      timestamptz NOT NULL DEFAULT now(),
  claimed_at           timestamptz,
  response_status_code integer,
  message              text NOT NULL DEFAULT '',

  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now(),

  UNIQUE (pixel_id, source_event_id)
);

CREATE INDEX pixel_deliveries_organization_id_idx ON pixel_deliveries (organization_id);
-- A pixel's delivery log, newest first.
CREATE INDEX pixel_deliveries_pixel_created_idx ON pixel_deliveries (pixel_id, created_at DESC);
-- The fair claim (internal/pgqueue): organizations with rows to attempt,
-- then each one's oldest due rows.
CREATE INDEX pixel_deliveries_fair_idx ON pixel_deliveries (organization_id, next_attempt_at, id)
  WHERE delivery_status IN ('queued', 'retrying');
CREATE INDEX pixel_deliveries_due_idx ON pixel_deliveries (next_attempt_at)
  WHERE delivery_status IN ('queued', 'retrying');
-- Retention: finished rows by age.
CREATE INDEX pixel_deliveries_finished_idx ON pixel_deliveries (created_at)
  WHERE delivery_status IN ('success', 'dead');

CREATE TRIGGER pixel_deliveries_set_updated_at
  BEFORE UPDATE ON pixel_deliveries
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Updated on every attempt, like postback_deliveries (00065).
ALTER TABLE pixel_deliveries SET (
  autovacuum_vacuum_scale_factor = 0.01,
  autovacuum_vacuum_threshold = 1000,
  autovacuum_analyze_scale_factor = 0.02
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE pixel_deliveries;
ALTER TABLE pixels
  DROP COLUMN event_names,
  DROP COLUMN url,
  DROP COLUMN test_event_code,
  DROP COLUMN access_token_last4,
  DROP COLUMN access_token;
-- +goose StatementEnd
