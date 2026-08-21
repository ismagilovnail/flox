package trafficsource

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const selectColumns = `id, organization_id, name, type, tracking_template, cost_integration, status, created_at, updated_at`

func scanSource(row pgx.Row) (TrafficSource, error) {
	var s TrafficSource
	err := row.Scan(&s.ID, &s.OrganizationID, &s.Name, &s.Type, &s.TrackingTemplate, &s.CostIntegration, &s.Status, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

// List returns every traffic source for organizationID, ordered by name —
// unpaginated (Phase 27's own shape): a campaign/cost-entry source picker
// and the traffic sources table (which paginates client-side, same as
// campaigns) both want the whole list, and per-org source counts are
// small enough that this doesn't need the Limit/Offset campaign.List has.
func (r *Repository) List(ctx context.Context, organizationID string) ([]TrafficSource, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+selectColumns+`
		FROM traffic_sources
		WHERE organization_id = $1
		ORDER BY name`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("trafficsource: listing: %w", err)
	}
	defer rows.Close()

	out := []TrafficSource{}
	for rows.Next() {
		s, err := scanSource(rows)
		if err != nil {
			return nil, fmt.Errorf("trafficsource: scanning: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("trafficsource: reading: %w", err)
	}
	return out, nil
}

func (r *Repository) GetByID(ctx context.Context, orgID, id string) (TrafficSource, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+selectColumns+`
		FROM traffic_sources
		WHERE id = $1 AND organization_id = $2`,
		id, orgID,
	)
	s, err := scanSource(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return TrafficSource{}, apierror.NotFound("traffic source not found")
	}
	return s, err
}

func (r *Repository) Create(ctx context.Context, id, orgID string, in CreateInput) (TrafficSource, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO traffic_sources (id, organization_id, name, type, tracking_template, cost_integration)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+selectColumns,
		id, orgID, in.Name, in.Type, in.TrackingTemplate, in.CostIntegration,
	)
	return scanSource(row)
}

// Update applies only the non-nil fields in in, via a dynamically built
// SET clause — a real partial update, matching campaign.Repository.Update.
func (r *Repository) Update(ctx context.Context, orgID, id string, in UpdateInput) (TrafficSource, error) {
	sets := []string{}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if in.Name != nil {
		sets = append(sets, "name = "+arg(*in.Name))
	}
	if in.Type != nil {
		sets = append(sets, "type = "+arg(*in.Type))
	}
	if in.TrackingTemplate != nil {
		sets = append(sets, "tracking_template = "+arg(*in.TrackingTemplate))
	}
	if in.CostIntegration != nil {
		sets = append(sets, "cost_integration = "+arg(*in.CostIntegration))
	}
	if in.Status != nil {
		sets = append(sets, "status = "+arg(*in.Status))
	}

	if len(sets) == 0 {
		return r.GetByID(ctx, orgID, id)
	}

	query := fmt.Sprintf(
		`UPDATE traffic_sources SET %s WHERE id = %s AND organization_id = %s RETURNING %s`,
		strings.Join(sets, ", "), arg(id), arg(orgID), selectColumns,
	)

	row := r.db.QueryRow(ctx, query, args...)
	s, err := scanSource(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return TrafficSource{}, apierror.NotFound("traffic source not found")
	}
	return s, err
}

// Delete has no FK cascade from campaigns.traffic_source_id (00005, NOT
// NULL REFERENCES with no ON DELETE clause — Postgres defaults to
// RESTRICT) — deliberately: a source with campaigns still pointing at it
// shouldn't silently vanish out from under them. A raw 23503 here would
// otherwise surface as an opaque 500; caught and turned into the same
// apierror.Conflict shape every other domain error in this API uses.
func (r *Repository) Delete(ctx context.Context, orgID, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM traffic_sources WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return apierror.Conflict("traffic source is still referenced by one or more campaigns")
		}
		return fmt.Errorf("deleting traffic source: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("traffic source not found")
	}
	return nil
}
