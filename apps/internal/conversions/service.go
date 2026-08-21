package conversions

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
)

const (
	defaultLimit = 50
	maxLimit     = 200
	// maxRangeDays is narrower than internal/analytics' 366-day cap: this
	// package returns raw rows, not pre-aggregated daily buckets, so a
	// wide range is a much heavier ClickHouse scan for the same guard to
	// prevent.
	maxRangeDays = 90
)

// Repository is the narrow slice of chstore.EventStore this package needs
// — satisfied structurally, the same decoupling pattern internal/analytics
// already uses.
type Repository interface {
	ListConversions(ctx context.Context, organizationID string, from, to time.Time, limit, offset int) ([]chstore.ConversionEvent, error)
	CountConversions(ctx context.Context, organizationID string, from, to time.Time) (int, error)
	ConversionsByClickID(ctx context.Context, organizationID, clickID string) ([]chstore.ConversionEvent, error)
	FunnelByClickID(ctx context.Context, organizationID, clickID string) ([]chstore.FunnelEvent, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func validateRange(organizationID string, from, to time.Time) error {
	if organizationID == "" {
		return apierror.Validation("missing organization", nil)
	}
	if to.Before(from) {
		return apierror.Validation("to must not be before from", map[string]string{"to": "before from"})
	}
	if to.Sub(from) > maxRangeDays*24*time.Hour {
		return apierror.Validation("date range too wide", map[string]string{"to": fmt.Sprintf("more than %d days after from", maxRangeDays)})
	}
	return nil
}

// List returns one organization's conversion events over [from, to],
// newest first, paginated.
func (s *Service) List(ctx context.Context, organizationID string, from, to time.Time, limit, offset int) (ListResult, error) {
	if err := validateRange(organizationID, from, to); err != nil {
		return ListResult{}, err
	}
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.repo.ListConversions(ctx, organizationID, from, to, limit, offset)
	if err != nil {
		return ListResult{}, err
	}
	total, err := s.repo.CountConversions(ctx, organizationID, from, to)
	if err != nil {
		return ListResult{}, err
	}

	conversions := make([]Conversion, len(rows))
	for i, r := range rows {
		conversions[i] = Conversion{
			EventAt: r.EventAt, Type: r.Type, CampaignID: r.CampaignID, ClickID: r.ClickID, NetworkID: r.NetworkID,
			Revenue: r.Revenue, Currency: r.Currency, USDValue: r.USDValue, HasUSDValue: r.HasUSDValue,
		}
	}
	return ListResult{Conversions: conversions, Total: total}, nil
}

// Timeline merges a click_id's funnel events (click_events + tracking_events)
// and conversion events (conversion_events) into one chronological list —
// the Conversion detail page's whole point (§29: Click → Landing → PWA →
// Offer → Conversion, whatever stages actually happened for this click_id,
// not a fixed six-item funnel every conversion is forced into).
func (s *Service) Timeline(ctx context.Context, organizationID, clickID string) (ClickTimeline, error) {
	if organizationID == "" {
		return ClickTimeline{}, apierror.Validation("missing organization", nil)
	}
	if clickID == "" {
		return ClickTimeline{}, apierror.Validation("missing click id", nil)
	}

	funnel, err := s.repo.FunnelByClickID(ctx, organizationID, clickID)
	if err != nil {
		return ClickTimeline{}, err
	}
	conversionRows, err := s.repo.ConversionsByClickID(ctx, organizationID, clickID)
	if err != nil {
		return ClickTimeline{}, err
	}
	if len(funnel) == 0 && len(conversionRows) == 0 {
		return ClickTimeline{}, apierror.NotFound("no events found for this click id")
	}

	events := make([]TimelineEvent, 0, len(funnel)+len(conversionRows))
	var campaignID, networkID string
	for _, f := range funnel {
		events = append(events, TimelineEvent{EventAt: f.EventAt, Type: f.Type})
		campaignID = f.CampaignID
	}
	for _, c := range conversionRows {
		events = append(events, TimelineEvent{
			EventAt: c.EventAt, Type: c.Type, IsConversion: true,
			Revenue: c.Revenue, Currency: c.Currency, USDValue: c.USDValue, HasUSDValue: c.HasUSDValue,
		})
		campaignID = c.CampaignID
		networkID = c.NetworkID
	}
	sort.Slice(events, func(i, j int) bool { return events[i].EventAt.Before(events[j].EventAt) })

	return ClickTimeline{ClickID: clickID, CampaignID: campaignID, NetworkID: networkID, Events: events}, nil
}
