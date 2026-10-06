-- +goose Up
-- +goose StatementBegin

-- The order the operator dragged the report table's columns into, saved with
-- the preset: column ids (dimension keys, metric keys, custom-metric ids), '{}'
-- meaning the natural order (dimensions, then metrics). Ids that no longer
-- exist in the report are simply ignored when the preset is applied.
ALTER TABLE report_presets ADD COLUMN column_order text[] NOT NULL DEFAULT '{}';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE report_presets DROP COLUMN column_order;

-- +goose StatementEnd
