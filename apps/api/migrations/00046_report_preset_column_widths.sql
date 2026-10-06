-- +goose Up
-- +goose StatementBegin

-- Column widths the operator dragged the report table's columns to, saved with
-- the preset: {"clicks": 140, "campaign": 320, ...} in pixels, '{}' meaning every
-- column at its natural auto-layout width. Ids no longer in the report are
-- ignored when the preset is applied.
ALTER TABLE report_presets ADD COLUMN column_widths jsonb NOT NULL DEFAULT '{}';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE report_presets DROP COLUMN column_widths;

-- +goose StatementEnd
