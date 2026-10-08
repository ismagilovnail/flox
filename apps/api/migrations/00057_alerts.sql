-- +goose Up
-- +goose StatementBegin

-- Marketer audit 2026-10-06, #10: no alerts for spend, ROI, a paused or
-- capped offer, or an expiring certificate. The worker evaluates
-- alert_rules (internal/alert) and writes what fired into notifications,
-- which the bell in the app shows and, per rule, Slack receives.

-- A ULID generated in SQL, for rows the database creates itself (the
-- default rules below). Time part from the clock, random part from
-- random(): these ids are identifiers, not secrets.
CREATE FUNCTION flox_new_ulid() RETURNS text AS $$
DECLARE
  alphabet constant text := '0123456789ABCDEFGHJKMNPQRSTVWXYZ';
  ms bigint := floor(extract(epoch FROM clock_timestamp()) * 1000)::bigint;
  result text := '';
BEGIN
  FOR i IN 1..10 LOOP
    result := substr(alphabet, (ms % 32)::int + 1, 1) || result;
    ms := ms / 32;
  END LOOP;
  FOR i IN 1..16 LOOP
    result := result || substr(alphabet, floor(random() * 32)::int + 1, 1);
  END LOOP;
  RETURN result;
END;
$$ LANGUAGE plpgsql VOLATILE;

-- kind decides what threshold means:
--   spend_over   — USD spent today (organization timezone)
--   roi_below    — ROI today, percent; min_spend is the spend below which
--                  ROI is too early to judge
--   offer_capped — none (fires when an offer reaches its daily cap)
--   offer_paused — none (fires when an offer is set to paused)
--   ssl_expiring — days before a domain's certificate expires
-- campaign_id narrows spend_over/roi_below to one campaign; NULL checks
-- every campaign separately.
CREATE TABLE alert_rules (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  kind            text NOT NULL CHECK (kind IN ('spend_over', 'roi_below', 'offer_capped', 'offer_paused', 'ssl_expiring')),
  campaign_id     ulid REFERENCES campaigns (id) ON DELETE CASCADE,
  threshold       numeric(14, 4),
  min_spend       numeric(14, 4),
  notify_slack    boolean NOT NULL DEFAULT false,
  enabled         boolean NOT NULL DEFAULT true,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX alert_rules_organization_id_idx ON alert_rules (organization_id);

CREATE TRIGGER alert_rules_set_updated_at
  BEFORE UPDATE ON alert_rules
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- One row per thing that fired. dedup_key makes each condition fire once
-- (per day, per offer, per certificate — see alert.Evaluator), however many
-- evaluations or workers see it. params holds the names and numbers the UI
-- and Slack render the message from.
CREATE TABLE notifications (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  rule_id         ulid REFERENCES alert_rules (id) ON DELETE SET NULL,
  kind            text NOT NULL,
  dedup_key       text NOT NULL,
  params          jsonb NOT NULL DEFAULT '{}',
  slack_status    text NOT NULL DEFAULT 'none' CHECK (slack_status IN ('none', 'pending', 'sent', 'failed')),
  slack_attempts  integer NOT NULL DEFAULT 0,
  slack_next_at   timestamptz NOT NULL DEFAULT now(),
  slack_error     text NOT NULL DEFAULT '',
  created_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (organization_id, dedup_key)
);

CREATE INDEX notifications_org_created_idx ON notifications (organization_id, created_at DESC);
CREATE INDEX notifications_slack_due_idx ON notifications (slack_next_at) WHERE slack_status = 'pending';

-- Per user and organization: everything created up to read_until is read.
CREATE TABLE notification_reads (
  user_id         ulid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  read_until      timestamptz NOT NULL,
  PRIMARY KEY (user_id, organization_id)
);

-- When an offer last changed status, so "offer paused" fires once per
-- pause and not on every later edit of a paused offer.
ALTER TABLE offers ADD COLUMN status_changed_at timestamptz;

CREATE FUNCTION offers_track_status_change() RETURNS trigger AS $$
BEGIN
  IF NEW.status IS DISTINCT FROM OLD.status THEN
    NEW.status_changed_at = now();
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER offers_status_changed
  BEFORE UPDATE OF status ON offers
  FOR EACH ROW EXECUTE FUNCTION offers_track_status_change();

-- Every organization starts with the alerts that need no threshold of
-- their own: offer capped, offer paused, certificate expiring in 14 days.
CREATE FUNCTION flox_default_alert_rules() RETURNS trigger AS $$
BEGIN
  INSERT INTO alert_rules (id, organization_id, kind, threshold) VALUES
    (flox_new_ulid(), NEW.id, 'offer_capped', NULL),
    (flox_new_ulid(), NEW.id, 'offer_paused', NULL),
    (flox_new_ulid(), NEW.id, 'ssl_expiring', 14);
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER organizations_default_alert_rules
  AFTER INSERT ON organizations
  FOR EACH ROW EXECUTE FUNCTION flox_default_alert_rules();

INSERT INTO alert_rules (id, organization_id, kind, threshold)
SELECT flox_new_ulid(), o.id, d.kind, d.threshold
FROM organizations o
CROSS JOIN (VALUES ('offer_capped', NULL::numeric), ('offer_paused', NULL), ('ssl_expiring', 14)) AS d (kind, threshold);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER organizations_default_alert_rules ON organizations;
DROP FUNCTION flox_default_alert_rules();
DROP TRIGGER offers_status_changed ON offers;
DROP FUNCTION offers_track_status_change();
ALTER TABLE offers DROP COLUMN status_changed_at;
DROP TABLE notification_reads;
DROP TABLE notifications;
DROP TABLE alert_rules;
DROP FUNCTION flox_new_ulid();
-- +goose StatementEnd
