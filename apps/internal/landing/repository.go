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
	"github.com/ismagilovnail/flox/apps/internal/pagination"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const selectColumns = `id, organization_id, name, title, logo_url, type, url, content, entry_file,
	bundle_prefix, file_count, bundle_size, status, created_at, updated_at`

func scanLanding(row pgx.Row) (Landing, error) {
	var l Landing
	err := row.Scan(&l.ID, &l.OrganizationID, &l.Name, &l.Title, &l.LogoURL, &l.Type, &l.URL, &l.Content, &l.EntryFile,
		&l.BundlePrefix, &l.FileCount, &l.BundleSize, &l.Status, &l.CreatedAt, &l.UpdatedAt)
	return l, err
}

func (r *Repository) List(ctx context.Context, organizationID string, page pagination.Page) ([]Landing, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+selectColumns+`
		FROM landings
		WHERE organization_id = $1
		ORDER BY name, id
		LIMIT $2 OFFSET $3`,
		organizationID, page.Limit, page.Offset,
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

// Create inserts l as-is — ID and OrganizationID included; the service
// has already validated and derived every field. An empty Status means
// the DB default (active).
func (r *Repository) Create(ctx context.Context, l Landing) (Landing, error) {
	if l.EntryFile == "" {
		l.EntryFile = "index.html"
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO landings (id, organization_id, name, title, logo_url, type, url, content, entry_file,
			bundle_prefix, file_count, bundle_size, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, COALESCE(NULLIF($13, ''), 'active'))
		RETURNING `+selectColumns,
		l.ID, l.OrganizationID, l.Name, l.Title, l.LogoURL, l.Type, l.URL, l.Content, l.EntryFile,
		l.BundlePrefix, l.FileCount, l.BundleSize, string(l.Status),
	)
	return scanLanding(row)
}

// SetBundle swaps in a newly uploaded archive's entry file and asset
// prefix in one statement — the ID (and everything flows reference it
// by) stays the same.
func (r *Repository) SetBundle(ctx context.Context, orgID, id string, b *bundle, prefix string) (Landing, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE landings
		SET content = $3, entry_file = $4, bundle_prefix = $5, file_count = $6, bundle_size = $7
		WHERE id = $1 AND organization_id = $2
		RETURNING `+selectColumns,
		id, orgID, b.EntryHTML, b.EntryFile, prefix, b.FileCount, b.TotalSize,
	)
	l, err := scanLanding(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Landing{}, apierror.NotFound("landing not found")
	}
	return l, err
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
	if in.Title != nil {
		sets = append(sets, "title = "+arg(*in.Title))
	}
	if in.LogoURL != nil {
		sets = append(sets, "logo_url = "+arg(*in.LogoURL))
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
// RESTRICT), and Flow CRUD populates it — a landing still used by a flow
// surfaces as a Conflict, same shape as network.Repository.Delete.
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

// Count is how many landings rows the organization has — List's total.
func (r *Repository) Count(ctx context.Context, organizationID string) (int, error) {
	return pagination.Count(ctx, r.db, "landings", organizationID)
}
