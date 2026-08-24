package postlanding

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

const selectColumns = `id, organization_id, name, url, events, status, created_at, updated_at`

func scanPostlanding(row pgx.Row) (Postlanding, error) {
	var p Postlanding
	err := row.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.URL, &p.Events, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (r *Repository) List(ctx context.Context, organizationID string) ([]Postlanding, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+selectColumns+`
		FROM postlandings
		WHERE organization_id = $1
		ORDER BY name`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("postlanding: listing: %w", err)
	}
	defer rows.Close()

	out := []Postlanding{}
	for rows.Next() {
		p, err := scanPostlanding(rows)
		if err != nil {
			return nil, fmt.Errorf("postlanding: scanning: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) GetByID(ctx context.Context, orgID, id string) (Postlanding, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+selectColumns+`
		FROM postlandings
		WHERE id = $1 AND organization_id = $2`,
		id, orgID,
	)
	p, err := scanPostlanding(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Postlanding{}, apierror.NotFound("postlanding not found")
	}
	return p, err
}

func (r *Repository) Create(ctx context.Context, id, orgID string, in CreateInput) (Postlanding, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO postlandings (id, organization_id, name, url, events)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+selectColumns,
		id, orgID, in.Name, in.URL, in.Events,
	)
	return scanPostlanding(row)
}

func (r *Repository) Update(ctx context.Context, orgID, id string, in UpdateInput) (Postlanding, error) {
	sets := []string{}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if in.Name != nil {
		sets = append(sets, "name = "+arg(*in.Name))
	}
	if in.URL != nil {
		sets = append(sets, "url = "+arg(*in.URL))
	}
	if in.Events != nil {
		sets = append(sets, "events = "+arg(*in.Events))
	}
	if in.Status != nil {
		sets = append(sets, "status = "+arg(*in.Status))
	}

	if len(sets) == 0 {
		return r.GetByID(ctx, orgID, id)
	}

	query := fmt.Sprintf(
		`UPDATE postlandings SET %s WHERE id = %s AND organization_id = %s RETURNING %s`,
		strings.Join(sets, ", "), arg(id), arg(orgID), selectColumns,
	)

	row := r.db.QueryRow(ctx, query, args...)
	p, err := scanPostlanding(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Postlanding{}, apierror.NotFound("postlanding not found")
	}
	return p, err
}

// Delete: flows.postlanding_id (00006) has no ON DELETE clause (defaults
// to RESTRICT) — no Flow CRUD exists yet to ever populate that column,
// but the 23503 catch is here defensively for when it does, same shape
// as landing.Repository.Delete/network.Repository.Delete.
func (r *Repository) Delete(ctx context.Context, orgID, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM postlandings WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return apierror.Conflict("postlanding is still referenced by one or more flows")
		}
		return fmt.Errorf("deleting postlanding: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("postlanding not found")
	}
	return nil
}
