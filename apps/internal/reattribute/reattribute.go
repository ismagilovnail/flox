// Package reattribute looks up the click of an unknown_click conversion
// again (tech audit T1).
//
// A click reaches ClickHouse through eventbuf → event_queue → the worker's
// flush: seconds normally, hours during a backlog or an outage. A postback
// arriving in that window is recorded at once by internal/conversion — its
// CPA event and outgoing postback leave immediately — with a row in
// attribution_retries in the same transaction. This package retries on
// conversion.AttributionRetryDelays. When the click turns up:
//
//  1. the postbacks row gets the attribution (one transaction with step 2);
//  2. the conversion's event, if still waiting in event_queue, is rewritten
//     there — nothing reached ClickHouse yet;
//  3. otherwise its copies in ClickHouse are deleted, the attributed event
//     inserted, and the daily bucket the old copy fed rebuilt.
//
// Step 3 runs only once Postgres is right, so a crash in between leaves a
// retry row that repeats step 3 alone (attributed_at set). The outgoing
// postback has already gone without the click's subs: a second one would
// count the conversion twice at the source, so none is sent. Its pixels are
// different: without a click they had no stream set and got nothing, so the
// attributed conversion is queued for them in step 1's transaction.
package reattribute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

const (
	// lease is how long a claimed row stays invisible to other workers; a
	// worker that dies mid-row hands it back when the lease runs out.
	lease = 5 * time.Minute
	// afterError is the wait after a failure that says nothing about the
	// click (ClickHouse or Postgres down). It does not use up an attempt:
	// the ~7 hours of AttributionRetryDelays are hours the click store
	// answered.
	afterError = 30 * time.Second
	// whileQueued is the wait while the conversion's event is being
	// flushed to ClickHouse.
	whileQueued = 30 * time.Second
)

// Conversions reads the recorded conversion (conversion.PostgresStore).
type Conversions interface {
	StoredSuccess(ctx context.Context, organizationID, id string) (conversion.StoredConversion, error)
}

// Events is the ClickHouse side (chstore.EventStore).
type Events interface {
	ConversionEventCopies(ctx context.Context, organizationID, eventID string, at time.Time) (chstore.StoredEvent, error)
	DeleteConversionEvents(ctx context.Context, organizationID string, eventIDs []string) error
	InsertBatch(ctx context.Context, events []event.Event) error
	RebuildDailyConversionBuckets(ctx context.Context, organizationID string, buckets []chstore.DailyBucket) error
}

// Retrier works through attribution_retries.
type Retrier struct {
	db          *pgxpool.Pool
	conversions Conversions
	attribution *attribution.Service
	events      Events
	// pixels queues a conversion attributed on retry for its click's
	// stream set's pixels; nil sends none.
	pixels conversion.PixelWriter
	logger *slog.Logger
	// retries and queue are attribution_retries and event_queue; tests
	// point them at private copies so the dev worker does not take their
	// rows (testpg.PrivateTable).
	retries, queue string
}

func New(db *pgxpool.Pool, conversions Conversions, clicks attribution.ClickResolver, events Events, logger *slog.Logger) *Retrier {
	return &Retrier{db: db, conversions: conversions, attribution: attribution.NewService(clicks), events: events, logger: logger,
		retries: "attribution_retries", queue: "event_queue"}
}

// WithPixels sets where conversions attributed on retry are queued for
// their pixels (internal/pixelsend).
func (r *Retrier) WithPixels(w conversion.PixelWriter) *Retrier {
	r.pixels = w
	return r
}

// due is one claimed attribution_retries row.
type due struct {
	postbackID, organizationID string
	attempt                    int
	attributed                 bool
}

