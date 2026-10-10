package landing_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/landing"
	"github.com/ismagilovnail/flox/apps/internal/pagination"
	"github.com/ismagilovnail/flox/apps/internal/storage"
)

func TestExternalCreateGetUpdateDelete(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := landing.NewService(landing.NewRepository(pool), nil, nil)

	created, err := svc.Create(ctx, orgID, landing.CreateInput{
		Name: "Advertiser Lander", Title: "Spring Promo", Type: landing.TypeExternal, URL: "https://example.com/lnd",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Status != landing.StatusActive || created.Title != "Spring Promo" {
		t.Fatalf("Create returned %+v, want active with the given title", created)
	}

	newName, newURL := "Advertiser Lander v2", "https://example.com/lnd-v2"
	updated, err := svc.Update(ctx, orgID, created.ID, landing.UpdateInput{Name: &newName, URL: &newURL})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != newName || updated.URL != newURL {
		t.Fatalf("Update returned %+v", updated)
	}

	if err := svc.Delete(ctx, orgID, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, orgID, created.ID); err == nil {
		t.Fatal("Get after Delete succeeded, want not-found")
	}
}

func TestCreateCanSetPausedStatus(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := landing.NewService(landing.NewRepository(pool), nil, nil)

	created, err := svc.Create(ctx, orgID, landing.CreateInput{
		Name: "Starts Paused", Type: landing.TypeExternal, URL: "https://example.com/p", Status: landing.StatusPaused,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Status != landing.StatusPaused {
		t.Fatalf("Status = %q, want paused", created.Status)
	}
}

func TestCreateRejectsInvalidShapes(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := landing.NewService(landing.NewRepository(pool), nil, nil)

	cases := map[string]struct {
		in    landing.CreateInput
		field string
	}{
		"invalid url":       {landing.CreateInput{Name: "Bad External", Type: landing.TypeExternal, URL: "not-a-url"}, "url"},
		"short name":        {landing.CreateInput{Name: "X", Type: landing.TypeExternal, URL: "https://example.com"}, "name"},
		"internal via JSON": {landing.CreateInput{Name: "Internal", Type: landing.TypeInternal}, "archive"},
		"invalid logo":      {landing.CreateInput{Name: "Logo", Type: landing.TypeExternal, URL: "https://example.com", LogoURL: "nope"}, "logoUrl"},
		"archived status":   {landing.CreateInput{Name: "Status", Type: landing.TypeExternal, URL: "https://example.com", Status: landing.StatusArchived}, "status"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Create(ctx, orgID, tc.in)
			assertFieldError(t, err, tc.field)
		})
	}
}

func TestTypeCannotChange(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := landing.NewService(landing.NewRepository(pool), nil, nil)

	created, err := svc.Create(ctx, orgID, landing.CreateInput{Name: "Fixed Type", Type: landing.TypeExternal, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	internal := landing.TypeInternal
	_, err = svc.Update(ctx, orgID, created.ID, landing.UpdateInput{Type: &internal})
	assertFieldError(t, err, "type")

	same := landing.TypeExternal
	if _, err := svc.Update(ctx, orgID, created.ID, landing.UpdateInput{Type: &same}); err != nil {
		t.Fatalf("resending the current type was rejected: %v", err)
	}
}

func TestPauseActivateTransitions(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := landing.NewService(landing.NewRepository(pool), nil, nil)

	created, err := svc.Create(ctx, orgID, landing.CreateInput{Name: "Toggle Lander", Type: landing.TypeExternal, URL: "https://example.com/lnd"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	paused, err := svc.Pause(ctx, orgID, created.ID)
	if err != nil || paused.Status != landing.StatusPaused {
		t.Fatalf("Pause: got %+v, err %v", paused, err)
	}
	if again, err := svc.Pause(ctx, orgID, created.ID); err != nil || again.Status != landing.StatusPaused {
		t.Fatalf("Pause (idempotent): got %+v, err %v", again, err)
	}

	active, err := svc.Activate(ctx, orgID, created.ID)
	if err != nil || active.Status != landing.StatusActive {
		t.Fatalf("Activate: got %+v, err %v", active, err)
	}

	archived := landing.StatusArchived
	if _, err := svc.Update(ctx, orgID, created.ID, landing.UpdateInput{Status: &archived}); err != nil {
		t.Fatalf("archiving via Update: %v", err)
	}
	if _, err := svc.Pause(ctx, orgID, created.ID); err == nil {
		t.Fatal("Pause on an archived landing succeeded, want a conflict error")
	}
	if _, err := svc.Activate(ctx, orgID, created.ID); err == nil {
		t.Fatal("Activate on an archived landing succeeded, want a conflict error")
	}
}

func TestArchiveUploadWithoutStorageIsUnavailable(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := landing.NewService(landing.NewRepository(pool), nil, nil)

	_, err := svc.CreateFromArchive(ctx, orgID, archiveInput("No Storage", map[string]string{"index.html": "<p>hi</p>"}))
	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusServiceUnavailable {
		t.Fatalf("err = %v, want 503 Unavailable", err)
	}
}

// TestArchiveLifecycle covers the whole bundle contract against real
// Postgres + object storage: upload, assets publicly served under the
// derived URL, template edit, re-upload keeping the ID and dropping the
// old version's objects, duplicate getting its own copy, delete cleaning
// everything up.
func TestArchiveLifecycle(t *testing.T) {
	pool := mustPool(t)
	store := mustStorage(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := landing.NewService(landing.NewRepository(pool), store, nil)

	res, err := svc.CreateFromArchive(ctx, orgID, archiveInput("Spin Wheel", map[string]string{
		"spin/index.html":   `<html><head><link href="/abs.css"></head><body><a href="#">Go</a></body></html>`,
		"spin/css/main.css": "body{color:red}",
		"spin/img/logo.png": "\x89PNG fake",
		"__MACOSX/spin/._x": "junk",
		"spin/.DS_Store":    "junk",
	}))
	if err != nil {
		t.Fatalf("CreateFromArchive: %v", err)
	}
	l := res.Landing
	if l.Type != landing.TypeInternal || l.EntryFile != "index.html" || l.FileCount != 3 {
		t.Fatalf("created %+v, want internal/index.html/3 files (top folder stripped, junk skipped)", l)
	}
	if len(res.Warnings) != 1 || res.Warnings[0] != "/abs.css" {
		t.Fatalf("Warnings = %v, want [/abs.css]", res.Warnings)
	}
	if !strings.HasSuffix(l.URL, "/") {
		t.Fatalf("URL = %q, want a base URL ending in /", l.URL)
	}
	assertServed(t, l.URL+"css/main.css", "body{color:red}")
	firstURL := l.URL

	edited := "<html><head></head><body><h1>Edited</h1></body></html>"
	updated, err := svc.Update(ctx, orgID, l.ID, landing.UpdateInput{Content: &edited})
	if err != nil || updated.Content != edited {
		t.Fatalf("template edit: %+v, %v", updated, err)
	}

	preview, err := svc.Preview(ctx, orgID, l.ID, nil)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if !strings.Contains(preview, "<h1>Edited</h1>") || !strings.Contains(preview, `<base href="`+firstURL+`">`) {
		t.Fatalf("Preview didn't render the stored template with its base URL:\n%s", preview)
	}

	replaced, err := svc.ReplaceArchive(ctx, orgID, l.ID, archiveFor(map[string]string{
		"index.twig": `<a href="{{ trackUrl }}">Go</a>`,
		"new/app.js": "console.log(1)",
	}))
	if err != nil {
		t.Fatalf("ReplaceArchive: %v", err)
	}
	if replaced.Landing.ID != l.ID || replaced.Landing.EntryFile != "index.twig" || replaced.Landing.Content != `<a href="{{ trackUrl }}">Go</a>` {
		t.Fatalf("replaced %+v, want same ID with the new archive's entry file", replaced.Landing)
	}
	if replaced.Landing.URL == firstURL {
		t.Fatal("re-upload reused the old version's prefix")
	}
	assertServed(t, replaced.Landing.URL+"new/app.js", "console.log(1)")
	assertGone(t, firstURL+"css/main.css")

	dup, err := svc.Duplicate(ctx, orgID, l.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if dup.ID == l.ID || dup.URL == replaced.Landing.URL || dup.Content != replaced.Landing.Content {
		t.Fatalf("duplicate %+v doesn't have its own bundle copy", dup)
	}
	assertServed(t, dup.URL+"new/app.js", "console.log(1)")

	if err := svc.Delete(ctx, orgID, l.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	assertGone(t, replaced.Landing.URL+"new/app.js")
	assertServed(t, dup.URL+"new/app.js", "console.log(1)") // the copy is unaffected

	if err := svc.Delete(ctx, orgID, dup.ID); err != nil {
		t.Fatalf("Delete duplicate: %v", err)
	}
}

func TestArchiveRejectsBadArchives(t *testing.T) {
	pool := mustPool(t)
	store := mustStorage(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := landing.NewService(landing.NewRepository(pool), store, nil)

	_, err := svc.CreateFromArchive(ctx, orgID, landing.ArchiveCreateInput{
		Name: "Not A Zip", Title: "T", LogoURL: "https://example.com/logo.png",
		Archive: landing.Archive{ReaderAt: strings.NewReader("plain text"), Size: 10},
	})
	assertFieldError(t, err, "archive")

	_, err = svc.CreateFromArchive(ctx, orgID, archiveInput("No Index", map[string]string{"page.html": "<p>hi</p>"}))
	assertFieldError(t, err, "archive")

	in := archiveInput("No Logo", map[string]string{"index.html": "<p>hi</p>"})
	in.LogoURL = ""
	_, err = svc.CreateFromArchive(ctx, orgID, in)
	assertFieldError(t, err, "logoUrl")

	ext, err := svc.Create(ctx, orgID, landing.CreateInput{Name: "External", Type: landing.TypeExternal, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err = svc.ReplaceArchive(ctx, orgID, ext.ID, archiveFor(map[string]string{"index.html": "<p>hi</p>"}))
	assertFieldError(t, err, "archive")
}

func TestCrossTenantIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)
	svc := landing.NewService(landing.NewRepository(pool), nil, nil)

	created, err := svc.Create(ctx, orgA, landing.CreateInput{Name: "Org A Lander", Type: landing.TypeExternal, URL: "https://example.com/lnd"})
	if err != nil {
		t.Fatalf("creating for org A: %v", err)
	}

	t.Run("get", func(t *testing.T) {
		if _, err := svc.Get(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B fetched org A's landing, want not-found")
		}
	})
	t.Run("update", func(t *testing.T) {
		name := "hijacked"
		if _, err := svc.Update(ctx, orgB, created.ID, landing.UpdateInput{Name: &name}); err == nil {
			t.Fatal("org B updated org A's landing, want not-found")
		}
	})
	t.Run("archive", func(t *testing.T) {
		if _, err := svc.ReplaceArchive(ctx, orgB, created.ID, archiveFor(map[string]string{"index.html": "x"})); err == nil {
			t.Fatal("org B replaced org A's archive, want not-found")
		}
	})
	t.Run("preview", func(t *testing.T) {
		if _, err := svc.Preview(ctx, orgB, created.ID, nil); err == nil {
			t.Fatal("org B previewed org A's landing, want not-found")
		}
	})
	t.Run("duplicate", func(t *testing.T) {
		if _, err := svc.Duplicate(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B duplicated org A's landing, want not-found")
		}
	})
	t.Run("delete", func(t *testing.T) {
		if err := svc.Delete(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B deleted org A's landing, want not-found")
		}
	})
	t.Run("list", func(t *testing.T) {
		landings, _, err := svc.List(ctx, orgB, pagination.Page{Limit: pagination.MaxLimit})
		if err != nil {
			t.Fatalf("listing as org B: %v", err)
		}
		if len(landings) != 0 {
			t.Fatalf("org B saw %d of org A's landings, want 0", len(landings))
		}
	})
}

func archiveInput(name string, files map[string]string) landing.ArchiveCreateInput {
	return landing.ArchiveCreateInput{
		Name: name, Title: name + " title", LogoURL: "https://example.com/logo.png",
		Archive: archiveFor(files),
	}
}

func archiveFor(files map[string]string) landing.Archive {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(body))
	}
	_ = zw.Close()
	return landing.Archive{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

func assertFieldError(t *testing.T, err error, field string) {
	t.Helper()
	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want a validation error on %q", err, field)
	}
	if _, ok := apiErr.Fields[field]; !ok {
		t.Fatalf("validation fields = %v, want %q", apiErr.Fields, field)
	}
}

func assertServed(t *testing.T, url, want string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != want {
		t.Fatalf("GET %s = %d %q, want 200 %q", url, resp.StatusCode, body, want)
	}
}

func assertGone(t *testing.T, url string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET %s = %d, want 404 (objects should have been removed)", url, resp.StatusCode)
	}
}

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
