// Package pixelsend sends FLOX events to the pixels of the stream set a
// click went through (§29, §74 PixelProvider; docs/pixels.md "Sending").
//
// An event is queued in pixel_deliveries (migration 00069), one row per
// matching pixel: a conversion inside the transaction that records it
// (conversion.PixelWriter), a tracker event (PWA_INSTALL) by the worker as
// it takes the event off the stream (TriggerSink). The worker's Deliverer
// claims due rows and sends each through the provider's Sender —
// server-side, off every request path (CLAUDE.md #9) — retrying with the
// postback schedule and dead-lettering what cannot succeed.
package pixelsend

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/pixel"
	"github.com/ismagilovnail/flox/apps/internal/postback"
)

// Status is a delivery's state.
type Status string

const (
	StatusQueued     Status = "queued"
	StatusProcessing Status = "processing"
	StatusSuccess    Status = "success"
	StatusRetrying   Status = "retrying"
	StatusDead       Status = "dead"
)

// attemptTimeout bounds one send; bookkeepingTimeout the writes after it.
const (
	attemptTimeout     = 10 * time.Second
	bookkeepingTimeout = 10 * time.Second
)

// DefaultConcurrency is how many sends of one batch run at once, so one
// platform that hangs does not hold every other organization's events.
const DefaultConcurrency = 10

// CredentialsSource reads a pixel for sending (pixel.Repository).
type CredentialsSource interface {
	Credentials(ctx context.Context, orgID, id string) (pixel.Credentials, error)
}

// Queue is the slice of PostgresStore the Deliverer uses.
type Queue interface {
	ClaimDue(ctx context.Context, limit int) ([]Delivery, error)
	Release(ctx context.Context, ids []string) error
	finish(ctx context.Context, id string, status Status, code int, message string, next time.Time) error
}

// Deliverer claims due deliveries and sends them.
type Deliverer struct {
	queue       Queue
	pixels      CredentialsSource
	senders     map[pixel.Provider]Sender
	logger      *slog.Logger
	concurrency int
	now         func() time.Time
}

func NewDeliverer(queue *PostgresStore, pixels CredentialsSource, senders map[pixel.Provider]Sender, logger *slog.Logger) *Deliverer {
	return &Deliverer{queue: queue, pixels: pixels, senders: senders, logger: logger, concurrency: DefaultConcurrency, now: time.Now}
}

// RunOnce claims up to limit due deliveries and sends them, returning how
// many were claimed once every started send has finished.
func (d *Deliverer) RunOnce(ctx context.Context, limit int) (int, error) {
	deliveries, err := d.queue.ClaimDue(ctx, limit)
	if err != nil {
		return 0, err
	}
	slots := make(chan struct{}, max(1, d.concurrency))
	var wg sync.WaitGroup
	for i, del := range deliveries {
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			d.release(ctx, deliveries[i:])
			break
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			// A send already started finishes on its own deadline: the
			// platform may already have the event.
			attemptCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), attemptTimeout+bookkeepingTimeout)
			defer cancel()
			d.attempt(attemptCtx, del)
		}()
	}
	wg.Wait()
	return len(deliveries), nil
}

func (d *Deliverer) release(ctx context.Context, rest []Delivery) {
	ids := make([]string, len(rest))
	for i, del := range rest {
		ids[i] = del.ID
	}
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), bookkeepingTimeout)
	defer cancel()
	if err := d.queue.Release(releaseCtx, ids); err != nil {
		// The reaper (internal/queuereaper) picks them up after StuckAfter.
		d.logger.Error("releasing unattempted pixel deliveries", "error", err, "count", len(ids))
	}
}

// PollLoop runs RunOnce until ctx is done: again at once after a full
// batch, after idle otherwise.
func (d *Deliverer) PollLoop(ctx context.Context, batchSize int, idle time.Duration) {
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := d.RunOnce(ctx, batchSize)
		if err != nil {
			d.logger.Error("pixel delivery poll failed", "error", err)
			n = 0
		}
		if n == batchSize {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(idle):
		}
	}
}

func (d *Deliverer) attempt(ctx context.Context, del Delivery) {
	creds, err := d.pixels.Credentials(ctx, del.OrganizationID, del.PixelID)
	var notFound *apierror.Error
	switch {
	case errors.As(err, &notFound) && notFound.Status == 404:
		// Deleted after queuing: its rows went with it (ON DELETE CASCADE),
		// this one included — nothing left to record.
		return
	case err != nil:
		d.record(ctx, del, "", Outcome{Err: fmt.Errorf("reading pixel: %w", err), Retry: true})
		return
	}
	if creds.Status != pixel.StatusActive {
		d.record(ctx, del, creds.Provider, Outcome{Err: fmt.Errorf("pixel is %s: not sent", creds.Status)})
		return
	}
	sender, ok := d.senders[creds.Provider]
	if !ok {
		d.record(ctx, del, creds.Provider, Outcome{Err: fmt.Errorf("provider %s cannot send events", creds.Provider)})
		return
	}
	sendCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()
	d.record(ctx, del, creds.Provider, sender.Send(sendCtx, creds, del.Event))
}

// record stores the outcome: success; a retryable failure retried on the
// postback schedule until postback.MaxAttempts, then dead; any other
// failure dead at once.
func (d *Deliverer) record(ctx context.Context, del Delivery, provider pixel.Provider, out Outcome) {
	status, message := StatusSuccess, ""
	var next time.Time
	if out.Err != nil {
		message = out.Err.Error()
		status = StatusDead
		if out.Retry && del.AttemptCount < postback.MaxAttempts {
			status = StatusRetrying
			next = d.now().UTC().Add(postback.NextAttemptDelay(del.AttemptCount))
		}
	}
	if err := d.queue.finish(ctx, del.ID, status, out.StatusCode, message, next); err != nil {
		d.logger.Error("recording pixel delivery", "error", err, "delivery_id", del.ID)
	}
	metrics.PixelDeliveriesTotal.WithLabelValues(string(provider), string(status)).Inc()
	if status != StatusSuccess {
		// Never the event itself: it carries the visitor's IP and user agent.
		d.logger.Warn("pixel delivery failed", "delivery_id", del.ID, "pixel_id", del.PixelID,
			"provider", provider, "status", status, "attempt", del.AttemptCount, "error", message)
	}
}
