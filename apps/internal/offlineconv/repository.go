package offlineconv

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/yandexmetrica"
)

// Repository reads and writes the export settings (Google's on
// ad_account_connections, Metrica's in metrica_connections) and the queue
// (offline_conversion_uploads).
// Settings reads/writes are scoped to one organization, from its session.
// The worker's methods (marked) run across every organization on a timer
// and are never reachable over HTTP.
type Repository struct {
	db *pgxpool.Pool
	// onlyOrg, when set, limits the worker's methods to one organization:
	// tests share the database with the dev worker and other packages.
	onlyOrg string
	// key encrypts Metrica tokens (ADACCOUNT_ENCRYPTION_KEY).
	key []byte
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// WithKey sets the key Metrica tokens are encrypted with; without it a
// Metrica connection can be neither written nor sent through.
func (r *Repository) WithKey(key []byte) *Repository {
	r.key = key
	return r
}

// connection is a Google Ads connection's id and export settings.
type connection struct {
	ID              string
	TrafficSourceID string
	AdAccountID     string
	Settings
}

const connectionColumns = `c.id, c.traffic_source_id, c.ad_account_id, c.offline_conversions, c.offline_since,
	c.offline_login_account_id, c.offline_lead_action_id, c.offline_purchase_action_id, c.offline_repurchase_action_id`

func scanConnection(row pgx.Row) (connection, error) {
	var c connection
	err := row.Scan(&c.ID, &c.TrafficSourceID, &c.AdAccountID, &c.Enabled, &c.Since,
		&c.LoginAccountID, &c.LeadActionID, &c.PurchaseActionID, &c.RepurchaseActionID)
	return c, err
}

// googleConnection is the traffic source's Google Ads connection, or a
// not-found error when it has none (another platform's connection counts
// as none: only Google Ads has offline conversions here).
func (r *Repository) googleConnection(ctx context.Context, orgID, trafficSourceID string) (connection, error) {
	c, err := scanConnection(r.db.QueryRow(ctx, `
		SELECT `+connectionColumns+`
		FROM ad_account_connections c
		JOIN traffic_sources ts ON ts.id = c.traffic_source_id AND ts.organization_id = c.organization_id
		WHERE c.organization_id = $1 AND c.traffic_source_id = $2 AND ts.cost_integration = 'google_ads'`,
		orgID, trafficSourceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return connection{}, apierror.NotFound("no Google Ads account connected for this traffic source")
	}
	if err != nil {
		return connection{}, fmt.Errorf("offlineconv: reading connection: %w", err)
	}
	return c, nil
}

// setSettings writes s; turning the export on (from off) starts its
// window now.
func (r *Repository) setSettings(ctx context.Context, orgID, connectionID string, s Settings) error {
	_, err := r.db.Exec(ctx, `
		UPDATE ad_account_connections SET
			offline_since = CASE WHEN $3 AND NOT offline_conversions THEN now() ELSE offline_since END,
			offline_conversions = $3,
			offline_login_account_id = $4,
			offline_lead_action_id = $5,
			offline_purchase_action_id = $6,
			offline_repurchase_action_id = $7,
			updated_at = now()
		WHERE organization_id = $1 AND id = $2`,
		orgID, connectionID, s.Enabled, s.LoginAccountID, s.LeadActionID, s.PurchaseActionID, s.RepurchaseActionID)
	if err != nil {
		return fmt.Errorf("offlineconv: writing settings: %w", err)
	}
	return nil
}

// status counts the connection's uploads.
func (r *Repository) status(ctx context.Context, orgID string, c connection) (Status, error) {
	t, err := r.tallyUploads(ctx, orgID, "connection_id", c.ID)
	if err != nil {
		return Status{}, err
	}
	return Status{Settings: c.Settings, Counts: t.Counts, Skipped: t.Skipped,
		LastError: t.LastError, LastErrorAt: t.LastErrorAt, LastSentAt: t.LastSentAt}, nil
}

// tally is what came of one connection's uploads.
type tally struct {
	Counts      map[string]int
	Skipped     map[string]int
	Processing  int
	LastError   string
	LastErrorAt *time.Time
	LastSentAt  *time.Time
}

// tallyUploads counts the uploads whose column (connection_id or
// metrica_connection_id — the caller's constant, never input) is id.
func (r *Repository) tallyUploads(ctx context.Context, orgID, column, id string) (tally, error) {
	t := tally{Counts: map[string]int{}, Skipped: map[string]int{}}
	rows, err := r.db.Query(ctx, `
		SELECT status, skip_reason, remote_status, count(*) FROM offline_conversion_uploads
		WHERE organization_id = $1 AND `+column+` = $2
		GROUP BY status, skip_reason, remote_status`, orgID, id)
	if err != nil {
		return tally{}, fmt.Errorf("offlineconv: counting uploads: %w", err)
	}
	for rows.Next() {
		var status, reason, remote string
		var n int
		if err := rows.Scan(&status, &reason, &remote, &n); err != nil {
			rows.Close()
			return tally{}, err
		}
		t.Counts[status] += n
		if status == StatusSkipped {
			t.Skipped[reason] += n
		}
		if status == StatusSent && column == "metrica_connection_id" && !yandexmetrica.Final(remote) {
			t.Processing += n
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return tally{}, err
	}
	err = r.db.QueryRow(ctx, `
		SELECT last_error, updated_at FROM offline_conversion_uploads
		WHERE organization_id = $1 AND `+column+` = $2 AND last_error <> '' AND status IN ('pending', 'failed')
		ORDER BY updated_at DESC LIMIT 1`, orgID, id).Scan(&t.LastError, &t.LastErrorAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return tally{}, fmt.Errorf("offlineconv: reading last error: %w", err)
	}
	if err := r.db.QueryRow(ctx, `
		SELECT max(sent_at) FROM offline_conversion_uploads
		WHERE organization_id = $1 AND `+column+` = $2`, orgID, id).Scan(&t.LastSentAt); err != nil {
		return tally{}, fmt.Errorf("offlineconv: reading last send: %w", err)
	}
	return t, nil
}

// --- the worker's (every organization, never over HTTP) ---

// exportingOrg is an organization with the export on for at least one
// source, and the earliest moment one was turned on.
type exportingOrg struct {
	OrganizationID string
	Since          time.Time
}

func (r *Repository) exportingOrgs(ctx context.Context) ([]exportingOrg, error) {
	rows, err := r.db.Query(ctx, `
		SELECT organization_id, min(since) FROM (
			SELECT organization_id, offline_since AS since FROM ad_account_connections
			WHERE offline_conversions AND offline_since IS NOT NULL
			UNION ALL
			SELECT organization_id, since FROM metrica_connections
			WHERE enabled AND since IS NOT NULL
		) e
		WHERE $1 = '' OR organization_id = $1
		GROUP BY organization_id ORDER BY organization_id`, r.onlyOrg)
	if err != nil {
		return nil, fmt.Errorf("offlineconv: listing exporting organizations: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[exportingOrg])
}

// candidate is a conversion postback not yet considered.
type candidate struct {
	PostbackID string
	Status     string
	ClickID    string
	CreatedAt  time.Time
	EventAt    time.Time
	Value      *float64
	Currency   string
}

// remainingValue is what is left of a purchase after its refunds (UNI-4:
// a reversal points at the purchase it reverses), never below zero; NULL
// for a postback without revenue.
const remainingValue = `CASE WHEN p.revenue IS NULL THEN NULL ELSE greatest(p.revenue - coalesce((
		SELECT sum(r.reversal_revenue) FROM postbacks r
		WHERE r.organization_id = p.organization_id AND r.reverses_postback_id = p.id AND r.result = 'success'), 0), 0)::float8 END`

func (r *Repository) candidates(ctx context.Context, orgID string, since time.Time, limit int) ([]candidate, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.id, p.status, coalesce(p.attributed_click_id, ''), p.created_at, coalesce(p.event_at, p.created_at),
		       `+remainingValue+`, coalesce(p.currency, '')
		FROM postbacks p
		WHERE p.organization_id = $1 AND p.direction = 'incoming' AND p.result = 'success'
		  AND p.status IN ('CPA_HOLD', 'CPA_ACCEPT', 'CPA_REDEP') AND p.created_at >= $2
		  AND NOT EXISTS (SELECT 1 FROM offline_conversion_uploads u WHERE u.postback_id = p.id)
		ORDER BY p.created_at
		LIMIT $3`, orgID, since, limit)
	if err != nil {
		return nil, fmt.Errorf("offlineconv: listing new conversions: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[candidate])
}

// campaignConnection is the Google Ads connection of the campaign's
// traffic source; found false when it has none.
func (r *Repository) campaignConnection(ctx context.Context, orgID, campaignID string) (connection, bool, error) {
	c, err := scanConnection(r.db.QueryRow(ctx, `
		SELECT `+connectionColumns+`
		FROM campaigns cm
		JOIN traffic_sources ts ON ts.id = cm.traffic_source_id AND ts.organization_id = cm.organization_id
		JOIN ad_account_connections c ON c.traffic_source_id = cm.traffic_source_id AND c.organization_id = cm.organization_id
		WHERE cm.organization_id = $1 AND cm.id = $2 AND ts.cost_integration = 'google_ads'`,
		orgID, campaignID))
	if errors.Is(err, pgx.ErrNoRows) {
		return connection{}, false, nil
	}
	if err != nil {
		return connection{}, false, fmt.Errorf("offlineconv: reading the campaign's connection: %w", err)
	}
	return c, true, nil
}

// upload is a row to insert, or a claimed one to send.
type upload struct {
	ID             string
	OrganizationID string
	PostbackID     string
	ConnectionID   *string
	// MetricaConnectionID instead of ConnectionID: a row for Metrica.
	MetricaConnectionID *string
	ClickID             string
	AdClickParam        string
	AdClickID           string
	ActionID            string
	EventAt             time.Time
	Value               *float64
	Currency            string
	Status              string
	SkipReason          string
	Attempts            int
}

// insert records u; a postback already recorded (another worker got there
// first) is left as it is.
func (r *Repository) insert(ctx context.Context, u upload) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO offline_conversion_uploads
			(id, organization_id, postback_id, connection_id, metrica_connection_id, click_id, ad_click_param, ad_click_id,
			 action_id, event_at, value, currency, status, skip_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (postback_id) DO NOTHING`,
		u.ID, u.OrganizationID, u.PostbackID, u.ConnectionID, u.MetricaConnectionID, u.ClickID, u.AdClickParam, u.AdClickID,
		u.ActionID, u.EventAt, u.Value, u.Currency, u.Status, u.SkipReason)
	if err != nil {
		return fmt.Errorf("offlineconv: recording upload: %w", err)
	}
	return nil
}

// restate puts back to pending, with the new value, every pending or sent
// Google upload of orgID whose purchase a refund has changed since.
// Returns how many. Metrica's rows are left: its API has no restating, and
// a second row for the same yclid and goal would count a second purchase.
func (r *Repository) restate(ctx context.Context, orgID string) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		WITH refunded AS (
			SELECT p.id, `+remainingValue+` AS remaining
			FROM postbacks p
			WHERE p.organization_id = $1 AND p.revenue IS NOT NULL AND EXISTS (
				SELECT 1 FROM postbacks r
				WHERE r.organization_id = $1 AND r.reverses_postback_id = p.id AND r.result = 'success')
		)
		UPDATE offline_conversion_uploads u
		SET value = refunded.remaining, status = 'pending', attempts = 0, next_attempt_at = now(), updated_at = now()
		FROM refunded
		WHERE u.organization_id = $1 AND u.postback_id = refunded.id AND u.connection_id IS NOT NULL
		  AND u.status IN ('pending', 'sent') AND u.value IS DISTINCT FROM refunded.remaining::numeric(14,4)`, orgID)
	if err != nil {
		return 0, fmt.Errorf("offlineconv: restating refunded uploads: %w", err)
	}
	return tag.RowsAffected(), nil
}

// claimLease is how long a claimed row is left to its sender before
// another worker may take it.
const claimLease = 10 * time.Minute

// claimDue takes up to limit pending uploads due now, of sources with the
// export (to Google or to Metrica) on, and leases them.
func (r *Repository) claimDue(ctx context.Context, limit int) ([]upload, error) {
	rows, err := r.db.Query(ctx, `
		UPDATE offline_conversion_uploads u
		SET next_attempt_at = now() + make_interval(secs => $2), updated_at = now()
		WHERE u.id IN (
			SELECT u2.id FROM offline_conversion_uploads u2
			LEFT JOIN ad_account_connections c ON c.id = u2.connection_id
			LEFT JOIN metrica_connections m ON m.id = u2.metrica_connection_id
			WHERE u2.status = 'pending' AND u2.next_attempt_at <= now() AND (c.offline_conversions OR m.enabled)
			  AND ($3 = '' OR u2.organization_id = $3)
			ORDER BY u2.next_attempt_at
			LIMIT $1
			FOR UPDATE OF u2 SKIP LOCKED)
		RETURNING u.id, u.organization_id, u.postback_id, u.connection_id, u.metrica_connection_id, u.click_id, u.ad_click_param, u.ad_click_id,
		          u.action_id, u.event_at, u.value::float8, u.currency, u.status, u.skip_reason, u.attempts`,
		limit, claimLease.Seconds(), r.onlyOrg)
	if err != nil {
		return nil, fmt.Errorf("offlineconv: claiming due uploads: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[upload])
}

// connectionByID is a connection with its organization, for the sender.
func (r *Repository) connectionByID(ctx context.Context, orgID, id string) (connection, error) {
	c, err := scanConnection(r.db.QueryRow(ctx, `
		SELECT `+connectionColumns+` FROM ad_account_connections c WHERE c.organization_id = $1 AND c.id = $2`, orgID, id))
	if err != nil {
		return connection{}, fmt.Errorf("offlineconv: reading connection %s: %w", id, err)
	}
	return c, nil
}

// markSent records that the network took u with value sent; remote is
// Metrica's status of the upload (empty for Google). A refund that changed
// the value while it was being sent leaves it pending, to go again with
// the new one (Google's rows only: restate never changes Metrica's).
func (r *Repository) markSent(ctx context.Context, id string, sent *float64, requestID, warning, remote string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE offline_conversion_uploads SET
			status = CASE WHEN value IS DISTINCT FROM $2::numeric(14,4) THEN 'pending' ELSE 'sent' END,
			sent_value = $2, request_id = $3, last_error = $4, remote_status = $5, attempts = 0,
			sent_at = now(), next_attempt_at = now(), updated_at = now()
		WHERE id = $1`, id, sent, requestID, warning, remote)
	if err != nil {
		return fmt.Errorf("offlineconv: marking upload sent: %w", err)
	}
	return nil
}

// markRetry leaves u pending until after; failed instead once it has
// been tried maxAttempts times.
func (r *Repository) markRetry(ctx context.Context, id, message string, after time.Duration) (failed bool, err error) {
	err = r.db.QueryRow(ctx, `
		UPDATE offline_conversion_uploads SET
			attempts = attempts + 1,
			status = CASE WHEN attempts + 1 >= $4 THEN 'failed' ELSE 'pending' END,
			last_error = $2, next_attempt_at = now() + make_interval(secs => $3), updated_at = now()
		WHERE id = $1
		RETURNING status = 'failed'`, id, message, after.Seconds(), maxAttempts).Scan(&failed)
	if err != nil {
		return false, fmt.Errorf("offlineconv: scheduling a retry: %w", err)
	}
	return failed, nil
}

// markFailed records a refusal sending again cannot fix.
func (r *Repository) markFailed(ctx context.Context, id, message string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE offline_conversion_uploads SET status = 'failed', attempts = attempts + 1, last_error = $2, updated_at = now()
		WHERE id = $1`, id, message)
	if err != nil {
		return fmt.Errorf("offlineconv: marking upload failed: %w", err)
	}
	return nil
}
