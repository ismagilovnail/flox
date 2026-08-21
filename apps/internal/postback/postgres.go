package postback

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// PostgresStore implements Store against postback_deliveries (migration
// 00014).
type PostgresStore struct {
	db *pgxpool.Pool
}

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore { return &PostgresStore{db: db} }

var _ Store = (*PostgresStore)(nil)

func (s *PostgresStore) Enqueue(ctx context.Context, in EnqueueInput) (string, error) {
	id := idgen.New()
	_, err := s.db.Exec(ctx, `
		INSERT INTO postback_deliveries (id, organization_id, network_id, source_postback_id, click_id, status, url)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, in.OrganizationID, in.NetworkID, in.SourcePostbackID, in.ClickID, string(in.Status), in.URL,
	)
	if err != nil {
		return "", fmt.Errorf("postback: enqueueing delivery: %w", err)
	}
	return id, nil
}

// ClaimDue is the standard Postgres job-queue pattern: SELECT ... FOR UPDATE
// SKIP LOCKED inside a CTE picks rows no other transaction currently holds,
// and the UPDATE joined to it claims exactly those rows atomically — two
// worker replicas polling at the same instant can never both claim the same
// delivery.
func (s *PostgresStore) ClaimDue(ctx context.Context, limit int) ([]Delivery, error) {
	rows, err := s.db.Query(ctx, `
		WITH due AS (
			SELECT id FROM postback_deliveries
			WHERE delivery_status IN ('queued', 'retrying') AND next_attempt_at <= now()
			ORDER BY next_attempt_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE postback_deliveries pd
		SET delivery_status = 'processing', attempt_count = attempt_count + 1
		FROM due
		WHERE pd.id = due.id
		RETURNING pd.id, pd.organization_id, pd.network_id, pd.source_postback_id, pd.click_id, pd.status, pd.url, pd.attempt_count`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("postback: claiming due deliveries: %w", err)
	}
	defer rows.Close()

	var out []Delivery
	for rows.Next() {
		var d Delivery
		var status string
		if err := rows.Scan(&d.ID, &d.OrganizationID, &d.NetworkID, &d.SourcePostbackID, &d.ClickID, &status, &d.URL, &d.AttemptCount); err != nil {
			return nil, fmt.Errorf("postback: scanning claimed delivery: %w", err)
		}
		d.Status = event.Type(status)
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postback: reading claimed deliveries: %w", err)
	}
	return out, nil
}

func (s *PostgresStore) MarkSuccess(ctx context.Context, id string, responseStatusCode int) error {
	_, err := s.db.Exec(ctx, `
		UPDATE postback_deliveries
		SET delivery_status = 'success', response_status_code = $2, message = ''
		WHERE id = $1`,
		id, nullableInt(responseStatusCode),
	)
	if err != nil {
		return fmt.Errorf("postback: marking delivery success: %w", err)
	}
	return nil
}

func (s *PostgresStore) MarkRetrying(ctx context.Context, id string, responseStatusCode int, message string, nextAttemptAt time.Time) error {
	_, err := s.db.Exec(ctx, `
		UPDATE postback_deliveries
		SET delivery_status = 'retrying', response_status_code = $2, message = $3, next_attempt_at = $4
		WHERE id = $1`,
		id, nullableInt(responseStatusCode), message, nextAttemptAt,
	)
	if err != nil {
		return fmt.Errorf("postback: marking delivery retrying: %w", err)
	}
	return nil
}

func (s *PostgresStore) MarkDead(ctx context.Context, id string, responseStatusCode int, message string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE postback_deliveries
		SET delivery_status = 'dead', response_status_code = $2, message = $3
		WHERE id = $1`,
		id, nullableInt(responseStatusCode), message,
	)
	if err != nil {
		return fmt.Errorf("postback: marking delivery dead: %w", err)
	}
	return nil
}

// nullableInt maps 0 (no HTTP response at all — timeout, connection
// refused, invalid URL) to NULL rather than storing a status code that
// isn't a real one; a genuine 0 status code doesn't exist in HTTP.
func nullableInt(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}
