package postbacklogs

import (
	"context"
	"errors"
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

// SourcePostbackLookup resolves the Postgres postbacks row a successful
// incoming attempt was recorded as, given its exact dedup key plus
// network_id — satisfied structurally by
// apps/internal/conversion.PostgresStore.FindSuccessID, the same
// decoupling pattern Repository above uses.
type SourcePostbackLookup interface {
	FindSuccessID(ctx context.Context, organizationID, networkID, clickID, status, eventRef string) (id string, found bool, err error)
}

// OutgoingEnqueuer is the narrow slice of apps/internal/postback.Store this
// package needs to replay a delivery — its own local ReplayInput, not
// apps/internal/postback.EnqueueInput, so this package never imports that
// one directly. The same decoupled-interface-per-consumer pattern
// apps/internal/conversion's DeliveryEnqueuer already uses; satisfied by
// apps/internal/postback.ReplayEnqueuer.
type OutgoingEnqueuer interface {
	Enqueue(ctx context.Context, in ReplayInput) (id string, err error)
}

// ReplayInput is what OutgoingEnqueuer.Enqueue persists — a fresh delivery
// row, not a mutation of whatever attempt is being replayed.
type ReplayInput struct {
	OrganizationID   string
	NetworkID        string
	SourcePostbackID string
	ClickID          string
	Status           string
	URL              string
}

// ErrIncomingNetworkNotFound is IncomingNetworkLookup's not-found sentinel
// — this package's own, so it never needs to import
// apps/internal/conversion just to recognize
// conversion.ErrNetworkNotFound; the adapter in
// apps/internal/conversion/replay.go translates between the two.
var ErrIncomingNetworkNotFound = errors.New("postbacklogs: network not found")

// IncomingNetworkLookup resolves a network id to the organization that
// owns it — the same tenant-scoping source apps/tracker's own postback
// handler uses for a real network hit (CLAUDE.md #5). Needed here because,
// unlike a real hit, ReplayIncoming arrives already inside an
// authenticated tenant session: the looked-up network's OrganizationID is
// checked against the caller's own before anything is recorded, so a
// replay can never be pointed at another org's network merely by knowing
// its id.
type IncomingNetworkLookup interface {
	ByID(ctx context.Context, networkID string) (IncomingNetwork, error)
}

// IncomingNetwork is just enough of a looked-up network for this package's
// own tenant check, plus the two fields a real postback also gets from the
// network (AcceptDuplicates, PostbackURL) — a local type, not
// apps/internal/conversion.Network, so this package still never imports
// conversion.
type IncomingNetwork struct {
	OrganizationID   string
	AcceptDuplicates bool
	PostbackURL      string
}

// IncomingRecord is what IncomingRecorder.Record persists — the same
// shape apps/internal/conversion.Postback carries, expressed as this
// package's own type so it never imports conversion directly.
type IncomingRecord struct {
	OrganizationID   string
	NetworkID        string
	AcceptDuplicates bool
	ClickID          string
	RawStatus        string
	NetworkTxnID     string
	Revenue          *float64
	Currency         string
	PostbackURL      string
	OccurredAt       time.Time
}

// IncomingOutcome is IncomingRecorder.Record's result — mirrors
// apps/internal/conversion.Result's three JSON-relevant fields.
type IncomingOutcome struct {
	ID      string
	Result  string
	Status  string
	Message string
}

// IncomingRecorder is the narrow slice of
// apps/internal/conversion.Service this package needs to replay an
// incoming postback attempt — the same decoupled-interface,
// no-conversion-import pattern as SourcePostbackLookup/OutgoingEnqueuer
// above; satisfied by apps/internal/conversion.ReplayRecorder.
type IncomingRecorder interface {
	Record(ctx context.Context, in IncomingRecord) (IncomingOutcome, error)
}

type Service struct {
	repo             Repository
	sourceLookup     SourcePostbackLookup
	deliveries       OutgoingEnqueuer
	incomingNetworks IncomingNetworkLookup
	incoming         IncomingRecorder
}

// NewService's last four arguments are nil-able: a deployment that hasn't
// wired replay yet (or a test exercising List alone) still gets a working
// read-only Service — ReplayOutgoing/ReplayIncoming on a nil-dependency
// Service fail loudly instead of panicking (see below), not silently.
func NewService(repo Repository, sourceLookup SourcePostbackLookup, deliveries OutgoingEnqueuer, incomingNetworks IncomingNetworkLookup, incoming IncomingRecorder) *Service {
	return &Service{repo: repo, sourceLookup: sourceLookup, deliveries: deliveries, incomingNetworks: incomingNetworks, incoming: incoming}
}

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
			Status: r.Status, RawStatus: r.RawStatus, EventRef: r.EventRef, Result: r.Result, Message: r.Message,
			AttemptCount: r.AttemptCount, ResponseStatusCode: r.ResponseStatusCode, URL: r.URL,
			Revenue: r.Revenue, Currency: r.Currency,
		}
	}
	return ListResult{Logs: logs, Total: total}, nil
}

