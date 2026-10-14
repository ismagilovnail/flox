package pushsend

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/landinglink"
	"github.com/ismagilovnail/flox/apps/internal/pgqueue"
)

// Delivery is one claimed push_deliveries row with what sending it needs.
type Delivery struct {
	ID             string
	OrganizationID string
	BroadcastID    string
	SubscriptionID string
	// AttemptCount already counts the attempt in progress.
	AttemptCount int

	Endpoint, P256dh, Auth string
	// Origin + /pwa/{LinkToken}/push/click/{ID} is the notification's
	// click-through.
	Origin, LinkToken string
	Claims            landinglink.Claims
	Country, Device   string
	Language          string

	Title, Body, IconURL, ImageURL string
}

// PostgresStore is the push_deliveries queue (migration 00070).
type PostgresStore struct {
	db *pgxpool.Pool
	// table is push_deliveries; tests use a private copy so the dev-stack
	// worker cannot take their rows.
	table string
	rot   pgqueue.Rotation
}

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{db: db, table: "push_deliveries"}
}

const pendingDelivery = `delivery_status IN ('queued', 'retrying')`

// claimedColumns: the claimed row, its subscription and broadcast. The
// broadcast's icon falls back to the PWA's.
const claimedColumns = `pd.id, pd.organization_id, pd.broadcast_id, pd.subscription_id, pd.attempt_count,
	s.endpoint, s.p256dh, s.auth, s.origin, s.link_token, s.context, s.country, s.device, s.language,
	b.title, b.body, COALESCE(NULLIF(b.icon_url, ''), p.icon_url, ''), b.image_url`

const claimedJoins = `
	JOIN push_subscriptions s ON s.id = pd.subscription_id AND s.organization_id = pd.organization_id
	JOIN push_broadcasts b ON b.id = pd.broadcast_id AND b.organization_id = pd.organization_id
	LEFT JOIN pwas p ON p.id = s.pwa_id AND p.organization_id = s.organization_id`

