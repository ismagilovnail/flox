package pixelsend

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/macro"
	"github.com/ismagilovnail/flox/apps/internal/pixel"
)

// trackerTriggered is the tracker-produced events a pixel can report
// (pixel.ValidEventTypes); CPA statuses are queued by the conversion's own
// transaction instead.
// IOS_INSTALL is an install too (docs/pwa-serving.md): it reaches the
// pixels set for PWA_INSTALL, as PWA_INSTALL (pixelType). TG_START and
// TG_JOIN come from the bot's webhook (internal/telegram), a request from
// Telegram's servers: the visitor's IP and user agent are the click's.
var trackerTriggered = map[event.Type]bool{
	event.PwaInstall: true,
	event.IosInstall: true,
	event.TgStart:    true,
	event.TgJoin:     true,
}

// pixelType is the event type a pixel is configured and mapped by.
func pixelType(t event.Type) event.Type {
	if t == event.IosInstall {
		return event.PwaInstall
	}
	return t
}

// Inserter is the ClickHouse event writer the worker's event pipelines
// call (chstore.EventStore).
type Inserter interface {
	InsertBatch(ctx context.Context, events []event.Event) error
}

// TriggerSink wraps the worker's ClickHouse writer: before a batch of
// tracker events is written, every event a pixel reports is queued for
// its stream set's pixels. Queued first, so a failure here fails the batch
// before anything is written and the pipeline retries it whole; a retried
// batch queues nothing twice (one row per pixel and source event).
type TriggerSink struct {
	Next  Inserter
	Queue *PostgresStore
	// Clicks finds the click an event belongs to, for the platform click
	// ids, the time of the click and — when the event has none — the
	// visitor's IP and user agent; nil or a miss sends without them.
	Clicks attribution.ClickResolver
	Logger *slog.Logger
}

func (t *TriggerSink) InsertBatch(ctx context.Context, events []event.Event) error {
	for _, ev := range events {
		if !trackerTriggered[ev.Type] || ev.StreamSetID == "" || ev.ID == "" {
			continue
		}
		if err := t.queue(ctx, ev); err != nil {
			return err
		}
	}
	return t.Next.InsertBatch(ctx, events)
}

func (t *TriggerSink) queue(ctx context.Context, ev event.Event) error {
	typ := pixelType(ev.Type)
	wanted, err := t.Queue.Wanted(ctx, ev.OrganizationID, ev.StreamSetID, typ)
	if err != nil {
		return err
	}
	if !wanted {
		return nil
	}
	pe := pixel.Event{
		OrganizationID: ev.OrganizationID,
		StreamSetID:    ev.StreamSetID,
		SourceEventID:  ev.ID,
		Type:           typ,
		EventAt:        ev.EventAt,
		ClickID:        ev.ClickID,
		IP:             ev.IP,
		UserAgent:      ev.UserAgent,
		FBClickID:      ev.FBClickID,
		TTClickID:      ev.TTClickID,
		Macros: macro.Values{
			ClickID:    ev.ClickID,
			Status:     string(typ),
			CampaignID: ev.CampaignID,
			OfferID:    ev.OfferID,
			Country:    ev.Country,
			Device:     ev.Device,
			Subs:       ev.Subs,
		},
	}
	if t.Clicks != nil && ev.ClickID != "" {
		click, err := t.Clicks.ByClickID(ctx, ev.OrganizationID, ev.ClickID)
		if err == nil {
			pe.ClickAt = click.OccurredAt
			if pe.FBClickID == "" {
				pe.FBClickID = click.FBClickID
			}
			if pe.TTClickID == "" {
				pe.TTClickID = click.TTClickID
			}
			if pe.IP == "" && pe.UserAgent == "" {
				pe.IP, pe.UserAgent = click.IP, click.UserAgent
			}
		} else if t.Logger != nil {
			t.Logger.Debug("pixel event without its click", "event_id", ev.ID, "error", err)
		}
	}
	if _, err := t.Queue.Enqueue(ctx, pe); err != nil {
		return fmt.Errorf("pixelsend: queueing %s: %w", ev.Type, err)
	}
	return nil
}
