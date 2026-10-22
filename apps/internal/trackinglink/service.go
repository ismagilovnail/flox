package trackinglink

import (
	"context"
	"regexp"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/campaignscope"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

const (
	slugMinLen = 1
	slugMaxLen = 64
)

// slugPattern matches apps/tracker/handler.go's own `/t/{trackingID}` path
// segment — lowercase alphanumeric + hyphens, no leading/trailing hyphen.
// Anything a URL path segment can't hold unescaped (spaces, slashes, ...)
// is rejected here rather than silently percent-encoded into something
// that no longer matches what an operator typed.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, orgID, campaignID string) ([]TrackingLink, error) {
	return s.repo.List(ctx, orgID, campaignID)
}

// ListByOrg is every campaign's tracking links, optionally of one status
// (empty: all).
func (s *Service) ListByOrg(ctx context.Context, orgID string, status Status) ([]TrackingLink, error) {
	if status != "" && !status.Valid() {
		return nil, apierror.Validation("invalid status", map[string]string{"status": "active, paused or archived"})
	}
	links, err := s.repo.ListByOrg(ctx, orgID, status)
	if err != nil {
		return nil, err
	}
	// Only the campaigns the request may read (campaignscope, audit TEAM-1b).
	scope, err := campaignscope.From(ctx)
	if err != nil {
		return nil, err
	}
	if scope.All {
		return links, nil
	}
	out := links[:0]
	for _, l := range links {
		if scope.Allows(l.CampaignID) {
			out = append(out, l)
		}
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, orgID, id string) (TrackingLink, error) {
	return s.repo.GetByID(ctx, orgID, id)
}

func (s *Service) Create(ctx context.Context, orgID, campaignID string, in CreateInput) (TrackingLink, error) {
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	fields := map[string]string{}
	if in.DomainID == "" {
		fields["domainId"] = "required"
	}
	if len(in.Slug) < slugMinLen || len(in.Slug) > slugMaxLen || !slugPattern.MatchString(in.Slug) {
		fields["slug"] = "lowercase letters, numbers, and hyphens only, 1-64 characters"
	}
	if len(fields) > 0 {
		return TrackingLink{}, apierror.Validation("invalid tracking link", fields)
	}
	return s.repo.Create(ctx, idgen.New(), orgID, campaignID, in)
}

func (s *Service) UpdateStatus(ctx context.Context, orgID, id string, status Status) (TrackingLink, error) {
	if !status.Valid() {
		return TrackingLink{}, apierror.Validation("invalid tracking link", map[string]string{"status": "must be one of active, paused, archived"})
	}
	return s.repo.UpdateStatus(ctx, orgID, id, status)
}

func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	return s.repo.Delete(ctx, orgID, id)
}
