package trafficsource

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
	nameMaxLen = 80
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, orgID string) ([]TrafficSource, error) {
	return s.repo.List(ctx, orgID)
}

func (s *Service) Get(ctx context.Context, orgID, id string) (TrafficSource, error) {
	return s.repo.GetByID(ctx, orgID, id)
}

func (s *Service) Create(ctx context.Context, orgID string, in CreateInput) (TrafficSource, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Type = strings.TrimSpace(in.Type)
	if in.CostIntegration == "" {
		in.CostIntegration = CostIntegrationNone
	}
	if fields := validateCreate(in); len(fields) > 0 {
		return TrafficSource{}, apierror.Validation("invalid traffic source", fields)
	}
	return s.repo.Create(ctx, idgen.New(), orgID, in)
}

func (s *Service) Update(ctx context.Context, orgID, id string, in UpdateInput) (TrafficSource, error) {
	fields := map[string]string{}

	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		in.Name = &trimmed
		if len(trimmed) < nameMinLen || len(trimmed) > nameMaxLen {
			fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
		}
	}
	if in.Type != nil {
		trimmed := strings.TrimSpace(*in.Type)
		in.Type = &trimmed
		if trimmed == "" {
			fields["type"] = "required"
		}
	}
	if in.TrackingTemplate != nil && !isValidURL(*in.TrackingTemplate) {
		fields["trackingTemplate"] = "must be a valid absolute URL"
	}
	if in.CostIntegration != nil && !in.CostIntegration.Valid() {
		fields["costIntegration"] = "must be one of none, manual, facebook_ads, tiktok_ads"
	}
	if in.Status != nil && !in.Status.Valid() {
		fields["status"] = "must be one of active, paused, archived"
	}
	if len(fields) > 0 {
		return TrafficSource{}, apierror.Validation("invalid traffic source", fields)
	}

	return s.repo.Update(ctx, orgID, id, in)
}

func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	return s.repo.Delete(ctx, orgID, id)
}

// Duplicate keeps the source's status as-is — unlike
// campaign.Service.Duplicate, which forces a fresh copy back to "draft."
// TrafficSource has no draft-equivalent status, and the mock store this
// replaces (stores/traffic-sources.ts duplicateSource) never reset it
// either.
func (s *Service) Duplicate(ctx context.Context, orgID, id string) (TrafficSource, error) {
	source, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return TrafficSource{}, err
	}

	created, err := s.repo.Create(ctx, idgen.New(), orgID, CreateInput{
		Name:             source.Name + " (Copy)",
		Type:             source.Type,
		TrackingTemplate: source.TrackingTemplate,
		CostIntegration:  source.CostIntegration,
	})
	if err != nil {
		return TrafficSource{}, err
	}
	if source.Status != StatusActive {
		status := source.Status
		return s.repo.Update(ctx, orgID, created.ID, UpdateInput{Status: &status})
	}
	return created, nil
}

// Pause/Activate mirror campaign.Service's own methods: idempotent from
// the target state, rejected from archived (an archived source has to be
// explicitly edited back via PATCH, not casually reactivated).

func (s *Service) Pause(ctx context.Context, orgID, id string) (TrafficSource, error) {
	src, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return TrafficSource{}, err
	}
	switch src.Status {
	case StatusPaused:
		return src, nil
	case StatusActive:
		status := StatusPaused
		return s.repo.Update(ctx, orgID, id, UpdateInput{Status: &status})
	default:
		return TrafficSource{}, apierror.Conflict(fmt.Sprintf("cannot pause a traffic source with status %q", src.Status))
	}
}

func (s *Service) Activate(ctx context.Context, orgID, id string) (TrafficSource, error) {
	src, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return TrafficSource{}, err
	}
	switch src.Status {
	case StatusActive:
		return src, nil
	case StatusPaused:
		status := StatusActive
		return s.repo.Update(ctx, orgID, id, UpdateInput{Status: &status})
	default:
		return TrafficSource{}, apierror.Conflict(fmt.Sprintf("cannot activate a traffic source with status %q", src.Status))
	}
}

func validateCreate(in CreateInput) map[string]string {
	fields := map[string]string{}
	if len(in.Name) < nameMinLen || len(in.Name) > nameMaxLen {
		fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
	}
	if in.Type == "" {
		fields["type"] = "required"
	}
	if !isValidURL(in.TrackingTemplate) {
		fields["trackingTemplate"] = "must be a valid absolute URL"
	}
	if !in.CostIntegration.Valid() {
		fields["costIntegration"] = "must be one of none, manual, facebook_ads, tiktok_ads"
	}
	return fields
}

func isValidURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
