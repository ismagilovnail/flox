package campaign

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/ismagilovnail/flox/apps/api/internal/apierror"
	"github.com/ismagilovnail/flox/apps/api/internal/idgen"
)

const (
	nameMinLen  = 2
	nameMaxLen  = 80
	notesMaxLen = 500
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, orgID string, filter ListFilter) (ListResult, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 200 {
		filter.Limit = 200
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return s.repo.List(ctx, orgID, filter)
}

func (s *Service) Get(ctx context.Context, orgID, id string) (Campaign, error) {
	return s.repo.GetByID(ctx, orgID, id)
}

func (s *Service) Create(ctx context.Context, orgID string, in CreateInput) (Campaign, error) {
	in.Name = strings.TrimSpace(in.Name)
	if fields := validateCreate(in); len(fields) > 0 {
		return Campaign{}, apierror.Validation("invalid campaign", fields)
	}

	belongs, err := s.repo.TrafficSourceBelongsToOrg(ctx, orgID, in.TrafficSourceID)
	if err != nil {
		return Campaign{}, fmt.Errorf("checking traffic source: %w", err)
	}
	if !belongs {
		return Campaign{}, apierror.Validation("invalid campaign", map[string]string{"trafficSourceId": "no such traffic source in this organization"})
	}

	return s.repo.Create(ctx, idgen.New(), orgID, in)
}

func (s *Service) Update(ctx context.Context, orgID, id string, in UpdateInput) (Campaign, error) {
	fields := map[string]string{}

	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		in.Name = &trimmed
		if len(trimmed) < nameMinLen || len(trimmed) > nameMaxLen {
			fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
		}
	}
	if in.FallbackURL != nil && !isValidURL(*in.FallbackURL) {
		fields["fallbackUrl"] = "must be a valid absolute URL"
	}
	if in.Notes != nil && len(*in.Notes) > notesMaxLen {
		fields["notes"] = fmt.Sprintf("must be at most %d characters", notesMaxLen)
	}
	if in.Status != nil && !in.Status.Valid() {
		fields["status"] = "must be one of active, paused, draft, archived"
	}
	if len(fields) > 0 {
		return Campaign{}, apierror.Validation("invalid campaign", fields)
	}

	if in.TrafficSourceID != nil {
		belongs, err := s.repo.TrafficSourceBelongsToOrg(ctx, orgID, *in.TrafficSourceID)
		if err != nil {
			return Campaign{}, fmt.Errorf("checking traffic source: %w", err)
		}
		if !belongs {
			return Campaign{}, apierror.Validation("invalid campaign", map[string]string{"trafficSourceId": "no such traffic source in this organization"})
		}
	}

	return s.repo.Update(ctx, orgID, id, in)
}

func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	return s.repo.Delete(ctx, orgID, id)
}

// Duplicate mirrors the frontend's stores/campaigns.ts duplicateCampaign:
// a fresh id, "{name} (Copy)", forced back to draft — a duplicate is a new
// starting point, not a running clone of an active campaign.
func (s *Service) Duplicate(ctx context.Context, orgID, id string) (Campaign, error) {
	source, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Campaign{}, err
	}

	return s.repo.Create(ctx, idgen.New(), orgID, CreateInput{
		TrafficSourceID: source.TrafficSourceID,
		Name:            source.Name + " (Copy)",
		FallbackURL:     source.FallbackURL,
		Notes:           source.Notes,
	})
}

// Pause/Activate are deliberately not bare status setters — §37 asks for
// domain-rule validation, and "what transitions are even legal" is exactly
// that: idempotent from the target state, rejected from archived (an
// archived campaign has to be explicitly edited back to another status via
// PATCH, not casually reactivated by a pause/activate toggle).

func (s *Service) Pause(ctx context.Context, orgID, id string) (Campaign, error) {
	c, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Campaign{}, err
	}
	switch c.Status {
	case StatusPaused:
		return c, nil
	case StatusActive:
		status := StatusPaused
		return s.repo.Update(ctx, orgID, id, UpdateInput{Status: &status})
	default:
		return Campaign{}, apierror.Conflict(fmt.Sprintf("cannot pause a campaign with status %q", c.Status))
	}
}

func (s *Service) Activate(ctx context.Context, orgID, id string) (Campaign, error) {
	c, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Campaign{}, err
	}
	switch c.Status {
	case StatusActive:
		return c, nil
	case StatusPaused, StatusDraft:
		status := StatusActive
		return s.repo.Update(ctx, orgID, id, UpdateInput{Status: &status})
	default:
		return Campaign{}, apierror.Conflict(fmt.Sprintf("cannot activate a campaign with status %q", c.Status))
	}
}

func validateCreate(in CreateInput) map[string]string {
	fields := map[string]string{}
	if len(in.Name) < nameMinLen || len(in.Name) > nameMaxLen {
		fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
	}
	if !idgen.IsValid(in.TrafficSourceID) {
		fields["trafficSourceId"] = "required"
	}
	if !isValidURL(in.FallbackURL) {
		fields["fallbackUrl"] = "must be a valid absolute URL"
	}
	if len(in.Notes) > notesMaxLen {
		fields["notes"] = fmt.Sprintf("must be at most %d characters", notesMaxLen)
	}
	return fields
}

func isValidURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
