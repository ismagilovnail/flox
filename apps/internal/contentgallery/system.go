package contentgallery

import (
	"archive/zip"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/landing"
)

// System content (§30.8 "Distinguish system-provided gallery content from
// team-uploaded content"): a read-only catalog shipped inside the binary
// (system/catalog.json and its files), the same for every organization.
// Nothing of it is stored per tenant until a team uses an item — then it
// is copied into that team's own library or builder, and from there on it
// is ordinary team content. The images and templates are generic samples
// with invented names (system/README.md).

//go:embed system
var systemFS embed.FS

// SystemKind says which builder a system item goes to.
type SystemKind string

const (
	// SystemCreative is an image: "use" copies it into the team library.
	SystemCreative SystemKind = "creative"
	// SystemLanding is a landing bundle: "use" creates an internal landing.
	SystemLanding SystemKind = "landing"
	// SystemPWA is a PWA store page: "use" copies its images into the team
	// library and returns the PWA form's fields; nothing is saved until
	// the form is submitted.
	SystemPWA SystemKind = "pwa"
)

// SystemItem is one catalog entry, as system/catalog.json has it.
type SystemItem struct {
	ID          string            `json:"id"`
	Kind        SystemKind        `json:"kind"`
	Name        map[string]string `json:"name"`
	Description map[string]string `json:"description"`
	Tags        []string          `json:"tags"`
	// Preview is the card image; a creative's is its own file.
	Preview string `json:"preview,omitempty"`
	// File: creative. Dir, Logo, Title: landing. Icon, Screenshots, PWA: pwa.
	File        string          `json:"file,omitempty"`
	Dir         string          `json:"dir,omitempty"`
	Logo        string          `json:"logo,omitempty"`
	Title       string          `json:"title,omitempty"`
	Icon        string          `json:"icon,omitempty"`
	Screenshots []string        `json:"screenshots,omitempty"`
	PWA         json.RawMessage `json:"pwa,omitempty"`
}

// systemCatalog is parsed and checked once; a broken catalog fails
// TestSystemCatalog and every start, never a request.
var systemCatalog = mustLoadCatalog()

func mustLoadCatalog() []SystemItem {
	items, err := loadCatalog(systemFS)
	if err != nil {
		panic(err)
	}
	return items
}

