package offlineconv

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/fx"
)

// metricaConnection is a source's Metrica connection; Token is still
// ciphertext until token() opens it.
type metricaConnection struct {
	ID              string
	TrafficSourceID string
	Token           string
	TokenPreview    string
	MetricaSettings
}

const metricaColumns = `m.id, m.traffic_source_id, m.access_token, m.token_preview, m.enabled, m.since,
	m.counter_id, m.currency, m.lead_goal, m.purchase_goal, m.repurchase_goal`

func scanMetrica(row pgx.Row) (metricaConnection, error) {
	var m metricaConnection
	err := row.Scan(&m.ID, &m.TrafficSourceID, &m.Token, &m.TokenPreview, &m.Enabled, &m.Since,
		&m.CounterID, &m.Currency, &m.LeadGoal, &m.PurchaseGoal, &m.RepurchaseGoal)
	return m, err
}

func (r *Repository) token(m metricaConnection) (string, error) {
	if r.key == nil {
		return "", errors.New("offlineconv: no encryption key for Metrica tokens")
	}
	return adaccount.Open(r.key, m.Token)
}

// sourceExists: the traffic source is orgID's.
func (r *Repository) sourceExists(ctx context.Context, orgID, trafficSourceID string) (bool, error) {
	var ok bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM traffic_sources WHERE organization_id = $1 AND id = $2)`,
		orgID, trafficSourceID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("offlineconv: reading traffic source: %w", err)
	}
	return ok, nil
}

// metricaBySource is the source's Metrica connection; found false when
// it has none.
func (r *Repository) metricaBySource(ctx context.Context, orgID, trafficSourceID string) (metricaConnection, bool, error) {
	m, err := scanMetrica(r.db.QueryRow(ctx, `
		SELECT `+metricaColumns+` FROM metrica_connections m
		WHERE m.organization_id = $1 AND m.traffic_source_id = $2`, orgID, trafficSourceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return metricaConnection{}, false, nil
	}
	if err != nil {
		return metricaConnection{}, false, fmt.Errorf("offlineconv: reading Metrica connection: %w", err)
	}
	return m, true, nil
}

// putMetrica creates or replaces the source's connection. token "" keeps
// the stored one (the caller has checked there is one: the insert's
// empty placeholder never lands); turning the export
// on (from off) starts its window now.
func (r *Repository) putMetrica(ctx context.Context, orgID, trafficSourceID, newID, token string, s MetricaSettings) error {
	var cipher, preview *string
	if token != "" {
		if r.key == nil {
			return errors.New("offlineconv: no encryption key for Metrica tokens")
		}
		enc, err := adaccount.Seal(r.key, token)
		if err != nil {
			return err
		}
		p := token[max(len(token)-4, 0):]
		cipher, preview = &enc, &p
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO metrica_connections (id, organization_id, traffic_source_id, counter_id, access_token, token_preview,
			currency, enabled, since, lead_goal, purchase_goal, repurchase_goal)
		VALUES ($1, $2, $3, $4, coalesce($5, ''), coalesce($6, ''), $7, $8, CASE WHEN $8 THEN now() END, $9, $10, $11)
		ON CONFLICT (traffic_source_id) DO UPDATE SET
			counter_id = EXCLUDED.counter_id,
			access_token = coalesce($5, metrica_connections.access_token),
			token_preview = coalesce($6, metrica_connections.token_preview),
			currency = EXCLUDED.currency,
			since = CASE WHEN $8 AND NOT metrica_connections.enabled THEN now() ELSE metrica_connections.since END,
			enabled = EXCLUDED.enabled,
			lead_goal = EXCLUDED.lead_goal,
			purchase_goal = EXCLUDED.purchase_goal,
			repurchase_goal = EXCLUDED.repurchase_goal
		WHERE metrica_connections.organization_id = $2`,
		newID, orgID, trafficSourceID, s.CounterID, cipher, preview, s.Currency, s.Enabled, s.LeadGoal, s.PurchaseGoal, s.RepurchaseGoal)
	if err != nil {
		return fmt.Errorf("offlineconv: writing Metrica connection: %w", err)
	}
	return nil
}

