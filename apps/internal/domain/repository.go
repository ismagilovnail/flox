package domain

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

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

const selectColumns = `id, organization_id, domain, status, ssl, purpose, registrar, dns_provider, expires_at, verified_at, ssl_expires_at, ssl_checked_at, ssl_error, ownership_verified_at, verification_token, created_at, updated_at, registrar_auto_renew`

func scanDomain(row pgx.Row) (Domain, error) {
	var d Domain
	var token string
	err := row.Scan(
		&d.ID, &d.OrganizationID, &d.Domain, &d.Status, &d.SSL, &d.Purpose, &d.Registrar, &d.DNSProvider,
		&d.ExpiresAt, &d.VerifiedAt, &d.SSLExpiresAt, &d.SSLCheckedAt, &d.SSLError, &d.OwnershipVerifiedAt, &token, &d.CreatedAt, &d.UpdatedAt, &d.RegistrarAutoRenew,
	)
	d.OwnershipRecord = ownershipRecord(d.Domain, token)
	return d, err
}

// List returns one page of organizationID's domains, ordered by domain
// name (internal/pagination; apps/web reads every page).
func (r *Repository) List(ctx context.Context, organizationID string, page pagination.Page) ([]Domain, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+selectColumns+`
		FROM domains
		WHERE organization_id = $1
		ORDER BY domain, id
		LIMIT $2 OFFSET $3`,
		organizationID, page.Limit, page.Offset,
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

// uniqueConflict turns a unique violation into what the user can act on:
// the organization already lists the name, or another one has it active
// (proved its DNS first).
func uniqueConflict(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return nil
	}
	if pgErr.ConstraintName == "domains_active_domain_key" {
		return apierror.Conflict("this domain is active in another organization, which proved control of its DNS")
	}
	return apierror.Conflict("this domain is already in your list")
}

// Create inserts a domain. verifiedAt is set only for a name nobody can
// own publicly (Service.Create); everything else starts unverified.
func (r *Repository) Create(ctx context.Context, id, orgID string, in CreateInput, verifiedAt *time.Time) (Domain, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO domains (id, organization_id, domain, status, purpose, registrar, dns_provider, ownership_verified_at, verification_token)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+selectColumns,
		id, orgID, in.Domain, in.Status, in.Purpose, in.Registrar, in.DNSProvider, verifiedAt, newVerificationToken(),
	)
	d, err := scanDomain(row)
	if err != nil {
		if c := uniqueConflict(err); c != nil {
			return Domain{}, c
		}
		return Domain{}, fmt.Errorf("domain: inserting: %w", err)
	}
	return d, nil
}

// newVerificationToken is a domain's ownership token: random, so another
// organization's TXT record for the same name proves nothing for this one.
func newVerificationToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ownershipReset is what a rename does to ownership: the new name is not
// verified (verifiedAt nil) unless nobody can own it publicly, and gets a
// fresh token.
type ownershipReset struct{ verifiedAt *time.Time }

// Update applies only the non-nil fields in in, via a dynamically built
// SET clause — matching trafficsource.Repository.Update/campaign.Repository.Update.
func (r *Repository) Update(ctx context.Context, orgID, id string, in UpdateInput, reset *ownershipReset) (Domain, error) {
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
	if reset != nil {
		sets = append(sets, "ownership_verified_at = "+arg(reset.verifiedAt), "verification_token = "+arg(newVerificationToken()))
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
	if c := uniqueConflict(err); c != nil {
		return Domain{}, c
	}
	return d, err
}

// MarkOwnershipVerified records a passed ownership check and makes the
// domain active.
func (r *Repository) MarkOwnershipVerified(ctx context.Context, orgID, id string, at time.Time) (Domain, error) {
	d, err := scanDomain(r.db.QueryRow(ctx, `
		UPDATE domains SET ownership_verified_at = $3, status = 'active'
		WHERE id = $1 AND organization_id = $2
		RETURNING `+selectColumns, id, orgID, at))
	if errors.Is(err, pgx.ErrNoRows) {
		return Domain{}, apierror.NotFound("domain not found")
	}
	if c := uniqueConflict(err); c != nil {
		return Domain{}, c
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
	AutoRenew  *bool
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
	if res.AutoRenew != nil {
		sets = append(sets, "registrar_auto_renew = "+arg(*res.AutoRenew))
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

// SSLCheckResult is what a real TLS connection to the domain found
// (internal/domain/tlsprobe). Like AutomationResult, it is never reachable
// from client input.
type SSLCheckResult struct {
	SSL SSL
	// ExpiresAt is the served certificate's expiry; nil when none was
	// received, in which case the last one seen is kept.
	ExpiresAt *time.Time
	Err       string
	CheckedAt time.Time
}

func (r *Repository) ApplySSLCheck(ctx context.Context, orgID, id string, res SSLCheckResult) (Domain, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE domains
		SET ssl = $1, ssl_expires_at = COALESCE($2, ssl_expires_at), ssl_error = $3, ssl_checked_at = $4
		WHERE id = $5 AND organization_id = $6
		RETURNING `+selectColumns,
		res.SSL, res.ExpiresAt, res.Err, res.CheckedAt, id, orgID,
	)
	d, err := scanDomain(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Domain{}, apierror.NotFound("domain not found")
	}
	if err != nil {
		return Domain{}, fmt.Errorf("domain: recording ssl check: %w", err)
	}
	return d, nil
}

// DueSSLCheck is one domain the SSL check loop should probe now.
type DueSSLCheck struct {
	ID             string
	OrganizationID string
	Domain         string
}

// ClaimSSLChecks picks up to limit domains, across every organization,
// whose last SSL check is older than issuedEvery (a domain with a valid
// certificate) or otherEvery (any other), and stamps them checked now so a
// second worker does not probe them too. This is the background job's only
// cross-tenant read: it returns each domain's organization_id, and every
// write after it filters on that id.
func (r *Repository) ClaimSSLChecks(ctx context.Context, limit int, issuedEvery, otherEvery time.Duration) ([]DueSSLCheck, error) {
	rows, err := r.db.Query(ctx, `
		UPDATE domains SET ssl_checked_at = now()
		WHERE id IN (
		  SELECT id FROM domains
		  WHERE ssl_checked_at IS NULL
		     OR ssl_checked_at < now() - make_interval(secs => CASE WHEN ssl = 'issued' THEN $1::float8 ELSE $2::float8 END)
		  ORDER BY ssl_checked_at NULLS FIRST
		  LIMIT $3
		  FOR UPDATE SKIP LOCKED)
		RETURNING id, organization_id, domain`,
		issuedEvery.Seconds(), otherEvery.Seconds(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("domain: claiming ssl checks: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[DueSSLCheck])
}

// Count is how many domains rows the organization has — List's total.
func (r *Repository) Count(ctx context.Context, organizationID string) (int, error) {
	return pagination.Count(ctx, r.db, "domains", organizationID)
}
