package integrations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// storedRow never carries the encrypted payload — the two read paths
// that need it (Service.verifyRow, on re-verify) fetch it separately via
// PayloadByProvider, so a future edit to List/Get can never accidentally
// widen a public response to include ciphertext it has no business
// returning.
type storedRow struct {
	Provider       Provider
	LastVerifiedAt time.Time
	LastError      *string
}

func scanRow(row pgx.Row) (storedRow, error) {
	var r storedRow
	err := row.Scan(&r.Provider, &r.LastVerifiedAt, &r.LastError)
	return r, err
}

func (r *Repository) List(ctx context.Context, orgID string) ([]storedRow, error) {
	rows, err := r.db.Query(ctx,
		`SELECT provider, last_verified_at, last_error FROM org_integrations WHERE organization_id = $1 ORDER BY provider`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing org integrations: %w", err)
	}
	defer rows.Close()

	var out []storedRow
	for rows.Next() {
		row, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning org integration: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// PayloadByProvider returns the still-encrypted credential payload for a
// trusted, Go-only caller (Service, which holds the decryption key) —
// never exposed via any handler directly.
func (r *Repository) PayloadByProvider(ctx context.Context, orgID string, provider Provider) (encryptedPayload string, found bool, err error) {
	err = r.db.QueryRow(ctx,
		`SELECT encrypted_payload FROM org_integrations WHERE organization_id = $1 AND provider = $2`,
		orgID, provider,
	).Scan(&encryptedPayload)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return encryptedPayload, true, nil
}

// Upsert stores a freshly-verified credential — always clears last_error
// and stamps last_verified_at to now, since Upsert is only ever called
// right after a successful Verify call (Service.Connect).
func (r *Repository) Upsert(ctx context.Context, id, orgID string, provider Provider, encryptedPayload string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO org_integrations (id, organization_id, provider, encrypted_payload, last_verified_at, last_error)
		VALUES ($1, $2, $3, $4, now(), NULL)
		ON CONFLICT (organization_id, provider) DO UPDATE SET
			encrypted_payload = EXCLUDED.encrypted_payload,
			last_verified_at = now(),
			last_error = NULL`,
		id, orgID, provider, encryptedPayload,
	)
	if err != nil {
		return fmt.Errorf("connecting integration: %w", err)
	}
	return nil
}

// RecordVerification stamps the outcome of a re-verification of an
// already-stored credential (POST .../verify) — last_verified_at always
// advances to now (a check was just run, pass or fail); verifyErr nil
// clears last_error (back to a real "connected" status), non-nil sets it
// (the row becomes "error" without deleting the stored credential, so
// the operator can see why rather than having it silently vanish).
func (r *Repository) RecordVerification(ctx context.Context, orgID string, provider Provider, verifyErr error) error {
	var errText *string
	if verifyErr != nil {
		msg := verifyErr.Error()
		errText = &msg
	}
	tag, err := r.db.Exec(ctx,
		`UPDATE org_integrations SET last_verified_at = now(), last_error = $3 WHERE organization_id = $1 AND provider = $2`,
		orgID, provider, errText,
	)
	if err != nil {
		return fmt.Errorf("recording integration verification: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("integration not connected")
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, orgID string, provider Provider) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM org_integrations WHERE organization_id = $1 AND provider = $2`, orgID, provider)
	if err != nil {
		return fmt.Errorf("disconnecting integration: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("integration not connected")
	}
	return nil
}