// deleteMetrica removes the source's connection; its uploads stay, unsent.
func (r *Repository) deleteMetrica(ctx context.Context, orgID, trafficSourceID string) (bool, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM metrica_connections WHERE organization_id = $1 AND traffic_source_id = $2`,
		orgID, trafficSourceID)
	if err != nil {
		return false, fmt.Errorf("offlineconv: deleting Metrica connection: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// --- the worker's (every organization, never over HTTP) ---

// campaignMetrica is the Metrica connection of the campaign's traffic
// source; found false when it has none.
func (r *Repository) campaignMetrica(ctx context.Context, orgID, campaignID string) (metricaConnection, bool, error) {
	m, err := scanMetrica(r.db.QueryRow(ctx, `
		SELECT `+metricaColumns+`
		FROM campaigns cm
		JOIN metrica_connections m ON m.traffic_source_id = cm.traffic_source_id AND m.organization_id = cm.organization_id
		WHERE cm.organization_id = $1 AND cm.id = $2`, orgID, campaignID))
	if errors.Is(err, pgx.ErrNoRows) {
		return metricaConnection{}, false, nil
	}
	if err != nil {
		return metricaConnection{}, false, fmt.Errorf("offlineconv: reading the campaign's Metrica connection: %w", err)
	}
	return m, true, nil
}

func (r *Repository) metricaByID(ctx context.Context, orgID, id string) (metricaConnection, error) {
	m, err := scanMetrica(r.db.QueryRow(ctx, `
		SELECT `+metricaColumns+` FROM metrica_connections m WHERE m.organization_id = $1 AND m.id = $2`, orgID, id))
	if err != nil {
		return metricaConnection{}, fmt.Errorf("offlineconv: reading Metrica connection %s: %w", id, err)
	}
	return m, nil
}

// inCurrency converts amount from one currency to another at the rates of
// at's date (the latest within fx.MaxStaleDays, as conversion.PostgresFX
// does); nil when either has no rate.
func (r *Repository) inCurrency(ctx context.Context, amount float64, from, to string, at time.Time) (*float64, error) {
	if from == to {
		return &amount, nil
	}
	var out *float64
	err := r.db.QueryRow(ctx, `
		WITH rate(currency, to_usd) AS (
			SELECT c, CASE WHEN c = 'USD' THEN 1::float8 ELSE (
				SELECT rate_to_usd::float8 FROM fx_rates
				WHERE currency = c AND rate_date <= $4::date AND rate_date > $4::date - $5::int
				ORDER BY rate_date DESC LIMIT 1) END
			FROM unnest(ARRAY[$2::text, $3::text]) c
		)
		SELECT $1::float8 * (SELECT to_usd FROM rate WHERE currency = $2) / nullif((SELECT to_usd FROM rate WHERE currency = $3), 0)`,
		amount, from, to, at.UTC().Format(time.DateOnly), fx.MaxStaleDays).Scan(&out)
	if err != nil {
		return nil, fmt.Errorf("offlineconv: converting %s to %s: %w", from, to, err)
	}
	return out, nil
}

// unchecked is a Metrica upload (one file) whose matching is not final.
type unchecked struct {
	OrganizationID      string
	MetricaConnectionID string
	RequestID           string
}

// uncheckedUploads: files sent in the last two days, not final, last
// looked at more than recheck ago (Metrica takes up to three hours, and
// its quota is 5000 requests a day per user).
func (r *Repository) uncheckedUploads(ctx context.Context, recheck time.Duration, limit int) ([]unchecked, error) {
	rows, err := r.db.Query(ctx, `
		SELECT organization_id, metrica_connection_id, request_id FROM offline_conversion_uploads
		WHERE metrica_connection_id IS NOT NULL AND status = 'sent' AND remote_status NOT IN ('PROCESSED', 'LINKAGE_FAILURE')
		  AND request_id <> '' AND sent_at > now() - interval '2 days' AND updated_at <= now() - make_interval(secs => $1)
		  AND ($3 = '' OR organization_id = $3)
		GROUP BY organization_id, metrica_connection_id, request_id
		ORDER BY min(sent_at)
		LIMIT $2`, recheck.Seconds(), limit, r.onlyOrg)
	if err != nil {
		return nil, fmt.Errorf("offlineconv: listing unchecked Metrica uploads: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[unchecked])
}

// setRemoteStatus records Metrica's status of a file on its rows. A file
// none of whose rows matched a visit fails them with message.
func (r *Repository) setRemoteStatus(ctx context.Context, u unchecked, status, message string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE offline_conversion_uploads SET
			remote_status = $4,
			status = CASE WHEN $4 = 'LINKAGE_FAILURE' THEN 'failed' ELSE status END,
			last_error = CASE WHEN $4 = 'LINKAGE_FAILURE' THEN $5 ELSE last_error END,
			updated_at = now()
		WHERE organization_id = $1 AND metrica_connection_id = $2 AND request_id = $3 AND status = 'sent'`,
		u.OrganizationID, u.MetricaConnectionID, u.RequestID, status, message)
	if err != nil {
		return fmt.Errorf("offlineconv: recording Metrica's upload status: %w", err)
	}
	return nil
}

// touchUnchecked marks a file looked at, so a failing check waits recheck
// before the next try.
func (r *Repository) touchUnchecked(ctx context.Context, u unchecked) error {
	_, err := r.db.Exec(ctx, `
		UPDATE offline_conversion_uploads SET updated_at = now()
		WHERE organization_id = $1 AND metrica_connection_id = $2 AND request_id = $3 AND status = 'sent'`,
		u.OrganizationID, u.MetricaConnectionID, u.RequestID)
	if err != nil {
		return fmt.Errorf("offlineconv: touching a Metrica upload: %w", err)
	}
	return nil
}
