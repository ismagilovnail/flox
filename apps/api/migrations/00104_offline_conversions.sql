-- Offline conversions to Google Ads (audit UNI-2 part 2;
-- docs/offline-conversions.md). A traffic source connected to Google Ads
-- can report its leads, purchases and repeat purchases back to Google by
-- the click's gclid/gbraid/wbraid, through the Data Manager API.
--
-- On ad_account_connections, the source's settings:
--   offline_conversions — on/off;
--   offline_since — when it was last turned on: only conversions recorded
--     from then on are sent (no backfill of older ones);
--   offline_login_account_id — the manager account the credentials reach
--     the ad account through, '' when they reach it directly;
--   offline_lead_action_id / _purchase_action_id / _repurchase_action_id —
--     the Google Ads conversion action each status is reported as, '' for
--     a status that is not sent.
--
-- offline_conversion_uploads is the queue: one row per conversion postback
-- considered, whatever came of it. status:
--   pending — to be sent (again: a refund changed its value);
--   sent    — Google accepted it with value sent_value;
--   failed  — Google refused it, or it kept failing (last_error);
--   skipped — not for Google (skip_reason): no Google click id, the
--     click's source is not connected or has the export off, no action for
--     the status. Kept so the same postback is not looked at again.

-- +goose Up
ALTER TABLE ad_account_connections
  ADD COLUMN offline_conversions boolean NOT NULL DEFAULT false,
  ADD COLUMN offline_since timestamptz,
  ADD COLUMN offline_login_account_id text NOT NULL DEFAULT '',
  ADD COLUMN offline_lead_action_id text NOT NULL DEFAULT '',
  ADD COLUMN offline_purchase_action_id text NOT NULL DEFAULT '',
  ADD COLUMN offline_repurchase_action_id text NOT NULL DEFAULT '';

CREATE TABLE offline_conversion_uploads (
  id ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  postback_id ulid NOT NULL UNIQUE REFERENCES postbacks (id) ON DELETE CASCADE,
  -- NULL when skipped before a source was found; a disconnected source's
  -- rows stay, unsent.
  connection_id ulid REFERENCES ad_account_connections (id) ON DELETE SET NULL,
  click_id text NOT NULL DEFAULT '',
  ad_click_param text NOT NULL DEFAULT '',
  ad_click_id text NOT NULL DEFAULT '',
  action_id text NOT NULL DEFAULT '',
  event_at timestamptz NOT NULL,
  value numeric(14,4),
  currency text NOT NULL DEFAULT '',
  sent_value numeric(14,4),
  status text NOT NULL CHECK (status IN ('pending', 'sent', 'failed', 'skipped')),
  skip_reason text NOT NULL DEFAULT '',
  attempts integer NOT NULL DEFAULT 0,
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  last_error text NOT NULL DEFAULT '',
  request_id text NOT NULL DEFAULT '',
  sent_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX offline_conversion_uploads_due_idx ON offline_conversion_uploads (next_attempt_at)
  WHERE status = 'pending';
CREATE INDEX offline_conversion_uploads_connection_idx ON offline_conversion_uploads (organization_id, connection_id, status);

-- +goose Down
DROP TABLE offline_conversion_uploads;
ALTER TABLE ad_account_connections
  DROP COLUMN offline_conversions,
  DROP COLUMN offline_since,
  DROP COLUMN offline_login_account_id,
  DROP COLUMN offline_lead_action_id,
  DROP COLUMN offline_purchase_action_id,
  DROP COLUMN offline_repurchase_action_id;
