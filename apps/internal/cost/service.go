package cost

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// FXConverter is the same shape as internal/conversion.FXConverter,
// satisfied structurally by conversion.PostgresFX — one fx_rates lookup
// implementation, reused here rather than duplicated (CLAUDE.md: no
// duplicate business logic).
type FXConverter interface {
	ToUSD(ctx context.Context, currency string, amount float64, at time.Time) (usd float64, ok bool, err error)
}

type Service struct {
	repo *Repository
	fx   FXConverter
}

func NewService(repo *Repository, fx FXConverter) *Service {
	return &Service{repo: repo, fx: fx}
}

func (s *Service) Upsert(ctx context.Context, orgID, campaignID string, in UpsertInput) (Entry, error) {
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.TrafficSourceID != nil && strings.TrimSpace(*in.TrafficSourceID) == "" {
		in.TrafficSourceID = nil
	}

	if fields := validateUpsert(in); len(fields) > 0 {
		return Entry{}, apierror.Validation("invalid cost entry", fields)
	}

	belongs, err := s.repo.CampaignBelongsToOrg(ctx, orgID, campaignID)
	if err != nil {
		return Entry{}, fmt.Errorf("checking campaign: %w", err)
	}
	if !belongs {
		return Entry{}, apierror.Validation("invalid cost entry", map[string]string{"campaignId": "no such campaign in this organization"})
	}

	if in.TrafficSourceID != nil {
		srcBelongs, err := s.repo.TrafficSourceBelongsToOrg(ctx, orgID, *in.TrafficSourceID)
		if err != nil {
			return Entry{}, fmt.Errorf("checking traffic source: %w", err)
		}
		if !srcBelongs {
			return Entry{}, apierror.Validation("invalid cost entry", map[string]string{"trafficSourceId": "no such traffic source in this organization"})
		}
	}

	usd, ok, err := s.fx.ToUSD(ctx, in.Currency, in.Amount, in.EntryDate)
	if err != nil {
		return Entry{}, fmt.Errorf("converting to usd: %w", err)
	}
	var amountUSD *float64
	if ok {
		amountUSD = &usd
	}

	return s.repo.Upsert(ctx, idgen.New(), orgID, campaignID, in, amountUSD)
}

func (s *Service) List(ctx context.Context, orgID, campaignID string, filter ListFilter) ([]Entry, error) {
	return s.repo.List(ctx, orgID, campaignID, filter)
}

func (s *Service) Delete(ctx context.Context, orgID, campaignID, id string) error {
	return s.repo.Delete(ctx, orgID, campaignID, id)
}

func (s *Service) DailyCampaignSpend(ctx context.Context, orgID, campaignID string, from, to time.Time) ([]DailySpend, error) {
	return s.repo.DailyCampaignSpend(ctx, orgID, campaignID, from, to)
}

func validateUpsert(in UpsertInput) map[string]string {
	fields := map[string]string{}
	if in.EntryDate.IsZero() {
		fields["entryDate"] = "required"
	}
	if in.Amount < 0 {
		fields["amount"] = "must not be negative"
	}
	if len(in.Currency) != 3 {
		fields["currency"] = "must be a 3-letter currency code"
	}
	if in.TrafficSourceID != nil && !idgen.IsValid(*in.TrafficSourceID) {
		fields["trafficSourceId"] = "invalid id"
	}
	return fields
}
