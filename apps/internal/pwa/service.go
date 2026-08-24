package pwa

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
	nameMinLen      = 2
	nameMaxLen      = 80
	shortNameMaxLen = 20
)

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, orgID string) ([]Pwa, error) {
	return s.repo.List(ctx, orgID)
}

func (s *Service) Get(ctx context.Context, orgID, id string) (Pwa, error) {
	return s.repo.GetByID(ctx, orgID, id)
}

func (s *Service) Create(ctx context.Context, orgID string, in CreateInput) (Pwa, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.ShortName = strings.TrimSpace(in.ShortName)
	in.StartURL = strings.TrimSpace(in.StartURL)

	if fields := validate(in.Name, in.ShortName, in.ThemeColor, in.BackgroundColor, in.IconURL, in.StartURL); len(fields) > 0 {
		return Pwa{}, apierror.Validation("invalid pwa", fields)
	}
	return s.repo.Create(ctx, idgen.New(), orgID, in)
}

func (s *Service) Update(ctx context.Context, orgID, id string, in UpdateInput) (Pwa, error) {
	fields := map[string]string{}

	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		in.Name = &trimmed
		if len(trimmed) < nameMinLen || len(trimmed) > nameMaxLen {
			fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
		}
	}
	if in.ShortName != nil {
		trimmed := strings.TrimSpace(*in.ShortName)
		in.ShortName = &trimmed
		if trimmed == "" || len(trimmed) > shortNameMaxLen {
			fields["shortName"] = fmt.Sprintf("required, up to %d characters", shortNameMaxLen)
		}
	}
	if in.ThemeColor != nil && !hexColor.MatchString(*in.ThemeColor) {
		fields["themeColor"] = "enter a hex color like #16a34a"
	}
	if in.BackgroundColor != nil && !hexColor.MatchString(*in.BackgroundColor) {
		fields["backgroundColor"] = "enter a hex color like #16a34a"
	}
	if in.IconURL != nil && !isValidURL(*in.IconURL) {
		fields["iconUrl"] = "enter a valid icon URL"
	}
	if in.StartURL != nil {
		trimmed := strings.TrimSpace(*in.StartURL)
		in.StartURL = &trimmed
		if trimmed == "" {
			fields["startUrl"] = "required"
		}
	}
	if in.Status != nil && !in.Status.Valid() {
		fields["status"] = "must be one of active, paused, archived"
	}
	if len(fields) > 0 {
		return Pwa{}, apierror.Validation("invalid pwa", fields)
	}

	return s.repo.Update(ctx, orgID, id, in)
}

func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	return s.repo.Delete(ctx, orgID, id)
}

// Duplicate keeps status as-is, same reasoning as
// landing.Service.Duplicate/network.Service.Duplicate: no draft-equivalent
// status exists to reset to.
func (s *Service) Duplicate(ctx context.Context, orgID, id string) (Pwa, error) {
	source, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Pwa{}, err
	}

	created, err := s.repo.Create(ctx, idgen.New(), orgID, CreateInput{
		Name:               source.Name + " (Copy)",
		ShortName:          source.ShortName,
		ThemeColor:         source.ThemeColor,
		BackgroundColor:    source.BackgroundColor,
		IconURL:            source.IconURL,
		StartURL:           source.StartURL,
		BounceInAppWebview: source.BounceInAppWebview,
	})
	if err != nil {
		return Pwa{}, err
	}
	if source.Status != StatusActive {
		status := source.Status
		return s.repo.Update(ctx, orgID, created.ID, UpdateInput{Status: &status})
	}
	return created, nil
}

func (s *Service) Pause(ctx context.Context, orgID, id string) (Pwa, error) {
	p, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Pwa{}, err
	}
	switch p.Status {
	case StatusPaused:
		return p, nil
	case StatusActive:
		status := StatusPaused
		return s.repo.Update(ctx, orgID, id, UpdateInput{Status: &status})
	default:
		return Pwa{}, apierror.Conflict(fmt.Sprintf("cannot pause a pwa with status %q", p.Status))
	}
}

func (s *Service) Activate(ctx context.Context, orgID, id string) (Pwa, error) {
	p, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Pwa{}, err
	}
	switch p.Status {
	case StatusActive:
		return p, nil
	case StatusPaused:
		status := StatusActive
		return s.repo.Update(ctx, orgID, id, UpdateInput{Status: &status})
	default:
		return Pwa{}, apierror.Conflict(fmt.Sprintf("cannot activate a pwa with status %q", p.Status))
	}
}

func validate(name, shortName, themeColor, backgroundColor, iconURL, startURL string) map[string]string {
	fields := map[string]string{}
	if len(name) < nameMinLen || len(name) > nameMaxLen {
		fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
	}
	if shortName == "" || len(shortName) > shortNameMaxLen {
		fields["shortName"] = fmt.Sprintf("required, up to %d characters", shortNameMaxLen)
	}
	if !hexColor.MatchString(themeColor) {
		fields["themeColor"] = "enter a hex color like #16a34a"
	}
	if !hexColor.MatchString(backgroundColor) {
		fields["backgroundColor"] = "enter a hex color like #16a34a"
	}
	if !isValidURL(iconURL) {
		fields["iconUrl"] = "enter a valid icon URL"
	}
	if startURL == "" {
		fields["startUrl"] = "required"
	}
	return fields
}

func isValidURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
