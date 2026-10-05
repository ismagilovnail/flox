-- +goose Up
-- +goose StatementBegin

-- pwa_comments: the operator's own hand-entered comment sets for a PWA's
-- install page, grouped by geo + language. position keeps the entry order.
-- Replaced wholesale on save (PUT /pwas/{id}/comments).
CREATE TABLE pwa_comments (
  id               ulid PRIMARY KEY,
  organization_id  ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  pwa_id           ulid NOT NULL REFERENCES pwas (id) ON DELETE CASCADE,
  geo              text NOT NULL,
  lang             text NOT NULL,
  author           text NOT NULL,
  rating           smallint NOT NULL CHECK (rating BETWEEN 1 AND 5),
  body             text NOT NULL,
  position         integer NOT NULL,
  created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX pwa_comments_pwa_idx ON pwa_comments (organization_id, pwa_id, position);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE pwa_comments;
-- +goose StatementEnd
