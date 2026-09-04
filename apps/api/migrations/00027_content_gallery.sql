-- +goose Up
-- +goose StatementBegin

-- Content Gallery (replaces the frontend-only "template gallery" mock —
-- see apps/internal/contentgallery's package doc for the full story): a
-- real, tenant-scoped media library with a flat folder structure, matching
-- the reference tracker's own Content Gallery UX. Folders don't nest (the
-- reference doesn't either — just a flat list you toggle between).
CREATE TABLE content_gallery_folders (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  name            text NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX content_gallery_folders_organization_id_idx ON content_gallery_folders (organization_id);

CREATE TRIGGER content_gallery_folders_set_updated_at
  BEFORE UPDATE ON content_gallery_folders
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- file_type's CHECK already lists 'video' even though this phase's service
-- layer (apps/internal/contentgallery) only ever accepts 'image' uploads —
-- video needs a transcoding pipeline (ffmpeg + worker job) this repo
-- doesn't have yet, deliberately deferred rather than built half-way.
-- Widening the CHECK later would need its own migration either way, so
-- there's no cost to leaving room in the enum now.
--
-- folder_id uses ON DELETE SET NULL, not CASCADE: deleting a folder moves
-- its files to the root (folder_id = NULL), it never deletes them — same
-- rule the reference's own folder-delete confirmation states.
CREATE TABLE content_gallery_files (
  id                  ulid PRIMARY KEY,
  organization_id     ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  folder_id           ulid REFERENCES content_gallery_folders (id) ON DELETE SET NULL,
  uploaded_by_user_id ulid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  file_type           text NOT NULL CHECK (file_type IN ('image', 'video')),
  object_key          text NOT NULL,
  original_filename   text NOT NULL,
  mime_type           text NOT NULL,
  size_bytes          bigint NOT NULL,
  status              text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX content_gallery_files_organization_id_idx ON content_gallery_files (organization_id);
CREATE INDEX content_gallery_files_folder_id_idx ON content_gallery_files (folder_id);
CREATE INDEX content_gallery_files_org_status_idx ON content_gallery_files (organization_id, status);

CREATE TRIGGER content_gallery_files_set_updated_at
  BEFORE UPDATE ON content_gallery_files
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE content_gallery_files;
DROP TABLE content_gallery_folders;
-- +goose StatementEnd
