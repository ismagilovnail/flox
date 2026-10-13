// Package pushsend sends push broadcasts (docs/push.md): the worker claims
// due push_deliveries (queued by apps/api when a broadcast is sent), sends
// each through a Sender (§74 PushProvider — Web Push with VAPID), and
// records the outcome on the delivery and its broadcast. A push service
// answering that the subscription is gone ends it and records
// NOTIFICATION_UNSUBSCRIBE on the subscriber's click.
package pushsend

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/postback"
)

const (
	attemptTimeout     = 10 * time.Second
	bookkeepingTimeout = 10 * time.Second
	// DefaultConcurrency is how many sends run at once.
	DefaultConcurrency = 20
	// MaxAttempts: a notification is worth sending for hours, not a day —
	// fewer tries than a postback.
	MaxAttempts = 4
)

// Subscriber is who a notification goes to.
type Subscriber struct {
	Endpoint, P256dh, Auth string
}

// Sender is the push provider (§74). Send returns the push service's HTTP
// status; err is set only when no response came.
type Sender interface {
	Send(ctx context.Context, to Subscriber, payload []byte) (status int, err error)
}

// EventWriter records the events a send produces (chstore.EventStore).
type EventWriter interface {
	InsertBatch(ctx context.Context, events []event.Event) error
}

// Queue is the slice of PostgresStore the Deliverer uses.
type Queue interface {
	ClaimDue(ctx context.Context, limit int) ([]Delivery, error)
	Release(ctx context.Context, ids []string) error
	finish(ctx context.Context, d Delivery, res Result, code int, message string, next time.Time) (bool, error)
}

type Deliverer struct {
	queue       Queue
	sender      Sender
	events      EventWriter
	logger      *slog.Logger
	concurrency int
	now         func() time.Time
}

func NewDeliverer(queue *PostgresStore, sender Sender, events EventWriter, logger *slog.Logger) *Deliverer {
	return &Deliverer{queue: queue, sender: sender, events: events, logger: logger, concurrency: DefaultConcurrency, now: time.Now}
}

// Payload is what the service worker shows (apps/tracker pwa.go sw.js).
type Payload struct {
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
	Icon  string `json:"icon,omitempty"`
	Image string `json:"image,omitempty"`
	URL   string `json:"url"`
}

func payloadOf(d Delivery) ([]byte, error) {
	return json.Marshal(Payload{
		Title: d.Title, Body: d.Body, Icon: d.IconURL, Image: d.ImageURL,
		URL: d.Origin + "/pwa/" + d.LinkToken + "/push/click/" + d.ID,
	})
}

// RunOnce claims up to limit due deliveries and sends them.
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
		d.logger.Error("releasing unattempted push deliveries", "error", err, "count", len(ids))
	}
}

// PollLoop runs RunOnce until ctx is done.
func (d *Deliverer) PollLoop(ctx context.Context, batchSize int, idle time.Duration) {
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := d.RunOnce(ctx, batchSize)
		if err != nil {
			d.logger.Error("push delivery poll failed", "error", err)
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
	payload, err := payloadOf(del)
	if err != nil {
		d.record(ctx, del, ResultFailed, 0, fmt.Sprintf("encoding notification: %v", err))
		return
	}
	sendCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
	code, err := d.sender.Send(sendCtx, Subscriber{Endpoint: del.Endpoint, P256dh: del.P256dh, Auth: del.Auth}, payload)
	cancel()
	res, message := Classify(code, err)
	d.record(ctx, del, res, code, message)
}

// Classify maps a push service's answer to an outcome: 2xx delivered;
// 404/410 the subscription is gone; no answer, 429 or 5xx retried; any
// other 4xx (a malformed message, a key the service rejects) cannot
// succeed by repeating it.
func Classify(code int, err error) (Result, string) {
	switch {
	case err != nil:
		return ResultRetry, "request failed: " + err.Error()
	case code >= 200 && code < 300:
		return ResultDelivered, ""
	case code == 404 || code == 410:
		return ResultGone, "subscription expired"
	case code == 429 || code >= 500:
		return ResultRetry, fmt.Sprintf("push service answered %d", code)
	default:
		return ResultFailed, fmt.Sprintf("push service refused it (%d)", code)
	}
}

func (d *Deliverer) record(ctx context.Context, del Delivery, res Result, code int, message string) {
	var next time.Time
	if res == ResultRetry {
		if del.AttemptCount >= MaxAttempts {
			res = ResultFailed
		} else {
			next = d.now().UTC().Add(postback.NextAttemptDelay(del.AttemptCount))
		}
	}
	ended, err := d.queue.finish(ctx, del, res, code, message, next)
	if err != nil {
		d.logger.Error("recording push delivery", "error", err, "delivery_id", del.ID)
		return
	}
	metrics.PushDeliveriesTotal.WithLabelValues(string(res)).Inc()
	if ended {
		ev := del.Claims.Event(event.NotificationUnsubscribe, d.now().UTC())
		ev.Country, ev.Device, ev.Language = del.Country, del.Device, del.Language
		if err := d.events.InsertBatch(ctx, []event.Event{ev}); err != nil {
			d.logger.Error("recording push unsubscribe", "error", err, "subscription_id", del.SubscriptionID)
		}
	}
	if res == ResultFailed || res == ResultRetry {
		// Never the endpoint: it is a capability to message the visitor.
		d.logger.Warn("push delivery failed", "delivery_id", del.ID, "broadcast_id", del.BroadcastID,
			"result", res, "attempt", del.AttemptCount, "error", message)
	}
}
