package eventqueue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
//
// The consumer side — claim, delete, requeue, reap, dead-letter, depth — is
// pgqueue.Queue's (tech audit CQ2); this package adds how events get in.
type PostgresQueue struct {
	*pgqueue.Queue[event.Event]
	db *pgxpool.Pool
	// table is event_queue; tests point it at a private copy so the
	// dev-stack worker polling the real queue cannot take their rows.
	table string
}

func NewPostgresQueue(db *pgxpool.Pool) *PostgresQueue {
	return newPostgresQueue(db, "event_queue")
}

func newPostgresQueue(db *pgxpool.Pool, table string) *PostgresQueue {
	return &PostgresQueue{Queue: pgqueue.NewQueue[event.Event](db, table, "eventqueue"), db: db, table: table}
}

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
	// eventbuf is told exactly which events to retry (tech audit RL2).
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
	return insert(ctx, q.db, q.table, events)
}

// InsertTx enqueues events inside the caller's transaction — the outbox half
// of a write that must land together with its own row (internal/conversion
// records a conversion and its CPA event in one transaction, so one is never
// committed without the other).
func InsertTx(ctx context.Context, tx pgx.Tx, events []event.Event) error {
	return insert(ctx, tx, "event_queue", events)
}

// batchSender is the part of *pgxpool.Pool and pgx.Tx insert needs.
type batchSender interface {
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

func insert(ctx context.Context, db batchSender, table string, events []event.Event) error {
	if len(events) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, e := range events {
		payload, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("%w: %w", errUnencodable, err)
		}
		// The event's own id is the row id: a batch written again after a
		// lost reply (eventbuf retries) is a no-op instead of a second copy.
		id := e.ID
		if id == "" {
			id = idgen.New()
		}
		batch.Queue(
			`INSERT INTO `+table+` (id, organization_id, type, payload) VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO NOTHING`,
			id, e.OrganizationID, string(e.Type), payload,
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
