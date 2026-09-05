package custommetrics

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const columns = `id, organization_id, name, group_name, formula, format, status, active, targets, created_by_user_id, created_at, updated_at`

func scan(row pgx.Row) (CustomMetric, error) {
	var m CustomMetric
	err := row.Scan(
		&m.ID, &m.OrganizationID, &m.Name, &m.GroupName, &m.Formula, &m.Format, &m.Status, &m.Active,
		&m.Targets, &m.CreatedByUserID, &m.CreatedAt, &m.UpdatedAt,
	)
	return m, err
}

// List returns every metric when includeUnpublished is true (the
// caller's own manage-any/manage-own tier); otherwise only
// published+active ones, matching the frontend's own visibleMetrics
// split — enforced here too, not just in the UI, per §52's "frontend
// permissions are only UX" doc comment on MeResponse.
func (r *Repository) List(ctx context.Context, orgID string, includeUnpublished bool) ([]CustomMetric, error) {
	query := `SELECT ` + columns + ` FROM custom_metrics WHERE organization_id = $1`
	if !includeUnpublished {
		query += ` AND status = 'published' AND active`
	}
	query += ` ORDER BY group_name, name`

	rows, err := r.db.Query(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("custommetrics: listing: %w", err)
	}
	defer rows.Close()

	out := []CustomMetric{}
	for rows.Next() {
		m, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("custommetrics: scanning: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repository) GetByID(ctx context.Context, orgID, id string) (CustomMetric, error) {
	row := r.db.QueryRow(ctx, `SELECT `+columns+` FROM custom_metrics WHERE id = $1 AND organization_id = $2`, id, orgID)
	m, err := scan(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return CustomMetric{}, apierror.NotFound("custom metric not found")
	}
	return m, err
}

func (r *Repository) Create(ctx context.Context, orgID, createdByUserID string, in CreateInput) (CustomMetric, error) {
	active := in.Status == StatusPublished
	// A nil Go slice encodes as SQL NULL, which the column's NOT NULL
	// constraint rejects (its DEFAULT '{}' only applies when the column
	// is omitted from the INSERT, not when NULL is passed explicitly) —
	// normalized here too, not just at each Service call site, so this
	// can't resurface from a future caller.
	targets := in.Targets
	if targets == nil {
		targets = []string{}
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO custom_metrics (id, organization_id, name, group_name, formula, format, status, active, targets, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING `+columns,
		idgen.New(), orgID, in.Name, in.GroupName, in.Formula, in.Format, in.Status, active, targets, createdByUserID,
	)
	return scan(row)
}

func (r *Repository) Update(ctx context.Context, orgID, id string, in UpdateInput) (CustomMetric, error) {
	sets := []string{}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if in.Name != nil {
		sets = append(sets, "name = "+arg(*in.Name))
	}
	if in.GroupName != nil {
		sets = append(sets, "group_name = "+arg(*in.GroupName))
	}
	if in.Formula != nil {
		sets = append(sets, "formula = "+arg(*in.Formula))
	}
	if in.Format != nil {
		sets = append(sets, "format = "+arg(*in.Format))
	}
	if in.Status != nil {
		sets = append(sets, "status = "+arg(*in.Status))
	}
	if in.Targets != nil {
		sets = append(sets, "targets = "+arg(*in.Targets))
	}
	if in.Active != nil {
		sets = append(sets, "active = "+arg(*in.Active))
	}

	if len(sets) == 0 {
		return r.GetByID(ctx, orgID, id)
	}

	query := fmt.Sprintf(
		`UPDATE custom_metrics SET %s WHERE id = %s AND organization_id = %s RETURNING %s`,
		strings.Join(sets, ", "), arg(id), arg(orgID), columns,
	)
	row := r.db.QueryRow(ctx, query, args...)
	m, err := scan(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return CustomMetric{}, apierror.NotFound("custom metric not found")
	}
	return m, err
}

func (r *Repository) Delete(ctx context.Context, orgID, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM custom_metrics WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		return fmt.Errorf("custommetrics: deleting: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("custom metric not found")
	}
	return nil
}

// CallerRoleKey looks up the caller's own role within orgID — used to
// distinguish the "manage any metric" tier (Owner/Admin) from "manage
// only metrics I created" (Manager); both hold the same coarse
// custom_metric.write permission, so RequirePermission alone can't tell
// them apart.
func (r *Repository) CallerRoleKey(ctx context.Context, orgID, userID string) (string, error) {
	var key string
	err := r.db.QueryRow(ctx, `
		SELECT r.key FROM memberships m
		JOIN roles r ON r.id = m.role_id
		WHERE m.user_id = $1 AND m.organization_id = $2`,
		userID, orgID,
	).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apierror.Forbidden("no membership in this organization")
	}
	return key, err
}
