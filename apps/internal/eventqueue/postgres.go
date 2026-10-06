package eventqueue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/eventbuf"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/pgqueue"
)

// PostgresQueue implements both Producer and Consumer against event_queue
// (migration 00015).
type PostgresQueue struct {
	db *pgxpool.Pool
}

func NewPostgresQueue(db *pgxpool.Pool) *PostgresQueue { return &PostgresQueue{db: db} }

var (
	_ Producer = (*PostgresQueue)(nil)
	_ Consumer = (*PostgresQueue)(nil)
)

func (q *PostgresQueue) EnqueueBatch(ctx context.Context, events []event.Event) error {
	if len(events) == 0 {
		return nil
	}
	err := q.enqueue(ctx, events)
	if err == nil {
		return nil
	}
	// The batch is one implicit transaction, so on any error nothing was
	// written. A transient failure (Postgres unreachable, a timeout, a
	// failover) is returned as is: eventbuf retries the whole batch.
	if !permanent(err) {
		return err
	}

	// A permanent error comes from one bad row — usually an event whose
	// organization was deleted while it sat in the tracker's buffer
	// (event_queue.organization_id is a foreign key). The buffer mixes
	// organizations, so retry per organization: the others are written, the
	// failing one is dropped for good instead of being retried forever, and
	// eventbuf is told exactly which events to retry (audit 2026-10-06, RL2).
	byOrg := map[string][]event.Event{}
	var order []string
	for _, e := range events {
		if _, ok := byOrg[e.OrganizationID]; !ok {
			order = append(order, e.OrganizationID)
		}
		byOrg[e.OrganizationID] = append(byOrg[e.OrganizationID], e)
	}
	if len(order) < 2 {
		return &eventbuf.SinkError{Dropped: len(events), Err: err}
	}
	out := &eventbuf.SinkError{}
	var errs []error
	for _, org := range order {
		orgErr := q.enqueue(ctx, byOrg[org])
		switch {
		case orgErr == nil:
		case permanent(orgErr):
			out.Dropped += len(byOrg[org])
			errs = append(errs, orgErr)
		default:
			out.Retry = append(out.Retry, byOrg[org]...)
			errs = append(errs, orgErr)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	out.Err = errors.Join(errs...)
	return out
}

// errUnencodable marks an event that cannot be serialized — retrying will
// never help.
var errUnencodable = errors.New("eventqueue: event cannot be encoded")

// permanent reports whether retrying err can never succeed: an integrity or
// data error from Postgres (SQLSTATE class 23 or 22), or an event that does
// not encode. Everything else — connection loss, timeouts, a server
// restarting — is worth retrying.
func permanent(err error) bool {
	if errors.Is(err, errUnencodable) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return strings.HasPrefix(pgErr.Code, "23") || strings.HasPrefix(pgErr.Code, "22")
	}
	return false
}

func (q *PostgresQueue) enqueue(ctx context.Context, events []event.Event) error {
	return insert(ctx, q.db, events)
}

// InsertTx enqueues events inside the caller's transaction — the outbox half
// of a write that must land together with its own row (internal/conversion
// records a conversion and its CPA event in one transaction, so one is never
// committed without the other).
func InsertTx(ctx context.Context, tx pgx.Tx, events []event.Event) error {
	return insert(ctx, tx, events)
}

// batchSender is the part of *pgxpool.Pool and pgx.Tx insert needs.
type batchSender interface {
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

func insert(ctx context.Context, db batchSender, events []event.Event) error {
	if len(events) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, e := range events {
		payload, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("%w: %w", errUnencodable, err)
		}
		batch.Queue(
			`INSERT INTO event_queue (id, organization_id, type, payload) VALUES ($1, $2, $3, $4)`,
			idgen.New(), e.OrganizationID, string(e.Type), payload,
		)
	}

	br := db.SendBatch(ctx, batch)
	defer br.Close()
	for range events {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("eventqueue: enqueueing batch: %w", err)
		}
	}
	return nil
}

func (q *PostgresQueue) ClaimDue(ctx context.Context, limit int) ([]QueuedEvent, error) {
	batchID, rows, err := pgqueue.ClaimBatch(ctx, q.db, "event_queue", limit)
	if err != nil {
		return nil, fmt.Errorf("eventqueue: %w", err)
	}
	out := make([]QueuedEvent, 0, len(rows))
	for _, r := range rows {
		var e event.Event
		if err := json.Unmarshal(r.Payload, &e); err != nil {
			return nil, fmt.Errorf("eventqueue: unmarshaling event %s: %w", r.ID, err)
		}
		out = append(out, QueuedEvent{ID: r.ID, BatchID: batchID, Event: e})
	}
	return out, nil
}

func (q *PostgresQueue) Delete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if _, err := q.db.Exec(ctx, `DELETE FROM event_queue WHERE id = ANY($1)`, ids); err != nil {
		return fmt.Errorf("eventqueue: deleting delivered events: %w", err)
	}
	return nil
}

func (q *PostgresQueue) Requeue(ctx context.Context, ids []string, next time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	if _, err := q.db.Exec(ctx,
		`UPDATE event_queue SET status = 'queued', claimed_at = NULL, next_attempt_at = $2 WHERE id = ANY($1)`,
		ids, next,
	); err != nil {
		return fmt.Errorf("eventqueue: requeueing events: %w", err)
	}
	return nil
}

// Depth counts rows not yet delivered to ClickHouse — both 'queued' (due
// or waiting for next_attempt_at) and 'processing' (claimed by a Flusher,
// mid-InsertBatch) — for §53's event_queue_depth gauge (see PollDepth).
// 'processing' rows are normally claimed and cleared within one RunOnce,
// but counting only 'queued' would undercount the true backlog during a
// slow ClickHouse insert.
func (q *PostgresQueue) Depth(ctx context.Context) (int, error) {
	var n int
	if err := q.db.QueryRow(ctx, `SELECT count(*) FROM event_queue WHERE status IN ('queued', 'processing')`).Scan(&n); err != nil {
		return 0, fmt.Errorf("eventqueue: counting queue depth: %w", err)
	}
	return n, nil
}

// ReapStuck implements queuereaper.Reaper: claims older than olderThan
// belong to a worker that died mid-batch and go back to the queue, due now.
func (q *PostgresQueue) ReapStuck(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := q.db.Exec(ctx, `
		UPDATE event_queue
		SET status = 'queued', claimed_at = NULL, next_attempt_at = now()
		WHERE status = 'processing' AND claimed_at < now() - $1::interval`,
		olderThan,
	)
	if err != nil {
		return 0, fmt.Errorf("eventqueue: reaping stuck claims: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (q *PostgresQueue) Fail(ctx context.Context, ids []string, rejected bool, lastError string) (bool, error) {
	return pgqueue.Fail(ctx, q.db, "event_queue", ids, rejected, lastError)
}
