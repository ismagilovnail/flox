-- +goose Up
-- +goose StatementBegin

-- Fourth round of store-listing fields (see 00023-00025). The
-- reference's "About this app" sheet carries data we had no column
-- for, and two things the preview was making up outright:
--
--   * contains_ads / in_app_purchases — the reference prints these
--     under the developer name ("Есть реклама", "Покупки в
--     приложении"). Our preview hardcoded the opposite claim ("no ads
--     or paid content") for every PWA, which is exactly the kind of
--     fabricated statement CLAUDE.md #6 rules out. Booleans, since
--     Google Play's own listing treats them as present/absent.
--
--   * updated_date — the reference shows a stored date ("21 авг. 2026
--     г."), both on the card and in the sheet. The preview was
--     printing today's date instead, i.e. asserting "updated today"
--     for every PWA forever. Nullable: never entered hides the row
--     rather than claiming a date.
ALTER TABLE pwas
  ADD COLUMN contains_ads     boolean NOT NULL DEFAULT false,
  ADD COLUMN in_app_purchases boolean NOT NULL DEFAULT false,
  ADD COLUMN updated_date     date;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE pwas
  DROP COLUMN contains_ads,
  DROP COLUMN in_app_purchases,
  DROP COLUMN updated_date;
-- +goose StatementEnd
