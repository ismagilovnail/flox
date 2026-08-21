package postback

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// attemptTimeout bounds a single delivery HTTP call. A network's postback
// endpoint hanging must not stall the whole worker's poll loop.
const attemptTimeout = 10 * time.Second

// HTTPClient is the narrow slice of *http.Client Deliverer needs, so tests
// can substitute a fake without a real network call.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Deliverer claims due deliveries and attempts them.
type Deliverer struct {
	store  Store
	client HTTPClient
	logger *slog.Logger
}

func NewDeliverer(store Store, client HTTPClient, logger *slog.Logger) *Deliverer {
	return &Deliverer{store: store, client: client, logger: logger}
}

// RunOnce claims up to limit due deliveries and attempts each one,
// returning how many were claimed.
func (d *Deliverer) RunOnce(ctx context.Context, limit int) (int, error) {
	deliveries, err := d.store.ClaimDue(ctx, limit)
	if err != nil {
		return 0, err
	}
	for _, del := range deliveries {
		d.attempt(ctx, del)
	}
	return len(deliveries), nil
}

// PollLoop runs RunOnce until ctx is done. A batch that returned a full
// limit's worth of work polls again immediately (draining a backlog
// quickly); an empty or partial batch waits idle before trying again,
// so a quiet queue doesn't spin the worker.
func (d *Deliverer) PollLoop(ctx context.Context, batchSize int, idle time.Duration) {
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := d.RunOnce(ctx, batchSize)
		if err != nil {
			d.logger.Error("postback delivery poll failed", "error", err)
			n = 0 // fall through to the idle wait so a DB blip doesn't spin
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
	reqCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, del.URL, nil)
	if err != nil {
		// A malformed URL will never become valid on retry — but it's
		// still logged in full and dead-lettered rather than silently
		// dropped, so a bad postback_url template shows up somewhere an
		// operator will see it (§45's "log every postback" spirit applies
		// to the outgoing side too).
		d.fail(ctx, del, 0, "invalid delivery URL: "+err.Error())
		return
	}

	resp, err := d.client.Do(req)
	if err != nil {
		d.fail(ctx, del, 0, "request failed: "+err.Error())
		return
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := d.store.MarkSuccess(ctx, del.ID, resp.StatusCode); err != nil {
			d.logger.Error("marking delivery success", "error", err, "delivery_id", del.ID)
		}
		return
	}
	d.fail(ctx, del, resp.StatusCode, fmt.Sprintf("network responded %d", resp.StatusCode))
}

func (d *Deliverer) fail(ctx context.Context, del Delivery, statusCode int, message string) {
	if del.AttemptCount >= MaxAttempts {
		if err := d.store.MarkDead(ctx, del.ID, statusCode, message); err != nil {
			d.logger.Error("marking delivery dead", "error", err, "delivery_id", del.ID)
		}
		return
	}
	next := time.Now().UTC().Add(NextAttemptDelay(del.AttemptCount))
	if err := d.store.MarkRetrying(ctx, del.ID, statusCode, message, next); err != nil {
		d.logger.Error("marking delivery retrying", "error", err, "delivery_id", del.ID)
	}
}
