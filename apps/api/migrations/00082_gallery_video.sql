-- Videos in the Content Gallery (docs/content-gallery.md "Video"): MP4 and
-- WebM stored as uploaded — no transcoding. The browser that uploads one
-- also sends its first frame as a poster image and its duration and frame
-- size (display only, read from the file by the browser); a video without
-- them (the browser could not decode it) still uploads. file_type 'video'
-- was allowed from the start (00027).

-- +goose Up
ALTER TABLE content_gallery_files
  ADD COLUMN poster_object_key text NOT NULL DEFAULT '',
  ADD COLUMN duration_ms integer CHECK (duration_ms > 0),
  ADD COLUMN width integer CHECK (width > 0),
  ADD COLUMN height integer CHECK (height > 0);

-- +goose Down
ALTER TABLE content_gallery_files
  DROP COLUMN poster_object_key,
  DROP COLUMN duration_ms,
  DROP COLUMN width,
  DROP COLUMN height;
