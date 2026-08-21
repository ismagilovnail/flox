package ltv

import (
	"context"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
)

// Repository is the narrow slice of chstore.EventStore this package needs
// — satisfied structurally by *chstore.EventStore, no adapter required,
// the same decoupling pattern used throughout (EventSink, DeliveryEnqueuer,
// analytics.Repository).
type Repository interface {
	ClicksByFTDAnchor(ctx context.Context, organizationID string, filter chstore.LTVFilter) ([]chstore.ClickHistory, error)
	ClicksByRegAnchor(ctx context.Context, organizationID string, filter chstore.LTVFilter) ([]chstore.ClickHistory, error)
}

// maxAnchorRangeDays bounds the [From, To) anchor window a query can ask
// for — not in §26.5, the same deliberate guard analytics.maxRangeDays is,
// against an unbounded ClickHouse scan. This bounds the COHORT FORMATION
// range, not the total data fetched: a cohort formed on day 1 of a
// 366-day-wide query still pulls its redeposits arbitrarily far past the
// range's own end (see chstore.ClicksByFTDAnchor's doc).
const maxAnchorRangeDays = 366

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func validateQuery(organizationID string, period CohortPeriod, filter chstore.LTVFilter) error {
	if organizationID == "" {
		return apierror.Validation("missing organization", nil)
	}
	switch period {
	case PeriodDay, PeriodWeek, PeriodMonth:
	default:
		return apierror.Validation("invalid period, want day|week|month", map[string]string{"period": string(period)})
	}
	if filter.To.Before(filter.From) {
		return apierror.Validation("to must not be before from", map[string]string{"to": "before from"})
	}
	if filter.To.Sub(filter.From) > maxAnchorRangeDays*24*time.Hour {
		return apierror.Validation("date range too wide", map[string]string{"to": "more than 366 days after from"})
	}
	return nil
}

// FTDCohorts computes §26.5's FTD cohort table for organizationID.
func (s *Service) FTDCohorts(ctx context.Context, organizationID string, filter chstore.LTVFilter, period CohortPeriod, asOf time.Time) ([]FTDCohort, error) {
	if err := validateQuery(organizationID, period, filter); err != nil {
		return nil, err
	}
	histories, err := s.repo.ClicksByFTDAnchor(ctx, organizationID, filter)
	if err != nil {
		return nil, err
	}
	return ComputeFTDCohorts(histories, period, asOf), nil
}

// RegCohorts computes §26.5's Reg cohort table for organizationID.
func (s *Service) RegCohorts(ctx context.Context, organizationID string, filter chstore.LTVFilter, period CohortPeriod, asOf time.Time) ([]RegCohort, error) {
	if err := validateQuery(organizationID, period, filter); err != nil {
		return nil, err
	}
	histories, err := s.repo.ClicksByRegAnchor(ctx, organizationID, filter)
	if err != nil {
		return nil, err
	}
	return ComputeRegCohorts(histories, period, asOf), nil
}