// ClaimDue claims up to limit due deliveries fairly across organizations
// (internal/pgqueue), as pixel and postback deliveries are claimed.
func (s *PostgresStore) ClaimDue(ctx context.Context, limit int) ([]Delivery, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("pushsend: beginning claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var ids []string
	orgs, err := pgqueue.PendingOrgs(ctx, tx, s.table, pendingDelivery, &s.rot, limit)
	if err != nil {
		return nil, fmt.Errorf("pushsend: %w", err)
	}
	if len(orgs) > 0 {
		got, err := claimIDs(ctx, tx, `
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
			FROM due WHERE pd.id = due.id
			RETURNING pd.id`,
			orgs, pgqueue.Quota(limit, len(orgs)), limit)
		if err != nil {
			return nil, err
		}
		ids = append(ids, got...)
	}
	if rest := limit - len(ids); rest > 0 {
		got, err := claimIDs(ctx, tx, `
			WITH due AS (
				SELECT id FROM `+s.table+`
				WHERE `+pendingDelivery+` AND next_attempt_at <= now()
				ORDER BY next_attempt_at
				LIMIT $1
				FOR UPDATE SKIP LOCKED
			)
			UPDATE `+s.table+` pd
			SET delivery_status = 'processing', attempt_count = attempt_count + 1, claimed_at = now()
			FROM due WHERE pd.id = due.id
			RETURNING pd.id`, rest)
		if err != nil {
			return nil, err
		}
		ids = append(ids, got...)
	}
	if len(ids) == 0 {
		return nil, tx.Commit(ctx)
	}
	rows, err := tx.Query(ctx, `SELECT `+claimedColumns+` FROM `+s.table+` pd `+claimedJoins+` WHERE pd.id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("pushsend: reading claimed deliveries: %w", err)
	}
	out, err := scanDeliveries(rows)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("pushsend: committing claim: %w", err)
	}
	return out, nil
}

func claimIDs(ctx context.Context, tx pgx.Tx, sql string, args ...any) ([]string, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("pushsend: claiming due deliveries: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("pushsend: claiming due deliveries: %w", err)
	}
	return ids, nil
}

func scanDeliveries(rows pgx.Rows) ([]Delivery, error) {
	defer rows.Close()
	var out []Delivery
	for rows.Next() {
		var d Delivery
		var claims []byte
		if err := rows.Scan(&d.ID, &d.OrganizationID, &d.BroadcastID, &d.SubscriptionID, &d.AttemptCount,
			&d.Endpoint, &d.P256dh, &d.Auth, &d.Origin, &d.LinkToken, &claims, &d.Country, &d.Device, &d.Language,
			&d.Title, &d.Body, &d.IconURL, &d.ImageURL); err != nil {
			return nil, fmt.Errorf("pushsend: scanning claimed delivery: %w", err)
		}
		if err := json.Unmarshal(claims, &d.Claims); err != nil {
			return nil, fmt.Errorf("pushsend: decoding delivery %s context: %w", d.ID, err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pushsend: reading claimed deliveries: %w", err)
	}
	return out, nil
}

// Result is what one attempt came to.
type Result string

const (
	ResultDelivered Result = "delivered" // the push service accepted it
	ResultGone      Result = "gone"      // the subscription no longer exists (404/410)
	ResultRetry     Result = "retry"     // try again later
	ResultFailed    Result = "failed"    // cannot succeed
)

// finish records an attempt and, when the delivery is done, counts it on
// its broadcast; ResultGone also ends the subscription. unsubscribed
// reports whether this call ended an active subscription — the caller
// records NOTIFICATION_UNSUBSCRIBE for it once.
func (s *PostgresStore) finish(ctx context.Context, d Delivery, res Result, code int, message string, next time.Time) (unsubscribed bool, err error) {
	status := map[Result]string{ResultDelivered: "success", ResultGone: "dead", ResultRetry: "retrying", ResultFailed: "dead"}[res]
	var nextAt *time.Time
	if res == ResultRetry {
		nextAt = &next
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("pushsend: beginning finish: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		UPDATE `+s.table+`
		SET delivery_status = $2, response_status_code = $3, message = $4,
		    next_attempt_at = COALESCE($5, next_attempt_at), claimed_at = NULL
		WHERE id = $1`, d.ID, status, nullableInt(code), message, nextAt); err != nil {
		return false, fmt.Errorf("pushsend: marking delivery: %w", err)
	}
	if res != ResultRetry {
		delivered, failed, gone := 0, 0, 0
		switch res {
		case ResultDelivered:
			delivered = 1
		case ResultGone:
			failed, gone = 1, 1
		default:
			failed = 1
		}
		if _, err := tx.Exec(ctx, `
			UPDATE push_broadcasts SET delivered = delivered + $3, failed = failed + $4, unsubscribed = unsubscribed + $5
			WHERE id = $1 AND organization_id = $2`, d.BroadcastID, d.OrganizationID, delivered, failed, gone); err != nil {
			return false, fmt.Errorf("pushsend: counting delivery: %w", err)
		}
		// The same counts per PWA × campaign × country and the 15-minute
		// bucket it was queued in (push.bucketOf), for the push metrics. An upsert: a delivery queued before its broadcast had
		// group rows still counts.
		if _, err := tx.Exec(ctx, `
			INSERT INTO push_broadcast_stats (organization_id, broadcast_id, bucket, pwa_id, campaign_id, country, delivered, failed, unsubscribed)
			SELECT organization_id, broadcast_id, date_bin('15 minutes', created_at, timestamptz '2000-01-01 00:00:00+00'),
			       pwa_id, campaign_id, country, $2, $3, $4 FROM `+s.table+` WHERE id = $1
			ON CONFLICT (broadcast_id, bucket, pwa_id, campaign_id, country) DO UPDATE
			SET delivered = push_broadcast_stats.delivered + EXCLUDED.delivered,
			    failed = push_broadcast_stats.failed + EXCLUDED.failed,
			    unsubscribed = push_broadcast_stats.unsubscribed + EXCLUDED.unsubscribed`,
			d.ID, delivered, failed, gone); err != nil {
			return false, fmt.Errorf("pushsend: counting delivery by group: %w", err)
		}
	}
	if res == ResultGone {
		tag, err := tx.Exec(ctx, `
			UPDATE push_subscriptions SET status = 'unsubscribed', unsubscribed_at = now()
			WHERE id = $1 AND organization_id = $2 AND status = 'active'`, d.SubscriptionID, d.OrganizationID)
		if err != nil {
			return false, fmt.Errorf("pushsend: ending subscription: %w", err)
		}
		unsubscribed = tag.RowsAffected() == 1
		// Its other pending deliveries cannot succeed either.
		if _, err := tx.Exec(ctx, `
			UPDATE `+s.table+` SET delivery_status = 'dead', message = 'subscription expired', claimed_at = NULL
			WHERE subscription_id = $1 AND id <> $2 AND `+pendingDelivery, d.SubscriptionID, d.ID); err != nil {
			return false, fmt.Errorf("pushsend: dropping deliveries of an ended subscription: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("pushsend: committing finish: %w", err)
	}
	return unsubscribed, nil
}

// Release returns claimed deliveries that were never attempted.
func (s *PostgresStore) Release(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.db.Exec(ctx, `
		UPDATE `+s.table+`
		SET attempt_count = attempt_count - 1,
		    delivery_status = CASE WHEN attempt_count - 1 > 0 THEN 'retrying' ELSE 'queued' END,
		    claimed_at = NULL, next_attempt_at = now()
		WHERE id = ANY($1) AND delivery_status = 'processing'`, ids)
	if err != nil {
		return fmt.Errorf("pushsend: releasing unattempted claims: %w", err)
	}
	return nil
}

// ReapStuck implements queuereaper.Reaper.
func (s *PostgresStore) ReapStuck(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE `+s.table+`
		SET delivery_status = 'retrying', claimed_at = NULL, next_attempt_at = now(),
		    message = 'claim abandoned by a stopped worker; retrying'
		WHERE delivery_status = 'processing' AND claimed_at < now() - $1::interval`, olderThan)
	if err != nil {
		return 0, fmt.Errorf("pushsend: reaping stuck claims: %w", err)
	}
	return tag.RowsAffected(), nil
}

func nullableInt(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}
