-- +goose Up
-- +goose StatementBegin

-- Internal landings become uploaded ZIP bundles (docs/landings.md, "ZIP
-- bundles"): the entry file's source (index.html or index.twig) keeps
-- living in `content` — it's what the template editor edits and what the
-- tracker will render — while every other file in the archive (CSS, JS,
-- images, fonts) is unpacked into object storage under bundle_prefix.
--
-- title/logo_url are the listing metadata the reference product requires
-- on every uploaded landing (display title + a square logo).
ALTER TABLE landings
  ADD COLUMN title         text    NOT NULL DEFAULT '',
  ADD COLUMN logo_url      text    NOT NULL DEFAULT '',
  ADD COLUMN entry_file    text    NOT NULL DEFAULT 'index.html' CHECK (entry_file IN ('index.html', 'index.twig')),
  ADD COLUMN bundle_prefix text    NOT NULL DEFAULT '', -- object-storage key prefix of the current upload; '' = no assets
  ADD COLUMN file_count    integer NOT NULL DEFAULT 0,
  ADD COLUMN bundle_size   bigint  NOT NULL DEFAULT 0;

-- An internal landing's url used to be a slug under cdn.floxlink.io that
-- nothing ever served. It's now derived at read time from bundle_prefix,
-- never stored — clear the dead values rather than keep showing them.
UPDATE landings SET url = '' WHERE type = 'internal';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE landings
  DROP COLUMN title,
  DROP COLUMN logo_url,
  DROP COLUMN entry_file,
  DROP COLUMN bundle_prefix,
  DROP COLUMN file_count,
  DROP COLUMN bundle_size;
-- +goose StatementEnd
