package adaccount

import (
	"context"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

const (
	adAccountIDMaxLen    = 100
	accessTokenMaxLen    = 500
	developerTokenMaxLen = 200
	clientIDMaxLen       = 200
	clientSecretMaxLen   = 200
	refreshTokenMaxLen   = 500
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
// has its CostIntegration set to facebook_ads/tiktok_ads/google_ads —
// connecting a credential for a source whose intent is still "none"/
// "manual" would store a credential nothing is configured to use,
// silently, which is worse than requiring the operator set the
// integration type first (the existing Traffic Source form already has
// that field). Which fields are required depends on that same
// cost_integration value: facebook_ads/tiktok_ads need AdAccountID +
// AccessToken; google_ads needs AdAccountID (its customer id) +
// DeveloperToken + ClientID + ClientSecret + RefreshToken — Google Ads'
// API has no long-lived access token equivalent to accept instead.
func (s *Service) Connect(ctx context.Context, orgID, trafficSourceID string, in ConnectInput) (Connection, error) {
	costIntegration, found, err := s.repo.TrafficSourceCostIntegration(ctx, orgID, trafficSourceID)
	if err != nil {
		return Connection{}, err
	}
	if !found {
		return Connection{}, apierror.NotFound("traffic source not found")
	}
	if costIntegration != "facebook_ads" && costIntegration != "tiktok_ads" && costIntegration != "google_ads" {
		return Connection{}, apierror.Validation("invalid ad account connection", map[string]string{
			"trafficSourceId": "this traffic source's cost integration must be Facebook Ads, TikTok Ads, or Google Ads before connecting an account",
		})
	}

	in.AdAccountID = strings.TrimSpace(in.AdAccountID)
	in.AccessToken = strings.TrimSpace(in.AccessToken)
	in.DeveloperToken = strings.TrimSpace(in.DeveloperToken)
	in.ClientID = strings.TrimSpace(in.ClientID)
	in.ClientSecret = strings.TrimSpace(in.ClientSecret)
	in.RefreshToken = strings.TrimSpace(in.RefreshToken)

	fields := map[string]string{}
	if in.AdAccountID == "" || len(in.AdAccountID) > adAccountIDMaxLen {
		fields["adAccountId"] = "required, must be at most 100 characters"
	}
	if costIntegration == "google_ads" {
		if in.DeveloperToken == "" || len(in.DeveloperToken) > developerTokenMaxLen {
			fields["developerToken"] = "required, must be at most 200 characters"
		}
		if in.ClientID == "" || len(in.ClientID) > clientIDMaxLen {
			fields["clientId"] = "required, must be at most 200 characters"
		}
		if in.ClientSecret == "" || len(in.ClientSecret) > clientSecretMaxLen {
			fields["clientSecret"] = "required, must be at most 200 characters"
		}
		if in.RefreshToken == "" || len(in.RefreshToken) > refreshTokenMaxLen {
			fields["refreshToken"] = "required, must be at most 500 characters"
		}
	} else {
		if in.AccessToken == "" || len(in.AccessToken) > accessTokenMaxLen {
			fields["accessToken"] = "required, must be at most 500 characters"
		}
	}
	if len(fields) > 0 {
		return Connection{}, apierror.Validation("invalid ad account connection", fields)
	}

	return s.repo.Connect(ctx, idgen.New(), orgID, trafficSourceID, in)
}

func (s *Service) Disconnect(ctx context.Context, orgID, trafficSourceID string) error {
	return s.repo.Disconnect(ctx, orgID, trafficSourceID)
}
