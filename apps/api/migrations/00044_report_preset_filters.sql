-- +goose Up
-- +goose StatementBegin

-- Filters saved with a report preset: [{"dimension": "...", "value": "..."}],
-- AND-combined, the same shape the report builder sends to /analytics/report.
-- JSONB rather than a child table: a small fixed list, always read and written
-- whole with its preset, never queried by field. Existing presets get [] —
-- exactly what they always meant (no filters).
ALTER TABLE report_presets ADD COLUMN filters jsonb NOT NULL DEFAULT '[]';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE report_presets DROP COLUMN filters;

-- +goose StatementEnd
