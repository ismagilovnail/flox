package landing

import (
	"archive/zip"
	"bytes"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

func zipOf(t *testing.T, files ...string) (*bytes.Reader, int64) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i := 0; i+1 < len(files); i += 2 {
		w, err := zw.Create(files[i])
		if err != nil {
			t.Fatalf("zip.Create(%q): %v", files[i], err)
		}
		_, _ = w.Write([]byte(files[i+1]))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(buf.Bytes()), int64(buf.Len())
}

func TestParseBundleLayouts(t *testing.T) {
	cases := []struct {
		name      string
		files     []string
		entry     string
		assets    []string
		wantError string
	}{
		{
			name:   "entry at root",
			files:  []string{"index.html", "<p>x</p>", "css/main.css", "a{}", "img/bg.jpg", "jpg"},
			entry:  "index.html",
			assets: []string{"css/main.css", "img/bg.jpg"},
		},
		{
			name:   "twig entry",
			files:  []string{"index.twig", "<p>{{ trackUrl }}</p>"},
			entry:  "index.twig",
			assets: nil,
		},
		{
			name:   "html preferred over twig",
			files:  []string{"index.twig", "t", "index.html", "h"},
			entry:  "index.html",
			assets: []string{"index.twig"},
		},
		{
			name:   "single top-level folder stripped",
			files:  []string{"lander/index.html", "<p>x</p>", "lander/js/app.js", "1", "__MACOSX/lander/._index.html", "junk", "lander/.DS_Store", "junk"},
			entry:  "index.html",
			assets: []string{"js/app.js"},
		},
		{
			name:      "no entry file",
			files:     []string{"page.html", "<p>x</p>"},
			wantError: "index.html or index.twig",
		},
		{
			name:      "entry nested two levels deep",
			files:     []string{"a/b/index.html", "<p>x</p>"},
			wantError: "index.html or index.twig",
		},
		{
			name:      "two top-level folders",
			files:     []string{"a/index.html", "<p>x</p>", "b/x.css", "a{}"},
			wantError: "index.html or index.twig",
		},
		{
			name:      "zip slip",
			files:     []string{"index.html", "<p>x</p>", "../../etc/evil", "x"},
			wantError: "unsafe path",
		},
		{
			name:      "absolute path",
			files:     []string{"index.html", "<p>x</p>", "/etc/evil", "x"},
			wantError: "unsafe path",
		},
		{
			name:      "empty entry",
			files:     []string{"index.html", "   "},
			wantError: "entry file is empty",
		},
		{
			name:      "non-utf8 entry",
			files:     []string{"index.html", "\xff\xfe\x00"},
			wantError: "UTF-8",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, size := zipOf(t, tc.files...)
			b, err := parseBundle(r, size)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(errorText(err), tc.wantError) {
					t.Fatalf("err = %v, want one mentioning %q", errorText(err), tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseBundle: %v", errorText(err))
			}
			if b.EntryFile != tc.entry {
				t.Fatalf("EntryFile = %q, want %q", b.EntryFile, tc.entry)
			}
			got := []string{}
			for _, a := range b.Assets {
				got = append(got, a.Path)
			}
			if strings.Join(got, ",") != strings.Join(tc.assets, ",") {
				t.Fatalf("assets = %v, want %v", got, tc.assets)
			}
			if b.FileCount != len(tc.assets)+1 {
				t.Fatalf("FileCount = %d, want %d", b.FileCount, len(tc.assets)+1)
			}
		})
	}
}

func TestParseBundleRejectsNonZipAndLimits(t *testing.T) {
	if _, err := parseBundle(strings.NewReader("not a zip at all"), 16); err == nil {
		t.Fatal("parsed plain text as a ZIP")
	}
	if _, err := parseBundle(strings.NewReader(""), 0); err == nil {
		t.Fatal("accepted an empty upload")
	}
	r, _ := zipOf(t, "index.html", "x")
	if _, err := parseBundle(r, maxArchiveBytes+1); err == nil {
		t.Fatal("accepted an archive over the size limit")
	}

	files := []string{"index.html", "<p>x</p>"}
	for i := 0; i <= maxBundleFiles; i++ {
		files = append(files, "f/"+strconv.Itoa(i)+".txt", "x")
	}
	r, size := zipOf(t, files...)
	if _, err := parseBundle(r, size); err == nil || !strings.Contains(errorText(err), "more than") {
		t.Fatalf("err = %v, want the file-count limit", errorText(err))
	}
}

func TestContentTypes(t *testing.T) {
	for name, want := range map[string]string{
		"css/main.css":      "text/css; charset=utf-8",
		"fonts/x.woff2":     "font/woff2",
		"video/promo.mp4":   "video/mp4",
		"sfx/spin.MP3":      "audio/mpeg",
		"unknown.zzz-thing": "application/octet-stream",
	} {
		if got := contentTypeFor(name); got != want {
			t.Errorf("contentTypeFor(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestAbsoluteAssetRefs(t *testing.T) {
	src := `<link href="/style.css"><script src='/js/app.js'></script>
<img src="img/ok.png"><script src="//cdn.example.com/x.js"></script>
<a href="https://example.com/">x</a><div style="background:url('/bg.jpg')"></div>
<link href="/style.css">`
	got := absoluteAssetRefs(src)
	want := []string{"/style.css", "/js/app.js", "/bg.jpg"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("absoluteAssetRefs = %v, want %v", got, want)
	}
}

func errorText(err error) string {
	if err == nil {
		return "<nil>"
	}
	s := err.Error()
	var apiErr *apierror.Error
	if errors.As(err, &apiErr) {
		for _, v := range apiErr.Fields {
			s += " " + v
		}
	}
	return s
}
