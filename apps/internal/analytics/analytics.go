// Package analytics is Phase 25's "analytics service" pipeline stage
// (§47): a thin read layer over chstore's minimal aggregate table, exposed
// as a REST endpoint by apps/api. Deliberately narrow — one query, one
// endpoint — proving the pipeline's last two stages work end to end;
// the real analytics surface (per-GEO/per-source/per-offer breakdowns,
// the metrics registry, custom metrics) is later work once Phase 26's
// real ClickHouse schema exists to query.
package analytics

import (
	"context"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
)

// Repository is the narrow slice of chstore.EventStore this package needs
// — satisfied structurally, no adapter required, the same decoupling
// pattern used throughout (EventSink, DeliveryEnqueuer, ClickHouseSink).
type Repository interface {
	DailyCampaignCounts(ctx context.Context, organizationID, campaignID string, from, to time.Time) ([]chstore.DailyCount, error)
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

// CampaignDaily returns daily (day, event type) counts for one campaign.
func (s *Service) CampaignDaily(ctx context.Context, organizationID, campaignID string, from, to time.Time) ([]chstore.DailyCount, error) {
	if organizationID == "" {
		return nil, apierror.Validation("missing organization", nil)
	}
	if campaignID == "" {
		return nil, apierror.Validation("missing campaign id", nil)
	}
	if to.Before(from) {
		return nil, apierror.Validation("to must not be before from", map[string]string{"to": "before from"})
	}
	if to.Sub(from) > maxRangeDays*24*time.Hour {
		return nil, apierror.Validation("date range too wide", map[string]string{"to": "more than 366 days after from"})
	}
	return s.repo.DailyCampaignCounts(ctx, organizationID, campaignID, from, to)
}
