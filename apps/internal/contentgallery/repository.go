package contentgallery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const folderColumns = `id, organization_id, name, created_at, updated_at`

func scanFolder(row pgx.Row) (Folder, error) {
	var f Folder
	err := row.Scan(&f.ID, &f.OrganizationID, &f.Name, &f.CreatedAt, &f.UpdatedAt)
	return f, err
}

func (r *Repository) ListFolders(ctx context.Context, orgID string) ([]Folder, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+folderColumns+`
		FROM content_gallery_folders
		WHERE organization_id = $1
		ORDER BY name`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("contentgallery: listing folders: %w", err)
	}
	defer rows.Close()

	out := []Folder{}
	for rows.Next() {
		f, err := scanFolder(rows)
		if err != nil {
			return nil, fmt.Errorf("contentgallery: scanning folder: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (r *Repository) GetFolder(ctx context.Context, orgID, id string) (Folder, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+folderColumns+`
		FROM content_gallery_folders
		WHERE id = $1 AND organization_id = $2`,
		id, orgID,
	)
	f, err := scanFolder(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Folder{}, apierror.NotFound("folder not found")
	}
	return f, err
}

func (r *Repository) CreateFolder(ctx context.Context, id, orgID, name string) (Folder, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO content_gallery_folders (id, organization_id, name)
		VALUES ($1, $2, $3)
		RETURNING `+folderColumns,
		id, orgID, name,
	)
	f, err := scanFolder(row)
	if err != nil {
		return Folder{}, fmt.Errorf("contentgallery: inserting folder: %w", err)
	}
	return f, nil
}

func (r *Repository) RenameFolder(ctx context.Context, orgID, id, name string) (Folder, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE content_gallery_folders
		SET name = $1
		WHERE id = $2 AND organization_id = $3
		RETURNING `+folderColumns,
		name, id, orgID,
	)
	f, err := scanFolder(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Folder{}, apierror.NotFound("folder not found")
	}
	return f, err
}

// DeleteFolder moves every file in the folder to the root first — the
// same order migration 00027's ON DELETE SET NULL would produce on its
// own, done explicitly here inside a transaction so the two writes commit
// together (the FK's own cascade only fires for a raw SQL DELETE, not a
// guarantee this Go-level two-step needs to rely on).
func (r *Repository) DeleteFolder(ctx context.Context, orgID, id string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("contentgallery: beginning delete-folder tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE content_gallery_files SET folder_id = NULL
		WHERE folder_id = $1 AND organization_id = $2`,
		id, orgID,
	); err != nil {
		return fmt.Errorf("contentgallery: moving files to root: %w", err)
	}

	tag, err := tx.Exec(ctx, `DELETE FROM content_gallery_folders WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		return fmt.Errorf("contentgallery: deleting folder: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("folder not found")
	}

	return tx.Commit(ctx)
}

const fileColumns = `id, organization_id, folder_id, uploaded_by_user_id, file_type, object_key, original_filename, mime_type, size_bytes, status, created_at, updated_at, poster_object_key, duration_ms, width, height, processing_status, processing_error`

func scanFile(row pgx.Row) (File, error) {
	var f File
	err := row.Scan(
		&f.ID, &f.OrganizationID, &f.FolderID, &f.UploadedByUserID, &f.FileType, &f.ObjectKey,
		&f.OriginalFilename, &f.MimeType, &f.SizeBytes, &f.Status, &f.CreatedAt, &f.UpdatedAt,
		&f.PosterObjectKey, &f.DurationMs, &f.Width, &f.Height, &f.ProcessingStatus, &f.ProcessingError,
	)
	return f, err
}

type CreateFileInput struct {
	FolderID         *string
	UploadedByUserID string
	FileType         FileType
	ObjectKey        string
	OriginalFilename string
	MimeType         string
	SizeBytes        int64
	// Video only (VideoMeta).
	PosterObjectKey string
	DurationMs      *int
	Width, Height   *int
	// Processing: ProcessingInProgress for a source to transcode; "" =
	// ready.
	Processing ProcessingStatus
}

func (r *Repository) CreateFile(ctx context.Context, id, orgID string, in CreateFileInput) (File, error) {
	processing := in.Processing
	if processing == "" {
		processing = ProcessingReady
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO content_gallery_files
			(id, organization_id, folder_id, uploaded_by_user_id, file_type, object_key, original_filename, mime_type, size_bytes,
			 poster_object_key, duration_ms, width, height, processing_status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING `+fileColumns,
		id, orgID, in.FolderID, in.UploadedByUserID, in.FileType, in.ObjectKey, in.OriginalFilename, in.MimeType, in.SizeBytes,
		in.PosterObjectKey, in.DurationMs, in.Width, in.Height, processing,
	)
	f, err := scanFile(row)
	if err != nil {
		return File{}, fmt.Errorf("contentgallery: inserting file: %w", err)
	}
	return f, nil
}

