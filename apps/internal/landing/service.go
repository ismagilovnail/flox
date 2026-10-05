package landing

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/storage"
)

const (
	nameMinLen  = 2
	nameMaxLen  = 100
	titleMaxLen = 200
)

type Service struct {
	repo *Repository
	// storage may be nil (object storage failed to initialize at
	// startup) — external landings and every read keep working; archive
	// uploads and duplicating a bundle answer Unavailable, same stance as
	// contentgallery.Service.
	storage *storage.Client
	logger  *slog.Logger
}

func NewService(repo *Repository, storageClient *storage.Client, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: repo, storage: storageClient, logger: logger}
}

func (s *Service) List(ctx context.Context, orgID string) ([]Landing, error) {
	landings, err := s.repo.List(ctx, orgID)
	if err != nil {
		return nil, err
	}
	for i := range landings {
		s.decorate(&landings[i])
	}
	return landings, nil
}

func (s *Service) Get(ctx context.Context, orgID, id string) (Landing, error) {
	l, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Landing{}, err
	}
	s.decorate(&l)
	return l, nil
}

// decorate derives an internal landing's URL — the public base its
// assets are served from — from BundlePrefix at read time.
func (s *Service) decorate(l *Landing) {
	if l.Type != TypeInternal {
		return
	}
	l.URL = ""
	if l.BundlePrefix != "" && s.storage != nil {
		l.URL = s.storage.PublicURL(l.BundlePrefix + "/")
	}
}

// Create is the JSON path, external landings only: an internal landing
// is a ZIP bundle and is created through CreateFromArchive.
func (s *Service) Create(ctx context.Context, orgID string, in CreateInput) (Landing, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Title = strings.TrimSpace(in.Title)
	in.LogoURL = strings.TrimSpace(in.LogoURL)
	fields := map[string]string{}

	validateName(in.Name, fields)
	validateTitle(in.Title, false, fields)
	validateLogo(in.LogoURL, false, fields)
	validateCreateStatus(in.Status, fields)
	switch {
	case !in.Type.Valid():
		fields["type"] = "must be internal or external"
	case in.Type == TypeInternal:
		fields["archive"] = "internal landings are created by uploading a ZIP archive"
	case !isValidURL(in.URL):
		fields["url"] = "must be a valid absolute URL"
	}
	if len(fields) > 0 {
		return Landing{}, apierror.Validation("invalid landing", fields)
	}

	return s.repo.Create(ctx, Landing{
		ID: idgen.New(), OrganizationID: orgID, Name: in.Name, Title: in.Title, LogoURL: in.LogoURL,
		Type: TypeExternal, URL: in.URL, Status: in.Status,
	})
}

// CreateFromArchive creates an internal landing from an uploaded ZIP:
// the entry file goes to Postgres, every other file to object storage
// under a fresh "landings/{org}/{id}/{version}" prefix.
func (s *Service) CreateFromArchive(ctx context.Context, orgID string, in ArchiveCreateInput) (UploadResult, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Title = strings.TrimSpace(in.Title)
	in.LogoURL = strings.TrimSpace(in.LogoURL)
	fields := map[string]string{}
	validateName(in.Name, fields)
	validateTitle(in.Title, true, fields)
	validateLogo(in.LogoURL, true, fields)
	validateCreateStatus(in.Status, fields)
	if len(fields) > 0 {
		return UploadResult{}, apierror.Validation("invalid landing", fields)
	}
	if s.storage == nil {
		return UploadResult{}, apierror.Unavailable("object storage is not configured; archive uploads are unavailable")
	}

	b, err := parseBundle(in.Archive.ReaderAt, in.Archive.Size)
	if err != nil {
		return UploadResult{}, err
	}

	id := idgen.New()
	prefix := bundlePrefix(orgID, id)
	if err := s.uploadAssets(ctx, prefix, b); err != nil {
		return UploadResult{}, err
	}

	l, err := s.repo.Create(ctx, Landing{
		ID: id, OrganizationID: orgID, Name: in.Name, Title: in.Title, LogoURL: in.LogoURL,
		Type: TypeInternal, Content: b.EntryHTML, EntryFile: b.EntryFile,
		BundlePrefix: prefix, FileCount: b.FileCount, BundleSize: b.TotalSize, Status: in.Status,
	})
	if err != nil {
		s.cleanup(prefix)
		return UploadResult{}, err
	}
	s.decorate(&l)
	return UploadResult{Landing: l, Warnings: b.Warnings}, nil
}

