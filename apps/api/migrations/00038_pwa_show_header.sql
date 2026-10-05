-- +goose Up
-- +goose StatementBegin

-- show_header toggles the install page's top bar (brand mark, "Apps" title,
-- search and help icons). On by default: existing PWAs are unchanged.
ALTER TABLE pwas
  ADD COLUMN show_header boolean NOT NULL DEFAULT true;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE pwas
  DROP COLUMN show_header;
-- +goose StatementEnd
