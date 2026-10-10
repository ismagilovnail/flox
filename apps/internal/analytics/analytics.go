// Package analytics is the "analytics service" pipeline stage (§47/§48): a
// read layer over chstore's event tables and materialized aggregates,
// exposed as REST endpoints by apps/api. Started narrow — two queries,
// two endpoints (click/filter volume, conversion revenue) for one
// campaign, proving the pipeline's last two stages worked end to end.
// Phase 35 (post-Phase-34, user-scoped — see CLAUDE.md CURRENT STATE) is
// the real analytics surface that narrow start deferred: multi-dimension
// group-by (report.go's Report/Breakdown), a daily timeseries, the §43
// funnel, and the Overview dashboard — replacing apps/web's mock data for
// both /analytics and /overview.
package analytics

import (
	"context"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/cost"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

// Repository is the narrow slice of chstore.EventStore this package needs
// — satisfied structurally, no adapter required, the same decoupling
// pattern used throughout (EventSink, DeliveryEnqueuer, ClickHouseSink).
type Repository interface {
	DailyCampaignCounts(ctx context.Context, organizationID, campaignID string, from, to time.Time) ([]chstore.DailyCount, error)
	DailyCampaignRevenue(ctx context.Context, organizationID, campaignID string, from, to time.Time, basis chstore.DateBasis) ([]chstore.DailyRevenue, error)
	ReportClicks(ctx context.Context, q chstore.ReportQuery) ([]chstore.ClickAgg, error)
	ReportLandViews(ctx context.Context, q chstore.ReportQuery) ([]chstore.LandAgg, error)
	ReportPwaInstalls(ctx context.Context, q chstore.ReportQuery) ([]chstore.InstallAgg, error)
	ReportConversions(ctx context.Context, q chstore.ReportQuery) ([]chstore.ConversionAgg, error)
	FunnelCounts(ctx context.Context, organizationID string, from, to time.Time, filters []chstore.ReportFilter) (map[string]uint64, error)
	DistinctValues(ctx context.Context, organizationID string, dim chstore.ReportDimension, from, to time.Time, limit int) ([]string, error)
}

// CostSource is the narrow slice of *cost.Service Phase 35 needs — same
// decoupling pattern as Repository above. Referencing cost.CampaignSpend/
// cost.DailySpend directly (rather than package-local equivalents) mirrors
// how Repository above already references chstore's own result types
// instead of redeclaring them; both are plain data-transfer shapes, not
// business logic, so importing them costs nothing this package doesn't
// already pay for chstore.
//
// Optional: a nil CostSource (e.g. an environment with no cost package
// wired in yet) simply means every report row's Cost/Profit/ROI/ROAS/
// CPC/CPA comes back nil — the same "no cost data on file" null CLAUDE.md
// #6 already requires for a real but empty cost_entries table, not a
// different code path.
type CostSource interface {
	SpendByCampaign(ctx context.Context, organizationID string, from, to time.Time) ([]cost.CampaignSpend, error)
	SpendBySource(ctx context.Context, organizationID string, from, to time.Time) ([]cost.SourceSpend, error)
	CampaignSpendByDay(ctx context.Context, organizationID string, from, to time.Time) ([]cost.CampaignDaySpend, error)
	SourceSpendByDay(ctx context.Context, organizationID string, from, to time.Time) ([]cost.SourceDaySpend, error)
	DailySpendForOrg(ctx context.Context, organizationID string, from, to time.Time) ([]cost.DailySpend, error)
	// AccountTimezones: the ad-account zones synced spend in [from, to] is
	// cut in (CostTimezones).
	AccountTimezones(ctx context.Context, organizationID string, from, to time.Time) ([]string, error)
}

// maxRangeDays bounds how wide a query can ask for. Not in §47 — a
// deliberate, documented guard against an unbounded ClickHouse scan from a
// stray `?to=2099-01-01`, the same spirit as campaign.Service capping List
// limits.
const maxRangeDays = 366

type Service struct {
	repo Repository
	cost CostSource
}

// costSource may be nil — see CostSource's own doc comment on what that
// means for report output.
func NewService(repo Repository, costSource CostSource) *Service {
	return &Service{repo: repo, cost: costSource}
}

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
// and USD revenue for one campaign, dated by conversion or by click.
func (s *Service) CampaignDailyRevenue(ctx context.Context, organizationID, campaignID string, from, to time.Time, basis chstore.DateBasis) ([]chstore.DailyRevenue, error) {
	if err := s.validateRange(organizationID, campaignID, from, to); err != nil {
		return nil, err
	}
	if err := validateBasis(basis); err != nil {
		return nil, err
	}
	defer observeLatency("campaign_daily_revenue")()
	return s.repo.DailyCampaignRevenue(ctx, organizationID, campaignID, from, to, basis)
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
