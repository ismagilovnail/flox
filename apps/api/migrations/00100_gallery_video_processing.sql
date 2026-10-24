-- Video transcoding in the Content Gallery (docs/content-gallery.md
-- "Transcoding").
--
-- A video in a format browsers do not all play (MOV, M4V, AVI, MKV, 3GP)
-- is stored as uploaded with processing_status 'processing'; apps/worker
-- turns it into an MP4 (H.264/AAC), replaces object_key with it, deletes
-- the source and marks it 'ready' — or 'failed' with processing_error.
-- Every file from before, and every image/MP4/WebM, is 'ready'.
--
-- processing_started_at / processing_attempts: the worker's claim (a
-- lease, so a crashed worker's file is taken again) and how many times it
-- was taken (a file that keeps crashing the worker is given up on).
--
-- IF NOT EXISTS: this was 00085 while it was being written, and dev
-- databases applied it under that number.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE content_gallery_files
  ADD COLUMN IF NOT EXISTS processing_status text NOT NULL DEFAULT 'ready'
    CHECK (processing_status IN ('ready', 'processing', 'failed')),
  ADD COLUMN IF NOT EXISTS processing_error text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS processing_started_at timestamptz,
  ADD COLUMN IF NOT EXISTS processing_attempts int NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS content_gallery_files_processing_idx ON content_gallery_files (created_at)
  WHERE processing_status = 'processing';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS content_gallery_files_processing_idx;
ALTER TABLE content_gallery_files
  DROP COLUMN IF EXISTS processing_status, DROP COLUMN IF EXISTS processing_error,
  DROP COLUMN IF EXISTS processing_started_at, DROP COLUMN IF EXISTS processing_attempts;
-- +goose StatementEnd
