package pixelsend

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/pgqueue"
	"github.com/ismagilovnail/flox/apps/internal/pixel"
)

// Delivery is one claimed pixel_deliveries row.
type Delivery struct {
	ID             string
	OrganizationID string
	PixelID        string
	Event          pixel.Event
	// AttemptCount already counts the attempt in progress (ClaimDue
	// increments it).
	AttemptCount int
}

// PostgresStore is the pixel_deliveries queue (migration 00069).
type PostgresStore struct {
	db *pgxpool.Pool
	// table is pixel_deliveries; tests use a private copy so the dev-stack
	// worker cannot take their rows.
	table string
	rot   pgqueue.Rotation
}

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{db: db, table: "pixel_deliveries"}
}

var _ conversion.PixelWriter = (*PostgresStore)(nil)

type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// InsertTx queues a conversion for its stream set's pixels inside the
// transaction that records it (conversion.PixelWriter).
func (s *PostgresStore) InsertTx(ctx context.Context, tx pgx.Tx, ev pixel.Event) error {
	_, err := enqueue(ctx, tx, s.table, ev)
	return err
}

// Enqueue queues a tracker event for its stream set's pixels and returns
// how many deliveries it created.
func (s *PostgresStore) Enqueue(ctx context.Context, ev pixel.Event) (int64, error) {
	return enqueue(ctx, s.db, s.table, ev)
}

// enqueue writes one delivery per pixel the event goes to: attached to the
// event's stream set, active, set to report this event type, and of a
// provider that can send. A pixel already holding this source event is
// skipped, so a retried batch never queues an event twice.
func enqueue(ctx context.Context, db execer, table string, ev pixel.Event) (int64, error) {
	if ev.StreamSetID == "" || ev.SourceEventID == "" {
		return 0, nil
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return 0, fmt.Errorf("pixelsend: encoding event: %w", err)
	}
	tag, err := db.Exec(ctx, `
		INSERT INTO `+table+` (id, organization_id, pixel_id, source_event_id, event_type, click_id, payload)
		SELECT flox_new_ulid(), p.organization_id, p.id, $3, $4, $5, $6
		FROM stream_set_pixels ssp
		JOIN pixels p ON p.id = ssp.pixel_id AND p.organization_id = ssp.organization_id
		WHERE ssp.organization_id = $1 AND ssp.stream_set_id = $2
		  AND p.status = 'active' AND $4 = ANY(p.events) AND p.provider <> 'twitter'
		ON CONFLICT (pixel_id, source_event_id) DO NOTHING`,
		ev.OrganizationID, ev.StreamSetID, ev.SourceEventID, string(ev.Type), ev.ClickID, payload,
	)
	if err != nil {
		return 0, fmt.Errorf("pixelsend: queueing pixel deliveries: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Wanted reports whether any pixel would take this event: the cheap check
// before a tracker event's click is looked up for it.
func (s *PostgresStore) Wanted(ctx context.Context, orgID, streamSetID string, t event.Type) (bool, error) {
	var ok bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM stream_set_pixels ssp
			JOIN pixels p ON p.id = ssp.pixel_id AND p.organization_id = ssp.organization_id
			WHERE ssp.organization_id = $1 AND ssp.stream_set_id = $2
			  AND p.status = 'active' AND $3 = ANY(p.events) AND p.provider <> 'twitter')`,
		orgID, streamSetID, string(t),
	).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("pixelsend: checking pixels: %w", err)
	}
	return ok, nil
}

const pendingDelivery = `delivery_status IN ('queued', 'retrying')`

const claimedColumns = `pd.id, pd.organization_id, pd.pixel_id, pd.payload, pd.attempt_count`

// ClaimDue claims up to limit due deliveries, fairly across organizations
// (internal/pgqueue fair.go) — the same claim as postback_deliveries: each
// organization with deliveries waiting gets its share first, the rest of
// the batch goes to the oldest due ones of anyone. FOR UPDATE SKIP LOCKED,
// so two worker replicas never claim the same row.
func (s *PostgresStore) ClaimDue(ctx context.Context, limit int) ([]Delivery, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("pixelsend: beginning claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var out []Delivery
	orgs, err := pgqueue.PendingOrgs(ctx, tx, s.table, pendingDelivery, &s.rot, limit)
	if err != nil {
		return nil, fmt.Errorf("pixelsend: %w", err)
	}
	if len(orgs) > 0 {
		out, err = claim(ctx, tx, `
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
		more, err := claim(ctx, tx, `
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
		return nil, fmt.Errorf("pixelsend: committing claim: %w", err)
	}
	return out, nil
}

func claim(ctx context.Context, tx pgx.Tx, sql string, args ...any) ([]Delivery, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("pixelsend: claiming due deliveries: %w", err)
	}
	defer rows.Close()

	var out []Delivery
	for rows.Next() {
		var d Delivery
		var payload []byte
		if err := rows.Scan(&d.ID, &d.OrganizationID, &d.PixelID, &payload, &d.AttemptCount); err != nil {
			return nil, fmt.Errorf("pixelsend: scanning claimed delivery: %w", err)
		}
		if err := json.Unmarshal(payload, &d.Event); err != nil {
			return nil, fmt.Errorf("pixelsend: decoding delivery %s: %w", d.ID, err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pixelsend: reading claimed deliveries: %w", err)
	}
	return out, nil
}

// finish records an attempt's outcome. message is what the operator reads
// in the pixel's delivery log; next is only used for 'retrying'.
func (s *PostgresStore) finish(ctx context.Context, id string, status Status, code int, message string, next time.Time) error {
	var nextAt *time.Time
	if status == StatusRetrying {
		nextAt = &next
	}
	_, err := s.db.Exec(ctx, `
		UPDATE `+s.table+`
		SET delivery_status = $2, response_status_code = $3, message = $4,
		    next_attempt_at = COALESCE($5, next_attempt_at), claimed_at = NULL
		WHERE id = $1`,
		id, string(status), nullableInt(code), message, nextAt,
	)
	if err != nil {
		return fmt.Errorf("pixelsend: marking delivery %s: %w", status, err)
	}
	return nil
}

// Release returns claimed deliveries that were never attempted (the worker
// is stopping), undoing the claim's attempt count.
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
		return fmt.Errorf("pixelsend: releasing unattempted claims: %w", err)
	}
	return nil
}

// ReapStuck implements queuereaper.Reaper: a claim a dead worker left
// behind is retried, its attempt kept counted — it may have been sent, and
// the platform deduplicates a resend by event_id.
func (s *PostgresStore) ReapStuck(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE `+s.table+`
		SET delivery_status = 'retrying', claimed_at = NULL, next_attempt_at = now(),
		    message = 'claim abandoned by a stopped worker; retrying'
		WHERE delivery_status = 'processing' AND claimed_at < now() - $1::interval`,
		olderThan,
	)
	if err != nil {
		return 0, fmt.Errorf("pixelsend: reaping stuck claims: %w", err)
	}
	return tag.RowsAffected(), nil
}

// nullableInt stores "no HTTP response" (0) as NULL.
func nullableInt(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}
