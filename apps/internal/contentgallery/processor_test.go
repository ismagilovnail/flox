package contentgallery_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/contentgallery"
	"github.com/ismagilovnail/flox/apps/internal/storage"
)

// fakeMedia stands in for ffmpeg: Probe answers info, ToMP4 and Poster
// write fixed bytes. beforeMP4, when set, runs first (a file deleted
// mid-way).
type fakeMedia struct {
	info      contentgallery.VideoInfo
	beforeMP4 func()
}

func (m *fakeMedia) Probe(context.Context, string) (contentgallery.VideoInfo, error) {
	return m.info, nil
}

func (m *fakeMedia) ToMP4(_ context.Context, _, dst string) error {
	if m.beforeMP4 != nil {
		m.beforeMP4()
	}
	return os.WriteFile(dst, minimalMP4, 0o600)
}

func (m *fakeMedia) Poster(_ context.Context, _, dst string, _ float64) error {
	return os.WriteFile(dst, onePixelPNG, 0o600)
}

func uploadMOV(t *testing.T, ctx context.Context, svc *contentgallery.Service, orgID, userID string) contentgallery.File {
	t.Helper()
	f, err := svc.UploadFile(ctx, orgID, userID, contentgallery.UploadInput{
		Filename: "IMG_0001.MOV", ContentType: "application/octet-stream",
		Size: int64(len(minimalMOV)), Reader: bytes.NewReader(minimalMOV), Head: minimalMOV,
	})
	if err != nil {
		t.Fatalf("UploadFile mov: %v", err)
	}
	return f
}

