package pixel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const selectColumns = `id, organization_id, name, provider, pixel_id, events, status, created_at, updated_at`

func scanPixel(row pgx.Row) (Pixel, error) {
	var p Pixel
	err := row.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Provider, &p.PixelID, &p.Events, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (r *Repository) List(ctx context.Context, organizationID string) ([]Pixel, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+selectColumns+`
		FROM pixels
		WHERE organization_id = $1
		ORDER BY name`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("pixel: listing: %w", err)
	}
	defer rows.Close()

	out := []Pixel{}
	for rows.Next() {
		p, err := scanPixel(rows)
		if err != nil {
			return nil, fmt.Errorf("pixel: scanning: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) GetByID(ctx context.Context, orgID, id string) (Pixel, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+selectColumns+`
		FROM pixels
		WHERE id = $1 AND organization_id = $2`,
		id, orgID,
	)
	p, err := scanPixel(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Pixel{}, apierror.NotFound("pixel not found")
	}
	return p, err
}

func (r *Repository) Create(ctx context.Context, id, orgID string, in CreateInput) (Pixel, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO pixels (id, organization_id, name, provider, pixel_id, events)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+selectColumns,
		id, orgID, in.Name, in.Provider, in.PixelID, in.Events,
	)
	return scanPixel(row)
}

func (r *Repository) Update(ctx context.Context, orgID, id string, in UpdateInput) (Pixel, error) {
	sets := []string{}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if in.Name != nil {
		sets = append(sets, "name = "+arg(*in.Name))
	}
	if in.Provider != nil {
		sets = append(sets, "provider = "+arg(*in.Provider))
	}
	if in.PixelID != nil {
		sets = append(sets, "pixel_id = "+arg(*in.PixelID))
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
		`UPDATE pixels SET %s WHERE id = %s AND organization_id = %s RETURNING %s`,
		strings.Join(sets, ", "), arg(id), arg(orgID), selectColumns,
	)

	row := r.db.QueryRow(ctx, query, args...)
	p, err := scanPixel(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Pixel{}, apierror.NotFound("pixel not found")
	}
	return p, err
}

// Delete: stream_set_pixels.pixel_id (00008) CASCADEs from pixels, so no
// FK conflict is possible there — unlike landing/pwa/postlanding's
// RESTRICT-guarded flows.*_id columns, deleting a pixel just drops its
// stream-set attachments along with it. No defensive 23503 catch needed,
// unlike those siblings' Delete.
func (r *Repository) Delete(ctx context.Context, orgID, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM pixels WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		return fmt.Errorf("deleting pixel: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("pixel not found")
	}
	return nil
}
