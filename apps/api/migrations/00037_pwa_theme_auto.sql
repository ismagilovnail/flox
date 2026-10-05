-- +goose Up
-- +goose StatementBegin

-- theme_auto makes the install page follow the visitor's device theme
-- (prefers-color-scheme) instead of the fixed light/dark derived from
-- background_color. background_color stays: it is still the manifest's
-- background and the fallback for browsers without the media query.
ALTER TABLE pwas
  ADD COLUMN theme_auto boolean NOT NULL DEFAULT false;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE pwas
  DROP COLUMN theme_auto;
-- +goose StatementEnd
