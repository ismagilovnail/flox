package costsync

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"
	// accountTimezone validates zone names: the zone database must ship
	// in the binary (the alpine images have none).
	_ "time/tzdata"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/campaign"
	"github.com/ismagilovnail/flox/apps/internal/cost"
)

// sourceByCostIntegration mirrors trafficsource.CostIntegration's own
// facebook_ads/tiktok_ads values (as stored on traffic_sources) onto the
// cost.Source enum a written cost_entries row must carry — kept local to
// this package rather than importing trafficsource for two string
// constants.
var sourceByCostIntegration = map[string]cost.Source{
	"facebook_ads":  cost.SourceFacebookAds,
	"tiktok_ads":    cost.SourceTikTokAds,
	"google_ads":    cost.SourceGoogleAds,
	"yandex_direct": cost.SourceYandexDirect,
}

func (p Providers) forCostIntegration(costIntegration string) (adaccount.CostProvider, bool) {
	switch costIntegration {
	case "facebook_ads":
		return p.FacebookAds, p.FacebookAds != nil
	case "tiktok_ads":
		return p.TikTokAds, p.TikTokAds != nil
	case "google_ads":
		return p.GoogleAds, p.GoogleAds != nil
	case "yandex_direct":
		return p.YandexDirect, p.YandexDirect != nil
	default:
		return nil, false
	}
}

type Service struct {
	adAccountRepo *adaccount.Repository
	campaignRepo  *campaign.Repository
	costSvc       *cost.Service
	providers     Providers
}

func NewService(adAccountRepo *adaccount.Repository, campaignRepo *campaign.Repository, costSvc *cost.Service, providers Providers) *Service {
	return &Service{adAccountRepo: adAccountRepo, campaignRepo: campaignRepo, costSvc: costSvc, providers: providers}
}

// Sync pulls [from, to] daily campaign spend for trafficSourceID's
// connected ad account and writes matching FLOX campaigns' cost_entries
// via cost.Service.UpsertFromSync. A record whose ExternalCampaignID
// matches no campaign (campaign.Repository.ListByExternalID, migration
// 00019) produces no cost_entries row and is not an error — CLAUDE.md
// invariant #6: missing cost shows as "—", never a false zero, and a
// real ad account will always have campaigns FLOX doesn't know about
// (unmapped or genuinely unrelated ones on the same ad account). A
// record matching more than one campaign (external id deliberately
// shared, migration 00019's own comment) writes the full day's spend to
// each — not divided between them, since each FLOX campaign's own
// traffic genuinely cost that ad platform's full reported amount for
// that day, split-testing two FLOX setups against one real spend is not
// the sync's concern to reconcile.
func (s *Service) Sync(ctx context.Context, orgID, trafficSourceID string, from, to time.Time) (Result, error) {
	costIntegration, found, err := s.adAccountRepo.TrafficSourceCostIntegration(ctx, orgID, trafficSourceID)
	if err != nil {
		return Result{}, fmt.Errorf("checking traffic source: %w", err)
	}
	if !found {
		return Result{}, apierror.NotFound("traffic source not found")
	}

	provider, ok := s.providers.forCostIntegration(costIntegration)
	if !ok {
		return Result{}, apierror.Validation("cannot sync", map[string]string{
			"trafficSourceId": "this traffic source's cost integration is not a supported ad network",
		})
	}
	source := sourceByCostIntegration[costIntegration]

	creds, err := s.adAccountRepo.CredentialsByTrafficSourceID(ctx, orgID, trafficSourceID)
	if err != nil {
		return Result{}, err
	}

	records, err := provider.DailySpendByCampaign(ctx, creds, from, to)
	if err != nil {
		// The ad platform's own reason (a refused token, a report not
		// ready) is the operator's to read on "Sync now", not a 500 — but
		// never a request URL: Facebook's carries the access token.
		if ctx.Err() != nil {
			return Result{}, fmt.Errorf("fetching spend: %w", err)
		}
		msg := err.Error()
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			msg = "could not reach the ad platform: " + urlErr.Err.Error()
		}
		return Result{}, apierror.Unavailable("the ad platform refused the spend request: " + msg)
	}

	result := Result{RecordsFetched: len(records)}
	if tz := accountTimezone(records); tz != "" {
		if err := s.adAccountRepo.SetTimezone(ctx, orgID, trafficSourceID, tz); err != nil {
			return result, err
		}
		result.AccountTimezone = tz
	}
	seenUnmatched := map[string]bool{}
	for _, rec := range records {
		campaigns, err := s.campaignRepo.ListByExternalID(ctx, orgID, trafficSourceID, rec.ExternalCampaignID)
		if err != nil {
			return result, fmt.Errorf("matching campaign for external id %q: %w", rec.ExternalCampaignID, err)
		}
		if len(campaigns) == 0 {
			if !seenUnmatched[rec.ExternalCampaignID] {
				seenUnmatched[rec.ExternalCampaignID] = true
				if len(result.UnmatchedExternalCampaignIDs) < maxUnmatchedListed {
					result.UnmatchedExternalCampaignIDs = append(result.UnmatchedExternalCampaignIDs, rec.ExternalCampaignID)
				} else {
					result.UnmatchedExternalCampaignsMax = true
				}
			}
			continue
		}
		for _, c := range campaigns {
			if _, err := s.costSvc.UpsertFromSync(ctx, orgID, c.ID, cost.UpsertInput{
				TrafficSourceID: &trafficSourceID,
				EntryDate:       rec.Date,
				Amount:          rec.Amount,
				Currency:        rec.Currency,
			}, source); err != nil {
				if errors.Is(err, cost.ErrSplitConflict) {
					result.EntriesSkippedSplit++
					continue
				}
				return result, fmt.Errorf("writing cost entry for campaign %s: %w", c.ID, err)
			}
			result.EntriesWritten++
		}
	}

	return result, nil
}

// accountTimezone is the ad account's zone as the records name it, when
// it is a zone Go knows (reports compare offsets with it).
func accountTimezone(records []adaccount.DailyCampaignSpendRecord) string {
	for _, r := range records {
		if r.Timezone == "" {
			continue
		}
		if _, err := time.LoadLocation(r.Timezone); err == nil {
			return r.Timezone
		}
		return ""
	}
	return ""
}
