-- +goose Up
-- +goose StatementBegin

-- Tech audit 2026-10-06, DO2. The backup drill (internal/backup) writes a
-- row here just before restoring Postgres from its base backup and archived
-- WAL; finding it in the restore proves the restore reaches the present,
-- not only the last base backup. tables is the public tables at that
-- moment — the restore must have every one. Not tenant data: no
-- organization_id, read by nobody else.
CREATE TABLE backup_drill_marks (
  token      text PRIMARY KEY,
  tables     text[] NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE backup_drill_marks;
-- +goose StatementEnd
