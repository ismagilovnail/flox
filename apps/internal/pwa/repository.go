package pwa

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

const selectColumns = `id, organization_id, name, short_name, theme_color, background_color, icon_url, start_url, bounce_in_app_webview, status, created_at, updated_at`

func scanPwa(row pgx.Row) (Pwa, error) {
	var p Pwa
	err := row.Scan(
		&p.ID, &p.OrganizationID, &p.Name, &p.ShortName, &p.ThemeColor, &p.BackgroundColor,
		&p.IconURL, &p.StartURL, &p.BounceInAppWebview, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	return p, err
}

func (r *Repository) List(ctx context.Context, organizationID string) ([]Pwa, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+selectColumns+`
		FROM pwas
		WHERE organization_id = $1
		ORDER BY name`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("pwa: listing: %w", err)
	}
	defer rows.Close()

	out := []Pwa{}
	for rows.Next() {
		p, err := scanPwa(rows)
		if err != nil {
			return nil, fmt.Errorf("pwa: scanning: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) GetByID(ctx context.Context, orgID, id string) (Pwa, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+selectColumns+`
		FROM pwas
		WHERE id = $1 AND organization_id = $2`,
		id, orgID,
	)
	p, err := scanPwa(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Pwa{}, apierror.NotFound("pwa not found")
	}
	return p, err
}

func (r *Repository) Create(ctx context.Context, id, orgID string, in CreateInput) (Pwa, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO pwas (id, organization_id, name, short_name, theme_color, background_color, icon_url, start_url, bounce_in_app_webview)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+selectColumns,
		id, orgID, in.Name, in.ShortName, in.ThemeColor, in.BackgroundColor, in.IconURL, in.StartURL, in.BounceInAppWebview,
	)
	return scanPwa(row)
}

func (r *Repository) Update(ctx context.Context, orgID, id string, in UpdateInput) (Pwa, error) {
	sets := []string{}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if in.Name != nil {
		sets = append(sets, "name = "+arg(*in.Name))
	}
	if in.ShortName != nil {
		sets = append(sets, "short_name = "+arg(*in.ShortName))
	}
	if in.ThemeColor != nil {
		sets = append(sets, "theme_color = "+arg(*in.ThemeColor))
	}
	if in.BackgroundColor != nil {
		sets = append(sets, "background_color = "+arg(*in.BackgroundColor))
	}
	if in.IconURL != nil {
		sets = append(sets, "icon_url = "+arg(*in.IconURL))
	}
	if in.StartURL != nil {
		sets = append(sets, "start_url = "+arg(*in.StartURL))
	}
	if in.BounceInAppWebview != nil {
		sets = append(sets, "bounce_in_app_webview = "+arg(*in.BounceInAppWebview))
	}
	if in.Status != nil {
		sets = append(sets, "status = "+arg(*in.Status))
	}

	if len(sets) == 0 {
		return r.GetByID(ctx, orgID, id)
	}

	query := fmt.Sprintf(
		`UPDATE pwas SET %s WHERE id = %s AND organization_id = %s RETURNING %s`,
		strings.Join(sets, ", "), arg(id), arg(orgID), selectColumns,
	)

	row := r.db.QueryRow(ctx, query, args...)
	p, err := scanPwa(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Pwa{}, apierror.NotFound("pwa not found")
	}
	return p, err
}

// Delete: flows.pwa_id (00006) has no ON DELETE clause (defaults to
// RESTRICT) — no Flow CRUD exists yet to ever populate that column, but
// the 23503 catch is here defensively for when it does, same shape as
// landing.Repository.Delete/network.Repository.Delete.
func (r *Repository) Delete(ctx context.Context, orgID, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM pwas WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return apierror.Conflict("pwa is still referenced by one or more flows")
		}
		return fmt.Errorf("deleting pwa: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("pwa not found")
	}
	return nil
}
