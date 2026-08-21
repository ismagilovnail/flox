package eventqueue

import (
	"context"

	"github.com/ismagilovnail/flox/apps/internal/event"
)

// Sink adapts a Producer to eventbuf.Sink, so apps/tracker's writer can
// hand it batches without eventbuf importing this package's concrete type
// — the same narrow-interface decoupling internal/conversion.EventSink
// already has from eventbuf.Writer, mirrored here in the other direction.
type Sink struct {
	producer Producer
}

func NewSink(producer Producer) Sink { return Sink{producer: producer} }

func (s Sink) Write(ctx context.Context, batch []event.Event) error {
	return s.producer.EnqueueBatch(ctx, batch)
}
