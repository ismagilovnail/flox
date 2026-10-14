package contentgallery

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// TestSystemCatalog: the shipped catalog loads, and every landing template
// zips into a bundle with index.html at its root.
func TestSystemCatalog(t *testing.T) {
	items, err := loadCatalog(systemFS)
	if err != nil {
		t.Fatalf("loadCatalog: %v", err)
	}
	kinds := map[SystemKind]int{}
	for _, it := range items {
		kinds[it.Kind]++
		if it.Kind != SystemLanding {
			continue
		}
		archive, err := systemBundle(it.Dir)
		if err != nil {
			t.Fatalf("%s: %v", it.ID, err)
		}
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			t.Fatalf("%s: %v", it.ID, err)
		}
		var names []string
		for _, f := range zr.File {
			names = append(names, f.Name)
		}
		if !strings.Contains(strings.Join(names, ","), "index.html") || strings.Contains(strings.Join(names, ","), "/index.html") {
			t.Fatalf("%s: bundle files %v, want index.html at the root", it.ID, names)
		}
	}
	if kinds[SystemCreative] == 0 || kinds[SystemLanding] == 0 || kinds[SystemPWA] == 0 {
		t.Fatalf("kinds = %v, want creatives, landings and PWAs", kinds)
	}
}

func TestSystemCatalogRejectsBrokenEntries(t *testing.T) {
	file := &fstest.MapFile{Data: []byte("png")}
	for name, catalog := range map[string]string{
		"unknown kind":  `{"items":[{"id":"a","kind":"video","name":{"en":"A","ru":"А"},"description":{"en":"d","ru":"о"},"file":"a.png"}]}`,
		"missing file":  `{"items":[{"id":"a","kind":"creative","name":{"en":"A","ru":"А"},"description":{"en":"d","ru":"о"},"file":"gone.png"}]}`,
		"no ru name":    `{"items":[{"id":"a","kind":"creative","name":{"en":"A"},"description":{"en":"d","ru":"о"},"file":"a.png"}]}`,
		"duplicate id":  `{"items":[{"id":"a","kind":"creative","name":{"en":"A","ru":"А"},"description":{"en":"d","ru":"о"},"file":"a.png"},{"id":"a","kind":"creative","name":{"en":"A","ru":"А"},"description":{"en":"d","ru":"о"},"file":"a.png"}]}`,
		"html asset":    `{"items":[{"id":"a","kind":"creative","name":{"en":"A","ru":"А"},"description":{"en":"d","ru":"о"},"file":"a.html"}]}`,
		"unknown field": `{"items":[{"id":"a","kind":"creative","name":{"en":"A","ru":"А"},"description":{"en":"d","ru":"о"},"file":"a.png","extra":1}]}`,
	} {
		fsys := fstest.MapFS{"system/catalog.json": {Data: []byte(catalog)}, "system/a.png": file, "system/a.html": file}
		if _, err := loadCatalog(fsys); err == nil {
			t.Errorf("%s: loadCatalog accepted it", name)
		}
	}
}

// TestServeSystemAsset: images and stylesheets only, never HTML, the
// catalog or a path outside system/.
func TestServeSystemAsset(t *testing.T) {
	for p, want := range map[string]int{
		"creatives/install-banner.png": http.StatusOK,
		"landings/cta/style.css":       http.StatusOK,
		"landings/cta/index.html":      http.StatusNotFound,
		"catalog.json":                 http.StatusNotFound,
		"../system.go":                 http.StatusNotFound,
		"creatives/missing.png":        http.StatusNotFound,
	} {
		rec := httptest.NewRecorder()
		ServeSystemAsset(rec, httptest.NewRequest(http.MethodGet, "/x", nil), p)
		if rec.Code != want {
			t.Errorf("%s = %d, want %d", p, rec.Code, want)
		}
		if want == http.StatusOK && rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: no nosniff header", p)
		}
	}
}

func TestSystemLandingPreview(t *testing.T) {
	svc := &Service{}
	html, err := svc.SystemLandingPreview("landing-cta", "http://api.test/v1/gallery-system/assets/landings/cta")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !strings.Contains(html, `<base href="http://api.test/v1/gallery-system/assets/landings/cta/">`) || strings.Contains(html, "{{") {
		t.Fatalf("preview html lacks the asset base or still has a macro:\n%.400s", html)
	}
	if _, err := svc.SystemLandingPreview("landing-cta", "javascript:alert(1)"); err == nil {
		t.Fatal("a non-http assetBase was accepted")
	}
	if _, err := svc.SystemLandingPreview("pwa-nova", "http://api.test/"); err == nil {
		t.Fatal("a PWA template previewed as a landing")
	}
}
