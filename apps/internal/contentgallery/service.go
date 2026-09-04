package contentgallery

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/storage"
)

const (
	// maxUploadBytes matches the reference's own stated video cap (20MB);
	// applied to images too as a sane default since the reference names no
	// separate image limit.
	maxUploadBytes = 20 << 20
	// maxSelection matches the reference's own bulk-selection limit.
	maxSelection = 50
	// folderNameMax matches the reference's own folder-name character cap.
	folderNameMax = 50
)

// allowedImageExt maps a lowercased filename extension to the content
// type http.DetectContentType must actually sniff from the file's own
// bytes — Handler never trusts the client-supplied Content-Type header,
// and this cross-check means a renamed-but-wrong file (e.g. a .exe saved
// as .png) is rejected instead of silently mis-labeled.
var allowedImageExt = map[string]string{
	".webp": "image/webp",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
}

// videoExt exists only to give a video upload a clear, honest rejection
// message ("not supported yet") instead of the generic "unsupported file
// type" a truly unknown extension gets.
var videoExt = map[string]bool{".mp4": true, ".mov": true, ".webm": true}

type Service struct {
	repo    *Repository
	storage *storage.Client
}

// storageClient may be nil (object storage failed to initialize at
// startup, matching apps/api/main.go's ClickHouse/Redis best-effort
// pattern) — every method that needs it checks and returns
// apierror.Unavailable rather than panicking.
func NewService(repo *Repository, storageClient *storage.Client) *Service {
	return &Service{repo: repo, storage: storageClient}
}

func (s *Service) ListFolders(ctx context.Context, orgID string) ([]Folder, error) {
	return s.repo.ListFolders(ctx, orgID)
}

func validateFolderName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", apierror.Validation("folder name is required", nil)
	}
	if len([]rune(name)) > folderNameMax {
		return "", apierror.Validation(fmt.Sprintf("folder name must be %d characters or fewer", folderNameMax), nil)
	}
	return name, nil
}

func (s *Service) CreateFolder(ctx context.Context, orgID, name string) (Folder, error) {
	name, err := validateFolderName(name)
	if err != nil {
		return Folder{}, err
	}
	return s.repo.CreateFolder(ctx, idgen.New(), orgID, name)
}

func (s *Service) RenameFolder(ctx context.Context, orgID, id, name string) (Folder, error) {
	name, err := validateFolderName(name)
	if err != nil {
		return Folder{}, err
	}
	return s.repo.RenameFolder(ctx, orgID, id, name)
}

// DeleteFolder moves the folder's files to the root, not into the void —
// see Repository.DeleteFolder and this package's own doc comment.
func (s *Service) DeleteFolder(ctx context.Context, orgID, id string) error {
	return s.repo.DeleteFolder(ctx, orgID, id)
}

type UploadInput struct {
	FolderID    *string // nil or "" = root
	Filename    string
	ContentType string // sniffed by Handler via http.DetectContentType, not client-supplied
	Size        int64
	Reader      io.Reader
}

