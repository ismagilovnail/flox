-- Tech audit 2026-10-06, AN5: reports can date conversions by their click
-- instead of their own time, so a campaign day's revenue sits beside that
-- day's clicks and spend. A preset remembers which.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE report_presets ADD COLUMN date_basis text NOT NULL DEFAULT 'conversion'
  CHECK (date_basis IN ('conversion', 'click'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE report_presets DROP COLUMN date_basis;
-- +goose StatementEnd
