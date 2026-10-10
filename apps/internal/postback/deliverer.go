package postback

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

// attemptTimeout bounds a single delivery HTTP call. A network's postback
// endpoint hanging must not stall the whole worker's poll loop.
const attemptTimeout = 10 * time.Second

// HTTPClient is the narrow slice of *http.Client Deliverer needs, so tests
// can substitute a fake without a real network call.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// DefaultConcurrency is how many attempts of one claimed batch run at once.
// One at a time, a partner endpoint that hangs would hold the whole batch
// for attemptTimeout per delivery, and every other organization's
// postbacks in it would wait (tech audit M2).
const DefaultConcurrency = 10

// Deliverer claims due deliveries and attempts them.
type Deliverer struct {
	store       Store
	client      HTTPClient
	attempts    AttemptLogger
	logger      *slog.Logger
	concurrency int
}

func NewDeliverer(store Store, client HTTPClient, attempts AttemptLogger, logger *slog.Logger) *Deliverer {
	return &Deliverer{store: store, client: client, attempts: attempts, logger: logger, concurrency: DefaultConcurrency}
}

// WithConcurrency sets how many attempts run at once (at least 1).
func (d *Deliverer) WithConcurrency(n int) *Deliverer {
	d.concurrency = max(1, n)
	return d
}

// RunOnce claims up to limit due deliveries and attempts them, up to
// d.concurrency at a time, returning how many were claimed once every
// started attempt has finished.
func (d *Deliverer) RunOnce(ctx context.Context, limit int) (int, error) {
	deliveries, err := d.store.ClaimDue(ctx, limit)
	if err != nil {
		return 0, err
	}
	slots := make(chan struct{}, d.concurrency)
	var wg sync.WaitGroup
	for i, del := range deliveries {
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			// Stopping: hand the rest of the claim back instead of leaving
			// it 'processing' (tech audit Q1).
			d.release(ctx, deliveries[i:])
			break
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			// An attempt already started runs to completion on its own
			// deadline — cancelling it mid-request would lose the outcome
			// of a postback the partner may already have received.
			attemptCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), attemptTimeout+bookkeepingTimeout)
			defer cancel()
			d.attempt(attemptCtx, del)
		}()
	}
	wg.Wait()
	return len(deliveries), nil
}

// bookkeepingTimeout bounds the store/audit writes that follow an attempt's
// HTTP call.
const bookkeepingTimeout = 10 * time.Second

func (d *Deliverer) release(ctx context.Context, rest []Delivery) {
	ids := make([]string, len(rest))
	for i, del := range rest {
		ids[i] = del.ID
	}
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), bookkeepingTimeout)
	defer cancel()
	if err := d.store.Release(releaseCtx, ids); err != nil {
		// The reaper (internal/queuereaper) picks them up after StuckAfter.
		d.logger.Error("releasing unattempted deliveries", "error", err, "count", len(ids))
	}
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
		d.logAttempt(ctx, del, StatusSuccess, resp.StatusCode, "")
		return
	}
	d.fail(ctx, del, resp.StatusCode, fmt.Sprintf("network responded %d", resp.StatusCode))
}

func (d *Deliverer) fail(ctx context.Context, del Delivery, statusCode int, message string) {
	if del.AttemptCount >= MaxAttempts {
		if err := d.store.MarkDead(ctx, del.ID, statusCode, message); err != nil {
			d.logger.Error("marking delivery dead", "error", err, "delivery_id", del.ID)
		}
		d.logAttempt(ctx, del, StatusDead, statusCode, message)
		return
	}
	next := time.Now().UTC().Add(NextAttemptDelay(del.AttemptCount))
	if err := d.store.MarkRetrying(ctx, del.ID, statusCode, message, next); err != nil {
		d.logger.Error("marking delivery retrying", "error", err, "delivery_id", del.ID)
	}
	d.logAttempt(ctx, del, StatusRetrying, statusCode, message)
}

// logAttempt is every attempt/fail outcome's one common call site (success,
// retrying, dead) — the natural, single place to record §53's
// postback_deliveries_total{outcome} alongside the existing audit log
// write, rather than duplicating an Inc() call at each of the three
// decision points in attempt/fail above.
func (d *Deliverer) logAttempt(ctx context.Context, del Delivery, result DeliveryStatus, responseStatusCode int, message string) {
	metrics.PostbackDeliveriesTotal.WithLabelValues(string(result)).Inc()
	d.attempts.LogAttempt(ctx, AttemptRecord{
		OrganizationID:     del.OrganizationID,
		NetworkID:          del.NetworkID,
		ClickID:            del.ClickID,
		Status:             del.Status,
		Result:             result,
		Message:            message,
		AttemptCount:       del.AttemptCount,
		ResponseStatusCode: responseStatusCode,
		URL:                del.URL,
		OccurredAt:         time.Now().UTC(),
	})
}
