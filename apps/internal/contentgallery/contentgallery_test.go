package contentgallery_test

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/contentgallery"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/storage"
)

func mustPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connecting to database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func mustStorage(t *testing.T) *storage.Client {
	t.Helper()
	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("S3_ENDPOINT not set, skipping integration test")
	}
	client, err := storage.NewClient(config.S3Config{
		Endpoint:        endpoint,
		Region:          os.Getenv("S3_REGION"),
		Bucket:          os.Getenv("S3_BUCKET"),
		AccessKeyID:     os.Getenv("S3_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("S3_SECRET_ACCESS_KEY"),
		PublicEndpoint:  os.Getenv("S3_PUBLIC_ENDPOINT"),
	})
	if err != nil {
		t.Fatalf("building storage client: %v", err)
	}
	if err := client.EnsureBucket(context.Background(), os.Getenv("S3_REGION")); err != nil {
		t.Skipf("object storage unreachable, skipping integration test: %v", err)
	}
	return client
}

func seedOrg(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, id, "Test Org "+id)
	if err != nil {
		t.Fatalf("seeding organization: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, id)
	})
	return id
}

func seedUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx, `INSERT INTO users (id, email, name) VALUES ($1, $2, $3)`, id, id+"@example.com", "Test User "+id)
	if err != nil {
		t.Fatalf("seeding user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id)
	})
	return id
}

// A minimal valid 1x1 PNG — real bytes (not a fabricated header), so
// http.DetectContentType inside Handler/Service genuinely sniffs
// "image/png" the same way a real upload would, rather than testing
// against something that only looks like an image.
var onePixelPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x62, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

func mustUpload(t *testing.T, ctx context.Context, svc *contentgallery.Service, orgID, userID string, folderID *string) contentgallery.File {
	t.Helper()
	f, err := svc.UploadFile(ctx, orgID, userID, contentgallery.UploadInput{
		FolderID:    folderID,
		Filename:    "pixel.png",
		ContentType: "image/png",
		Size:        int64(len(onePixelPNG)),
		Reader:      bytes.NewReader(onePixelPNG),
	})
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	return f
}

func TestFolderCreateRenameDelete(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := contentgallery.NewService(contentgallery.NewRepository(pool), nil)

	folder, err := svc.CreateFolder(ctx, orgID, "  Banners  ")
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	if folder.Name != "Banners" {
		t.Fatalf("Name = %q, want trimmed", folder.Name)
	}

	renamed, err := svc.RenameFolder(ctx, orgID, folder.ID, "Sweeps Banners")
	if err != nil {
		t.Fatalf("RenameFolder: %v", err)
	}
	if renamed.Name != "Sweeps Banners" {
		t.Fatalf("Name = %q after rename", renamed.Name)
	}

	folders, err := svc.ListFolders(ctx, orgID)
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if len(folders) != 1 || folders[0].ID != folder.ID {
		t.Fatalf("folders = %+v, want just the one", folders)
	}

	if err := svc.DeleteFolder(ctx, orgID, folder.ID); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	folders, err = svc.ListFolders(ctx, orgID)
	if err != nil {
		t.Fatalf("ListFolders after delete: %v", err)
	}
	if len(folders) != 0 {
		t.Fatalf("folders after delete = %+v, want none", folders)
	}
}

func TestCreateFolderRejectsBlankAndOverlong(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := contentgallery.NewService(contentgallery.NewRepository(pool), nil)

	if _, err := svc.CreateFolder(ctx, orgID, "   "); !isValidation(err) {
		t.Fatalf("blank name: err = %v, want validation", err)
	}

	overlong := ""
	for range 51 {
		overlong += "a"
	}
	if _, err := svc.CreateFolder(ctx, orgID, overlong); !isValidation(err) {
		t.Fatalf("51-char name: err = %v, want validation", err)
	}
}

