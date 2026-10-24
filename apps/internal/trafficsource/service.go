package trafficsource

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/pagination"
	"github.com/ismagilovnail/flox/apps/internal/urlsafety"
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

// List returns one page of the organization's traffic_sources and their total.
func (s *Service) List(ctx context.Context, orgID string, page pagination.Page) ([]TrafficSource, int, error) {
	items, err := s.repo.List(ctx, orgID, page)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.repo.Count(ctx, orgID)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
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
	in.PostbackURL = strings.TrimSpace(in.PostbackURL)
	in.TemplateKey = strings.TrimSpace(in.TemplateKey)
	if len(in.PostbackStatuses) == 0 {
		in.PostbackStatuses = []string{"CPA_ACCEPT"}
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
		fields["costIntegration"] = "must be one of none, manual, facebook_ads, tiktok_ads, google_ads, yandex_direct"
	}
	if in.PostbackURL != nil {
		trimmed := strings.TrimSpace(*in.PostbackURL)
		in.PostbackURL = &trimmed
		if msg := validatePostbackURL(trimmed); msg != "" {
			fields["postbackUrl"] = msg
		}
	}
	if in.PostbackStatuses != nil {
		if msg := validatePostbackStatuses(*in.PostbackStatuses); msg != "" {
			fields["postbackStatuses"] = msg
		}
	}
	if in.TemplateKey != nil {
		trimmed := strings.TrimSpace(*in.TemplateKey)
		in.TemplateKey = &trimmed
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
		PostbackURL:      source.PostbackURL,
		PostbackStatuses: source.PostbackStatuses,
		TemplateKey:      source.TemplateKey,
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
		fields["costIntegration"] = "must be one of none, manual, facebook_ads, tiktok_ads, google_ads, yandex_direct"
	}
	if msg := validatePostbackURL(in.PostbackURL); msg != "" {
		fields["postbackUrl"] = msg
	}
	if msg := validatePostbackStatuses(in.PostbackStatuses); msg != "" {
		fields["postbackStatuses"] = msg
	}
	return fields
}

// validatePostbackURL: empty (no postback), or an http(s) URL that is no
// literal private address (urlsafety; the worker checks again at dial
// time) and has no template placeholder left — a catalog template marks
// the account-specific parts REPLACE_… for the operator to fill in.
func validatePostbackURL(raw string) string {
	if raw == "" {
		return ""
	}
	if err := urlsafety.ValidateURL(raw); err != nil {
		return "must be a public http(s) URL"
	}
	if strings.Contains(strings.ToUpper(raw), "REPLACE") {
		return "replace the REPLACE_… parts with the values from your account at the network"
	}
	return ""
}

// validatePostbackStatuses: CPA statuses only, at least one.
func validatePostbackStatuses(statuses []string) string {
	if len(statuses) == 0 {
		return "choose at least one status"
	}
	for _, st := range statuses {
		if !event.Type(st).IsCPA() {
			return "must be CPA statuses (CPA_HOLD, CPA_ACCEPT, CPA_REDEP, CPA_DECLINE, CPA_TRASH)"
		}
	}
	return ""
}

func isValidURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
