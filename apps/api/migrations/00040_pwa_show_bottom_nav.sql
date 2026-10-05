-- +goose Up
-- +goose StatementBegin

-- show_bottom_nav toggles the install page's bottom bar with the menu
-- buttons (Games / Apps / Books / Kids). On by default: existing PWAs are
-- unchanged.
ALTER TABLE pwas
  ADD COLUMN show_bottom_nav boolean NOT NULL DEFAULT true;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE pwas
  DROP COLUMN show_bottom_nav;
-- +goose StatementEnd
