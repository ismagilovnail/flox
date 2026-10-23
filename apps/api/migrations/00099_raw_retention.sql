-- +goose Up
-- +goose StatementBegin

-- How long a plan keeps raw clicks and tracking events in ClickHouse
-- (audit DATA-6, docs/retention.md "Raw clicks by plan"). Reports outlive
-- them through the hourly rollup; conversions, LTV and aggregates are
-- kept whatever the plan.
ALTER TABLE plans ADD COLUMN raw_retention_months int NOT NULL DEFAULT 24
  CHECK (raw_retention_months BETWEEN 1 AND 120);
UPDATE plans SET raw_retention_months = 3 WHERE code = 'free';
UPDATE plans SET raw_retention_months = 6 WHERE code = 'start';
UPDATE plans SET raw_retention_months = 12 WHERE code = 'pro';
UPDATE plans SET raw_retention_months = 24 WHERE code = 'business';

-- The retention an organization's ClickHouse rows carry (months) and its
-- plan's (target, since when). internal/retention.Plans rewrites the rows
-- to the target: at once when it is longer, after a waiting period when
-- it is shorter — so a lapsed payment or a short downgrade does not
-- delete history. apps/worker writes new rows with months.
CREATE TABLE organization_retention (
  organization_id ulid PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
  months          int NOT NULL CHECK (months BETWEEN 1 AND 120),
  target          int NOT NULL CHECK (target BETWEEN 1 AND 120),
  target_since    timestamptz NOT NULL DEFAULT now(),
  applied_at      timestamptz,
  updated_at      timestamptz NOT NULL DEFAULT now()
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE organization_retention;
ALTER TABLE plans DROP COLUMN raw_retention_months;
-- +goose StatementEnd
