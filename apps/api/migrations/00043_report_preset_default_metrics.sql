-- +goose Up
-- +goose StatementBegin

-- The system default preset (never editable through the CRUD surface) moves
-- to the classic media-buyer table: traffic, CR, cost, revenue, profit, ROI,
-- EPC, CPA over the last 7 days. Already-seeded orgs get the same values a
-- freshly seeded org gets from reportpresets.defaultPresetSeed.
UPDATE report_presets
SET metrics = ARRAY['clicks', 'conversions', 'cvr', 'cost', 'revenue', 'profit', 'roi', 'epc', 'cpa'],
    period_days = 7,
    updated_at = now()
WHERE is_default;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

UPDATE report_presets
SET metrics = ARRAY['clicks', 'conversions', 'revenue', 'roi'],
    period_days = 30,
    updated_at = now()
WHERE is_default;

-- +goose StatementEnd
