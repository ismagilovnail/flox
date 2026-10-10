package tags

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/pagination"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const tagColumns = `id, organization_id, name, color, created_at, updated_at`

func scanTag(row pgx.Row) (Tag, error) {
	var t Tag
	err := row.Scan(&t.ID, &t.OrganizationID, &t.Name, &t.Color, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func (r *Repository) List(ctx context.Context, orgID string, page pagination.Page) ([]Tag, error) {
	rows, err := r.db.Query(ctx, `SELECT `+tagColumns+` FROM tags WHERE organization_id = $1 ORDER BY name, id LIMIT $2 OFFSET $3`, orgID, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("tags: listing: %w", err)
	}
	defer rows.Close()

	out := []Tag{}
	for rows.Next() {
		t, err := scanTag(rows)
		if err != nil {
			return nil, fmt.Errorf("tags: scanning: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) Create(ctx context.Context, orgID string, in CreateTagInput) (Tag, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO tags (id, organization_id, name, color)
		VALUES ($1, $2, $3, $4)
		RETURNING `+tagColumns,
		idgen.New(), orgID, in.Name, in.Color,
	)
	return scanTag(row)
}

func (r *Repository) Update(ctx context.Context, orgID, id string, in UpdateTagInput) (Tag, error) {
	sets := []string{}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if in.Name != nil {
		sets = append(sets, "name = "+arg(*in.Name))
	}
	if in.Color != nil {
		sets = append(sets, "color = "+arg(*in.Color))
	}
	if len(sets) == 0 {
		row := r.db.QueryRow(ctx, `SELECT `+tagColumns+` FROM tags WHERE id = $1 AND organization_id = $2`, id, orgID)
		t, err := scanTag(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return Tag{}, apierror.NotFound("tag not found")
		}
		return t, err
	}

	query := fmt.Sprintf(
		`UPDATE tags SET %s WHERE id = %s AND organization_id = %s RETURNING %s`,
		strings.Join(sets, ", "), arg(id), arg(orgID), tagColumns,
	)
	row := r.db.QueryRow(ctx, query, args...)
	t, err := scanTag(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Tag{}, apierror.NotFound("tag not found")
	}
	return t, err
}

// Delete cascades to taggables via the FK (ON DELETE CASCADE) — removing
// a tag removes it from every entity it was assigned to, per spec.
func (r *Repository) Delete(ctx context.Context, orgID, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM tags WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		return fmt.Errorf("tags: deleting: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("tag not found")
	}
	return nil
}

// EntityExists checks the entity_id belongs to a real, same-org row of
// entityType — entityTable's values are a fixed internal whitelist
// (never user input), so the interpolated table name here can't be an
// injection vector.
func (r *Repository) EntityExists(ctx context.Context, orgID string, entityType EntityType, entityID string) (bool, error) {
	table, ok := entityTable[entityType]
	if !ok {
		return false, fmt.Errorf("tags: unknown entity type %q", entityType)
	}
	var exists bool
	err := r.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM %s WHERE id = $1 AND organization_id = $2)`, table),
		entityID, orgID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("tags: checking entity ownership: %w", err)
	}
	return exists, nil
}

// ExistingTagIDs returns the subset of tagIDs that are real tags owned
// by orgID — used to reject a request that names another org's tag_id
// before it ever reaches a write. Without this, nothing would stop an
// assignment row from pointing at a foreign tag: no data would leak
// (the frontend only ever resolves tag_id against its own org's tag
// list), but the foreign tag's own later deletion would cascade-delete
// this org's assignment row too, a real cross-org side effect.
func (r *Repository) ExistingTagIDs(ctx context.Context, orgID string, tagIDs []string) (map[string]bool, error) {
	rows, err := r.db.Query(ctx, `SELECT id FROM tags WHERE organization_id = $1 AND id = ANY($2)`, orgID, tagIDs)
	if err != nil {
		return nil, fmt.Errorf("tags: checking tag ownership: %w", err)
	}
	defer rows.Close()

	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("tags: scanning owned tag id: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}

const assignmentColumns = `id, organization_id, tag_id, entity_type, entity_id, created_at`

func scanAssignment(row pgx.Row) (Assignment, error) {
	var a Assignment
	err := row.Scan(&a.ID, &a.OrganizationID, &a.TagID, &a.EntityType, &a.EntityID, &a.CreatedAt)
	return a, err
}

// ListAssignments returns every assignment for one entity type, org-wide
// — the frontend hydrates a whole list page's worth of tag badges/filter
// state from this one call, then does its own client-side lookup per
// row (matching how the mock store held the full array and filtered in
// a useMemo — same shape, real data).
func (r *Repository) ListAssignments(ctx context.Context, orgID string, entityType EntityType) ([]Assignment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+assignmentColumns+`
		FROM taggables
		WHERE organization_id = $1 AND entity_type = $2`,
		orgID, entityType,
	)
	if err != nil {
		return nil, fmt.Errorf("tags: listing assignments: %w", err)
	}
	defer rows.Close()

	out := []Assignment{}
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			return nil, fmt.Errorf("tags: scanning assignment: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetEntityTags replaces the full tag set on one entity: removes every
// existing assignment not in tagIDs, adds every one in tagIDs not
// already present. Both steps commit together.
func (r *Repository) SetEntityTags(ctx context.Context, orgID string, entityType EntityType, entityID string, tagIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("tags: beginning set-entity-tags tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		DELETE FROM taggables
		WHERE organization_id = $1 AND entity_type = $2 AND entity_id = $3 AND NOT (tag_id = ANY($4))`,
		orgID, entityType, entityID, tagIDs,
	); err != nil {
		return fmt.Errorf("tags: clearing entity tags: %w", err)
	}

	for _, tagID := range tagIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO taggables (id, organization_id, tag_id, entity_type, entity_id)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (tag_id, entity_type, entity_id) DO NOTHING`,
			idgen.New(), orgID, tagID, entityType, entityID,
		); err != nil {
			return fmt.Errorf("tags: assigning tag: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// BulkEdit applies §30.6's bulk semantics: toAdd tags -> every entity in
// entityIDs (no-op where present), toRemove tags -> unassigned from
// every entity in entityIDs (no-op where absent). Both directions
// commit together.
func (r *Repository) BulkEdit(ctx context.Context, orgID string, entityType EntityType, entityIDs, toAdd, toRemove []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("tags: beginning bulk-edit tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if len(toRemove) > 0 {
		if _, err := tx.Exec(ctx, `
			DELETE FROM taggables
			WHERE organization_id = $1 AND entity_type = $2 AND entity_id = ANY($3) AND tag_id = ANY($4)`,
			orgID, entityType, entityIDs, toRemove,
		); err != nil {
			return fmt.Errorf("tags: bulk removing: %w", err)
		}
	}

	for _, entityID := range entityIDs {
		for _, tagID := range toAdd {
			if _, err := tx.Exec(ctx, `
				INSERT INTO taggables (id, organization_id, tag_id, entity_type, entity_id)
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (tag_id, entity_type, entity_id) DO NOTHING`,
				idgen.New(), orgID, tagID, entityType, entityID,
			); err != nil {
				return fmt.Errorf("tags: bulk assigning: %w", err)
			}
		}
	}

	return tx.Commit(ctx)
}

// Count is how many tags rows the organization has — List's total.
func (r *Repository) Count(ctx context.Context, orgID string) (int, error) {
	return pagination.Count(ctx, r.db, "tags", orgID)
}