// RunLoop runs RunOnce every interval until ctx is done.
func (r *Retrier) RunLoop(ctx context.Context, interval time.Duration, batch int) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if _, err := r.RunOnce(ctx, batch); err != nil && ctx.Err() == nil {
			r.logger.Warn("attribution retry run failed", "error", err)
		}
		r.reportPending(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// RunOnce claims up to limit due rows and works each one, returning how
// many it claimed. A row that fails is put back for afterError; that is
// logged, not returned — one bad row must not hold up the rest.
func (r *Retrier) RunOnce(ctx context.Context, limit int) (int, error) {
	rows, err := r.claim(ctx, limit)
	if err != nil {
		return 0, err
	}
	for _, d := range rows {
		if ctx.Err() != nil {
			break // the lease hands the rest back
		}
		if err := r.process(ctx, d); err != nil {
			r.logger.Warn("attribution retry failed, will try again",
				"organization_id", d.organizationID, "postback_id", d.postbackID, "error", err)
			_ = r.reschedule(context.WithoutCancel(ctx), d, d.attempt, afterError)
		}
	}
	return len(rows), nil
}

func (r *Retrier) claim(ctx context.Context, limit int) ([]due, error) {
	rs, err := r.db.Query(ctx, `
		WITH d AS (
			SELECT postback_id FROM `+r.retries+`
			WHERE next_attempt_at <= now()
			ORDER BY next_attempt_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE `+r.retries+` a
		SET next_attempt_at = now() + make_interval(secs => $2)
		FROM d
		WHERE a.postback_id = d.postback_id
		RETURNING a.postback_id, a.organization_id, a.attempt, a.attributed_at IS NOT NULL`,
		limit, lease.Seconds())
	if err != nil {
		return nil, fmt.Errorf("reattribute: claiming due retries: %w", err)
	}
	defer rs.Close()
	var out []due
	for rs.Next() {
		var d due
		if err := rs.Scan(&d.postbackID, &d.organizationID, &d.attempt, &d.attributed); err != nil {
			return nil, fmt.Errorf("reattribute: scanning due retry: %w", err)
		}
		out = append(out, d)
	}
	return out, rs.Err()
}

func (r *Retrier) process(ctx context.Context, d due) error {
	c, err := r.conversions.StoredSuccess(ctx, d.organizationID, d.postbackID)
	if errors.Is(err, conversion.ErrStoredNotFound) {
		return r.finish(ctx, d)
	}
	if err != nil {
		return err
	}

	attr, err := r.attribution.AttributeConversion(ctx, attribution.Conversion{
		OrganizationID: c.OrganizationID,
		ClickID:        c.ClickID,
		Status:         c.Status,
		OccurredAt:     c.EventAt,
	})
	if err != nil {
		return err
	}
	if !attr.Outcome.Attributed() {
		if d.attributed {
			// Postgres has the click but ClickHouse no longer finds it
			// (deleted since): nothing to rebuild the event from.
			r.logger.Warn("attributed click is gone, event left as it is",
				"organization_id", d.organizationID, "postback_id", d.postbackID)
			return r.finish(ctx, d)
		}
		return r.miss(ctx, d)
	}

	c.AttributionOutcome = string(attr.Outcome)
	c.AttributionMethod = string(attr.Method)
	c.AttributedClickID = attr.Click.ClickID
	ms := attr.TimeToConversion.Milliseconds()
	c.TimeToConversionMS = &ms
	ev := conversion.EventFor(c, &attr.Click)

	if !d.attributed {
		done, err := r.attributePostgres(ctx, d, c, attr, ev)
		if err != nil || done {
			return err
		}
	}
	return r.rewriteClickHouse(ctx, d, c, ev)
}

// miss schedules the next lookup, or gives up after the last one.
func (r *Retrier) miss(ctx context.Context, d due) error {
	next := d.attempt + 1
	if next < len(conversion.AttributionRetryDelays) {
		return r.reschedule(ctx, d, next, conversion.AttributionRetryDelays[next])
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("reattribute: beginning give-up tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		UPDATE postbacks SET message = message || $3
		WHERE organization_id = $1 AND id = $2`,
		d.organizationID, d.postbackID,
		fmt.Sprintf("; still no click after %d retries", len(conversion.AttributionRetryDelays))); err != nil {
		return fmt.Errorf("reattribute: noting give-up: %w", err)
	}
	if err := r.deleteRetry(ctx, tx, d); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("reattribute: committing give-up: %w", err)
	}
	metrics.ReattributionsTotal.WithLabelValues("gave_up").Inc()
	return nil
}

// attributePostgres writes the attribution to the postbacks row and, when
// the event has not been flushed yet, into its event_queue row. done means
// nothing is left for ClickHouse.
func (r *Retrier) attributePostgres(ctx context.Context, d due, c conversion.StoredConversion, attr attribution.Attribution, ev event.Event) (done bool, err error) {
	payload, err := json.Marshal(ev)
	if err != nil {
		return false, fmt.Errorf("reattribute: encoding event: %w", err)
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("reattribute: beginning tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		UPDATE postbacks
		SET attribution_outcome = $3, attribution_method = $4, attributed_click_id = $5,
		    time_to_conversion_ms = $6, message = message || $7
		WHERE organization_id = $1 AND id = $2`,
		d.organizationID, d.postbackID, string(attr.Outcome), string(attr.Method), attr.Click.ClickID,
		attr.TimeToConversion.Milliseconds(), "; attributed on retry: "+attr.Reason); err != nil {
		return false, fmt.Errorf("reattribute: updating postback: %w", err)
	}
	// Only a row no worker has claimed: a claimed batch is retried with
	// the same ClickHouse dedup token, and different rows under one token
	// would be dropped (audit D2).
	tag, err := tx.Exec(ctx, `
		UPDATE `+r.queue+` SET payload = $3
		WHERE organization_id = $1 AND id = $2 AND status = 'queued' AND batch_id IS NULL`,
		d.organizationID, d.postbackID, payload)
	if err != nil {
		return false, fmt.Errorf("reattribute: rewriting queued event: %w", err)
	}
	if r.pixels != nil {
		if pe := conversion.PixelEventFor(c, attr.Click); pe != nil {
			if err := r.pixels.InsertTx(ctx, tx, *pe); err != nil {
				return false, fmt.Errorf("reattribute: queueing pixel events: %w", err)
			}
		}
	}
	done = tag.RowsAffected() == 1
	if done {
		err = r.deleteRetry(ctx, tx, d)
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE `+r.retries+` SET attributed_at = now()
			WHERE organization_id = $1 AND postback_id = $2`, d.organizationID, d.postbackID)
	}
	if err != nil {
		return false, fmt.Errorf("reattribute: updating retry: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("reattribute: committing attribution: %w", err)
	}
	if done {
		metrics.ReattributionsTotal.WithLabelValues("attributed").Inc()
	}
	return done, nil
}

// rewriteClickHouse replaces the event's copies in ClickHouse with ev,
// once it has left event_queue.
func (r *Retrier) rewriteClickHouse(ctx context.Context, d due, c conversion.StoredConversion, ev event.Event) error {
	var status string
	err := r.db.QueryRow(ctx, `SELECT status FROM `+r.queue+` WHERE organization_id = $1 AND id = $2`,
		d.organizationID, d.postbackID).Scan(&status)
	switch {
	case err == nil && status == "dead":
		// Dead-lettered: it does not reach ClickHouse on its own. The
		// reconciliation re-emits a missing event from its postbacks row,
		// which carries the attribution now.
		r.logger.Warn("conversion event is dead-lettered, the reconciliation will re-emit it attributed",
			"organization_id", d.organizationID, "postback_id", d.postbackID)
		return r.finish(ctx, d)
	case err == nil:
		// In flight: the flush inserts it, then deletes the row.
		return r.reschedule(ctx, d, d.attempt, whileQueued)
	case !errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf("reattribute: checking event queue: %w", err)
	}

	got, err := r.events.ConversionEventCopies(ctx, d.organizationID, d.postbackID, c.EventAt)
	if err != nil {
		return err
	}
	if got.Copies == 1 && got.CampaignID == ev.CampaignID {
		return r.finish(ctx, d) // rewritten by an earlier run that stopped short
	}
	if got.Copies > 0 {
		if err := r.events.DeleteConversionEvents(ctx, d.organizationID, []string{d.postbackID}); err != nil {
			return err
		}
	}
	// Its own batch id: a new write, not a retry of the batch that carried
	// the old copy.
	if err := r.events.InsertBatch(chstore.WithBatchID(ctx, "reattribute-"+idgen.New()), []event.Event{ev}); err != nil {
		return fmt.Errorf("reattribute: inserting attributed event: %w", err)
	}
	if got.Copies > 0 {
		// The daily aggregate only ever adds on insert: the new copy was
		// counted under its campaign, the deleted one still counts under
		// the old.
		day := c.EventAt.UTC().Truncate(24 * time.Hour)
		if err := r.events.RebuildDailyConversionBuckets(ctx, d.organizationID, []chstore.DailyBucket{
			{CampaignID: got.CampaignID, Type: string(c.Status), Day: day},
		}); err != nil {
			return err
		}
	}
	if err := r.finish(ctx, d); err != nil {
		return err
	}
	metrics.ReattributionsTotal.WithLabelValues("attributed").Inc()
	return nil
}

func (r *Retrier) reschedule(ctx context.Context, d due, attempt int, after time.Duration) error {
	_, err := r.db.Exec(ctx, `
		UPDATE `+r.retries+` SET attempt = $3, next_attempt_at = now() + make_interval(secs => $4)
		WHERE organization_id = $1 AND postback_id = $2`,
		d.organizationID, d.postbackID, attempt, after.Seconds())
	if err != nil {
		return fmt.Errorf("reattribute: rescheduling: %w", err)
	}
	return nil
}

func (r *Retrier) finish(ctx context.Context, d due) error {
	return r.deleteRetry(ctx, r.db, d)
}

type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (r *Retrier) deleteRetry(ctx context.Context, q execer, d due) error {
	if _, err := q.Exec(ctx, `DELETE FROM `+r.retries+` WHERE organization_id = $1 AND postback_id = $2`,
		d.organizationID, d.postbackID); err != nil {
		return fmt.Errorf("reattribute: deleting retry: %w", err)
	}
	return nil
}

func (r *Retrier) reportPending(ctx context.Context) {
	var n int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM `+r.retries).Scan(&n); err != nil {
		if ctx.Err() == nil {
			r.logger.Warn("counting attribution retries", "error", err)
		}
		return
	}
	metrics.AttributionRetriesPending.Set(float64(n))
}
