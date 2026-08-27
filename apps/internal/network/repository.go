package network

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

const selectColumns = `id, organization_id, name, postback_url, accept_duplicates, status, created_at, updated_at`

func scanNetwork(row pgx.Row) (Network, error) {
	var n Network
	err := row.Scan(&n.ID, &n.OrganizationID, &n.Name, &n.PostbackURL, &n.AcceptDuplicates, &n.Status, &n.CreatedAt, &n.UpdatedAt)
	return n, err
}

func (r *Repository) List(ctx context.Context, organizationID string) ([]Network, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+selectColumns+`
		FROM networks
		WHERE organization_id = $1
		ORDER BY name`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("network: listing: %w", err)
	}
	defer rows.Close()

	out := []Network{}
	for rows.Next() {
		n, err := scanNetwork(rows)
		if err != nil {
			return nil, fmt.Errorf("network: scanning: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *Repository) GetByID(ctx context.Context, orgID, id string) (Network, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+selectColumns+`
		FROM networks
		WHERE id = $1 AND organization_id = $2`,
		id, orgID,
	)
	n, err := scanNetwork(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Network{}, apierror.NotFound("network not found")
	}
	return n, err
}

func (r *Repository) Create(ctx context.Context, id, orgID string, in CreateInput, postbackSecretHash string) (Network, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO networks (id, organization_id, name, postback_url, accept_duplicates, postback_secret_hash)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+selectColumns,
		id, orgID, in.Name, in.PostbackURL, in.AcceptDuplicates, postbackSecretHash,
	)
	return scanNetwork(row)
}

// RegenerateSecret overwrites a network's postback_secret_hash in place —
// the old secret stops working the instant this commits, same
// "resending/regenerating invalidates the previous one" precedent as
// apps/internal/auth's own invite-resend.
func (r *Repository) RegenerateSecret(ctx context.Context, orgID, id, postbackSecretHash string) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE networks SET postback_secret_hash = $1 WHERE id = $2 AND organization_id = $3`,
		postbackSecretHash, id, orgID,
	)
	if err != nil {
		return fmt.Errorf("network: regenerating postback secret: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("network not found")
	}
	return nil
}

func (r *Repository) Update(ctx context.Context, orgID, id string, in UpdateInput) (Network, error) {
	sets := []string{}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if in.Name != nil {
		sets = append(sets, "name = "+arg(*in.Name))
	}
	if in.PostbackURL != nil {
		sets = append(sets, "postback_url = "+arg(*in.PostbackURL))
	}
	if in.AcceptDuplicates != nil {
		sets = append(sets, "accept_duplicates = "+arg(*in.AcceptDuplicates))
	}
	if in.Status != nil {
		sets = append(sets, "status = "+arg(*in.Status))
	}

	if len(sets) == 0 {
		return r.GetByID(ctx, orgID, id)
	}

	query := fmt.Sprintf(
		`UPDATE networks SET %s WHERE id = %s AND organization_id = %s RETURNING %s`,
		strings.Join(sets, ", "), arg(id), arg(orgID), selectColumns,
	)

	row := r.db.QueryRow(ctx, query, args...)
	n, err := scanNetwork(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Network{}, apierror.NotFound("network not found")
	}
	return n, err
}

// Delete: offers.network_id CASCADEs (00003), so a network's own offers
// are removed along with it automatically — no conflict from that side.
// flows.destination_network_id (00006) has no ON DELETE clause (defaults
// to RESTRICT) — no Flow CRUD exists yet to ever populate that column,
// but the 23503 catch is here defensively for when it does, same shape
// as trafficsource.Repository.Delete.
func (r *Repository) Delete(ctx context.Context, orgID, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM networks WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return apierror.Conflict("network is still referenced by one or more flows")
		}
		return fmt.Errorf("deleting network: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("network not found")
	}
	return nil
}
