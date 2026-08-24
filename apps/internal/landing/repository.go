package landing

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

const selectColumns = `id, organization_id, name, type, url, content, status, created_at, updated_at`

func scanLanding(row pgx.Row) (Landing, error) {
	var l Landing
	err := row.Scan(&l.ID, &l.OrganizationID, &l.Name, &l.Type, &l.URL, &l.Content, &l.Status, &l.CreatedAt, &l.UpdatedAt)
	return l, err
}

func (r *Repository) List(ctx context.Context, organizationID string) ([]Landing, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+selectColumns+`
		FROM landings
		WHERE organization_id = $1
		ORDER BY name`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("landing: listing: %w", err)
	}
	defer rows.Close()

	out := []Landing{}
	for rows.Next() {
		l, err := scanLanding(rows)
		if err != nil {
			return nil, fmt.Errorf("landing: scanning: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *Repository) GetByID(ctx context.Context, orgID, id string) (Landing, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+selectColumns+`
		FROM landings
		WHERE id = $1 AND organization_id = $2`,
		id, orgID,
	)
	l, err := scanLanding(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Landing{}, apierror.NotFound("landing not found")
	}
	return l, err
}

func (r *Repository) Create(ctx context.Context, id, orgID string, in CreateInput) (Landing, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO landings (id, organization_id, name, type, url, content)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+selectColumns,
		id, orgID, in.Name, in.Type, in.URL, in.Content,
	)
	return scanLanding(row)
}

func (r *Repository) Update(ctx context.Context, orgID, id string, in UpdateInput) (Landing, error) {
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
	if in.URL != nil {
		sets = append(sets, "url = "+arg(*in.URL))
	}
	if in.Content != nil {
		sets = append(sets, "content = "+arg(*in.Content))
	}
	if in.Status != nil {
		sets = append(sets, "status = "+arg(*in.Status))
	}

	if len(sets) == 0 {
		return r.GetByID(ctx, orgID, id)
	}

	query := fmt.Sprintf(
		`UPDATE landings SET %s WHERE id = %s AND organization_id = %s RETURNING %s`,
		strings.Join(sets, ", "), arg(id), arg(orgID), selectColumns,
	)

	row := r.db.QueryRow(ctx, query, args...)
	l, err := scanLanding(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Landing{}, apierror.NotFound("landing not found")
	}
	return l, err
}

// Delete: flows.landing_id (00006) has no ON DELETE clause (defaults to
// RESTRICT) — no Flow CRUD exists yet to ever populate that column, but
// the 23503 catch is here defensively for when it does, same shape as
// network.Repository.Delete.
func (r *Repository) Delete(ctx context.Context, orgID, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM landings WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return apierror.Conflict("landing is still referenced by one or more flows")
		}
		return fmt.Errorf("deleting landing: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("landing not found")
	}
	return nil
}
