package postbacklog

import (
	"context"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/postback"
)

// DeliveryAttemptLogger adapts a Producer to postback.AttemptLogger —
// internal/postback never needs to know this package or ClickHouse exist,
// the same decoupling ConversionAttemptLogger gives internal/conversion.
type DeliveryAttemptLogger struct {
	producer Producer
}

func NewDeliveryAttemptLogger(producer Producer) DeliveryAttemptLogger {
	return DeliveryAttemptLogger{producer: producer}
}

var _ postback.AttemptLogger = DeliveryAttemptLogger{}

func (l DeliveryAttemptLogger) LogAttempt(ctx context.Context, rec postback.AttemptRecord) {
	l.producer.EnqueueAttempt(ctx, chstore.PostbackAttempt{
		OrganizationID:     rec.OrganizationID,
		EventAt:            rec.OccurredAt,
		Direction:          "outgoing",
		NetworkID:          rec.NetworkID,
		ClickID:            rec.ClickID,
		Status:             string(rec.Status),
		Result:             string(rec.Result),
		Message:            rec.Message,
		AttemptCount:       int64(rec.AttemptCount),
		ResponseStatusCode: int64(rec.ResponseStatusCode),
		URL:                rec.URL,
	})
}
