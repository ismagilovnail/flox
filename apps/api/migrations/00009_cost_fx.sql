-- +goose Up
-- +goose StatementBegin

-- §27-COST: "Cost or it doesn't exist. Manual cost entry first; FB/TikTok
-- import later." One row = one day's spend for one campaign (optionally
-- scoped further to a traffic source, since the same campaign can pull
-- from more than one source). No cost row for a given campaign/date means
-- ROI/ROAS render as "—", never computed against an implicit zero — that
-- logic lives in the query layer (Phase 18+), not this schema.
CREATE TABLE cost_entries (
  id                ulid PRIMARY KEY,
  organization_id   ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  campaign_id       ulid NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
  traffic_source_id ulid REFERENCES traffic_sources (id),
  entry_date        date NOT NULL,
  amount            numeric(14, 4) NOT NULL,
  currency          text NOT NULL DEFAULT 'USD',
  source            text NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'facebook_ads', 'tiktok_ads')),
  created_by_user_id ulid NOT NULL REFERENCES users (id),
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX cost_entries_organization_id_idx ON cost_entries (organization_id);
-- The read pattern every ROI/profit query needs: "this campaign's spend
-- over a date range."
CREATE INDEX cost_entries_campaign_id_entry_date_idx ON cost_entries (campaign_id, entry_date);
-- One manually-entered amount per (campaign, source, date) — re-entering
-- the same day updates it, it doesn't stack. Two partial indexes rather
-- than one COALESCE'd expression index, since traffic_source_id is NULLable
-- and a sentinel placeholder value would itself need to be a valid ulid.
CREATE UNIQUE INDEX cost_entries_dedup_key_with_source
  ON cost_entries (campaign_id, traffic_source_id, entry_date) WHERE traffic_source_id IS NOT NULL;
CREATE UNIQUE INDEX cost_entries_dedup_key_without_source
  ON cost_entries (campaign_id, entry_date) WHERE traffic_source_id IS NULL;

CREATE TRIGGER cost_entries_set_updated_at
  BEFORE UPDATE ON cost_entries
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- §50-FX: "Normalize to USD using the rate on the event date, never the
-- current rate." Global reference data, not tenant-scoped — an exchange
-- rate is an objective market fact, not something each organization has
-- its own copy of. Composite natural key (currency, rate_date) rather than
-- a ulid surrogate: there is exactly one correct rate for a given currency
-- on a given day, so the natural key IS the identity.
CREATE TABLE fx_rates (
  currency    text NOT NULL,
  rate_date   date NOT NULL,
  rate_to_usd numeric(20, 10) NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (currency, rate_date)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE fx_rates;
DROP TABLE cost_entries;
-- +goose StatementEnd
