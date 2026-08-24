package conversion

import (
	"context"
	"errors"

	"github.com/ismagilovnail/flox/apps/internal/postbacklogs"
)

// ReplayNetworkLookup and ReplayRecorder adapt this package's own
// NetworkLookup/*Service to apps/internal/postbacklogs' decoupled
// IncomingNetworkLookup/IncomingRecorder interfaces — the same
// adapter-lives-with-the-engine pattern apps/internal/postback's
// ReplayEnqueuer already uses for outgoing replay (postbacklogs never
// imports conversion or postback directly; both of those import
// postbacklogs instead, one-directionally, to satisfy its interfaces).

// ReplayNetworkLookup adapts a NetworkLookup to
// postbacklogs.IncomingNetworkLookup.
type ReplayNetworkLookup struct {
	lookup NetworkLookup
}

func NewReplayNetworkLookup(lookup NetworkLookup) *ReplayNetworkLookup {
	return &ReplayNetworkLookup{lookup: lookup}
}

var _ postbacklogs.IncomingNetworkLookup = (*ReplayNetworkLookup)(nil)

func (l *ReplayNetworkLookup) ByID(ctx context.Context, networkID string) (postbacklogs.IncomingNetwork, error) {
	n, err := l.lookup.ByID(ctx, networkID)
	if errors.Is(err, ErrNetworkNotFound) {
		return postbacklogs.IncomingNetwork{}, postbacklogs.ErrIncomingNetworkNotFound
	}
	if err != nil {
		return postbacklogs.IncomingNetwork{}, err
	}
	return postbacklogs.IncomingNetwork{
		OrganizationID:   n.OrganizationID,
		AcceptDuplicates: n.AcceptDuplicates,
		PostbackURL:      n.PostbackURL,
	}, nil
}

// ReplayRecorder adapts *Service to postbacklogs.IncomingRecorder — the
// exact same Record call apps/tracker's PostbackHandler makes for a real
// network hit, just fed from a replay's own reconstructed Postback
// instead of a live HTTP request.
type ReplayRecorder struct {
	svc *Service
}

func NewReplayRecorder(svc *Service) *ReplayRecorder {
	return &ReplayRecorder{svc: svc}
}

var _ postbacklogs.IncomingRecorder = (*ReplayRecorder)(nil)

func (r *ReplayRecorder) Record(ctx context.Context, in postbacklogs.IncomingRecord) (postbacklogs.IncomingOutcome, error) {
	result, err := r.svc.Record(ctx, Postback{
		OrganizationID:   in.OrganizationID,
		NetworkID:        in.NetworkID,
		AcceptDuplicates: in.AcceptDuplicates,
		ClickID:          in.ClickID,
		RawStatus:        in.RawStatus,
		NetworkTxnID:     in.NetworkTxnID,
		Revenue:          in.Revenue,
		Currency:         in.Currency,
		PostbackURL:      in.PostbackURL,
		OccurredAt:       in.OccurredAt,
	})
	if err != nil {
		return postbacklogs.IncomingOutcome{}, err
	}
	return postbacklogs.IncomingOutcome{
		ID:      result.ID,
		Result:  string(result.Kind),
		Status:  string(result.Status),
		Message: result.Message,
	}, nil
}
