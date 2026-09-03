-- +goose Up
-- +goose StatementBegin

-- Second round of store-listing preview fields (see 00023): the user
-- asked for the full listing page (screenshots gallery, "what's new",
-- per-star rating breakdown), not just the compact card. screenshots is
-- an ordered list of image URLs, same text[] shape postlandings.events
-- already uses. rating_stars_5..1 are independently nullable — a
-- breakdown percentage that was never entered stays "—" in the UI, not
-- a fabricated 0% (same CLAUDE.md #6 reasoning as 00023's rating/
-- rating_count/size_mb).
ALTER TABLE pwas
  ADD COLUMN screenshots    text[] NOT NULL DEFAULT '{}',
  ADD COLUMN whats_new      text NOT NULL DEFAULT '',
  ADD COLUMN rating_stars_5 numeric(5,2),
  ADD COLUMN rating_stars_4 numeric(5,2),
  ADD COLUMN rating_stars_3 numeric(5,2),
  ADD COLUMN rating_stars_2 numeric(5,2),
  ADD COLUMN rating_stars_1 numeric(5,2);

ALTER TABLE pwas ADD CONSTRAINT pwas_rating_stars_5_range CHECK (rating_stars_5 IS NULL OR (rating_stars_5 >= 0 AND rating_stars_5 <= 100));
ALTER TABLE pwas ADD CONSTRAINT pwas_rating_stars_4_range CHECK (rating_stars_4 IS NULL OR (rating_stars_4 >= 0 AND rating_stars_4 <= 100));
ALTER TABLE pwas ADD CONSTRAINT pwas_rating_stars_3_range CHECK (rating_stars_3 IS NULL OR (rating_stars_3 >= 0 AND rating_stars_3 <= 100));
ALTER TABLE pwas ADD CONSTRAINT pwas_rating_stars_2_range CHECK (rating_stars_2 IS NULL OR (rating_stars_2 >= 0 AND rating_stars_2 <= 100));
ALTER TABLE pwas ADD CONSTRAINT pwas_rating_stars_1_range CHECK (rating_stars_1 IS NULL OR (rating_stars_1 >= 0 AND rating_stars_1 <= 100));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE pwas
  DROP COLUMN screenshots,
  DROP COLUMN whats_new,
  DROP COLUMN rating_stars_5,
  DROP COLUMN rating_stars_4,
  DROP COLUMN rating_stars_3,
  DROP COLUMN rating_stars_2,
  DROP COLUMN rating_stars_1;
-- +goose StatementEnd
