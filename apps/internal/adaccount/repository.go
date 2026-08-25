package adaccount

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// selectColumns never includes access_token — every public read goes
// through this, so there is no query path in this package that could
// accidentally leak the raw token into a JSON response. tokenPreview is
// computed in SQL (last 4 characters) rather than in Go, for the same
// reason: it's one less place a future edit could reintroduce the full
// column into a Go struct that then gets marshaled.
const selectColumns = `id, organization_id, traffic_source_id, ad_account_id, right(access_token, 4), created_at, updated_at`

func scanConnection(row pgx.Row) (Connection, error) {
	var c Connection
	err := row.Scan(&c.ID, &c.OrganizationID, &c.TrafficSourceID, &c.AdAccountID, &c.TokenPreview, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (r *Repository) GetByTrafficSourceID(ctx context.Context, orgID, trafficSourceID string) (Connection, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+selectColumns+`
		FROM ad_account_connections
		WHERE traffic_source_id = $1 AND organization_id = $2`,
		trafficSourceID, orgID,
	)
	c, err := scanConnection(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Connection{}, apierror.NotFound("no ad account connected for this traffic source")
	}
	return c, err
}

// Connect upserts by traffic_source_id — re-connecting (a fresh token,
// or the same one re-pasted) replaces the row in place, same "re-
// submitting updates it, it doesn't stack" precedent as cost_entries'
// own dedup-key-as-identity design (00009).
func (r *Repository) Connect(ctx context.Context, id, orgID, trafficSourceID string, in ConnectInput) (Connection, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO ad_account_connections (id, organization_id, traffic_source_id, ad_account_id, access_token)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (traffic_source_id) DO UPDATE SET
			ad_account_id = EXCLUDED.ad_account_id,
			access_token = EXCLUDED.access_token
		RETURNING `+selectColumns,
		id, orgID, trafficSourceID, in.AdAccountID, in.AccessToken,
	)
	return scanConnection(row)
}

func (r *Repository) Disconnect(ctx context.Context, orgID, trafficSourceID string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM ad_account_connections WHERE traffic_source_id = $1 AND organization_id = $2`, trafficSourceID, orgID)
	if err != nil {
		return fmt.Errorf("disconnecting ad account: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("no ad account connected for this traffic source")
	}
	return nil
}

// TrafficSourceCostIntegration returns the traffic source's own
// cost_integration value (a raw query against trafficsource's table,
// same as streamset.Repository's own cross-domain checks — not an
// import of the trafficsource package, to avoid a needless dependency
// for one column read) and whether it belongs to orgID at all.
func (r *Repository) TrafficSourceCostIntegration(ctx context.Context, orgID, trafficSourceID string) (costIntegration string, found bool, err error) {
	err = r.db.QueryRow(ctx,
		`SELECT cost_integration FROM traffic_sources WHERE id = $1 AND organization_id = $2`,
		trafficSourceID, orgID,
	).Scan(&costIntegration)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return costIntegration, true, nil
}