// ReplaceArchive re-uploads an internal landing's bundle in place: same
// ID (flows keep pointing at it), new content. The new version is fully
// uploaded under its own prefix before the row is switched over, so a
// failed upload never leaves a half-replaced landing; the old version's
// objects are removed afterwards. Template edits made since the last
// upload are replaced by the archive's entry file — the archive is the
// new source of truth.
func (s *Service) ReplaceArchive(ctx context.Context, orgID, id string, archive Archive) (UploadResult, error) {
	current, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return UploadResult{}, err
	}
	if current.Type != TypeInternal {
		return UploadResult{}, apierror.Validation("invalid landing", map[string]string{"archive": "only internal landings have an archive"})
	}
	if s.storage == nil {
		return UploadResult{}, apierror.Unavailable("object storage is not configured; archive uploads are unavailable")
	}

	b, err := parseBundle(archive.ReaderAt, archive.Size)
	if err != nil {
		return UploadResult{}, err
	}
	prefix := bundlePrefix(orgID, id)
	if err := s.uploadAssets(ctx, prefix, b); err != nil {
		return UploadResult{}, err
	}
	l, err := s.repo.SetBundle(ctx, orgID, id, b, prefix)
	if err != nil {
		s.cleanup(prefix)
		return UploadResult{}, err
	}
	if current.BundlePrefix != "" {
		s.cleanup(current.BundlePrefix)
	}
	s.decorate(&l)
	return UploadResult{Landing: l, Warnings: b.Warnings}, nil
}

func (s *Service) Update(ctx context.Context, orgID, id string, in UpdateInput) (Landing, error) {
	current, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Landing{}, err
	}
	internal := current.Type == TypeInternal

	fields := map[string]string{}
	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		in.Name = &trimmed
		validateName(trimmed, fields)
	}
	if in.Title != nil {
		trimmed := strings.TrimSpace(*in.Title)
		in.Title = &trimmed
		validateTitle(trimmed, internal, fields)
	}
	if in.LogoURL != nil {
		trimmed := strings.TrimSpace(*in.LogoURL)
		in.LogoURL = &trimmed
		validateLogo(trimmed, internal, fields)
	}
	// Type is fixed at creation: switching would mean conjuring or
	// discarding a whole archive through a metadata PATCH.
	if in.Type != nil && *in.Type != current.Type {
		fields["type"] = "cannot be changed after creation"
	}
	if in.Status != nil && !in.Status.Valid() {
		fields["status"] = "must be one of active, paused, archived"
	}
	if internal {
		in.URL = nil // derived, never stored
		if in.Content != nil {
			validateContent(*in.Content, fields)
		}
	} else {
		in.Content = nil // an external page has no hosted source
		if in.URL != nil && !isValidURL(*in.URL) {
			fields["url"] = "must be a valid absolute URL"
		}
	}
	if len(fields) > 0 {
		return Landing{}, apierror.Validation("invalid landing", fields)
	}
	in.Type = nil

	l, err := s.repo.Update(ctx, orgID, id, in)
	if err != nil {
		return Landing{}, err
	}
	s.decorate(&l)
	return l, nil
}

// Delete removes the row, then (best-effort) every bundle version the
// landing ever had.
func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	if err := s.repo.Delete(ctx, orgID, id); err != nil {
		return err
	}
	s.cleanup(landingPrefix(orgID, id))
	return nil
}

// Duplicate copies the row and, for a bundle, its assets (server-side) to
// a prefix of its own — so re-uploading either landing later can never
// change the other. Status is preserved, same reasoning as
// network.Service.Duplicate: no draft-equivalent status exists.
func (s *Service) Duplicate(ctx context.Context, orgID, id string) (Landing, error) {
	source, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Landing{}, err
	}

	copyName := source.Name + " (Copy)"
	if len(copyName) > nameMaxLen {
		copyName = source.Name
	}
	dup := source
	dup.ID = idgen.New()
	dup.Name = copyName
	dup.BundlePrefix = ""

	if source.BundlePrefix != "" {
		if s.storage == nil {
			return Landing{}, apierror.Unavailable("object storage is not configured; this landing's files can't be copied")
		}
		dup.BundlePrefix = bundlePrefix(orgID, dup.ID)
		if err := s.storage.CopyPrefix(ctx, source.BundlePrefix+"/", dup.BundlePrefix+"/"); err != nil {
			s.cleanup(dup.BundlePrefix)
			return Landing{}, fmt.Errorf("landing: copying bundle: %w", err)
		}
	}

	created, err := s.repo.Create(ctx, dup)
	if err != nil {
		if dup.BundlePrefix != "" {
			s.cleanup(dup.BundlePrefix)
		}
		return Landing{}, err
	}
	s.decorate(&created)
	return created, nil
}