// ReplayOutgoing re-enqueues a fresh delivery for a past outgoing attempt
// — the exact same apps/internal/postback.Store.Enqueue path a first
// attempt already takes, so apps/worker's Deliverer picks it up, attempts
// it, and logs the outcome the normal way. It never mutates the attempt
// being replayed; a "dead" row stays dead in the log, and the replay shows
// up as its own new row once the worker gets to it.
//
// The one lookup this needs — resolving in.ClickID/in.Status/in.EventRef
// back to the Postgres postbacks row that originally triggered a delivery
// — exists because apps/internal/postback's delivery table has a NOT NULL
// source_postback_id FK (migration 00014); a first attempt already knows
// this id in-process (apps/internal/conversion.Service.Record has just
// written it), but a replay, happening long after, does not.
func (s *Service) ReplayOutgoing(ctx context.Context, organizationID string, in ReplayOutgoingInput) (ReplayOutgoingResult, error) {
	if s.sourceLookup == nil || s.deliveries == nil {
		return ReplayOutgoingResult{}, fmt.Errorf("postbacklogs: outgoing replay not configured")
	}
	if organizationID == "" {
		return ReplayOutgoingResult{}, apierror.Validation("missing organization", nil)
	}
	fields := map[string]string{}
	if in.NetworkID == "" {
		fields["networkId"] = "required"
	}
	if in.ClickID == "" {
		fields["clickId"] = "required"
	}
	if in.Status == "" {
		fields["status"] = "required"
	}
	if in.URL == "" {
		fields["url"] = "required"
	}
	if len(fields) > 0 {
		return ReplayOutgoingResult{}, apierror.Validation("missing required field(s)", fields)
	}

	sourceID, found, err := s.sourceLookup.FindSuccessID(ctx, organizationID, in.NetworkID, in.ClickID, in.Status, in.EventRef)
	if err != nil {
		return ReplayOutgoingResult{}, err
	}
	if !found {
		return ReplayOutgoingResult{}, apierror.NotFound("no successful conversion found to replay this delivery against")
	}

	id, err := s.deliveries.Enqueue(ctx, ReplayInput{
		OrganizationID:   organizationID,
		NetworkID:        in.NetworkID,
		SourcePostbackID: sourceID,
		ClickID:          in.ClickID,
		Status:           in.Status,
		URL:              in.URL,
	})
	if err != nil {
		return ReplayOutgoingResult{}, err
	}
	return ReplayOutgoingResult{DeliveryID: id}, nil
}

// ReplayIncoming re-runs one incoming postback attempt through the exact
// same conversion-recording path apps/tracker's own /postback/{networkId}
// handler uses for a real network hit — the same Record call, just
// triggered by an operator off a past log row's own fields instead of a
// live HTTP request from the network. Dedup/status-progression rules
// apply exactly as they would for a genuine retry: a re-submit of an
// already-successful row correctly comes back "duplicate", not a special
// case, and this can itself insert a brand-new attempt row and, on
// success, trigger the same downstream event emission and
// outgoing-delivery enqueue a first success would.
func (s *Service) ReplayIncoming(ctx context.Context, organizationID string, in ReplayIncomingInput) (ReplayIncomingResult, error) {
	if s.incomingNetworks == nil || s.incoming == nil {
		return ReplayIncomingResult{}, fmt.Errorf("postbacklogs: incoming replay not configured")
	}
	if organizationID == "" {
		return ReplayIncomingResult{}, apierror.Validation("missing organization", nil)
	}
	fields := map[string]string{}
	if in.NetworkID == "" {
		fields["networkId"] = "required"
	}
	if in.ClickID == "" {
		fields["clickId"] = "required"
	}
	if in.RawStatus == "" {
		fields["rawStatus"] = "required"
	}
	if len(fields) > 0 {
		return ReplayIncomingResult{}, apierror.Validation("missing required field(s)", fields)
	}

	network, err := s.incomingNetworks.ByID(ctx, in.NetworkID)
	if errors.Is(err, ErrIncomingNetworkNotFound) {
		return ReplayIncomingResult{}, apierror.NotFound("network not found")
	}
	if err != nil {
		return ReplayIncomingResult{}, err
	}
	// Never confirm another org's network id exists (CLAUDE.md #5) — same
	// response as a genuine not-found.
	if network.OrganizationID != organizationID {
		return ReplayIncomingResult{}, apierror.NotFound("network not found")
	}

	outcome, err := s.incoming.Record(ctx, IncomingRecord{
		OrganizationID:   organizationID,
		NetworkID:        in.NetworkID,
		AcceptDuplicates: network.AcceptDuplicates,
		ClickID:          in.ClickID,
		RawStatus:        in.RawStatus,
		NetworkTxnID:     in.EventRef,
		Revenue:          in.Revenue,
		Currency:         in.Currency,
		PostbackURL:      network.PostbackURL,
		OccurredAt:       time.Now().UTC(),
	})
	if err != nil {
		return ReplayIncomingResult{}, err
	}
	return ReplayIncomingResult{ID: outcome.ID, Result: outcome.Result, Status: outcome.Status, Message: outcome.Message}, nil
}
