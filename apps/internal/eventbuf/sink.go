package eventbuf

import (
	"context"
	"log/slog"

	"github.com/ismagilovnail/flox/apps/internal/event"
)

// LogSink is the honest default until the real pipeline exists (§43:
// Tracker → Event Queue → Worker → ClickHouse; the worker is Phase 24 and
// ClickHouse ingestion comes with it). It emits one structured log line
// per event so the tracker is genuinely observable today, and makes no
// pretence of durability — it is explicitly NOT a queue, and events are
// gone when the process exits.
//
// Swapping in the durable queue producer later touches this file only:
// the Writer, the tracker hot path, and the event model all stay as they
// are, because they only ever see the Sink interface.
type LogSink struct {
	Logger *slog.Logger
}

func (s LogSink) Write(ctx context.Context, batch []event.Event) error {
	for _, e := range batch {
		s.Logger.Info("event",
			"type", e.Type,
			"event_at", e.EventAt,
			"organization_id", e.OrganizationID,
			"campaign_id", e.CampaignID,
			"click_id", e.ClickID,
			"stream_set_id", e.StreamSetID,
			"flow_id", e.FlowID,
			"destination", e.Destination,
			"sticky_applied", e.StickyApplied,
			"country", e.Country,
			"device", e.Device,
			"is_bot", e.IsBot,
			// §42's diagnostic: how many of sub1..sub10 actually arrived,
			// so subs-less traffic is measurable rather than invisible.
			"sub_count", e.Subs.SubCount(),
			"filter_reason", e.FilterReason,
		)
	}
	return nil
}

// DiscardSink drops everything. Used by tests that care about the hot
// path's behaviour rather than what the sink did with the batch.
type DiscardSink struct{}

func (DiscardSink) Write(ctx context.Context, batch []event.Event) error { return nil }
