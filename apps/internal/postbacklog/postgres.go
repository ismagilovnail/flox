package postbacklog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/pgqueue"
)

// PostgresQueue implements both Producer and Consumer against
// postback_attempt_queue (migration 00016).
type PostgresQueue struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

// NewPostgresQueue builds a Producer/Consumer. logger is used only by the
// Producer half (EnqueueAttempt has no error return, so a failed insert can
// only be logged, never propagated) — see the package doc for why.
func NewPostgresQueue(db *pgxpool.Pool, logger *slog.Logger) *PostgresQueue {
	return &PostgresQueue{db: db, logger: logger}
}

var (
	_ Producer = (*PostgresQueue)(nil)
	_ Consumer = (*PostgresQueue)(nil)
)

func (q *PostgresQueue) EnqueueAttempt(ctx context.Context, attempt chstore.PostbackAttempt) {
	payload, err := json.Marshal(attempt)
	if err != nil {
		q.logger.Error("postbacklog: marshaling attempt", "error", err)
		return
	}
	if _, err := q.db.Exec(ctx,
		`INSERT INTO postback_attempt_queue (id, organization_id, payload) VALUES ($1, $2, $3)`,
		idgen.New(), attempt.OrganizationID, payload,
	); err != nil {
		q.logger.Error("postbacklog: enqueueing attempt", "error", err,
			"organization_id", attempt.OrganizationID, "direction", attempt.Direction, "click_id", attempt.ClickID)
	}
}

func (q *PostgresQueue) ClaimDue(ctx context.Context, limit int) ([]QueuedAttempt, error) {
	batchID, rows, err := pgqueue.ClaimBatch(ctx, q.db, "postback_attempt_queue", limit)
	if err != nil {
		return nil, fmt.Errorf("postbacklog: %w", err)
	}
	out := make([]QueuedAttempt, 0, len(rows))
	for _, r := range rows {
		var a chstore.PostbackAttempt
		if err := json.Unmarshal(r.Payload, &a); err != nil {
			return nil, fmt.Errorf("postbacklog: unmarshaling attempt %s: %w", r.ID, err)
		}
		out = append(out, QueuedAttempt{ID: r.ID, BatchID: batchID, Attempt: a})
	}
	return out, nil
}

func (q *PostgresQueue) Delete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if _, err := q.db.Exec(ctx, `DELETE FROM postback_attempt_queue WHERE id = ANY($1)`, ids); err != nil {
		return fmt.Errorf("postbacklog: deleting flushed attempts: %w", err)
	}
	return nil
}

func (q *PostgresQueue) Requeue(ctx context.Context, ids []string, next time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	if _, err := q.db.Exec(ctx,
		`UPDATE postback_attempt_queue SET status = 'queued', claimed_at = NULL, next_attempt_at = $2 WHERE id = ANY($1)`,
		ids, next,
	); err != nil {
		return fmt.Errorf("postbacklog: requeueing attempts: %w", err)
	}
	return nil
}

// ReapStuck implements queuereaper.Reaper, same contract as
// eventqueue.PostgresQueue.ReapStuck.
func (q *PostgresQueue) ReapStuck(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := q.db.Exec(ctx, `
		UPDATE postback_attempt_queue
		SET status = 'queued', claimed_at = NULL, next_attempt_at = now()
		WHERE status = 'processing' AND claimed_at < now() - $1::interval`,
		olderThan,
	)
	if err != nil {
		return 0, fmt.Errorf("postbacklog: reaping stuck claims: %w", err)
	}
	return tag.RowsAffected(), nil
}