// Preview renders content (or, when nil, the stored entry file) exactly
// as a visitor would get it, with trackUrl pointed back into the preview
// frame. Read-only: nothing is saved. Internal landings only.
func (s *Service) Preview(ctx context.Context, orgID, id string, content *string) (string, error) {
	l, err := s.Get(ctx, orgID, id)
	if err != nil {
		return "", err
	}
	if l.Type != TypeInternal {
		return "", apierror.Validation("invalid landing", map[string]string{"type": "only internal landings have a template"})
	}
	source := l.Content
	if content != nil {
		if len(*content) > maxEntryBytes {
			return "", apierror.Validation("invalid landing", map[string]string{"content": fmt.Sprintf("must be at most %dMB", maxEntryBytes>>20)})
		}
		source = *content
	}
	return Render(source, l.URL, PreviewTrackURL), nil
}

func (s *Service) Pause(ctx context.Context, orgID, id string) (Landing, error) {
	l, err := s.Get(ctx, orgID, id)
	if err != nil {
		return Landing{}, err
	}
	switch l.Status {
	case StatusPaused:
		return l, nil
	case StatusActive:
		status := StatusPaused
		return s.Update(ctx, orgID, id, UpdateInput{Status: &status})
	default:
		return Landing{}, apierror.Conflict(fmt.Sprintf("cannot pause a landing with status %q", l.Status))
	}
}

func (s *Service) Activate(ctx context.Context, orgID, id string) (Landing, error) {
	l, err := s.Get(ctx, orgID, id)
	if err != nil {
		return Landing{}, err
	}
	switch l.Status {
	case StatusActive:
		return l, nil
	case StatusPaused:
		status := StatusActive
		return s.Update(ctx, orgID, id, UpdateInput{Status: &status})
	default:
		return Landing{}, apierror.Conflict(fmt.Sprintf("cannot activate a landing with status %q", l.Status))
	}
}

// uploadAssets streams every non-entry file into storage under prefix.
// On failure, whatever was already uploaded is removed.
func (s *Service) uploadAssets(ctx context.Context, prefix string, b *bundle) error {
	for _, a := range b.Assets {
		rc, err := a.Open()
		if err != nil {
			s.cleanup(prefix)
			return archiveError(fmt.Sprintf("could not read %q from the archive", a.Path))
		}
		err = s.storage.PutObject(ctx, prefix+"/"+a.Path, rc, a.Size(), a.ContentType)
		rc.Close()
		if err != nil {
			s.cleanup(prefix)
			return fmt.Errorf("landing: uploading bundle: %w", err)
		}
	}
	return nil
}

// cleanup removes objects under prefix, best-effort: an orphaned object
// costs storage, not correctness. Runs on a fresh context so a cancelled
// request still cleans up after itself.
func (s *Service) cleanup(prefix string) {
	if s.storage == nil || prefix == "" {
		return
	}
	if err := s.storage.DeletePrefix(context.Background(), strings.TrimSuffix(prefix, "/")+"/"); err != nil {
		s.logger.Warn("landing: bundle cleanup failed", "prefix", prefix, "error", err)
	}
}

// landingPrefix holds every bundle version a landing ever had;
// bundlePrefix is one fresh version under it.
func landingPrefix(orgID, id string) string { return "landings/" + orgID + "/" + id }

func bundlePrefix(orgID, id string) string { return landingPrefix(orgID, id) + "/" + idgen.New() }

func validateName(name string, fields map[string]string) {
	if len(name) < nameMinLen || len(name) > nameMaxLen {
		fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
	}
}

func validateTitle(title string, required bool, fields map[string]string) {
	switch {
	case required && title == "":
		fields["title"] = "add a title"
	case len(title) > titleMaxLen:
		fields["title"] = fmt.Sprintf("must be at most %d characters", titleMaxLen)
	}
}

func validateLogo(logoURL string, required bool, fields map[string]string) {
	switch {
	case logoURL == "" && required:
		fields["logoUrl"] = "add a logo"
	case logoURL != "" && !isValidURL(logoURL):
		fields["logoUrl"] = "must be a valid absolute URL"
	}
}

func validateCreateStatus(status Status, fields map[string]string) {
	if status != "" && status != StatusActive && status != StatusPaused {
		fields["status"] = "must be active or paused"
	}
}

func validateContent(content string, fields map[string]string) {
	switch {
	case strings.TrimSpace(content) == "":
		fields["content"] = "add page content"
	case len(content) > maxEntryBytes:
		fields["content"] = fmt.Sprintf("must be at most %dMB", maxEntryBytes>>20)
	case !utf8.ValidString(content):
		fields["content"] = "must be UTF-8 text"
	}
}

func isValidURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
