package eventmapping

import (
	"context"
	"fmt"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

const networkStatusMaxLen = 80

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service { return &Service{repo: repo} }

func (s *Service) List(ctx context.Context, orgID string) ([]EventMapping, error) {
	return s.repo.List(ctx, orgID)
}

func (s *Service) Create(ctx context.Context, orgID string, in CreateInput) (EventMapping, error) {
	in.NetworkStatus = strings.TrimSpace(in.NetworkStatus)

	fields := map[string]string{}
	if in.NetworkID == "" {
		fields["networkId"] = "required"
	}
	if in.NetworkStatus == "" || len(in.NetworkStatus) > networkStatusMaxLen {
		fields["networkStatus"] = fmt.Sprintf("must be 1-%d characters", networkStatusMaxLen)
	}
	if !in.FloxStatus.IsCPA() {
		fields["floxStatus"] = "must be one of CPA_HOLD, CPA_ACCEPT, CPA_REDEP, CPA_DECLINE, CPA_TRASH"
	}
	if len(fields) > 0 {
		return EventMapping{}, apierror.Validation("invalid event mapping", fields)
	}

	belongs, err := s.repo.NetworkBelongsToOrg(ctx, orgID, in.NetworkID)
	if err != nil {
		return EventMapping{}, fmt.Errorf("checking network: %w", err)
	}
	if !belongs {
		return EventMapping{}, apierror.Validation("invalid event mapping", map[string]string{"networkId": "no such network in this organization"})
	}

	return s.repo.Create(ctx, idgen.New(), orgID, in)
}

func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	return s.repo.Delete(ctx, orgID, id)
}