// DeleteFolder must move files to root, never delete them (this package's
// own doc comment states the rule, matching the reference tracker).
func TestDeleteFolderMovesFilesToRoot(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	userID := seedUser(t, ctx, pool)
	storageClient := mustStorage(t)
	svc := contentgallery.NewService(contentgallery.NewRepository(pool), storageClient)

	folder, err := svc.CreateFolder(ctx, orgID, "Temp")
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	uploaded := mustUpload(t, ctx, svc, orgID, userID, &folder.ID)

	if err := svc.DeleteFolder(ctx, orgID, folder.ID); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}

	result, err := svc.ListFiles(ctx, orgID, contentgallery.ListFilesParams{})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(result.Files) != 1 || result.Files[0].ID != uploaded.ID {
		t.Fatalf("files after folder delete = %+v, want the one file surviving", result.Files)
	}
	if result.Files[0].FolderID != nil {
		t.Fatalf("FolderID = %v after folder delete, want nil (root)", result.Files[0].FolderID)
	}
}

func TestUploadListFilterPaginateArchiveRestoreMove(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	userID := seedUser(t, ctx, pool)
	storageClient := mustStorage(t)
	svc := contentgallery.NewService(contentgallery.NewRepository(pool), storageClient)

	folder, err := svc.CreateFolder(ctx, orgID, "Icons")
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}

	inFolder := mustUpload(t, ctx, svc, orgID, userID, &folder.ID)
	if inFolder.URL == "" {
		t.Fatalf("URL is empty, want a presigned download URL")
	}
	if inFolder.FileType != contentgallery.FileTypeImage {
		t.Fatalf("FileType = %q, want image", inFolder.FileType)
	}
	atRoot := mustUpload(t, ctx, svc, orgID, userID, nil)

	// Folder filter: "root only" (empty-string pointer) vs. a specific
	// folder vs. no filter at all — ListFilesParams.FolderID's tri-state.
	root := ""
	rootOnly, err := svc.ListFiles(ctx, orgID, contentgallery.ListFilesParams{FolderID: &root})
	if err != nil {
		t.Fatalf("ListFiles root only: %v", err)
	}
	if len(rootOnly.Files) != 1 || rootOnly.Files[0].ID != atRoot.ID {
		t.Fatalf("root-only files = %+v, want just %s", rootOnly.Files, atRoot.ID)
	}

	inFolderOnly, err := svc.ListFiles(ctx, orgID, contentgallery.ListFilesParams{FolderID: &folder.ID})
	if err != nil {
		t.Fatalf("ListFiles folder filter: %v", err)
	}
	if len(inFolderOnly.Files) != 1 || inFolderOnly.Files[0].ID != inFolder.ID {
		t.Fatalf("folder-filtered files = %+v, want just %s", inFolderOnly.Files, inFolder.ID)
	}

	all, err := svc.ListFiles(ctx, orgID, contentgallery.ListFilesParams{Limit: 1})
	if err != nil {
		t.Fatalf("ListFiles paginated: %v", err)
	}
	if all.Total != 2 || len(all.Files) != 1 {
		t.Fatalf("paginated result = %+v, want total 2, page of 1", all)
	}

	// Archive, then confirm it disappears from the default (active) list
	// and appears under an explicit archived filter — matching the
	// reference's own Active/Inactive toggle.
	if n, err := svc.ArchiveFiles(ctx, orgID, []string{atRoot.ID}); err != nil || n != 1 {
		t.Fatalf("ArchiveFiles: n=%d err=%v", n, err)
	}
	active, err := svc.ListFiles(ctx, orgID, contentgallery.ListFilesParams{})
	if err != nil {
		t.Fatalf("ListFiles active: %v", err)
	}
	if len(active.Files) != 1 || active.Files[0].ID != inFolder.ID {
		t.Fatalf("active files after archive = %+v, want just %s", active.Files, inFolder.ID)
	}
	archived, err := svc.ListFiles(ctx, orgID, contentgallery.ListFilesParams{Status: contentgallery.FileStatusArchived})
	if err != nil {
		t.Fatalf("ListFiles archived: %v", err)
	}
	if len(archived.Files) != 1 || archived.Files[0].ID != atRoot.ID {
		t.Fatalf("archived files = %+v, want just %s", archived.Files, atRoot.ID)
	}

	// Restore, then move into the folder.
	if n, err := svc.RestoreFiles(ctx, orgID, []string{atRoot.ID}); err != nil || n != 1 {
		t.Fatalf("RestoreFiles: n=%d err=%v", n, err)
	}
	if n, err := svc.MoveFiles(ctx, orgID, []string{atRoot.ID}, &folder.ID); err != nil || n != 1 {
		t.Fatalf("MoveFiles: n=%d err=%v", n, err)
	}
	inFolderOnly, err = svc.ListFiles(ctx, orgID, contentgallery.ListFilesParams{FolderID: &folder.ID})
	if err != nil {
		t.Fatalf("ListFiles after move: %v", err)
	}
	if len(inFolderOnly.Files) != 2 {
		t.Fatalf("folder files after move = %+v, want both files", inFolderOnly.Files)
	}
}

