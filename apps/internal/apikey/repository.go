package apikey

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const columns = `id, organization_id, name, prefix, scope, created_by_user_id, created_at, last_used_at, revoked_at`

func scan(row pgx.Row) (ApiKey, error) {
	var k ApiKey
	err := row.Scan(&k.ID, &k.OrganizationID, &k.Name, &k.Prefix, &k.Scope, &k.CreatedByUserID, &k.CreatedAt, &k.LastUsedAt, &k.RevokedAt)
	return k, err
}

// List returns only non-revoked keys — matching the mock's own
// revoke-removes-from-list semantics (stores/settings.ts's
// revokeApiKey filters the key out entirely, it doesn't just flag it).
func (r *Repository) List(ctx context.Context, orgID string) ([]ApiKey, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+columns+`
		FROM api_keys
		WHERE organization_id = $1 AND revoked_at IS NULL
		ORDER BY created_at DESC`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("apikey: listing: %w", err)
	}
	defer rows.Close()

	out := []ApiKey{}
	for rows.Next() {
		k, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("apikey: scanning: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (r *Repository) Create(ctx context.Context, orgID, createdByUserID, prefix, keyHash string, in CreateInput) (ApiKey, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO api_keys (id, organization_id, name, prefix, key_hash, scope, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+columns,
		idgen.New(), orgID, in.Name, prefix, keyHash, in.Scope, createdByUserID,
	)
	return scan(row)
}

// Revoke sets revoked_at rather than deleting the row — the raw key is
// already unusable the instant a caller checks revoked_at (see
// Service.Authenticate's own use of this table, if/when one exists),
// and keeping the row preserves the audit trail (who created it, when,
// with what scope) for as long as the org exists, matching api_keys'
// own schema-level design (migration 00010 added revoked_at as a
// nullable timestamp specifically for this, never a hard delete).
func (r *Repository) Revoke(ctx context.Context, orgID, id string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE api_keys SET revoked_at = now()
		WHERE id = $1 AND organization_id = $2 AND revoked_at IS NULL`,
		id, orgID,
	)
	if err != nil {
		return fmt.Errorf("apikey: revoking: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("api key not found")
	}
	return nil
}
