package postback

import (
	"context"
	"log/slog"

	"github.com/ismagilovnail/flox/apps/internal/conversion"
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
