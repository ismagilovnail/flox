package reportpresets

import (
	"context"
	"encoding/json"
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

// period_from/period_to are read back through to_char rather than scanned
// as time.Time — same "plain YYYY-MM-DD string, not a timestamp" precedent
// pwa.updated_date already established, since a report period boundary is
// a date, not a moment.
const presetColumns = `
	id, organization_id, name, dimensions, metrics, filters, column_order, column_widths, group_by,
	period_type, period_days, to_char(period_from, 'YYYY-MM-DD'), to_char(period_to, 'YYYY-MM-DD'),
	timezone, is_default, created_by_user_id, created_at, updated_at`

func scanPreset(row pgx.Row) (ReportPreset, error) {
	var p ReportPreset
	var periodFrom, periodTo *string
	err := row.Scan(
		&p.ID, &p.OrganizationID, &p.Name, &p.Dimensions, &p.Metrics, &p.Filters, &p.ColumnOrder, &p.ColumnWidths, &p.GroupBy,
		&p.Period.Type, &p.Period.Days, &periodFrom, &periodTo,
		&p.Timezone, &p.IsDefault, &p.CreatedByID, &p.CreatedAt, &p.UpdatedAt,
	)
	p.Period.From = periodFrom
	p.Period.To = periodTo
	if p.Filters == nil {
		p.Filters = []Filter{} // serialize as [], never null
	}
	if p.ColumnOrder == nil {
		p.ColumnOrder = []string{}
	}
	if p.ColumnWidths == nil {
		p.ColumnWidths = map[string]int{}
	}
	return p, err
}

// nonNil keeps a nil slice from being sent as SQL NULL into a NOT NULL array column.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// widthsJSON is the jsonb parameter for a preset's column widths: nil is {}.
func widthsJSON(w map[string]int) string {
	if len(w) == 0 {
		return "{}"
	}
	b, err := json.Marshal(w) // a map of string to int cannot fail to marshal
	if err != nil {
		return "{}"
	}
	return string(b)
}

// filtersJSON is the jsonb parameter for a preset's filters: nil is [].
func filtersJSON(f []Filter) string {
	if len(f) == 0 {
		return "[]"
	}
	b, err := json.Marshal(f)
	if err != nil { // a slice of two strings cannot fail to marshal
		return "[]"
	}
	return string(b)
}

func (r *Repository) List(ctx context.Context, orgID string) ([]ReportPreset, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+presetColumns+`
		FROM report_presets
		WHERE organization_id = $1
		ORDER BY is_default DESC, name`, orgID)
	if err != nil {
		return nil, fmt.Errorf("reportpresets: listing: %w", err)
	}
	defer rows.Close()

	out := []ReportPreset{}
	for rows.Next() {
		p, err := scanPreset(rows)
		if err != nil {
			return nil, fmt.Errorf("reportpresets: scanning: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) Get(ctx context.Context, orgID, id string) (ReportPreset, error) {
	row := r.db.QueryRow(ctx, `SELECT `+presetColumns+` FROM report_presets WHERE id = $1 AND organization_id = $2`, id, orgID)
	p, err := scanPreset(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReportPreset{}, apierror.NotFound("report preset not found")
	}
	return p, err
}

func (r *Repository) Create(ctx context.Context, orgID, createdByUserID string, in CreateInput) (ReportPreset, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO report_presets
			(id, organization_id, name, dimensions, metrics, filters, column_order, column_widths, group_by, period_type, period_days, period_from, period_to, timezone, is_default, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8::jsonb, $9, $10, $11, $12, $13, $14, false, $15)
		RETURNING `+presetColumns,
		idgen.New(), orgID, in.Name, in.Dimensions, in.Metrics, filtersJSON(in.Filters), nonNil(in.ColumnOrder), widthsJSON(in.ColumnWidths), in.GroupBy,
		in.Period.Type, in.Period.Days, in.Period.From, in.Period.To,
		in.Timezone, createdByUserID,
	)
	return scanPreset(row)
}

// EnsureDefault lazily seeds the org's system default preset if it has
// none yet — no admin action or migration-time seed step required. The
// ON CONFLICT target matches migration 00030's partial unique index
// exactly, so concurrent first-callers from the same org race safely: at
// most one INSERT wins, everyone else falls through to the SELECT below.
func (r *Repository) EnsureDefault(ctx context.Context, orgID string) error {
	def := defaultPresetSeed
	_, err := r.db.Exec(ctx, `
		INSERT INTO report_presets
			(id, organization_id, name, dimensions, metrics, group_by, period_type, period_days, timezone, is_default, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, true, NULL)
		ON CONFLICT (organization_id) WHERE is_default DO NOTHING`,
		idgen.New(), orgID, def.Name, def.Dimensions, def.Metrics, def.GroupBy,
		def.Period.Type, def.Period.Days, def.Timezone,
	)
	if err != nil {
		return fmt.Errorf("reportpresets: seeding default: %w", err)
	}
	return nil
}

func (r *Repository) Update(ctx context.Context, orgID, id string, in UpdateInput) (ReportPreset, error) {
	sets := []string{}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if in.Name != nil {
		sets = append(sets, "name = "+arg(*in.Name))
	}
	if in.Dimensions != nil {
		sets = append(sets, "dimensions = "+arg(in.Dimensions))
	}
	if in.Metrics != nil {
		sets = append(sets, "metrics = "+arg(in.Metrics))
	}
	if in.Filters != nil {
		sets = append(sets, "filters = "+arg(filtersJSON(in.Filters))+"::jsonb")
	}
	if in.ColumnOrder != nil {
		sets = append(sets, "column_order = "+arg(in.ColumnOrder))
	}
	if in.ColumnWidths != nil {
		sets = append(sets, "column_widths = "+arg(widthsJSON(in.ColumnWidths))+"::jsonb")
	}
	if in.GroupBy != nil {
		sets = append(sets, "group_by = "+arg(*in.GroupBy))
	}
	if in.Period != nil {
		sets = append(sets, "period_type = "+arg(in.Period.Type))
		sets = append(sets, "period_days = "+arg(in.Period.Days))
		sets = append(sets, "period_from = "+arg(in.Period.From))
		sets = append(sets, "period_to = "+arg(in.Period.To))
	}
	if in.Timezone != nil {
		sets = append(sets, "timezone = "+arg(*in.Timezone))
	}
	if len(sets) == 0 {
		return r.Get(ctx, orgID, id)
	}

	query := fmt.Sprintf(
		`UPDATE report_presets SET %s WHERE id = %s AND organization_id = %s AND is_default = false RETURNING %s`,
		strings.Join(sets, ", "), arg(id), arg(orgID), presetColumns,
	)
	row := r.db.QueryRow(ctx, query, args...)
	p, err := scanPreset(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReportPreset{}, apierror.NotFound("report preset not found")
	}
	return p, err
}

// Delete's own WHERE clause excludes is_default rows too — the Service's
// pre-flight Get-and-check exists for a clearer error message ("can't
// delete the default preset" vs. plain not-found), but this clause is the
// real, load-bearing guarantee: no code path can delete a default preset
// even if a future caller skips the service layer.
func (r *Repository) Delete(ctx context.Context, orgID, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM report_presets WHERE id = $1 AND organization_id = $2 AND is_default = false`, id, orgID)
	if err != nil {
		return fmt.Errorf("reportpresets: deleting: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("report preset not found")
	}
	return nil
}