func loadCatalog(fsys fs.FS) ([]SystemItem, error) {
	raw, err := fs.ReadFile(fsys, "system/catalog.json")
	if err != nil {
		return nil, fmt.Errorf("contentgallery: reading the system catalog: %w", err)
	}
	var doc struct {
		Items []SystemItem `json:"items"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("contentgallery: parsing the system catalog: %w", err)
	}
	seen := map[string]bool{}
	exists := func(id, p string) error {
		if _, err := fs.Stat(fsys, "system/"+p); err != nil {
			return fmt.Errorf("contentgallery: system item %s: %s: %w", id, p, err)
		}
		return nil
	}
	for i := range doc.Items {
		it := &doc.Items[i]
		if it.ID == "" || seen[it.ID] {
			return nil, fmt.Errorf("contentgallery: system item %d: missing or duplicate id %q", i, it.ID)
		}
		seen[it.ID] = true
		if it.Name["en"] == "" || it.Name["ru"] == "" || it.Description["en"] == "" || it.Description["ru"] == "" {
			return nil, fmt.Errorf("contentgallery: system item %s: name and description need en and ru", it.ID)
		}
		var files []string
		switch it.Kind {
		case SystemCreative:
			if it.Preview == "" {
				it.Preview = it.File
			}
			files = []string{it.File}
		case SystemLanding:
			if it.Title == "" {
				return nil, fmt.Errorf("contentgallery: system item %s: a landing needs a title", it.ID)
			}
			files = []string{it.Dir + "/index.html", it.Logo}
		case SystemPWA:
			if len(it.PWA) == 0 {
				return nil, fmt.Errorf("contentgallery: system item %s: a pwa needs its fields", it.ID)
			}
			files = append([]string{it.Icon}, it.Screenshots...)
		default:
			return nil, fmt.Errorf("contentgallery: system item %s: unknown kind %q", it.ID, it.Kind)
		}
		for _, p := range append(files, it.Preview) {
			if p == "" || !systemAsset(p) && !strings.HasSuffix(p, "/index.html") {
				return nil, fmt.Errorf("contentgallery: system item %s: %q is not a servable asset", it.ID, p)
			}
			if err := exists(it.ID, p); err != nil {
				return nil, err
			}
		}
	}
	return doc.Items, nil
}

// systemAssetTypes are the only system files served over HTTP — images
// and stylesheets. HTML is never served from the API's origin: a landing
// preview renders into a sandboxed iframe from JSON (SystemLandingPreview).
var systemAssetTypes = map[string]string{
	".png": "image/png",
	".css": "text/css; charset=utf-8",
}

func systemAsset(p string) bool {
	_, ok := systemAssetTypes[path.Ext(p)]
	return ok
}

// SystemAssetBasePath is where the API serves system assets (ServeSystemAsset),
// relative to its own /v1 root; the web app prefixes its API URL.
const SystemAssetBasePath = "/v1/gallery-system/assets/"

func systemItem(id string) (SystemItem, error) {
	for _, it := range systemCatalog {
		if it.ID == id {
			return it, nil
		}
	}
	return SystemItem{}, apierror.NotFound("system gallery item")
}

// SystemItems is the whole catalog, in its own order.
func (s *Service) SystemItems() []SystemItem { return systemCatalog }

// LandingCreator creates an internal landing from a ZIP — *landing.Service.
type LandingCreator interface {
	CreateFromArchive(ctx context.Context, orgID string, in landing.ArchiveCreateInput) (landing.UploadResult, error)
}

// WithLandings lets "use" on a landing template create the landing.
func (s *Service) WithLandings(l LandingCreator) *Service {
	s.landings = l
	return s
}

// SystemUseInput is what "use" may override: the new landing's name.
type SystemUseInput struct {
	Name string
}

// SystemUseResult is the hand-off: exactly one field is set, by kind.
type SystemUseResult struct {
	Kind    SystemKind       `json:"kind"`
	File    *File            `json:"file,omitempty"`
	Landing *landing.Landing `json:"landing,omitempty"`
	PWA     map[string]any   `json:"pwa,omitempty"`
}

// UseSystemItem hands a system item to the team: a creative is copied
// into the library; a landing template becomes an internal landing (its
// logo copied into the library for the landing's logo URL); a PWA
// template's icon and screenshots are copied into the library and its
// fields returned with their URLs, for the PWA form to open prefilled.
func (s *Service) UseSystemItem(ctx context.Context, orgID, userID, id string, in SystemUseInput) (SystemUseResult, error) {
	it, err := systemItem(id)
	if err != nil {
		return SystemUseResult{}, err
	}
	switch it.Kind {
	case SystemCreative:
		f, err := s.copySystemImage(ctx, orgID, userID, it.File)
		if err != nil {
			return SystemUseResult{}, err
		}
		return SystemUseResult{Kind: it.Kind, File: &f}, nil

	case SystemLanding:
		if s.landings == nil {
			return SystemUseResult{}, apierror.Unavailable("landings are unavailable")
		}
		archive, err := systemBundle(it.Dir)
		if err != nil {
			return SystemUseResult{}, err
		}
		logo, err := s.copySystemImage(ctx, orgID, userID, it.Logo)
		if err != nil {
			return SystemUseResult{}, err
		}
		name := strings.TrimSpace(in.Name)
		if name == "" {
			name = it.Name["en"]
		}
		res, err := s.landings.CreateFromArchive(ctx, orgID, landing.ArchiveCreateInput{
			Name: name, Title: it.Title, LogoURL: logo.URL,
			Archive: landing.Archive{ReaderAt: bytes.NewReader(archive), Size: int64(len(archive))},
		})
		if err != nil {
			return SystemUseResult{}, err
		}
		return SystemUseResult{Kind: it.Kind, Landing: &res.Landing}, nil

	case SystemPWA:
		fields := map[string]any{}
		if err := json.Unmarshal(it.PWA, &fields); err != nil {
			return SystemUseResult{}, fmt.Errorf("contentgallery: system item %s: %w", it.ID, err)
		}
		icon, err := s.copySystemImage(ctx, orgID, userID, it.Icon)
		if err != nil {
			return SystemUseResult{}, err
		}
		fields["iconUrl"] = icon.URL
		shots := make([]string, 0, len(it.Screenshots))
		for _, p := range it.Screenshots {
			f, err := s.copySystemImage(ctx, orgID, userID, p)
			if err != nil {
				return SystemUseResult{}, err
			}
			shots = append(shots, f.URL)
		}
		fields["screenshots"] = shots
		return SystemUseResult{Kind: it.Kind, PWA: fields}, nil
	}
	return SystemUseResult{}, fmt.Errorf("contentgallery: system item %s: unknown kind %q", it.ID, it.Kind)
}

// copySystemImage uploads one system image into the team library through
// UploadFile, so it gets every check and the public URL a team upload gets.
func (s *Service) copySystemImage(ctx context.Context, orgID, userID, p string) (File, error) {
	data, err := systemFS.ReadFile("system/" + p)
	if err != nil {
		return File{}, fmt.Errorf("contentgallery: reading system file %s: %w", p, err)
	}
	return s.UploadFile(ctx, orgID, userID, UploadInput{
		Filename:    path.Base(p),
		ContentType: http.DetectContentType(data),
		Size:        int64(len(data)),
		Reader:      bytes.NewReader(data),
	})
}

// systemBundle zips one landing template's directory, its files at the
// archive root — what the landing upload expects.
func systemBundle(dir string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	root := "system/" + dir
	err := fs.WalkDir(systemFS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := systemFS.ReadFile(p)
		if err != nil {
			return err
		}
		w, err := zw.Create(strings.TrimPrefix(p, root+"/"))
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
	if err == nil {
		err = zw.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("contentgallery: zipping system landing %s: %w", dir, err)
	}
	return buf.Bytes(), nil
}

// SystemLandingPreview renders a landing template the way the landing
// editor previews one (landing.Render, links disabled), its assets loaded
// from assetBase — the public system asset URL of the template's
// directory, which only the browser knows.
func (s *Service) SystemLandingPreview(id, assetBase string) (string, error) {
	it, err := systemItem(id)
	if err != nil {
		return "", err
	}
	if it.Kind != SystemLanding {
		return "", apierror.NotFound("landing template")
	}
	if !strings.HasPrefix(assetBase, "http://") && !strings.HasPrefix(assetBase, "https://") {
		return "", apierror.Validation("assetBase must be an absolute http(s) URL", nil)
	}
	src, err := systemFS.ReadFile("system/" + it.Dir + "/index.html")
	if err != nil {
		return "", fmt.Errorf("contentgallery: reading system landing %s: %w", it.ID, err)
	}
	return landing.Render(string(src), strings.TrimSuffix(assetBase, "/")+"/", "javascript:void(0)"), nil
}

// ServeSystemAsset serves a system image or stylesheet by its path under
// system/. Public and cacheable: the same bytes for everyone, no tenant
// data. Anything else is a 404.
func ServeSystemAsset(w http.ResponseWriter, r *http.Request, p string) {
	p = path.Clean("/" + p)[1:]
	ct, ok := systemAssetTypes[path.Ext(p)]
	if !ok || p == "" {
		http.NotFound(w, r)
		return
	}
	data, err := systemFS.ReadFile("system/" + p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}
