package clicklogs

import (
	"context"
	"fmt"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/campaignscope"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
)

const (
	defaultLimit = 50
	maxLimit     = 200
	// maxRangeDays matches apps/internal/postbacklogs' own cap — raw rows,
	// not pre-aggregated buckets, so a wide range is a much heavier
	// ClickHouse scan than internal/analytics' aggregate report queries.
	maxRangeDays = 90
)

// Repository is the narrow slice of chstore.EventStore this package needs —
// the same structural-interface decoupling internal/postbacklogs and
// internal/conversions already use.
type Repository interface {
	ListClickLog(ctx context.Context, organizationID string, from, to time.Time, clickID string, limit, offset int, campaigns []string) ([]chstore.ClickLogRow, error)
	CountClickLog(ctx context.Context, organizationID string, from, to time.Time, clickID string, campaigns []string) (int, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// List returns one organization's click_events rows over [from, to], newest
// first, paginated.
func (s *Service) List(ctx context.Context, organizationID string, from, to time.Time, clickID string, limit, offset int) (ListResult, error) {
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

	// Only the campaigns the request may read (campaignscope, audit TEAM-1b).
	campaigns, err := campaignscope.IDs(ctx)
	if err != nil {
		return ListResult{}, err
	}
	rows, err := s.repo.ListClickLog(ctx, organizationID, from, to, clickID, limit, offset, campaigns)
	if err != nil {
		return ListResult{}, err
	}
	total, err := s.repo.CountClickLog(ctx, organizationID, from, to, clickID, campaigns)
	if err != nil {
		return ListResult{}, err
	}

	logs := make([]ClickLog, len(rows))
	for i, r := range rows {
		logs[i] = ClickLog{
			EventAt: r.EventAt, Type: r.Type,
			CampaignID: r.CampaignID, ClickID: r.ClickID, StreamSetID: r.StreamSetID, FlowID: r.FlowID,
			Destination: r.Destination, StickyApplied: r.StickyApplied,
			Country: r.Country, Region: r.Region, City: r.City, Device: r.Device, Platform: r.Platform,
			OS: r.OS, OSVersion: r.OSVersion, Browser: r.Browser, BrowserVersion: r.BrowserVersion, Language: r.Language,
			IsBot: r.IsBot, IsProxy: r.IsProxy, ASN: r.ASN, ConnectionType: r.ConnectionType,
			IP: r.IP, UserAgent: r.UserAgent, Referrer: r.Referrer,
			UTMSource: r.UTMSource, UTMMedium: r.UTMMedium, UTMCampaign: r.UTMCampaign, UTMContent: r.UTMContent, UTMTerm: r.UTMTerm,
			Sub1: r.Subs[0], Sub2: r.Subs[1], Sub3: r.Subs[2], Sub4: r.Subs[3], Sub5: r.Subs[4],
			Sub6: r.Subs[5], Sub7: r.Subs[6], Sub8: r.Subs[7], Sub9: r.Subs[8], Sub10: r.Subs[9],
			ExternalClickID: r.ExternalClickID, FBClickID: r.FBClickID, TTClickID: r.TTClickID, FilterReason: r.FilterReason,
		}
	}
	return ListResult{Logs: logs, Total: total}, nil
}