// Permanent deletion is gated to already-archived files — an id for an
// active file passed alongside an archived one must be silently skipped,
// not deleted, matching Repository.DeleteFiles' own WHERE-clause rule.
func TestDeleteFilesOnlyRemovesArchived(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	userID := seedUser(t, ctx, pool)
	storageClient := mustStorage(t)
	svc := contentgallery.NewService(contentgallery.NewRepository(pool), storageClient)

	active := mustUpload(t, ctx, svc, orgID, userID, nil)
	archived := mustUpload(t, ctx, svc, orgID, userID, nil)
	if n, err := svc.ArchiveFiles(ctx, orgID, []string{archived.ID}); err != nil || n != 1 {
		t.Fatalf("ArchiveFiles: n=%d err=%v", n, err)
	}

	n, err := svc.DeleteFiles(ctx, orgID, []string{active.ID, archived.ID})
	if err != nil {
		t.Fatalf("DeleteFiles: %v", err)
	}
	if n != 1 {
		t.Fatalf("DeleteFiles deleted %d files, want 1 (only the archived one)", n)
	}

	remaining, err := svc.ListFiles(ctx, orgID, contentgallery.ListFilesParams{})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(remaining.Files) != 1 || remaining.Files[0].ID != active.ID {
		t.Fatalf("active files after delete = %+v, want just %s untouched", remaining.Files, active.ID)
	}

	archivedList, err := svc.ListFiles(ctx, orgID, contentgallery.ListFilesParams{Status: contentgallery.FileStatusArchived})
	if err != nil {
		t.Fatalf("ListFiles archived: %v", err)
	}
	if len(archivedList.Files) != 0 {
		t.Fatalf("archived files after delete = %+v, want none (it was permanently deleted)", archivedList.Files)
	}
}

func TestUploadRejectsOversizeWrongContentAndMov(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	userID := seedUser(t, ctx, pool)
	storageClient := mustStorage(t)
	svc := contentgallery.NewService(contentgallery.NewRepository(pool), storageClient)

	// A .mp4 whose bytes are not an MP4 (the handler's sniff says text).
	_, err := svc.UploadFile(ctx, orgID, userID, contentgallery.UploadInput{
		Filename: "clip.mp4", ContentType: "text/plain; charset=utf-8", Size: 18, Reader: bytes.NewReader([]byte("not really a video")),
	})
	if !isValidation(err) {
		t.Fatalf("fake video upload: err = %v, want validation", err)
	}
	// MOV is refused with what to do instead.
	_, err = svc.UploadFile(ctx, orgID, userID, contentgallery.UploadInput{
		Filename: "IMG_0001.MOV", ContentType: "application/octet-stream", Size: 8, Reader: bytes.NewReader([]byte("whatever")),
	})
	if !isValidation(err) || !strings.Contains(err.Error(), "MP4") {
		t.Fatalf("mov upload: err = %v, want a validation error pointing at MP4", err)
	}

	_, err = svc.UploadFile(ctx, orgID, userID, contentgallery.UploadInput{
		Filename: "huge.png", ContentType: "image/png", Size: 21 << 20, Reader: bytes.NewReader(onePixelPNG),
	})
	if !isValidation(err) {
		t.Fatalf("oversize upload: err = %v, want validation", err)
	}

	// Extension says .png, sniffed bytes say something else entirely —
	// Service must trust the sniff (service.go's own allowedImageExt doc)
	// over the filename.
	_, err = svc.UploadFile(ctx, orgID, userID, contentgallery.UploadInput{
		Filename: "fake.png", ContentType: "text/plain; charset=utf-8", Size: 5, Reader: bytes.NewReader([]byte("hello")),
	})
	if !isValidation(err) {
		t.Fatalf("mismatched content: err = %v, want validation", err)
	}
}

