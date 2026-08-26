package eventqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
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

	batch := &pgx.Batch{}
	for _, e := range events {
		payload, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("eventqueue: marshaling event: %w", err)
		}
		batch.Queue(
			`INSERT INTO event_queue (id, organization_id, type, payload) VALUES ($1, $2, $3, $4)`,
			idgen.New(), e.OrganizationID, string(e.Type), payload,
		)
	}

	br := q.db.SendBatch(ctx, batch)
	defer br.Close()
	for range events {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("eventqueue: enqueueing batch: %w", err)
		}
	}
	return nil
}

func (q *PostgresQueue) ClaimDue(ctx context.Context, limit int) ([]QueuedEvent, error) {
	rows, err := q.db.Query(ctx, `
		WITH due AS (
			SELECT id FROM event_queue
			WHERE status = 'queued' AND next_attempt_at <= now()
			ORDER BY next_attempt_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE event_queue eq
		SET status = 'processing'
		FROM due
		WHERE eq.id = due.id
		RETURNING eq.id, eq.payload`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("eventqueue: claiming due events: %w", err)
	}
	defer rows.Close()

	var out []QueuedEvent
	for rows.Next() {
		var id string
		var payload []byte
		if err := rows.Scan(&id, &payload); err != nil {
			return nil, fmt.Errorf("eventqueue: scanning claimed event: %w", err)
		}
		var e event.Event
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, fmt.Errorf("eventqueue: unmarshaling event %s: %w", id, err)
		}
		out = append(out, QueuedEvent{ID: id, Event: e})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("eventqueue: reading claimed events: %w", err)
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
		`UPDATE event_queue SET status = 'queued', next_attempt_at = $2 WHERE id = ANY($1)`,
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