func (s *Service) UploadFile(ctx context.Context, orgID, uploadedByUserID string, in UploadInput) (File, error) {
	if s.storage == nil {
		return File{}, apierror.Unavailable("object storage is not configured; uploads are unavailable")
	}
	if in.Size <= 0 {
		return File{}, apierror.Validation("file is empty", nil)
	}
	if in.Size > maxUploadBytes {
		return File{}, apierror.Validation(fmt.Sprintf("file exceeds the %dMB limit", maxUploadBytes>>20), nil)
	}

	ext := strings.ToLower(filepath.Ext(in.Filename))
	if videoExt[ext] {
		return File{}, apierror.Validation("video uploads aren't supported yet", nil)
	}
	wantType, ok := allowedImageExt[ext]
	if !ok {
		return File{}, apierror.Validation("unsupported file type — allowed: webp, jpg, jpeg, png, gif", nil)
	}
	if in.ContentType != wantType {
		return File{}, apierror.Validation("file content doesn't match its extension", nil)
	}

	var folderID *string
	if in.FolderID != nil && *in.FolderID != "" {
		if _, err := s.repo.GetFolder(ctx, orgID, *in.FolderID); err != nil {
			return File{}, err
		}
		folderID = in.FolderID
	}

	id := idgen.New()
	objectKey := orgID + "/" + id + ext
	if err := s.storage.PutObject(ctx, objectKey, in.Reader, in.Size, in.ContentType); err != nil {
		return File{}, fmt.Errorf("contentgallery: uploading: %w", err)
	}

	f, err := s.repo.CreateFile(ctx, id, orgID, CreateFileInput{
		FolderID:         folderID,
		UploadedByUserID: uploadedByUserID,
		FileType:         FileTypeImage,
		ObjectKey:        objectKey,
		OriginalFilename: in.Filename,
		MimeType:         in.ContentType,
		SizeBytes:        in.Size,
	})
	if err != nil {
		_ = s.storage.DeleteObject(ctx, objectKey) // best-effort: avoid an orphaned object outliving its (failed) DB row
		return File{}, err
	}

	f.URL = s.storage.PublicURL(f.ObjectKey)
	return f, nil
}

func (s *Service) ListFiles(ctx context.Context, orgID string, p ListFilesParams) (ListFilesResult, error) {
	if s.storage == nil {
		return ListFilesResult{}, apierror.Unavailable("object storage is not configured")
	}
	if p.Limit <= 0 {
		p.Limit = 50
	}
	if p.Limit > 200 {
		p.Limit = 200
	}
	if p.Offset < 0 {
		p.Offset = 0
	}

	result, err := s.repo.ListFiles(ctx, orgID, p)
	if err != nil {
		return ListFilesResult{}, err
	}

	for i := range result.Files {
		result.Files[i].URL = s.storage.PublicURL(result.Files[i].ObjectKey)
	}
	return result, nil
}

func validateIDs(ids []string) error {
	if len(ids) == 0 {
		return apierror.Validation("select at least one file", nil)
	}
	if len(ids) > maxSelection {
		return apierror.Validation(fmt.Sprintf("select at most %d files at a time", maxSelection), nil)
	}
	return nil
}

func (s *Service) MoveFiles(ctx context.Context, orgID string, ids []string, folderID *string) (int, error) {
	if err := validateIDs(ids); err != nil {
		return 0, err
	}
	if folderID != nil && *folderID != "" {
		if _, err := s.repo.GetFolder(ctx, orgID, *folderID); err != nil {
			return 0, err
		}
	} else {
		folderID = nil
	}
	return s.repo.MoveFiles(ctx, orgID, ids, folderID)
}

func (s *Service) ArchiveFiles(ctx context.Context, orgID string, ids []string) (int, error) {
	if err := validateIDs(ids); err != nil {
		return 0, err
	}
	return s.repo.SetFilesStatus(ctx, orgID, ids, FileStatusArchived)
}

func (s *Service) RestoreFiles(ctx context.Context, orgID string, ids []string) (int, error) {
	if err := validateIDs(ids); err != nil {
		return 0, err
	}
	return s.repo.SetFilesStatus(ctx, orgID, ids, FileStatusActive)
}

// DeleteFiles permanently removes already-archived files — the one hard-
// delete this package has, deliberately gated to the Inactive list (see
// Repository.DeleteFiles' own doc: its WHERE clause silently skips any id
// that isn't already archived, so this can never be used to instantly
// destroy an active file). Object storage cleanup is best-effort: a
// failed DeleteObject here leaves an orphaned object, which costs storage
// but not correctness — the DB row is already gone either way.
func (s *Service) DeleteFiles(ctx context.Context, orgID string, ids []string) (int, error) {
	if err := validateIDs(ids); err != nil {
		return 0, err
	}
	keys, err := s.repo.DeleteFiles(ctx, orgID, ids)
	if err != nil {
		return 0, err
	}
	if s.storage != nil {
		for _, key := range keys {
			_ = s.storage.DeleteObject(ctx, key)
		}
	}
	return len(keys), nil
}
