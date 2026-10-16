package postback

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/pgqueue"
)

// PostgresStore implements Store against postback_deliveries (migration
// 00014).
type PostgresStore struct {
	db *pgxpool.Pool
	// table is postback_deliveries; tests use a private copy so the
	// dev-stack worker polling the real one cannot take their rows.
	table string
	// rot carries the fair claim's starting organization between claims.
	rot pgqueue.Rotation
}

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{db: db, table: "postback_deliveries"}
}

var _ Store = (*PostgresStore)(nil)

func (s *PostgresStore) Enqueue(ctx context.Context, in EnqueueInput) (string, error) {
	return insertDelivery(ctx, s.db, s.table, in)
}

// InsertTx queues a delivery inside the transaction that records its source
// conversion (conversion.DeliveryWriter): the conversion and its outgoing
// postback commit together or not at all (tech audit D1): a delivery is
// never lost after the network has been told "success".
func (s *PostgresStore) InsertTx(ctx context.Context, tx pgx.Tx, req conversion.DeliveryRequest) error {
	_, err := insertDelivery(ctx, tx, "postback_deliveries", EnqueueInput{
		OrganizationID:   req.OrganizationID,
		NetworkID:        req.NetworkID,
		TrafficSourceID:  req.TrafficSourceID,
		SourcePostbackID: req.SourcePostbackID,
		ClickID:          req.ClickID,
		Status:           req.Status,
		URL:              req.URL,
	})
	return err
}

var _ conversion.DeliveryWriter = (*PostgresStore)(nil)

type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func insertDelivery(ctx context.Context, db execer, table string, in EnqueueInput) (string, error) {
	id := idgen.New()
	_, err := db.Exec(ctx, `
		INSERT INTO `+table+` (id, organization_id, network_id, traffic_source_id, source_postback_id, click_id, status, url)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), $5, $6, $7, $8)`,
		id, in.OrganizationID, in.NetworkID, in.TrafficSourceID, in.SourcePostbackID, in.ClickID, string(in.Status), in.URL,
	)
	if err != nil {
		return "", fmt.Errorf("postback: enqueueing delivery: %w", err)
	}
	return id, nil
}

// pendingDelivery: a delivery still to attempt — what migration 00066's
// fair claim index covers.
const pendingDelivery = `delivery_status IN ('queued', 'retrying')`

// claimedColumns is what a claim returns of each delivery.
const claimedColumns = `pd.id, pd.organization_id, coalesce(pd.network_id, ''), pd.source_postback_id, pd.click_id, pd.status, pd.url, pd.attempt_count`

// ClaimDue is the standard Postgres job-queue pattern: SELECT ... FOR UPDATE
// SKIP LOCKED picks rows no other transaction currently holds, and the
// UPDATE joined to it claims exactly those rows atomically — two worker
// replicas polling at the same instant can never both claim the same
// delivery.
//
// Fairly across organizations (tech audit M2; internal/pgqueue fair.go):
// each organization with deliveries waiting gets its share of the claim
// first, its oldest due ones, then the rest of the batch goes to the oldest
// due deliveries of anyone. A partner endpoint that hangs and piles up
// retries for one organization does not fill every batch.
func (s *PostgresStore) ClaimDue(ctx context.Context, limit int) ([]Delivery, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("postback: beginning claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var out []Delivery
	orgs, err := pgqueue.PendingOrgs(ctx, tx, s.table, pendingDelivery, &s.rot, limit)
	if err != nil {
		return nil, fmt.Errorf("postback: %w", err)
	}
	if len(orgs) > 0 {
		out, err = claimDeliveries(ctx, tx, `
			WITH due AS (
				SELECT c.id FROM unnest($1::text[]) AS o(org)
				CROSS JOIN LATERAL (
					SELECT id FROM `+s.table+`
					WHERE organization_id = o.org AND `+pendingDelivery+` AND next_attempt_at <= now()
					ORDER BY next_attempt_at, id
					LIMIT $2
					FOR UPDATE SKIP LOCKED
				) c
				LIMIT $3
			)
			UPDATE `+s.table+` pd
			SET delivery_status = 'processing', attempt_count = attempt_count + 1, claimed_at = now()
			FROM due
			WHERE pd.id = due.id
			RETURNING `+claimedColumns,
			orgs, pgqueue.Quota(limit, len(orgs)), limit)
		if err != nil {
			return nil, err
		}
	}
	if rest := limit - len(out); rest > 0 {
		more, err := claimDeliveries(ctx, tx, `
			WITH due AS (
				SELECT id FROM `+s.table+`
				WHERE `+pendingDelivery+` AND next_attempt_at <= now()
				ORDER BY next_attempt_at
				LIMIT $1
				FOR UPDATE SKIP LOCKED
			)
			UPDATE `+s.table+` pd
			SET delivery_status = 'processing', attempt_count = attempt_count + 1, claimed_at = now()
			FROM due
			WHERE pd.id = due.id
			RETURNING `+claimedColumns,
			rest)
		if err != nil {
			return nil, err
		}
		out = append(out, more...)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("postback: committing claim: %w", err)
	}
	return out, nil
}

func claimDeliveries(ctx context.Context, tx pgx.Tx, sql string, args ...any) ([]Delivery, error) {
	rows, err := tx.Query(ctx, sql, args...)
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
		UPDATE `+s.table+`
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
		UPDATE `+s.table+`
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
		UPDATE `+s.table+`
		SET delivery_status = 'dead', response_status_code = $2, message = $3
		WHERE id = $1`,
		id, nullableInt(responseStatusCode), message,
	)
	if err != nil {
		return fmt.Errorf("postback: marking delivery dead: %w", err)
	}
	return nil
}

// Release undoes a claim that was never attempted — the worker is stopping
// and will not get to these rows. The claim's attempt_count increment is
// taken back, so a delivery is never dead-lettered for attempts it never
// had; a row that had no earlier attempt goes back to 'queued'.
func (s *PostgresStore) Release(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.db.Exec(ctx, `
		UPDATE `+s.table+`
		SET attempt_count = attempt_count - 1,
		    delivery_status = CASE WHEN attempt_count - 1 > 0 THEN 'retrying' ELSE 'queued' END,
		    claimed_at = NULL, next_attempt_at = now()
		WHERE id = ANY($1) AND delivery_status = 'processing'`,
		ids,
	)
	if err != nil {
		return fmt.Errorf("postback: releasing unattempted claims: %w", err)
	}
	return nil
}

// ReapStuck implements queuereaper.Reaper. Unlike Release, a reaped row may
// well have been sent before its worker died, so its attempt stays counted
// and it is retried as 'retrying' — at worst the partner receives it twice,
// which a lost conversion would be worse than.
func (s *PostgresStore) ReapStuck(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE `+s.table+`
		SET delivery_status = 'retrying', claimed_at = NULL, next_attempt_at = now(),
		    message = 'claim abandoned by a stopped worker; retrying'
		WHERE delivery_status = 'processing' AND claimed_at < now() - $1::interval`,
		olderThan,
	)
	if err != nil {
		return 0, fmt.Errorf("postback: reaping stuck claims: %w", err)
	}
	return tag.RowsAffected(), nil
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
