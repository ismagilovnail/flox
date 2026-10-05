-- +goose Up
-- +goose StatementBegin

-- Per-comment details entered by the operator: avatar, likes, an explicit
-- comment date (NULL = none shown) and the developer's reply.
ALTER TABLE pwa_comments
  ADD COLUMN avatar_url   text NOT NULL DEFAULT '',
  ADD COLUMN likes        integer NOT NULL DEFAULT 0 CHECK (likes >= 0),
  ADD COLUMN comment_date date,
  ADD COLUMN reply        text NOT NULL DEFAULT '';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE pwa_comments
  DROP COLUMN avatar_url,
  DROP COLUMN likes,
  DROP COLUMN comment_date,
  DROP COLUMN reply;
-- +goose StatementEnd
