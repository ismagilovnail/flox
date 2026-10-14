package apikey

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const columns = `id, organization_id, name, prefix, scope, created_by_user_id, created_at, last_used_at, revoked_at, expires_at, replaced_by_id`

func scan(row pgx.Row) (ApiKey, error) {
	var k ApiKey
	err := row.Scan(&k.ID, &k.OrganizationID, &k.Name, &k.Prefix, &k.Scope, &k.CreatedByUserID, &k.CreatedAt, &k.LastUsedAt, &k.RevokedAt, &k.ExpiresAt, &k.ReplacedByID)
	return k, err
}

// List returns the keys that still authenticate: not revoked, and not
// past a rotation's grace period — matching the mock's own
// revoke-removes-from-list semantics (stores/settings.ts's
// revokeApiKey filters the key out entirely, it doesn't just flag it).
// A rotated key in its grace period is listed, with its expiresAt.
func (r *Repository) List(ctx context.Context, orgID string) ([]ApiKey, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+columns+`
		FROM api_keys
		WHERE organization_id = $1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())
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

// FindByHash looks up a key by its SHA-256 hash regardless of org — the
// caller (Service.Authenticate) doesn't know which org a bearer token
// belongs to until this returns it, unlike every other method here, which
// is always scoped to an orgID the caller already resolved another way
// (a session). Revoked keys are still returned (RevokedAt set, not
// filtered here) so Authenticate can tell "wrong key" apart from "right
// key, but revoked" for its own error message, and so a future audit
// trail has the full row rather than a bare "not found."
func (r *Repository) FindByHash(ctx context.Context, hash string) (ApiKey, error) {
	row := r.db.QueryRow(ctx, `SELECT `+columns+` FROM api_keys WHERE key_hash = $1`, hash)
	k, err := scan(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ApiKey{}, apierror.Unauthorized("invalid api key")
		}
		return ApiKey{}, fmt.Errorf("apikey: finding by hash: %w", err)
	}
	return k, nil
}

// UpdateLastUsed stamps last_used_at on a successful Authenticate — best-
// effort from the caller's side (Service logs and continues on failure
// rather than rejecting a request over a bookkeeping write); id is already
// known-valid at this point (FindByHash just returned it), so no rows-
// affected check is needed the way Revoke's has one.
func (r *Repository) UpdateLastUsed(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `UPDATE api_keys SET last_used_at = now() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("apikey: updating last_used_at: %w", err)
	}
	return nil
}

// Revoke sets revoked_at rather than deleting the row — the raw key is
// already unusable the instant a caller checks revoked_at (see
// Service.Authenticate), and keeping the row preserves the audit trail
// (who created it, when, with what scope) for as long as the org exists,
// matching api_keys' own schema-level design (migration 00010 added
// revoked_at as a nullable timestamp specifically for this, never a hard
// delete).
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

// Rotate mints the successor of key id — same name and scope, a new
// secret, created by userID — and gives the old key expiresAt = now() +
// grace and replaced_by_id, in one transaction. The old key must still
// authenticate and must not have a successor already: rotating twice
// would leave two secrets in their grace period for one integration.
func (r *Repository) Rotate(ctx context.Context, orgID, id, userID, prefix, keyHash string, grace time.Duration) (ApiKey, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return ApiKey{}, fmt.Errorf("apikey: beginning rotation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	old, err := scan(tx.QueryRow(ctx, `
		SELECT `+columns+` FROM api_keys
		WHERE id = $1 AND organization_id = $2 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())
		FOR UPDATE`, id, orgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ApiKey{}, apierror.NotFound("api key not found")
	}
	if err != nil {
		return ApiKey{}, fmt.Errorf("apikey: locking key to rotate: %w", err)
	}
	if old.ReplacedByID != nil {
		return ApiKey{}, apierror.Conflict("this key was already rotated; rotate its replacement instead")
	}

	created, err := scan(tx.QueryRow(ctx, `
		INSERT INTO api_keys (id, organization_id, name, prefix, key_hash, scope, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+columns,
		idgen.New(), orgID, old.Name, prefix, keyHash, old.Scope, userID))
	if err != nil {
		return ApiKey{}, fmt.Errorf("apikey: creating rotated key: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE api_keys SET expires_at = now() + make_interval(secs => $3), replaced_by_id = $4
		WHERE id = $1 AND organization_id = $2`,
		id, orgID, grace.Seconds(), created.ID); err != nil {
		return ApiKey{}, fmt.Errorf("apikey: expiring rotated key: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ApiKey{}, fmt.Errorf("apikey: committing rotation: %w", err)
	}
	return created, nil
}
