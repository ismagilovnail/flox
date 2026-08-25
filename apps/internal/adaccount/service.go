package adaccount

import (
	"context"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

const (
	adAccountIDMaxLen = 100
	accessTokenMaxLen = 500
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Get(ctx context.Context, orgID, trafficSourceID string) (Connection, error) {
	return s.repo.GetByTrafficSourceID(ctx, orgID, trafficSourceID)
}

// Connect validates the traffic source exists in this org AND already
// has its CostIntegration set to facebook_ads/tiktok_ads — connecting a
// credential for a source whose intent is still "none"/"manual" would
// store a token nothing is configured to use, silently, which is worse
// than requiring the operator set the integration type first (the
// existing Traffic Source form already has that field).
func (s *Service) Connect(ctx context.Context, orgID, trafficSourceID string, in ConnectInput) (Connection, error) {
	in.AdAccountID = strings.TrimSpace(in.AdAccountID)
	in.AccessToken = strings.TrimSpace(in.AccessToken)

	fields := map[string]string{}
	if in.AdAccountID == "" || len(in.AdAccountID) > adAccountIDMaxLen {
		fields["adAccountId"] = "required, must be at most 100 characters"
	}
	if in.AccessToken == "" || len(in.AccessToken) > accessTokenMaxLen {
		fields["accessToken"] = "required, must be at most 500 characters"
	}
	if len(fields) > 0 {
		return Connection{}, apierror.Validation("invalid ad account connection", fields)
	}

	costIntegration, found, err := s.repo.TrafficSourceCostIntegration(ctx, orgID, trafficSourceID)
	if err != nil {
		return Connection{}, err
	}
	if !found {
		return Connection{}, apierror.NotFound("traffic source not found")
	}
	if costIntegration != "facebook_ads" && costIntegration != "tiktok_ads" {
		return Connection{}, apierror.Validation("invalid ad account connection", map[string]string{
			"trafficSourceId": "this traffic source's cost integration must be Facebook Ads or TikTok Ads before connecting an account",
		})
	}

	return s.repo.Connect(ctx, idgen.New(), orgID, trafficSourceID, in)
}

func (s *Service) Disconnect(ctx context.Context, orgID, trafficSourceID string) error {
	return s.repo.Disconnect(ctx, orgID, trafficSourceID)
}
