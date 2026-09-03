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

const selectColumns = `id, organization_id, name, short_name, theme_color, background_color, icon_url, start_url, bounce_in_app_webview, developer, description, rating, rating_count, downloads_label, size_mb, screenshots, whats_new, rating_stars_5, rating_stars_4, rating_stars_3, rating_stars_2, rating_stars_1, status, created_at, updated_at`

func scanPwa(row pgx.Row) (Pwa, error) {
	var p Pwa
	err := row.Scan(
		&p.ID, &p.OrganizationID, &p.Name, &p.ShortName, &p.ThemeColor, &p.BackgroundColor,
		&p.IconURL, &p.StartURL, &p.BounceInAppWebview, &p.Developer, &p.Description,
		&p.Rating, &p.RatingCount, &p.DownloadsLabel, &p.SizeMB, &p.Screenshots, &p.WhatsNew,
		&p.RatingStars5, &p.RatingStars4, &p.RatingStars3, &p.RatingStars2, &p.RatingStars1,
		&p.Status, &p.CreatedAt, &p.UpdatedAt,
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
		INSERT INTO pwas (id, organization_id, name, short_name, theme_color, background_color, icon_url, start_url, bounce_in_app_webview, developer, description, rating, rating_count, downloads_label, size_mb, screenshots, whats_new, rating_stars_5, rating_stars_4, rating_stars_3, rating_stars_2, rating_stars_1)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
		RETURNING `+selectColumns,
		id, orgID, in.Name, in.ShortName, in.ThemeColor, in.BackgroundColor, in.IconURL, in.StartURL, in.BounceInAppWebview,
		in.Developer, in.Description, in.Rating, in.RatingCount, in.DownloadsLabel, in.SizeMB,
		in.Screenshots, in.WhatsNew, in.RatingStars5, in.RatingStars4, in.RatingStars3, in.RatingStars2, in.RatingStars1,
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
	if in.Developer != nil {
		sets = append(sets, "developer = "+arg(*in.Developer))
	}
	if in.Description != nil {
		sets = append(sets, "description = "+arg(*in.Description))
	}
	if in.Rating != nil {
		sets = append(sets, "rating = "+arg(*in.Rating))
	}
	if in.RatingCount != nil {
		sets = append(sets, "rating_count = "+arg(*in.RatingCount))
	}
	if in.DownloadsLabel != nil {
		sets = append(sets, "downloads_label = "+arg(*in.DownloadsLabel))
	}
	if in.SizeMB != nil {
		sets = append(sets, "size_mb = "+arg(*in.SizeMB))
	}
	if in.Screenshots != nil {
		sets = append(sets, "screenshots = "+arg(*in.Screenshots))
	}
	if in.WhatsNew != nil {
		sets = append(sets, "whats_new = "+arg(*in.WhatsNew))
	}
	if in.RatingStars5 != nil {
		sets = append(sets, "rating_stars_5 = "+arg(*in.RatingStars5))
	}
	if in.RatingStars4 != nil {
		sets = append(sets, "rating_stars_4 = "+arg(*in.RatingStars4))
	}
	if in.RatingStars3 != nil {
		sets = append(sets, "rating_stars_3 = "+arg(*in.RatingStars3))
	}
	if in.RatingStars2 != nil {
		sets = append(sets, "rating_stars_2 = "+arg(*in.RatingStars2))
	}
	if in.RatingStars1 != nil {
		sets = append(sets, "rating_stars_1 = "+arg(*in.RatingStars1))
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
