package postbacklog

import (
	"context"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
)

// ConversionAttemptLogger adapts a Producer to conversion.AttemptLogger —
// internal/conversion never needs to know this package or ClickHouse
// exist, the same decoupling DeliveryEnqueuer already has from
// internal/postback.
type ConversionAttemptLogger struct {
	producer Producer
}

func NewConversionAttemptLogger(producer Producer) ConversionAttemptLogger {
	return ConversionAttemptLogger{producer: producer}
}

var _ conversion.AttemptLogger = ConversionAttemptLogger{}

func (l ConversionAttemptLogger) LogAttempt(ctx context.Context, rec conversion.AttemptRecord) {
	attempt := chstore.PostbackAttempt{
		OrganizationID: rec.OrganizationID,
		EventAt:        rec.OccurredAt,
		Direction:      "incoming",
		NetworkID:      rec.NetworkID,
		ClickID:        rec.ClickID,
		Status:         string(rec.Status),
		EventRef:       rec.EventRef,
		RawStatus:      rec.RawStatus,
		Result:         string(rec.Result),
		Message:        rec.Message,
		Currency:       rec.Currency,
	}
	if rec.Revenue != nil {
		attempt.Revenue = *rec.Revenue
	}
	l.producer.EnqueueAttempt(ctx, attempt)
}