func TestBulkOpsRejectMoreThan50Ids(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := contentgallery.NewService(contentgallery.NewRepository(pool), nil)

	ids := make([]string, 51)
	for i := range ids {
		ids[i] = idgen.New()
	}
	if _, err := svc.ArchiveFiles(ctx, orgID, ids); !isValidation(err) {
		t.Fatalf("51 ids: err = %v, want validation", err)
	}
	if _, err := svc.ArchiveFiles(ctx, orgID, nil); !isValidation(err) {
		t.Fatalf("0 ids: err = %v, want validation", err)
	}
}

// Cross-tenant isolation (CLAUDE.md §36-TENANCY, non-negotiable #5): org
// B must never see org A's files or folders, and can't move/archive them
// by id even if it somehow learned one.
func TestCrossOrgIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)
	userID := seedUser(t, ctx, pool)
	storageClient := mustStorage(t)
	svc := contentgallery.NewService(contentgallery.NewRepository(pool), storageClient)

	folderA, err := svc.CreateFolder(ctx, orgA, "A's folder")
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	fileA := mustUpload(t, ctx, svc, orgA, userID, nil)

	foldersB, err := svc.ListFolders(ctx, orgB)
	if err != nil {
		t.Fatalf("ListFolders (org B): %v", err)
	}
	if len(foldersB) != 0 {
		t.Fatalf("org B sees org A's folders: %+v", foldersB)
	}

	filesB, err := svc.ListFiles(ctx, orgB, contentgallery.ListFilesParams{})
	if err != nil {
		t.Fatalf("ListFiles (org B): %v", err)
	}
	if len(filesB.Files) != 0 {
		t.Fatalf("org B sees org A's files: %+v", filesB.Files)
	}

	if n, err := svc.ArchiveFiles(ctx, orgB, []string{fileA.ID}); err != nil || n != 0 {
		t.Fatalf("org B archived org A's file: n=%d err=%v", n, err)
	}

	if _, err := svc.RenameFolder(ctx, orgB, folderA.ID, "stolen"); !isNotFound(err) {
		t.Fatalf("org B renaming org A's folder: err = %v, want not_found", err)
	}
}

func isValidation(err error) bool {
	apiErr, ok := err.(*apierror.Error)
	return ok && apiErr.Code == "validation"
}

func isNotFound(err error) bool {
	apiErr, ok := err.(*apierror.Error)
	return ok && apiErr.Code == "not_found"
}

// minimalMP4 and minimalWebM are just enough of each format's header for
// http.DetectContentType — what the handler sniffs — to name it.
var (
	minimalMP4  = append([]byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'm', 'p', '4', '2', 0, 0, 0, 0, 'm', 'p', '4', '2', 'i', 's', 'o', 'm'}, make([]byte, 64)...)
	minimalWebM = append([]byte{0x1A, 0x45, 0xDF, 0xA3, 0x9F, 0x42, 0x86, 0x81, 0x01, 0x42, 0xF7, 0x81, 0x01, 0x42, 0xF2, 0x81, 0x04, 0x42, 0xF3, 0x81, 0x08, 0x42, 0x82, 0x84, 'w', 'e', 'b', 'm'}, make([]byte, 64)...)
)

func intp(v int) *int { return &v }

