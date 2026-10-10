package eventbuf

import (
	"context"

	"github.com/ismagilovnail/flox/apps/internal/event"
)

// DiscardSink drops everything. Used by tests that care about the hot
// path's behaviour rather than what the sink did with the batch.
type DiscardSink struct{}

func (DiscardSink) Write(ctx context.Context, batch []event.Event) error { return nil }
