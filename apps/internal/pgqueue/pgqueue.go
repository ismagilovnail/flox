// Package pgqueue is the batch claim shared by apps/worker's Postgres
// queues that flush into ClickHouse (event_queue, postback_attempt_queue).
//
// A batch is fixed when it is first claimed: its rows get one batch_id, and
// Requeue/the reaper keep it. A retry claims exactly that batch again — never
// re-mixed with fresh rows — because the ClickHouse insert is deduplicated by
// a token derived from batch_id, and one token over different rows would
// make ClickHouse silently drop the new ones (audit 2026-10-06, D2).
package pgqueue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// Row is one claimed queue row.
type Row struct {
	ID      string
	Payload []byte
}

// retryCandidates bounds how many due retry batches one claim looks at
// while another worker holds the first ones.
const retryCandidates = 10

// ClaimBatch claims a due batch from table: a whole retry batch first, so a
// failed insert is retried before newer work, otherwise up to limit fresh
// rows under a new batch id. table must be a queue table name constant,
// never input. An empty result means nothing is due.
func ClaimBatch(ctx context.Context, db *pgxpool.Pool, table string, limit int) (string, []Row, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("pgqueue: beginning claim on %s: %w", table, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	batchID, rows, err := claimRetry(ctx, tx, table)
	if err == nil && len(rows) == 0 {
		batchID, rows, err = claimFresh(ctx, tx, table, limit)
	}
	if err != nil {
		return "", nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", nil, fmt.Errorf("pgqueue: committing claim on %s: %w", table, err)
	}
	return batchID, rows, nil
}

// claimRetry takes one due batch that was claimed before. Two workers must
// never split a batch between them. The row locks of the UPDATE below already
// make a second claimer wait and then find the rows 'processing'; the
// advisory lock on the batch id makes that a guarantee independent of scan
// order, and lets the second worker skip to another batch instead of
// waiting.
func claimRetry(ctx context.Context, tx pgx.Tx, table string) (string, []Row, error) {
	ids, err := queryStrings(ctx, tx, `
		SELECT batch_id FROM `+table+`
		WHERE status = 'queued' AND batch_id IS NOT NULL AND next_attempt_at <= now()
		GROUP BY batch_id
		ORDER BY min(next_attempt_at)
		LIMIT $1`, retryCandidates)
	if err != nil {
		return "", nil, fmt.Errorf("pgqueue: listing retry batches on %s: %w", table, err)
	}
	for _, batchID := range ids {
		var locked bool
		if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1, 0))`,
			table+":"+batchID).Scan(&locked); err != nil {
			return "", nil, fmt.Errorf("pgqueue: locking batch %s: %w", batchID, err)
		}
		if !locked {
			continue
		}
		rows, err := queryRows(ctx, tx, `
			UPDATE `+table+`
			SET status = 'processing', claimed_at = now()
			WHERE batch_id = $1 AND status = 'queued'
			RETURNING id, payload`, batchID)
		if err != nil {
			return "", nil, fmt.Errorf("pgqueue: claiming batch %s on %s: %w", batchID, table, err)
		}
		if len(rows) > 0 {
			return batchID, rows, nil
		}
	}
	return "", nil, nil
}

func claimFresh(ctx context.Context, tx pgx.Tx, table string, limit int) (string, []Row, error) {
	batchID := idgen.New()
	rows, err := queryRows(ctx, tx, `
		WITH due AS (
			SELECT id FROM `+table+`
			WHERE status = 'queued' AND batch_id IS NULL AND next_attempt_at <= now()
			ORDER BY next_attempt_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE `+table+` q
		SET status = 'processing', claimed_at = now(), batch_id = $2
		FROM due
		WHERE q.id = due.id
		RETURNING q.id, q.payload`, limit, batchID)
	if err != nil {
		return "", nil, fmt.Errorf("pgqueue: claiming due rows on %s: %w", table, err)
	}
	if len(rows) == 0 {
		return "", nil, nil
	}
	return batchID, rows, nil
}

func queryRows(ctx context.Context, tx pgx.Tx, sql string, args ...any) ([]Row, error) {
	rs, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []Row
	for rs.Next() {
		var r Row
		if err := rs.Scan(&r.ID, &r.Payload); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rs.Err()
}

func queryStrings(ctx context.Context, tx pgx.Tx, sql string, args ...any) ([]string, error) {
	rs, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []string
	for rs.Next() {
		var s string
		if err := rs.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rs.Err()
}
