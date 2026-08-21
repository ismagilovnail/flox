package postbacklogs

import (
	"context"
	"fmt"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
)

const (
	defaultLimit = 50
	maxLimit     = 200
	// maxRangeDays matches apps/internal/conversions' own cap — raw rows,
	// not pre-aggregated daily buckets, so a wide range is a much heavier
	// ClickHouse scan than internal/analytics' 366-day cap was designed
	// to guard against.
	maxRangeDays = 90
)

// Repository is the narrow slice of chstore.EventStore this package needs
// — satisfied structurally, the same decoupling pattern
// internal/analytics/internal/conversions already use.
type Repository interface {
	ListPostbackAttempts(ctx context.Context, organizationID string, from, to time.Time, limit, offset int) ([]chstore.PostbackAttempt, error)
	CountPostbackAttempts(ctx context.Context, organizationID string, from, to time.Time) (int, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service { return &Service{repo: repo} }

// List returns one organization's postback attempts (both directions)
// over [from, to], newest first, paginated.
func (s *Service) List(ctx context.Context, organizationID string, from, to time.Time, limit, offset int) (ListResult, error) {
	if organizationID == "" {
		return ListResult{}, apierror.Validation("missing organization", nil)
	}
	if to.Before(from) {
		return ListResult{}, apierror.Validation("to must not be before from", map[string]string{"to": "before from"})
	}
	if to.Sub(from) > maxRangeDays*24*time.Hour {
		return ListResult{}, apierror.Validation("date range too wide", map[string]string{"to": fmt.Sprintf("more than %d days after from", maxRangeDays)})
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

	rows, err := s.repo.ListPostbackAttempts(ctx, organizationID, from, to, limit, offset)
	if err != nil {
		return ListResult{}, err
	}
	total, err := s.repo.CountPostbackAttempts(ctx, organizationID, from, to)
	if err != nil {
		return ListResult{}, err
	}

	logs := make([]PostbackLog, len(rows))
	for i, r := range rows {
		logs[i] = PostbackLog{
			EventAt: r.EventAt, Direction: r.Direction, NetworkID: r.NetworkID, ClickID: r.ClickID,
			Status: r.Status, RawStatus: r.RawStatus, Result: r.Result, Message: r.Message,
			AttemptCount: r.AttemptCount, ResponseStatusCode: r.ResponseStatusCode, URL: r.URL,
			Revenue: r.Revenue, Currency: r.Currency,
		}
	}
	return ListResult{Logs: logs, Total: total}, nil
}
