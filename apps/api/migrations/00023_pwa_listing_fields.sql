-- +goose Up
-- +goose StatementBegin

-- §28/§73 follow-up: the PWA create page grew a live "app store listing"
-- preview (a Google-Play-style card) alongside the existing Web App
-- Manifest form — these columns are the real, operator-entered values
-- that preview renders, not decorative-only frontend state. rating/
-- rating_count/size_mb are nullable together: "no rating/size entered
-- yet" is a real, distinct state from "0," same reasoning cost/ROI
-- fields use elsewhere (CLAUDE.md #6 — never compute/display against a
-- fabricated zero).
ALTER TABLE pwas
  ADD COLUMN developer      text NOT NULL DEFAULT '',
  ADD COLUMN description    text NOT NULL DEFAULT '',
  ADD COLUMN rating         numeric(2,1),
  ADD COLUMN rating_count   integer,
  ADD COLUMN downloads_label text NOT NULL DEFAULT '',
  ADD COLUMN size_mb        numeric(7,1);

ALTER TABLE pwas ADD CONSTRAINT pwas_rating_range CHECK (rating IS NULL OR (rating >= 0 AND rating <= 5));
ALTER TABLE pwas ADD CONSTRAINT pwas_rating_count_nonneg CHECK (rating_count IS NULL OR rating_count >= 0);
ALTER TABLE pwas ADD CONSTRAINT pwas_size_mb_nonneg CHECK (size_mb IS NULL OR size_mb >= 0);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE pwas
  DROP COLUMN developer,
  DROP COLUMN description,
  DROP COLUMN rating,
  DROP COLUMN rating_count,
  DROP COLUMN downloads_label,
  DROP COLUMN size_mb;
-- +goose StatementEnd
