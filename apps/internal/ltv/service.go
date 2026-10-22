package ltv

import (
	"context"
	"fmt"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/campaignscope"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/fx"
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
	repo  Repository
	money MoneySource
}

func NewService(repo Repository) *Service { return &Service{repo: repo} }

// MoneySource is the organization's reporting currency (§50-FX) —
// *fx.Store in production; same as analytics.MoneySource.
type MoneySource interface {
	ReportingConversion(ctx context.Context, organizationID string, from, to time.Time) (*fx.Conversion, error)
}

// WithMoney makes cohort revenue come out in the organization's reporting
// currency, each deposit at its own day's rate.
func (s *Service) WithMoney(m MoneySource) *Service {
	s.money = m
	return s
}

// Currency is the currency cohort revenue comes out in.
func (s *Service) Currency(ctx context.Context, organizationID string) (string, error) {
	if s.money == nil {
		return "USD", nil
	}
	now := time.Now()
	c, err := s.money.ReportingConversion(ctx, organizationID, now, now)
	if err != nil {
		return "", err
	}
	return c.Code(), nil
}

// convert rewrites each deposit's USD value into the reporting currency. A
// deposit is never earlier than its anchor, so [filter.From, now] covers
// them all. A deposit on a day with no rate counts as having no value —
// how a deposit with no USD value is already treated.
func (s *Service) convert(ctx context.Context, organizationID string, from time.Time, histories []chstore.ClickHistory) error {
	if s.money == nil {
		return nil
	}
	conv, err := s.money.ReportingConversion(ctx, organizationID, from.AddDate(0, 0, -1), time.Now().AddDate(0, 0, 1))
	if err != nil {
		return fmt.Errorf("ltv: loading the reporting currency: %w", err)
	}
	if conv.Identity() {
		return nil
	}
	for i := range histories {
		for j := range histories[i].Deposits {
			d := &histories[i].Deposits[j]
			if !d.HasUSDValue {
				continue
			}
			if f, ok := conv.Factor(d.EventAt); ok {
				d.USDValue *= f
			} else {
				d.USDValue, d.HasUSDValue = 0, false
			}
		}
	}
	return nil
}

func validateQuery(organizationID string, period CohortPeriod, filter chstore.LTVFilter, groupBy []Dimension) error {
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
	seen := map[Dimension]bool{}
	for _, d := range groupBy {
		if !ValidDimension(d) || seen[d] {
			return apierror.Validation("invalid groupBy, want campaign|source|offer|network|country, each once", map[string]string{"groupBy": string(d)})
		}
		seen[d] = true
	}
	return nil
}

// FTDCohorts computes §26.5's FTD cohort table for organizationID.
func (s *Service) FTDCohorts(ctx context.Context, organizationID string, filter chstore.LTVFilter, period CohortPeriod, groupBy []Dimension, asOf time.Time) ([]FTDCohort, error) {
	if err := validateQuery(organizationID, period, filter, groupBy); err != nil {
		return nil, err
	}
	if err := scope(ctx, &filter); err != nil {
		return nil, err
	}
	histories, err := s.repo.ClicksByFTDAnchor(ctx, organizationID, filter)
	if err != nil {
		return nil, err
	}
	if err := s.convert(ctx, organizationID, filter.From, histories); err != nil {
		return nil, err
	}
	return ComputeFTDCohorts(histories, period, groupBy, asOf), nil
}

// RegCohorts computes §26.5's Reg cohort table for organizationID.
func (s *Service) RegCohorts(ctx context.Context, organizationID string, filter chstore.LTVFilter, period CohortPeriod, groupBy []Dimension, asOf time.Time) ([]RegCohort, error) {
	if err := validateQuery(organizationID, period, filter, groupBy); err != nil {
		return nil, err
	}
	if err := scope(ctx, &filter); err != nil {
		return nil, err
	}
	histories, err := s.repo.ClicksByRegAnchor(ctx, organizationID, filter)
	if err != nil {
		return nil, err
	}
	if err := s.convert(ctx, organizationID, filter.From, histories); err != nil {
		return nil, err
	}
	return ComputeRegCohorts(histories, period, groupBy, asOf), nil
}

// scope narrows filter to the campaigns the request may read
// (campaignscope, audit TEAM-1b): a Buyer's cohorts hold only their
// campaigns' clicks.
func scope(ctx context.Context, filter *chstore.LTVFilter) error {
	s, err := campaignscope.From(ctx)
	if err != nil {
		return err
	}
	if !s.All {
		filter.Campaigns = s.IDs
	}
	return nil
}
