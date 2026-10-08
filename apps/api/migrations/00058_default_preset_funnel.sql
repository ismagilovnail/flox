-- +goose Up
-- +goose StatementBegin

-- Marketer audit #12: the funnel metrics (PWA installs, registrations,
-- purchases, repurchases, step rates, cost per step) were not in the
-- default column set. The system default preset is not editable by users
-- (reportpresets.Service refuses), so it is FLOX's to update: every
-- organization's default preset gets the new metric set, and the
-- organization's timezone instead of UTC (marketer audit #2). Column order,
-- widths and hidden columns are reset, since they referred to the old set.
UPDATE report_presets p
SET metrics = ARRAY[
      'clicks', 'pwaInstalls', 'clickToInstall', 'registrations', 'installToReg',
      'purchases', 'regToPurchase', 'repurchases',
      'cost', 'costPerInstall', 'costPerReg', 'costPerPurchase',
      'revenue', 'profit', 'roi', 'epc'],
    column_order = '{}',
    column_widths = '{}',
    hidden_columns = '{}',
    timezone = o.timezone
FROM organizations o
WHERE p.organization_id = o.id AND p.is_default;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE report_presets
SET metrics = ARRAY['clicks', 'conversions', 'cvr', 'cost', 'revenue', 'profit', 'roi', 'epc', 'cpa'],
    timezone = 'UTC'
WHERE is_default;
-- +goose StatementEnd
