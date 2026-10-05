-- +goose Up
-- +goose StatementBegin

-- The install page's top bar can carry the operator's own logo and title
-- (see 00038 show_header). Empty means "use the default mark and 'Apps'".
ALTER TABLE pwas
  ADD COLUMN header_title    text NOT NULL DEFAULT '',
  ADD COLUMN header_logo_url text NOT NULL DEFAULT '';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE pwas
  DROP COLUMN header_title,
  DROP COLUMN header_logo_url;
-- +goose StatementEnd