func getFile(t *testing.T, ctx context.Context, svc *contentgallery.Service, orgID, id string) contentgallery.File {
	t.Helper()
	res, err := svc.ListFiles(ctx, orgID, contentgallery.ListFilesParams{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Files {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("file %s not listed", id)
	return contentgallery.File{}
}

func processor(pool *pgxpool.Pool, st *storage.Client, orgID string, m contentgallery.Media) *contentgallery.Processor {
	return &contentgallery.Processor{
		Repo: contentgallery.NewRepository(pool), Storage: st, Media: m, OrgID: orgID,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), TempDir: os.TempDir(),
	}
}

// A MOV is stored as uploaded and processing — no link to the source —
// then the worker replaces it with the MP4 and a poster and removes the
// source; another organization's processor never takes it. The live dev
// worker polls every 5 s, so it could claim a test file first only in the
// instant between upload and RunOnce.
func TestTranscodeReplacesSourceWithMP4(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	otherOrg := seedOrg(t, ctx, pool)
	userID := seedUser(t, ctx, pool)
	st := mustStorage(t)
	svc := contentgallery.NewService(contentgallery.NewRepository(pool), st)

	src := uploadMOV(t, ctx, svc, orgID, userID)
	if src.ProcessingStatus != contentgallery.ProcessingInProgress || src.URL != "" || src.FileType != contentgallery.FileTypeVideo || src.MimeType != "video/quicktime" {
		t.Fatalf("uploaded mov = %+v, want a processing video without a URL", src)
	}
	media := &fakeMedia{info: contentgallery.VideoInfo{HasVideo: true, DurationMs: 4200, Width: 1080, Height: 1920}}

	if did, err := processor(pool, st, otherOrg, media).RunOnce(ctx); err != nil || did {
		t.Fatalf("other org's processor: did=%v err=%v, want nothing taken", did, err)
	}
	if did, err := processor(pool, st, orgID, media).RunOnce(ctx); err != nil || !did {
		t.Fatalf("RunOnce: did=%v err=%v", did, err)
	}
	f := getFile(t, ctx, svc, orgID, src.ID)
	if f.ProcessingStatus != contentgallery.ProcessingReady || f.MimeType != "video/mp4" || f.SizeBytes != int64(len(minimalMP4)) ||
		f.URL == "" || f.PosterURL == "" || f.DurationMs == nil || *f.DurationMs != 4200 || *f.Width != 1080 || *f.Height != 1920 {
		t.Fatalf("transcoded = %+v", f)
	}
	if !strings.Contains(f.URL, src.ID+".mp4") || !objectExists(t, f.URL) || !objectExists(t, f.PosterURL) {
		t.Fatal("the MP4 or its poster is not in storage")
	}
	if objectExists(t, st.PublicURL(orgID+"/"+src.ID+"-source.mov")) {
		t.Fatal("the source is still in storage")
	}
	if did, err := processor(pool, st, orgID, media).RunOnce(ctx); err != nil || did {
		t.Fatalf("second RunOnce: did=%v err=%v, want nothing left", did, err)
	}
}

// What the tools refuse is a failed file with the reason, not a retry; a
// file that took the worker down three times is given up on.
func TestTranscodeFailures(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	userID := seedUser(t, ctx, pool)
	st := mustStorage(t)
	svc := contentgallery.NewService(contentgallery.NewRepository(pool), st)

	for name, tc := range map[string]struct {
		info     contentgallery.VideoInfo
		attempts int
		want     string
	}{
		"no video stream": {info: contentgallery.VideoInfo{DurationMs: 3000}, want: "no video stream"},
		"too long":        {info: contentgallery.VideoInfo{HasVideo: true, DurationMs: 11 * 60 * 1000}, want: "longer than 10 minutes"},
		"crashed 3 times": {info: contentgallery.VideoInfo{HasVideo: true, DurationMs: 3000}, attempts: 3, want: "gave up after 3 attempts"},
	} {
		src := uploadMOV(t, ctx, svc, orgID, userID)
		if tc.attempts > 0 {
			if _, err := pool.Exec(ctx, `UPDATE content_gallery_files SET processing_attempts = $2 WHERE id = $1`, src.ID, tc.attempts); err != nil {
				t.Fatal(err)
			}
		}
		if did, err := processor(pool, st, orgID, &fakeMedia{info: tc.info}).RunOnce(ctx); err != nil || !did {
			t.Fatalf("%s: RunOnce did=%v err=%v", name, did, err)
		}
		f := getFile(t, ctx, svc, orgID, src.ID)
		if f.ProcessingStatus != contentgallery.ProcessingFailed || !strings.Contains(f.ProcessingError, tc.want) || f.URL != "" {
			t.Fatalf("%s: file = %+v, want failed with %q", name, f, tc.want)
		}
		// A failed file is deleted like any other, source included.
		if _, err := svc.ArchiveFiles(ctx, orgID, []string{src.ID}); err != nil {
			t.Fatal(err)
		}
		if n, err := svc.DeleteFiles(ctx, orgID, []string{src.ID}); err != nil || n != 1 {
			t.Fatalf("%s: DeleteFiles = %d, %v", name, n, err)
		}
		if objectExists(t, st.PublicURL(orgID+"/"+src.ID+"-source.mov")) {
			t.Fatalf("%s: the source is still in storage after delete", name)
		}
	}
}

// A file deleted while it was being transcoded keeps nothing: the MP4 and
// poster just made are removed instead of stored.
func TestTranscodeOfADeletedFileLeavesNothing(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	userID := seedUser(t, ctx, pool)
	st := mustStorage(t)
	svc := contentgallery.NewService(contentgallery.NewRepository(pool), st)

	src := uploadMOV(t, ctx, svc, orgID, userID)
	media := &fakeMedia{info: contentgallery.VideoInfo{HasVideo: true, DurationMs: 3000}, beforeMP4: func() {
		if _, err := svc.ArchiveFiles(ctx, orgID, []string{src.ID}); err != nil {
			t.Error(err)
		}
		if _, err := svc.DeleteFiles(ctx, orgID, []string{src.ID}); err != nil {
			t.Error(err)
		}
	}}
	if did, err := processor(pool, st, orgID, media).RunOnce(ctx); err != nil || !did {
		t.Fatalf("RunOnce: did=%v err=%v", did, err)
	}
	for _, key := range []string{src.ID + ".mp4", src.ID + "-poster.jpg", src.ID + "-source.mov"} {
		if objectExists(t, st.PublicURL(orgID+"/"+key)) {
			t.Fatalf("%s is in storage after the file was deleted", key)
		}
	}
}
