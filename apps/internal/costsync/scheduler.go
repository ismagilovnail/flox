package costsync

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
)

// connectionLister is the narrow slice of *adaccount.Repository Scheduler
// needs, so a test can substitute a fake without a real Postgres pool —
// same reasoning as this package's own Providers interface.
type connectionLister interface {
	ListAllConnections(ctx context.Context) ([]adaccount.ConnectionRef, error)
}

// Scheduler is the automated counterpart to the handler's on-demand
// POST .../connection/sync — it runs Service.Sync for every currently
// connected ad account, across every org, on a fixed interval (apps/worker,
// no HTTP surface of its own). One connection failing to sync (an expired
// token, a transient API error, a traffic source disconnected mid-run) must
// never block any other org's or any other traffic source's sync, so each
// is attempted independently and its error is only logged, never returned —
// the same "one bad row doesn't stall the batch" spirit as apps/worker's
// existing poll loops (postback.Deliverer, eventqueue.Flusher).
type Scheduler struct {
	svc          *Service
	connections  connectionLister
	lookbackDays int
	logger       *slog.Logger
}

// NewScheduler uses defaultLookbackDays (handler.go) as its sync window —
// the same 30-day-including-today re-pull an on-demand "Sync now" defaults
// to, since ad platforms commonly revise very recent days' reported spend.
func NewScheduler(svc *Service, connections connectionLister, logger *slog.Logger) *Scheduler {
	return &Scheduler{svc: svc, connections: connections, lookbackDays: defaultLookbackDays, logger: logger}
}

// RunOnce syncs every currently-connected ad account once and returns how
// many connections were attempted.
func (s *Scheduler) RunOnce(ctx context.Context) (int, error) {
	refs, err := s.connections.ListAllConnections(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing connected ad accounts: %w", err)
	}

	to := time.Now().UTC()
	from := to.AddDate(0, 0, -s.lookbackDays)

	for _, ref := range refs {
		result, err := s.svc.Sync(ctx, ref.OrganizationID, ref.TrafficSourceID, from, to)
		if err != nil {
			s.logger.Error("scheduled ad spend sync failed",
				"organization_id", ref.OrganizationID,
				"traffic_source_id", ref.TrafficSourceID,
				"error", err,
			)
			continue
		}
		s.logger.Info("scheduled ad spend sync complete",
			"organization_id", ref.OrganizationID,
			"traffic_source_id", ref.TrafficSourceID,
			"records_fetched", result.RecordsFetched,
			"entries_written", result.EntriesWritten,
			"unmatched_external_campaign_ids", len(result.UnmatchedExternalCampaignIDs),
		)
	}
	return len(refs), nil
}

// RunLoop calls RunOnce immediately, then again every interval, until ctx is
// done. It never returns early on a RunOnce error (e.g. a transient DB blip
// listing connections) — that error is logged and the loop waits for its
// next tick rather than exiting, since a scheduler that gives up after one
// bad listing would silently stop syncing everyone's spend until the worker
// process itself restarted.
func (s *Scheduler) RunLoop(ctx context.Context, interval time.Duration) {
	s.tick(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	n, err := s.RunOnce(ctx)
	if err != nil {
		s.logger.Error("scheduled ad spend sync run failed", "error", err)
		return
	}
	s.logger.Info("scheduled ad spend sync run finished", "connections_attempted", n)
}
