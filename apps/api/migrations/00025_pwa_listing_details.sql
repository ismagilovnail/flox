-- +goose Up
-- +goose StatementBegin

-- Third round of store-listing preview fields (see 00023, 00024): the
-- reference listing's "О приложении" section opens a detail sheet with a
-- "Подробная информация" table — updated / size / downloads / current
-- version / age rating / developer / category / interactive elements.
-- Four of those had no column yet. They're free-text labels rather than
-- enums on purpose: the reference shows operator-chosen strings there
-- ("12+", "Casino", "1.0.0"), and inventing a fixed vocabulary would
-- only get in the way of whatever the operator's real listing says.
--
-- Empty string, not NULL, is "never entered" for these (matching
-- developer/downloads_label/whats_new from 00023-00024) — the UI hides
-- the whole row rather than printing a fabricated value, so there is no
-- "explicitly blank vs. never set" distinction to preserve, unlike the
-- numeric rating/size columns.
ALTER TABLE pwas
  ADD COLUMN version              text NOT NULL DEFAULT '',
  ADD COLUMN age_rating           text NOT NULL DEFAULT '',
  ADD COLUMN category             text NOT NULL DEFAULT '',
  ADD COLUMN interactive_elements text NOT NULL DEFAULT '';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE pwas
  DROP COLUMN version,
  DROP COLUMN age_rating,
  DROP COLUMN category,
  DROP COLUMN interactive_elements;
-- +goose StatementEnd
