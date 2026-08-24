package postlanding

import (
	"context"
	"fmt"
	"net/url"
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

func (s *Service) List(ctx context.Context, orgID string) ([]Postlanding, error) {
	return s.repo.List(ctx, orgID)
}

func (s *Service) Get(ctx context.Context, orgID, id string) (Postlanding, error) {
	return s.repo.GetByID(ctx, orgID, id)
}

func (s *Service) Create(ctx context.Context, orgID string, in CreateInput) (Postlanding, error) {
	in.Name = strings.TrimSpace(in.Name)
	fields := map[string]string{}

	if len(in.Name) < nameMinLen || len(in.Name) > nameMaxLen {
		fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
	}
	if !isValidURL(in.URL) {
		fields["url"] = "must be a valid absolute URL"
	}
	if err := validateEvents(in.Events); err != "" {
		fields["events"] = err
	}

	if len(fields) > 0 {
		return Postlanding{}, apierror.Validation("invalid postlanding", fields)
	}
	return s.repo.Create(ctx, idgen.New(), orgID, in)
}

func (s *Service) Update(ctx context.Context, orgID, id string, in UpdateInput) (Postlanding, error) {
	if _, err := s.repo.GetByID(ctx, orgID, id); err != nil {
		return Postlanding{}, err
	}

	fields := map[string]string{}

	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		in.Name = &trimmed
		if len(trimmed) < nameMinLen || len(trimmed) > nameMaxLen {
			fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
		}
	}
	if in.URL != nil && !isValidURL(*in.URL) {
		fields["url"] = "must be a valid absolute URL"
	}
	if in.Events != nil {
		if err := validateEvents(*in.Events); err != "" {
			fields["events"] = err
		}
	}
	if in.Status != nil && !in.Status.Valid() {
		fields["status"] = "must be one of active, paused, archived"
	}
	if len(fields) > 0 {
		return Postlanding{}, apierror.Validation("invalid postlanding", fields)
	}

	return s.repo.Update(ctx, orgID, id, in)
}

func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	return s.repo.Delete(ctx, orgID, id)
}

// Duplicate copies fields directly (no server-computed value to
// recompute, unlike landing.Service.Duplicate) — status is preserved
// separately via a follow-up Update, same reasoning as
// landing.Service.Duplicate/network.Service.Duplicate: no draft-
// equivalent status exists to reset to.
func (s *Service) Duplicate(ctx context.Context, orgID, id string) (Postlanding, error) {
	source, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Postlanding{}, err
	}

	created, err := s.repo.Create(ctx, idgen.New(), orgID, CreateInput{
		Name:   source.Name + " (Copy)",
		URL:    source.URL,
		Events: source.Events,
	})
	if err != nil {
		return Postlanding{}, err
	}
	if source.Status != StatusActive {
		status := source.Status
		return s.repo.Update(ctx, orgID, created.ID, UpdateInput{Status: &status})
	}
	return created, nil
}

func (s *Service) Pause(ctx context.Context, orgID, id string) (Postlanding, error) {
	p, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Postlanding{}, err
	}
	switch p.Status {
	case StatusPaused:
		return p, nil
	case StatusActive:
		status := StatusPaused
		return s.repo.Update(ctx, orgID, id, UpdateInput{Status: &status})
	default:
		return Postlanding{}, apierror.Conflict(fmt.Sprintf("cannot pause a postlanding with status %q", p.Status))
	}
}

func (s *Service) Activate(ctx context.Context, orgID, id string) (Postlanding, error) {
	p, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Postlanding{}, err
	}
	switch p.Status {
	case StatusActive:
		return p, nil
	case StatusPaused:
		status := StatusActive
		return s.repo.Update(ctx, orgID, id, UpdateInput{Status: &status})
	default:
		return Postlanding{}, apierror.Conflict(fmt.Sprintf("cannot activate a postlanding with status %q", p.Status))
	}
}

func isValidURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// validateEvents returns a non-empty field-error message when events is
// invalid, or "" when it's fine. At least one event is required (matches
// the frontend zod schema's z.array(...).min(1)); every value must be one
// of ValidEventTypes.
func validateEvents(events []string) string {
	if len(events) == 0 {
		return "select at least one event"
	}
	for _, e := range events {
		if !isValidEventType(e) {
			return fmt.Sprintf("%q is not a recognized event type", e)
		}
	}
	return ""
}
