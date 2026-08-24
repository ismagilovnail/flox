package postback

import (
	"context"
	"log/slog"

	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/postbacklogs"
)

// Enqueuer adapts Store to conversion.DeliveryEnqueuer, so
// internal/conversion never needs to import this package or know anything
// about the delivery lifecycle — the same decoupling EventSink already has
// from eventbuf.Writer.
type Enqueuer struct {
	store  Store
	logger *slog.Logger
}

func NewEnqueuer(store Store, logger *slog.Logger) *Enqueuer {
	return &Enqueuer{store: store, logger: logger}
}

var _ conversion.DeliveryEnqueuer = (*Enqueuer)(nil)

// Enqueue is best-effort by conversion.DeliveryEnqueuer's contract: a
// conversion that is already durably recorded must not be reported back to
// the network as failed just because queuing its outgoing notification hit
// a database blip.
func (e *Enqueuer) Enqueue(ctx context.Context, req conversion.DeliveryRequest) {
	_, err := e.store.Enqueue(ctx, EnqueueInput{
		OrganizationID:   req.OrganizationID,
		NetworkID:        req.NetworkID,
		SourcePostbackID: req.SourcePostbackID,
		ClickID:          req.ClickID,
		Status:           req.Status,
		URL:              req.URL,
	})
	if err != nil {
		e.logger.Error("enqueueing outgoing postback delivery", "error", err,
			"organization_id", req.OrganizationID, "network_id", req.NetworkID, "click_id", req.ClickID)
	}
}

// ReplayEnqueuer adapts Store to postbacklogs.OutgoingEnqueuer — the
// replay counterpart to Enqueuer above, same decoupling reason: this
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
