// Package analytics is the "analytics service" pipeline stage (§47/§48): a
// thin read layer over chstore's materialized aggregates, exposed as REST
// endpoints by apps/api. Deliberately narrow — two queries, two endpoints
// (click/filter volume, conversion revenue) — proving the pipeline's last
// two stages work end to end; the real analytics surface (per-GEO/per-
// source/per-offer breakdowns, the metrics registry, custom metrics) is
// later work once Phase 27's frontend integration needs it.
package analytics

import (
	"context"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

// Repository is the narrow slice of chstore.EventStore this package needs
// — satisfied structurally, no adapter required, the same decoupling
// pattern used throughout (EventSink, DeliveryEnqueuer, ClickHouseSink).
type Repository interface {
	DailyCampaignCounts(ctx context.Context, organizationID, campaignID string, from, to time.Time) ([]chstore.DailyCount, error)
	DailyCampaignRevenue(ctx context.Context, organizationID, campaignID string, from, to time.Time) ([]chstore.DailyRevenue, error)
}

// maxRangeDays bounds how wide a query can ask for. Not in §47 — a
// deliberate, documented guard against an unbounded ClickHouse scan from a
// stray `?to=2099-01-01`, the same spirit as campaign.Service capping List
// limits.
const maxRangeDays = 366

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) validateRange(organizationID, campaignID string, from, to time.Time) error {
	if organizationID == "" {
		return apierror.Validation("missing organization", nil)
	}
	if campaignID == "" {
		return apierror.Validation("missing campaign id", nil)
	}
	if to.Before(from) {
		return apierror.Validation("to must not be before from", map[string]string{"to": "before from"})
	}
	if to.Sub(from) > maxRangeDays*24*time.Hour {
		return apierror.Validation("date range too wide", map[string]string{"to": "more than 366 days after from"})
	}
	return nil
}

// CampaignDaily returns daily (day, event type) click/filter counts for one
// campaign, from click_events_daily_campaign.
func (s *Service) CampaignDaily(ctx context.Context, organizationID, campaignID string, from, to time.Time) ([]chstore.DailyCount, error) {
	if err := s.validateRange(organizationID, campaignID, from, to); err != nil {
		return nil, err
	}
	defer observeLatency("campaign_daily")()
	return s.repo.DailyCampaignCounts(ctx, organizationID, campaignID, from, to)
}

// CampaignDailyRevenue returns daily (day, CPA status) conversion counts
// and USD revenue for one campaign, from conversion_events_daily_campaign.
func (s *Service) CampaignDailyRevenue(ctx context.Context, organizationID, campaignID string, from, to time.Time) ([]chstore.DailyRevenue, error) {
	if err := s.validateRange(organizationID, campaignID, from, to); err != nil {
		return nil, err
	}
	defer observeLatency("campaign_daily_revenue")()
	return s.repo.DailyCampaignRevenue(ctx, organizationID, campaignID, from, to)
}

// observeLatency times only the ClickHouse round trip (validation already
// ran by the time either method above defers this) — call it, then defer
// its return value so the Observe happens on return, not on call.
func observeLatency(endpoint string) func() {
	start := time.Now()
	return func() {
		metrics.AnalyticsQueryLatencySeconds.WithLabelValues(endpoint).Observe(time.Since(start).Seconds())
	}
}
