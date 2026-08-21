package eventmapping

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const selectColumns = `id, organization_id, network_id, network_status, flox_status, created_at, updated_at`

func scanEventMapping(row pgx.Row) (EventMapping, error) {
	var m EventMapping
	err := row.Scan(&m.ID, &m.OrganizationID, &m.NetworkID, &m.NetworkStatus, &m.FloxStatus, &m.CreatedAt, &m.UpdatedAt)
	return m, err
}

// List returns every mapping across the whole organization, not scoped to
// one network — the frontend panel groups them by network client-side
// (one card per network), matching the old mock's org-wide EVENT_MAPPINGS
// array, so one request beats one per network.
func (r *Repository) List(ctx context.Context, orgID string) ([]EventMapping, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+selectColumns+`
		FROM event_mappings
		WHERE organization_id = $1
		ORDER BY network_id, created_at`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("eventmapping: listing: %w", err)
	}
	defer rows.Close()

	mappings := []EventMapping{}
	for rows.Next() {
		m, err := scanEventMapping(rows)
		if err != nil {
			return nil, fmt.Errorf("eventmapping: scanning: %w", err)
		}
		mappings = append(mappings, m)
	}
	return mappings, rows.Err()
}

func (r *Repository) NetworkBelongsToOrg(ctx context.Context, orgID, networkID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM networks WHERE id = $1 AND organization_id = $2)`, networkID, orgID).Scan(&exists)
	return exists, err
}

// Create relies on event_mappings_network_status_idx (00012's unique
// index on (network_id, lower(network_status))) to catch a duplicate
// mapping — cheaper and race-free compared with a check-then-insert.
func (r *Repository) Create(ctx context.Context, id, orgID string, in CreateInput) (EventMapping, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO event_mappings (id, organization_id, network_id, network_status, flox_status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+selectColumns,
		id, orgID, in.NetworkID, in.NetworkStatus, string(in.FloxStatus),
	)
	m, err := scanEventMapping(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return EventMapping{}, apierror.Conflict("a mapping for this network's status already exists")
		}
		return EventMapping{}, fmt.Errorf("eventmapping: creating: %w", err)
	}
	return m, nil
}

func (r *Repository) Delete(ctx context.Context, orgID, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM event_mappings WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		return fmt.Errorf("eventmapping: deleting: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("event mapping not found")
	}
	return nil
}
