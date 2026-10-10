package eventbuf

import (
	"context"
	"log/slog"

	"github.com/ismagilovnail/flox/apps/internal/event"
)

// LogSink emits one structured log line per event. It is NOT durable:
// events are gone when the process exits.
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
			"network_id", e.NetworkID,
			"revenue", e.Revenue,
			"currency", e.Currency,
			"usd_value", e.USDValue,
			"has_usd_value", e.HasUSDValue,
			"event_ref", e.EventRef,
			"attribution_outcome", e.AttributionOutcome,
		)
	}
	return nil
}

// DiscardSink drops everything. Used by tests that care about the hot
// path's behaviour rather than what the sink did with the batch.
type DiscardSink struct{}

func (DiscardSink) Write(ctx context.Context, batch []event.Event) error { return nil }
