package pgqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Item is one claimed row, its payload decoded.
type Item[T any] struct {
	ID string
	// BatchID is shared by every row claimed together and kept across
	// retries; the ClickHouse insert is deduplicated by it.
	BatchID string
	Value   T
}

// Queue is the consumer side of a Postgres queue table whose rows carry a
// JSON payload of T — event_queue (event.Event) and postback_attempt_queue
// (chstore.PostbackAttempt). Claim, delete, requeue, reap and dead-letter
// live here once instead of once per queue (tech audit CQ2); each queue's
// package keeps only how rows get in.
type Queue[T any] struct {
	db    *pgxpool.Pool
	table string
	// name prefixes errors: the queue's package name.
	name string
	// rot carries the fair claim's starting organization between claims.
	rot Rotation
}

// NewQueue serves table, a queue table name constant — never input.
func NewQueue[T any](db *pgxpool.Pool, table, name string) *Queue[T] {
	return &Queue[T]{db: db, table: table, name: name}
}

// Table is the table the queue serves.
func (q *Queue[T]) Table() string { return q.table }

// ClaimDue claims one batch (ClaimBatch) and decodes its payloads.
func (q *Queue[T]) ClaimDue(ctx context.Context, limit int) ([]Item[T], error) {
	batchID, rows, err := ClaimBatchRotating(ctx, q.db, q.table, limit, &q.rot)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", q.name, err)
	}
	out := make([]Item[T], 0, len(rows))
	for _, r := range rows {
		var v T
		if err := json.Unmarshal(r.Payload, &v); err != nil {
			return nil, fmt.Errorf("%s: unmarshaling row %s: %w", q.name, r.ID, err)
		}
		out = append(out, Item[T]{ID: r.ID, BatchID: batchID, Value: v})
	}
	return out, nil
}

// Delete removes rows once their batch is durably in ClickHouse.
func (q *Queue[T]) Delete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if _, err := q.db.Exec(ctx, `DELETE FROM `+q.table+` WHERE id = ANY($1)`, ids); err != nil {
		return fmt.Errorf("%s: deleting flushed rows: %w", q.name, err)
	}
	return nil
}

// Requeue puts claimed rows back to 'queued', due at next.
func (q *Queue[T]) Requeue(ctx context.Context, ids []string, next time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	if _, err := q.db.Exec(ctx,
		`UPDATE `+q.table+` SET status = 'queued', claimed_at = NULL, next_attempt_at = $2 WHERE id = ANY($1)`,
		ids, next,
	); err != nil {
		return fmt.Errorf("%s: requeueing rows: %w", q.name, err)
	}
	return nil
}

// Depth counts rows not yet in ClickHouse: queued and processing (a slow
// insert would otherwise hide its batch from the backlog).
func (q *Queue[T]) Depth(ctx context.Context) (int, error) {
	var n int
	if err := q.db.QueryRow(ctx, `SELECT count(*) FROM `+q.table+` WHERE status IN ('queued', 'processing')`).Scan(&n); err != nil {
		return 0, fmt.Errorf("%s: counting queue depth: %w", q.name, err)
	}
	return n, nil
}

// ReapStuck implements queuereaper.Reaper: claims older than olderThan
// belong to a worker that died mid-batch and go back to the queue, due now.
func (q *Queue[T]) ReapStuck(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := q.db.Exec(ctx, `
		UPDATE `+q.table+`
		SET status = 'queued', claimed_at = NULL, next_attempt_at = now()
		WHERE status = 'processing' AND claimed_at < now() - $1::interval`,
		olderThan,
	)
	if err != nil {
		return 0, fmt.Errorf("%s: reaping stuck claims: %w", q.name, err)
	}
	return tag.RowsAffected(), nil
}

// Fail records a failed insert for a claimed batch (the package's Fail).
func (q *Queue[T]) Fail(ctx context.Context, ids []string, rejected bool, lastError string) (bool, error) {
	return Fail(ctx, q.db, q.table, ids, rejected, lastError)
}
