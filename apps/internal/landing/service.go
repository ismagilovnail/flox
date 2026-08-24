package landing

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

const (
	nameMinLen = 2
	nameMaxLen = 100
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, orgID string) ([]Landing, error) {
	return s.repo.List(ctx, orgID)
}

func (s *Service) Get(ctx context.Context, orgID, id string) (Landing, error) {
	return s.repo.GetByID(ctx, orgID, id)
}

// Create validates the type-specific shape and, for TypeInternal, computes
// URL server-side from Name (§28: internal landings are hosted on our CDN)
// — a client-supplied URL for that type is never trusted, matching how
// every other stage in this domain keeps its resolved/derived values on
// the server, not the browser.
func (s *Service) Create(ctx context.Context, orgID string, in CreateInput) (Landing, error) {
	in.Name = strings.TrimSpace(in.Name)
	fields := map[string]string{}

	if len(in.Name) < nameMinLen || len(in.Name) > nameMaxLen {
		fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
	}
	if !in.Type.Valid() {
		fields["type"] = "must be internal or external"
	} else if in.Type == TypeExternal {
		if !isValidURL(in.URL) {
			fields["url"] = "must be a valid absolute URL"
		}
		in.Content = ""
	} else {
		if strings.TrimSpace(in.Content) == "" {
			fields["content"] = "add page content"
		}
		in.URL = cdnURL(in.Name)
	}

	if len(fields) > 0 {
		return Landing{}, apierror.Validation("invalid landing", fields)
	}
	return s.repo.Create(ctx, idgen.New(), orgID, in)
}

func (s *Service) Update(ctx context.Context, orgID, id string, in UpdateInput) (Landing, error) {
	current, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Landing{}, err
	}

	fields := map[string]string{}

	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		in.Name = &trimmed
		if len(trimmed) < nameMinLen || len(trimmed) > nameMaxLen {
			fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
		}
	}
	if in.Type != nil && !in.Type.Valid() {
		fields["type"] = "must be internal or external"
	}
	if in.Status != nil && !in.Status.Valid() {
		fields["status"] = "must be one of active, paused, archived"
	}
	if len(fields) > 0 {
		return Landing{}, apierror.Validation("invalid landing", fields)
	}

	effectiveType := current.Type
	if in.Type != nil {
		effectiveType = *in.Type
	}

	if effectiveType == TypeExternal {
		effectiveURL := current.URL
		if in.URL != nil {
			effectiveURL = *in.URL
		}
		if !isValidURL(effectiveURL) {
			return Landing{}, apierror.Validation("invalid landing", map[string]string{"url": "must be a valid absolute URL"})
		}
		if in.Type != nil && current.Type == TypeInternal {
			// Switching internal -> external: the hosted content no
			// longer applies to an advertiser-owned page.
			empty := ""
			in.Content = &empty
		}
		return s.repo.Update(ctx, orgID, id, in)
	}

	effectiveContent := current.Content
	if in.Content != nil {
		effectiveContent = *in.Content
	}
	if strings.TrimSpace(effectiveContent) == "" {
		return Landing{}, apierror.Validation("invalid landing", map[string]string{"content": "add page content"})
	}
	// Recompute the CDN URL only when the effective name or type actually
	// changed — an idempotent no-op otherwise, but this keeps a
	// pause/activate/status-only PATCH from touching a column it has no
	// business rewriting.
	if in.Name != nil || in.Type != nil {
		effectiveName := current.Name
		if in.Name != nil {
			effectiveName = *in.Name
		}
		newURL := cdnURL(effectiveName)
		in.URL = &newURL
	}
	return s.repo.Update(ctx, orgID, id, in)
}

func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	return s.repo.Delete(ctx, orgID, id)
}

// Duplicate goes through Create (not repo.Create directly) so a
// TypeInternal source's URL is recomputed for the new "(Copy)" name
// rather than copied verbatim — status is preserved separately, same
// reasoning as network.Service.Duplicate: no draft-equivalent status
// exists to reset to.
func (s *Service) Duplicate(ctx context.Context, orgID, id string) (Landing, error) {
	source, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Landing{}, err
	}

	created, err := s.Create(ctx, orgID, CreateInput{
		Name:    source.Name + " (Copy)",
		Type:    source.Type,
		URL:     source.URL,
		Content: source.Content,
	})
	if err != nil {
		return Landing{}, err
	}
	if source.Status != StatusActive {
		status := source.Status
		return s.repo.Update(ctx, orgID, created.ID, UpdateInput{Status: &status})
	}
	return created, nil
}

func (s *Service) Pause(ctx context.Context, orgID, id string) (Landing, error) {
	l, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Landing{}, err
	}
	switch l.Status {
	case StatusPaused:
		return l, nil
	case StatusActive:
		status := StatusPaused
		return s.repo.Update(ctx, orgID, id, UpdateInput{Status: &status})
	default:
		return Landing{}, apierror.Conflict(fmt.Sprintf("cannot pause a landing with status %q", l.Status))
	}
}

func (s *Service) Activate(ctx context.Context, orgID, id string) (Landing, error) {
	l, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Landing{}, err
	}
	switch l.Status {
	case StatusActive:
		return l, nil
	case StatusPaused:
		status := StatusActive
		return s.repo.Update(ctx, orgID, id, UpdateInput{Status: &status})
	default:
		return Landing{}, apierror.Conflict(fmt.Sprintf("cannot activate a landing with status %q", l.Status))
	}
}

func isValidURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

var nonAlphaNumeric = regexp.MustCompile(`[^a-z0-9]+`)

// slugify mirrors apps/web/src/lib/utils.ts's slugify exactly, so the
// form's client-side "Hosted URL" preview and the server-computed value
// this package actually persists always agree.
func slugify(name string) string {
	s := nonAlphaNumeric.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "untitled"
	}
	return s
}

func cdnURL(name string) string {
	return "https://cdn.floxlink.io/lnd/" + slugify(name)
}
