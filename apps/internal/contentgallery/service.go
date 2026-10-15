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

// allowedVideoExt is allowedImageExt for videos: stored as uploaded, no
// transcoding (docs/content-gallery.md "Video") — the two formats every
// browser plays. http.DetectContentType recognises both (an MP4's ftyp
// box with an mp4 brand, WebM's EBML header).
var allowedVideoExt = map[string]string{
	".mp4":  "video/mp4",
	".webm": "video/webm",
}

// posterTypes are the images a video's poster may be (the uploading
// browser draws the first frame on a canvas: JPEG, else WebP/PNG).
var posterTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/webp": ".webp",
	"image/png":  ".png",
}

const (
	// maxPosterBytes bounds a poster: one frame, downscaled by the browser.
	maxPosterBytes = 2 << 20
	// maxVideoDurationMs and maxVideoSide bound the display metadata.
	maxVideoDurationMs = 24 * 60 * 60 * 1000
	maxVideoSide       = 16384
)

type Service struct {
	repo     *Repository
	storage  *storage.Client
	landings LandingCreator // nil until WithLandings: landing templates unavailable
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
	// Poster and Meta come with a video only (ignored for an image), both
	// optional: made by the uploading browser.
	Poster *PosterInput
	Meta   VideoMeta
	Size   int64
	Reader io.Reader
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
	fileType := FileTypeImage
	wantType, ok := allowedImageExt[ext]
	if !ok {
		if wantType, ok = allowedVideoExt[ext]; ok {
			fileType = FileTypeVideo
		}
	}
	switch {
	case ext == ".mov":
		return File{}, apierror.Validation("MOV isn't supported — save the video as MP4 (H.264) and upload that", nil)
	case !ok:
		return File{}, apierror.Validation("unsupported file type — allowed: webp, jpg, jpeg, png, gif, mp4, webm", nil)
	case in.ContentType != wantType:
		return File{}, apierror.Validation("file content doesn't match its extension", nil)
	}
	if fileType == FileTypeVideo {
		if err := in.Meta.validate(); err != nil {
			return File{}, err
		}
		if in.Poster != nil {
			if err := in.Poster.validate(); err != nil {
				return File{}, err
			}
		}
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
	create := CreateFileInput{
		FolderID:         folderID,
		UploadedByUserID: uploadedByUserID,
		FileType:         fileType,
		ObjectKey:        objectKey,
		OriginalFilename: in.Filename,
		MimeType:         in.ContentType,
		SizeBytes:        in.Size,
	}
	if fileType == FileTypeVideo {
		create.DurationMs, create.Width, create.Height = in.Meta.DurationMs, in.Meta.Width, in.Meta.Height
		if in.Poster != nil {
			posterKey := orgID + "/" + id + "-poster" + posterTypes[in.Poster.ContentType]
			if err := s.storage.PutObject(ctx, posterKey, in.Poster.Reader, in.Poster.Size, in.Poster.ContentType); err != nil {
				_ = s.storage.DeleteObject(ctx, objectKey)
				return File{}, fmt.Errorf("contentgallery: uploading poster: %w", err)
			}
			create.PosterObjectKey = posterKey
		}
	}

	f, err := s.repo.CreateFile(ctx, id, orgID, create)
	if err != nil {
		// best-effort: avoid orphaned objects outliving their (failed) DB row
		_ = s.storage.DeleteObject(ctx, objectKey)
		if create.PosterObjectKey != "" {
			_ = s.storage.DeleteObject(ctx, create.PosterObjectKey)
		}
		return File{}, err
	}

	s.withURLs(&f)
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
		s.withURLs(&result.Files[i])
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
	deleted, err := s.repo.DeleteFiles(ctx, orgID, ids)
	if err != nil {
		return 0, err
	}
	if s.storage != nil {
		for _, d := range deleted {
			_ = s.storage.DeleteObject(ctx, d.Key)
			if d.PosterKey != "" {
				_ = s.storage.DeleteObject(ctx, d.PosterKey)
			}
		}
	}
	return len(deleted), nil
}

// withURLs fills a file's public URLs from its storage keys.
func (s *Service) withURLs(f *File) {
	f.URL = s.storage.PublicURL(f.ObjectKey)
	if f.PosterObjectKey != "" {
		f.PosterURL = s.storage.PublicURL(f.PosterObjectKey)
	}
}

// PosterInput is a video's poster image, sniffed by Handler.
type PosterInput struct {
	ContentType string
	Size        int64
	Reader      io.Reader
}

func (p PosterInput) validate() error {
	if _, ok := posterTypes[p.ContentType]; !ok {
		return apierror.Validation("the video's poster must be a JPEG, WebP or PNG image", nil)
	}
	if p.Size <= 0 || p.Size > maxPosterBytes {
		return apierror.Validation(fmt.Sprintf("the video's poster exceeds the %dMB limit", maxPosterBytes>>20), nil)
	}
	return nil
}

// VideoMeta is what the uploading browser read from a video — for display
// only, never trusted for anything else; each nil when not sent.
type VideoMeta struct {
	DurationMs, Width, Height *int
}

func (m VideoMeta) validate() error {
	inRange := func(v *int, max int) bool { return v == nil || (*v > 0 && *v <= max) }
	if !inRange(m.DurationMs, maxVideoDurationMs) || !inRange(m.Width, maxVideoSide) || !inRange(m.Height, maxVideoSide) {
		return apierror.Validation("invalid video duration or frame size", nil)
	}
	return nil
}