// ListFiles applies ListFilesParams.FolderID's tri-state filter (see that
// field's own doc comment) plus the optional type/uploader filters, newest
// first, and returns the total matching row count alongside the page for
// pagination — same two-query shape apps/internal/clicklogs uses.
func (r *Repository) ListFiles(ctx context.Context, orgID string, p ListFilesParams) (ListFilesResult, error) {
	where := []string{"organization_id = $1"}
	args := []any{orgID}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if p.FolderID != nil {
		if *p.FolderID == "" {
			where = append(where, "folder_id IS NULL")
		} else {
			where = append(where, "folder_id = "+arg(*p.FolderID))
		}
	}
	if p.FileType != "" {
		where = append(where, "file_type = "+arg(p.FileType))
	}
	status := p.Status
	if status == "" {
		status = FileStatusActive
	}
	where = append(where, "status = "+arg(status))
	if p.UploadedByUserID != "" {
		where = append(where, "uploaded_by_user_id = "+arg(p.UploadedByUserID))
	}

	whereClause := strings.Join(where, " AND ")

	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM content_gallery_files WHERE `+whereClause, args...).Scan(&total); err != nil {
		return ListFilesResult{}, fmt.Errorf("contentgallery: counting files: %w", err)
	}

	limitArg := arg(p.Limit)
	offsetArg := arg(p.Offset)
	rows, err := r.db.Query(ctx, `
		SELECT `+fileColumns+`
		FROM content_gallery_files
		WHERE `+whereClause+`
		ORDER BY created_at DESC
		LIMIT `+limitArg+` OFFSET `+offsetArg,
		args...,
	)
	if err != nil {
		return ListFilesResult{}, fmt.Errorf("contentgallery: listing files: %w", err)
	}
	defer rows.Close()

	files := []File{}
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return ListFilesResult{}, fmt.Errorf("contentgallery: scanning file: %w", err)
		}
		files = append(files, f)
	}
	if err := rows.Err(); err != nil {
		return ListFilesResult{}, fmt.Errorf("contentgallery: reading files: %w", err)
	}

	return ListFilesResult{Files: files, Total: total}, nil
}

// MoveFiles and SetFilesStatus both use = ANY($n) over the id list rather
// than one UPDATE per id — Service caps ids at 50 (matching the
// reference's own bulk-selection limit), so this is always a small,
// single-round-trip statement.

func (r *Repository) MoveFiles(ctx context.Context, orgID string, ids []string, folderID *string) (int, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE content_gallery_files
		SET folder_id = $1
		WHERE organization_id = $2 AND id = ANY($3)`,
		folderID, orgID, ids,
	)
	if err != nil {
		return 0, fmt.Errorf("contentgallery: moving files: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (r *Repository) SetFilesStatus(ctx context.Context, orgID string, ids []string, status FileStatus) (int, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE content_gallery_files
		SET status = $1
		WHERE organization_id = $2 AND id = ANY($3)`,
		status, orgID, ids,
	)
	if err != nil {
		return 0, fmt.Errorf("contentgallery: updating file status: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// DeleteFiles permanently removes rows whose status is already
// FileStatusArchived — the WHERE clause itself is the "must be archived
// first" rule (Service doesn't need a separate check), matching the
// reference's own Active/Inactive-then-permanently-delete flow: hard
// deletion only ever happens from the Inactive list, never directly off
// an active file. Returns the deleted rows' object keys so the caller can
// clean them up in object storage.
// deletedFile is a deleted row's storage objects: the file and, for a
// video, its poster ("" when none).
type deletedFile struct {
	Key, PosterKey string
}

func (r *Repository) DeleteFiles(ctx context.Context, orgID string, ids []string) ([]deletedFile, error) {
	rows, err := r.db.Query(ctx, `
		DELETE FROM content_gallery_files
		WHERE organization_id = $1 AND id = ANY($2) AND status = $3
		RETURNING object_key, poster_object_key`,
		orgID, ids, FileStatusArchived,
	)
	if err != nil {
		return nil, fmt.Errorf("contentgallery: deleting files: %w", err)
	}
	defer rows.Close()

	out := []deletedFile{}
	for rows.Next() {
		var d deletedFile
		if err := rows.Scan(&d.Key, &d.PosterKey); err != nil {
			return nil, fmt.Errorf("contentgallery: scanning deleted key: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// processingJob is a source video claimed for transcoding.
type processingJob struct {
	ID, OrganizationID, SourceKey string
	Attempts                      int
}

// claimProcessing takes the oldest video waiting for transcoding whose
// lease (processing_started_at) is free or older than lease — a worker
// that died mid-way gives its file back that way — and counts the attempt.
// orgID "" is every organization; tests scope it to their own.
func (r *Repository) claimProcessing(ctx context.Context, lease time.Duration, orgID string) (processingJob, bool, error) {
	var j processingJob
	err := r.db.QueryRow(ctx, `
		UPDATE content_gallery_files f
		SET processing_started_at = now(), processing_attempts = f.processing_attempts + 1
		WHERE f.id = (
			SELECT id FROM content_gallery_files
			WHERE processing_status = 'processing'
			  AND (processing_started_at IS NULL OR processing_started_at < now() - make_interval(secs => $1))
			  AND ($2 = '' OR organization_id = $2)
			ORDER BY created_at, id
			FOR UPDATE SKIP LOCKED LIMIT 1)
		RETURNING f.id, f.organization_id, f.object_key, f.processing_attempts`, lease.Seconds(), orgID,
	).Scan(&j.ID, &j.OrganizationID, &j.SourceKey, &j.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return processingJob{}, false, nil
	}
	if err != nil {
		return processingJob{}, false, fmt.Errorf("contentgallery: claiming a video to transcode: %w", err)
	}
	return j, true, nil
}

// transcoded is what the worker made of a source.
type transcoded struct {
	ObjectKey, PosterKey string
	Size                 int64
	DurationMs           int
	Width, Height        int
}

// finishProcessing replaces job's source with the MP4. False: the file is
// gone or no longer this job's (deleted meanwhile) — nothing changed.
func (r *Repository) finishProcessing(ctx context.Context, j processingJob, t transcoded) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE content_gallery_files
		SET object_key = $3, mime_type = 'video/mp4', size_bytes = $4, poster_object_key = $5,
		    duration_ms = $6, width = $7, height = $8,
		    processing_status = 'ready', processing_error = '', processing_started_at = NULL, updated_at = now()
		WHERE id = $1 AND object_key = $2 AND processing_status = 'processing'`,
		j.ID, j.SourceKey, t.ObjectKey, t.Size, t.PosterKey, t.DurationMs, t.Width, t.Height)
	if err != nil {
		return false, fmt.Errorf("contentgallery: storing a transcoded video: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// failProcessing marks job's file failed with a reason for the operator.
// The source stays until the file is deleted.
func (r *Repository) failProcessing(ctx context.Context, j processingJob, reason string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE content_gallery_files
		SET processing_status = 'failed', processing_error = $3, processing_started_at = NULL, updated_at = now()
		WHERE id = $1 AND object_key = $2 AND processing_status = 'processing'`, j.ID, j.SourceKey, reason)
	if err != nil {
		return fmt.Errorf("contentgallery: marking a video failed: %w", err)
	}
	return nil
}