func objectExists(t *testing.T, url string) bool {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Videos: MP4 and WebM (as the handler sniffs them) are stored as video
// with their poster and display metadata; the type filter finds them;
// deleting removes the poster object too; bad metadata and a poster that
// is not an image are refused.
func TestUploadVideo(t *testing.T) {
	if got := http.DetectContentType(minimalMP4); got != "video/mp4" {
		t.Fatalf("sniffed MP4 = %q", got)
	}
	if got := http.DetectContentType(minimalWebM); got != "video/webm" {
		t.Fatalf("sniffed WebM = %q", got)
	}
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	userID := seedUser(t, ctx, pool)
	storageClient := mustStorage(t)
	svc := contentgallery.NewService(contentgallery.NewRepository(pool), storageClient)

	video, err := svc.UploadFile(ctx, orgID, userID, contentgallery.UploadInput{
		Filename: "promo.mp4", ContentType: "video/mp4", Size: int64(len(minimalMP4)), Reader: bytes.NewReader(minimalMP4),
		Poster: &contentgallery.PosterInput{ContentType: "image/png", Size: int64(len(onePixelPNG)), Reader: bytes.NewReader(onePixelPNG)},
		Meta:   contentgallery.VideoMeta{DurationMs: intp(15300), Width: intp(1080), Height: intp(1920)},
	})
	if err != nil {
		t.Fatalf("UploadFile mp4: %v", err)
	}
	if video.FileType != contentgallery.FileTypeVideo || video.MimeType != "video/mp4" || video.PosterURL == "" ||
		video.DurationMs == nil || *video.DurationMs != 15300 || *video.Width != 1080 || *video.Height != 1920 {
		t.Fatalf("video = %+v", video)
	}
	if !objectExists(t, video.URL) || !objectExists(t, video.PosterURL) {
		t.Fatal("the video or its poster is not in storage")
	}
	webm, err := svc.UploadFile(ctx, orgID, userID, contentgallery.UploadInput{
		Filename: "loop.webm", ContentType: "video/webm", Size: int64(len(minimalWebM)), Reader: bytes.NewReader(minimalWebM),
	})
	if err != nil || webm.FileType != contentgallery.FileTypeVideo || webm.PosterURL != "" || webm.DurationMs != nil {
		t.Fatalf("webm without a poster = %+v, %v", webm, err)
	}
	image := mustUpload(t, ctx, svc, orgID, userID, nil)

	videos, err := svc.ListFiles(ctx, orgID, contentgallery.ListFilesParams{FileType: contentgallery.FileTypeVideo})
	if err != nil || videos.Total != 2 {
		t.Fatalf("videos = %d, %v; want 2", videos.Total, err)
	}
	for _, f := range videos.Files {
		if f.ID == video.ID && (f.PosterURL != video.PosterURL || f.DurationMs == nil) {
			t.Fatalf("listed video = %+v, want its poster and duration", f)
		}
	}
	if images, _ := svc.ListFiles(ctx, orgID, contentgallery.ListFilesParams{FileType: contentgallery.FileTypeImage}); images.Total != 1 || images.Files[0].ID != image.ID {
		t.Fatalf("images = %+v, want the PNG only", images.Files)
	}

	for name, in := range map[string]contentgallery.UploadInput{
		"duration 0":       {Meta: contentgallery.VideoMeta{DurationMs: intp(0)}},
		"too wide":         {Meta: contentgallery.VideoMeta{Width: intp(20000)}},
		"poster not image": {Poster: &contentgallery.PosterInput{ContentType: "text/plain; charset=utf-8", Size: 5, Reader: bytes.NewReader([]byte("hello"))}},
		"poster too big":   {Poster: &contentgallery.PosterInput{ContentType: "image/png", Size: 3 << 20, Reader: bytes.NewReader(onePixelPNG)}},
	} {
		in.Filename, in.ContentType, in.Size, in.Reader = "x.mp4", "video/mp4", int64(len(minimalMP4)), bytes.NewReader(minimalMP4)
		if _, err := svc.UploadFile(ctx, orgID, userID, in); !isValidation(err) {
			t.Fatalf("%s: err = %v, want validation", name, err)
		}
	}

	if _, err := svc.ArchiveFiles(ctx, orgID, []string{video.ID}); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.DeleteFiles(ctx, orgID, []string{video.ID}); err != nil || n != 1 {
		t.Fatalf("DeleteFiles = %d, %v; want 1", n, err)
	}
	if objectExists(t, video.URL) || objectExists(t, video.PosterURL) {
		t.Fatal("the video or its poster is still in storage after delete")
	}
}
