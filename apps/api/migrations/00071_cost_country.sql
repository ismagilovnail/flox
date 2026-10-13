-- Spend per country (§27-COST "per campaign/source/day/country"; gap 6 of
-- docs/spec-conformance-2026-10-11.md).
--
-- country: ISO 3166-1 alpha-2, or '' for spend not split by country (every
-- row before this, and every synced row: the ad platforms' daily campaign
-- spend has no country). It joins the entry's identity, so one day can hold
-- a row per country. A day of a (campaign, source) is either one total or a
-- split by country, never both — cost.Repository.Upsert enforces it — so the
-- rows of a day always add up to its spend.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE cost_entries ADD COLUMN country text NOT NULL DEFAULT ''
  CHECK (country = '' OR country ~ '^[A-Z]{2}$');

DROP INDEX cost_entries_dedup_key_with_source;
DROP INDEX cost_entries_dedup_key_without_source;
CREATE UNIQUE INDEX cost_entries_dedup_key_with_source
  ON cost_entries (campaign_id, traffic_source_id, entry_date, country) WHERE traffic_source_id IS NOT NULL;
CREATE UNIQUE INDEX cost_entries_dedup_key_without_source
  ON cost_entries (campaign_id, entry_date, country) WHERE traffic_source_id IS NULL;
-- Spend by country over a range, across the organization.
CREATE INDEX cost_entries_org_date_idx ON cost_entries (organization_id, entry_date);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX cost_entries_org_date_idx;
DROP INDEX cost_entries_dedup_key_with_source;
DROP INDEX cost_entries_dedup_key_without_source;
-- A split day cannot go back to one row per day: its countries are summed.
-- Countries in different currencies cannot share one amount, so such a day
-- keeps its USD sum as a USD amount.
WITH split AS (
  SELECT campaign_id, traffic_source_id, entry_date, min(id) AS keep, sum(amount) AS amount, sum(amount_usd) AS amount_usd,
         count(DISTINCT currency) > 1 AND bool_and(amount_usd IS NOT NULL) AS to_usd
  FROM cost_entries WHERE country <> ''
  GROUP BY campaign_id, traffic_source_id, entry_date
)
UPDATE cost_entries ce SET
  amount = CASE WHEN split.to_usd THEN split.amount_usd ELSE split.amount END,
  currency = CASE WHEN split.to_usd THEN 'USD' ELSE ce.currency END,
  amount_usd = split.amount_usd
FROM split WHERE ce.id = split.keep;
DELETE FROM cost_entries WHERE country <> '' AND id NOT IN (
  SELECT min(id) FROM cost_entries WHERE country <> '' GROUP BY campaign_id, traffic_source_id, entry_date);
ALTER TABLE cost_entries DROP COLUMN country;
CREATE UNIQUE INDEX cost_entries_dedup_key_with_source
  ON cost_entries (campaign_id, traffic_source_id, entry_date) WHERE traffic_source_id IS NOT NULL;
CREATE UNIQUE INDEX cost_entries_dedup_key_without_source
  ON cost_entries (campaign_id, entry_date) WHERE traffic_source_id IS NULL;
-- +goose StatementEnd
