package postback

import (
	"context"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/postbacklogs"
)

// ReplayEnqueuer adapts Store to postbacklogs.OutgoingEnqueuer, so this
// package's own delivery-lifecycle types (EnqueueInput, event.Type) stay
// out of postbacklogs.
type ReplayEnqueuer struct {
	store Store
}

func NewReplayEnqueuer(store Store) *ReplayEnqueuer {
	return &ReplayEnqueuer{store: store}
}

var _ postbacklogs.OutgoingEnqueuer = (*ReplayEnqueuer)(nil)

// Enqueue queues a brand-new delivery row — replaying an attempt never
// mutates the one being replayed (see postbacklogs.Service.ReplayOutgoing).
// Unlike Enqueuer.Enqueue (best-effort, called from inside
// conversion.Service.Record where a queuing failure must never be
// reported back to the network as a conversion failure), this IS the
// entire point of the HTTP request that called it, so the error goes
// straight back to the caller instead of being logged and swallowed.
func (e *ReplayEnqueuer) Enqueue(ctx context.Context, in postbacklogs.ReplayInput) (string, error) {
	return e.store.Enqueue(ctx, EnqueueInput{
		OrganizationID:   in.OrganizationID,
		NetworkID:        in.NetworkID,
		SourcePostbackID: in.SourcePostbackID,
		ClickID:          in.ClickID,
		Status:           event.Type(in.Status),
		URL:              in.URL,
	})
}
