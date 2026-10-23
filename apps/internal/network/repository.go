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
	"github.com/ismagilovnail/flox/apps/internal/pagination"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const selectColumns = `id, organization_id, name, postback_url, accept_duplicates, currency, status, created_at, updated_at, template_key`

func scanNetwork(row pgx.Row) (Network, error) {
	var n Network
	err := row.Scan(&n.ID, &n.OrganizationID, &n.Name, &n.PostbackURL, &n.AcceptDuplicates, &n.Currency, &n.Status, &n.CreatedAt, &n.UpdatedAt, &n.TemplateKey)
	return n, err
}

func (r *Repository) List(ctx context.Context, organizationID string, page pagination.Page) ([]Network, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+selectColumns+`
		FROM networks
		WHERE organization_id = $1
		ORDER BY name, id
		LIMIT $2 OFFSET $3`,
		organizationID, page.Limit, page.Offset,
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

// Diagnostics reads the network's postbacks of the last DiagnosticsWindow.
// Scoped to orgID (CLAUDE.md #5): another organization's network is not
// found.
func (r *Repository) Diagnostics(ctx context.Context, orgID, id string) (Diagnostics, error) {
	if _, err := r.GetByID(ctx, orgID, id); err != nil {
		return Diagnostics{}, err
	}
	d := Diagnostics{WindowDays: int(DiagnosticsWindow.Hours() / 24)}
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE result IN ('success', 'duplicate')),
		       count(*) FILTER (WHERE result = 'duplicate'),
		       max(created_at)
		FROM postbacks
		WHERE organization_id = $1 AND network_id = $2
		  AND status = 'CPA_REDEP' AND event_ref = '' AND direction = 'incoming'
		  AND created_at > now() - make_interval(secs => $3)`,
		orgID, id, DiagnosticsWindow.Seconds(),
	).Scan(&d.RedepWithoutTxn.Received, &d.RedepWithoutTxn.Duplicates, &d.RedepWithoutTxn.LastAt)
	if err != nil {
		return Diagnostics{}, fmt.Errorf("network: reading diagnostics: %w", err)
	}
	return d, nil
}

func (r *Repository) Create(ctx context.Context, id, orgID string, in CreateInput, postbackSecretHash string) (Network, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO networks (id, organization_id, name, postback_url, accept_duplicates, currency, postback_secret_hash, template_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+selectColumns,
		id, orgID, in.Name, in.PostbackURL, in.AcceptDuplicates, in.Currency, postbackSecretHash, in.TemplateKey,
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
	if in.TemplateKey != nil {
		sets = append(sets, "template_key = "+arg(strings.TrimSpace(*in.TemplateKey)))
	}
	if in.Currency != nil {
		sets = append(sets, "currency = "+arg(*in.Currency))
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

// Delete: offers, event mappings and outgoing deliveries CASCADE with the
// network. Two references RESTRICT it, and each answers its own 409
// (audit BUG-1 — every refusal used to say "flows"):
//   - flows.destination_network_id (or destination_offer_id, through the
//     cascaded offers): a flow still sends traffic to it;
//   - postbacks.network_id: it has recorded conversions, which must keep
//     their network — archive it instead.
func (r *Repository) Delete(ctx context.Context, orgID, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM networks WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			switch pgErr.ConstraintName {
			case "postbacks_network_id_fkey":
				return apierror.Conflict("the network has recorded postbacks: archive it instead, so its conversions keep their network")
			case "flows_destination_network_id_fkey", "flows_destination_offer_id_fkey":
				return apierror.Conflict("the network is the destination of one or more flows: change those flows first")
			}
			return apierror.Conflict("the network is still in use and cannot be deleted; archive it instead")
		}
		return fmt.Errorf("deleting network: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("network not found")
	}
	return nil
}

// Count is how many networks rows the organization has — List's total.
func (r *Repository) Count(ctx context.Context, organizationID string) (int, error) {
	return pagination.Count(ctx, r.db, "networks", organizationID)
}
