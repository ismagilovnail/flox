package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const selectColumns = `id, organization_id, domain, status, ssl, purpose, registrar, dns_provider, expires_at, verified_at, created_at, updated_at`

func scanDomain(row pgx.Row) (Domain, error) {
	var d Domain
	err := row.Scan(
		&d.ID, &d.OrganizationID, &d.Domain, &d.Status, &d.SSL, &d.Purpose, &d.Registrar, &d.DNSProvider,
		&d.ExpiresAt, &d.VerifiedAt, &d.CreatedAt, &d.UpdatedAt,
	)
	return d, err
}

// List returns every domain for organizationID, ordered by domain name —
// unpaginated, matching trafficsource.Repository.List's own reasoning (a
// tracking-link domain picker and the domains table both want the whole
// list, and per-org domain counts are small).
func (r *Repository) List(ctx context.Context, organizationID string) ([]Domain, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+selectColumns+`
		FROM domains
		WHERE organization_id = $1
		ORDER BY domain`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("domain: listing: %w", err)
	}
	defer rows.Close()

	out := []Domain{}
	for rows.Next() {
		d, err := scanDomain(rows)
		if err != nil {
			return nil, fmt.Errorf("domain: scanning: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("domain: reading: %w", err)
	}
	return out, nil
}

func (r *Repository) GetByID(ctx context.Context, orgID, id string) (Domain, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+selectColumns+`
		FROM domains
		WHERE id = $1 AND organization_id = $2`,
		id, orgID,
	)
	d, err := scanDomain(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Domain{}, apierror.NotFound("domain not found")
	}
	return d, err
}

// Create's unique violation on domains_domain_key (case-insensitive,
// migration 00007) is a real cross-org race, not a per-org one: two
// different organizations both trying to park the same hostname genuinely
// can't both own it, the same way two people can't both register the same
// DNS name. Reported as a plain conflict rather than a per-field
// validation error since it isn't knowable until the INSERT itself runs.
func (r *Repository) Create(ctx context.Context, id, orgID string, in CreateInput) (Domain, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO domains (id, organization_id, domain, status, purpose, registrar, dns_provider)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+selectColumns,
		id, orgID, in.Domain, in.Status, in.Purpose, in.Registrar, in.DNSProvider,
	)
	d, err := scanDomain(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Domain{}, apierror.Conflict("this domain is already parked")
		}
		return Domain{}, fmt.Errorf("domain: inserting: %w", err)
	}
	return d, nil
}

// Update applies only the non-nil fields in in, via a dynamically built
// SET clause — matching trafficsource.Repository.Update/campaign.Repository.Update.
func (r *Repository) Update(ctx context.Context, orgID, id string, in UpdateInput) (Domain, error) {
	sets := []string{}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if in.Domain != nil {
		sets = append(sets, "domain = "+arg(*in.Domain))
	}
	if in.Purpose != nil {
		sets = append(sets, "purpose = "+arg(*in.Purpose))
	}
	if in.Registrar != nil {
		sets = append(sets, "registrar = "+arg(*in.Registrar))
	}
	if in.DNSProvider != nil {
		sets = append(sets, "dns_provider = "+arg(*in.DNSProvider))
	}
	if in.Status != nil {
		sets = append(sets, "status = "+arg(*in.Status))
	}

	if len(sets) == 0 {
		return r.GetByID(ctx, orgID, id)
	}

	query := fmt.Sprintf(
		`UPDATE domains SET %s WHERE id = %s AND organization_id = %s RETURNING %s`,
		strings.Join(sets, ", "), arg(id), arg(orgID), selectColumns,
	)

	row := r.db.QueryRow(ctx, query, args...)
	d, err := scanDomain(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Domain{}, apierror.NotFound("domain not found")
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Domain{}, apierror.Conflict("this domain is already parked")
		}
	}
	return d, err
}

// AutomationResult is what a real Verify/Renew/SyncDNS call found or
// changed — a separate, narrower shape from UpdateInput specifically
// because UpdateInput's own doc comment says these fields must never be
// reachable from a client PATCH; this is the one path that's allowed to
// set them, and it's only ever called from Service.Verify/Renew/
// SyncDNS after a real registrar/DNS API call actually confirmed the
// value.
type AutomationResult struct {
	Status     *Status
	SSL        *SSL
	ExpiresAt  *time.Time
	VerifiedAt *time.Time
}

// ApplyAutomationResult persists the outcome of a real registrar/DNS
// check or action — see AutomationResult's own doc comment for why this
// is separate from Update.
func (r *Repository) ApplyAutomationResult(ctx context.Context, orgID, id string, res AutomationResult) (Domain, error) {
	sets := []string{}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if res.Status != nil {
		sets = append(sets, "status = "+arg(*res.Status))
	}
	if res.SSL != nil {
		sets = append(sets, "ssl = "+arg(*res.SSL))
	}
	if res.ExpiresAt != nil {
		sets = append(sets, "expires_at = "+arg(*res.ExpiresAt))
	}
	if res.VerifiedAt != nil {
		sets = append(sets, "verified_at = "+arg(*res.VerifiedAt))
	}
	if len(sets) == 0 {
		return r.GetByID(ctx, orgID, id)
	}

	query := fmt.Sprintf(
		`UPDATE domains SET %s WHERE id = %s AND organization_id = %s RETURNING %s`,
		strings.Join(sets, ", "), arg(id), arg(orgID), selectColumns,
	)
	row := r.db.QueryRow(ctx, query, args...)
	d, err := scanDomain(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Domain{}, apierror.NotFound("domain not found")
	}
	return d, err
}

// Delete cascades onto tracking_links (ON DELETE CASCADE, migration
// 00007) — removing a domain that still has tracking links pointing at it
// removes those too, exactly the "campaigns still pointing at it will
// break" warning the frontend's own confirm dialog gives before calling
// this.
func (r *Repository) Delete(ctx context.Context, orgID, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM domains WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		return fmt.Errorf("domain: deleting: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("domain not found")
	}
	return nil
}
