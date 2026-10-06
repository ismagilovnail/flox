-- +goose Up
-- +goose StatementBegin

-- The columns the operator hid in the report table, saved with the preset:
-- column ids (dimension keys, metric keys, custom-metric ids), '{}' meaning
-- every column is shown. Ids that no longer exist in the report are ignored
-- when the preset is applied.
ALTER TABLE report_presets ADD COLUMN hidden_columns text[] NOT NULL DEFAULT '{}';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE report_presets DROP COLUMN hidden_columns;

-- +goose StatementEnd
