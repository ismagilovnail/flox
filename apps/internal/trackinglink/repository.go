package trackinglink

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const selectColumns = `tl.id, tl.organization_id, tl.campaign_id, tl.domain_id, d.domain, d.ssl = 'issued', tl.slug, tl.status, tl.created_at, tl.updated_at`

func scanLink(row pgx.Row) (TrackingLink, error) {
	var l TrackingLink
	err := row.Scan(&l.ID, &l.OrganizationID, &l.CampaignID, &l.DomainID, &l.Domain, &l.Secure, &l.Slug, &l.Status, &l.CreatedAt, &l.UpdatedAt)
	if err == nil {
		l.URL = buildURL(l.Domain, l.Slug, l.Secure)
	}
	return l, err
}

// buildURL gives https once the domain serves a valid certificate
// (domains.ssl, kept true by internal/sslcheck). Before that an https link
// would fail in the visitor's browser, so the link stays http and the
// Domains page says why.
func buildURL(domain, slug string, secure bool) string {
	scheme := "http://"
	if secure {
		scheme = "https://"
	}
	return scheme + domain + "/t/" + slug
}

// List returns every tracking link for one campaign, newest first — a
// campaign typically has one or a handful (per domain), so no pagination.
func (r *Repository) List(ctx context.Context, orgID, campaignID string) ([]TrackingLink, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+selectColumns+`
		FROM tracking_links tl
		JOIN domains d ON d.id = tl.domain_id
		WHERE tl.organization_id = $1 AND tl.campaign_id = $2
		ORDER BY tl.created_at DESC`,
		orgID, campaignID,
	)
	if err != nil {
		return nil, fmt.Errorf("trackinglink: listing: %w", err)
	}
	defer rows.Close()

	out := []TrackingLink{}
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, fmt.Errorf("trackinglink: scanning: %w", err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("trackinglink: reading: %w", err)
	}
	return out, nil
}

func (r *Repository) GetByID(ctx context.Context, orgID, id string) (TrackingLink, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+selectColumns+`
		FROM tracking_links tl
		JOIN domains d ON d.id = tl.domain_id
		WHERE tl.id = $1 AND tl.organization_id = $2`,
		id, orgID,
	)
	l, err := scanLink(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return TrackingLink{}, apierror.NotFound("tracking link not found")
	}
	return l, err
}

// Create resolves both domainID and campaignID against organization_id in
// the same INSERT (rather than separate SELECTs then an INSERT) so
// neither check can race a concurrent delete/reassignment between check
// and write. This is CLAUDE.md #5's tenant isolation applied to the two
// client-supplied IDs that cross into another table's ownership here —
// domain_id from the request body, and campaign_id from the route's own
// {campaignId}, which nothing upstream of this method re-validates
// against orgID (unlike a plain List/Get, a wrong-org campaign_id here
// would otherwise silently create a row, not just silently return zero).
func (r *Repository) Create(ctx context.Context, id, orgID, campaignID string, in CreateInput) (TrackingLink, error) {
	var insertedID string
	err := r.db.QueryRow(ctx, `
		INSERT INTO tracking_links (id, organization_id, campaign_id, domain_id, slug)
		SELECT $1, $2::ulid, c.id, d.id, $5
		FROM domains d, campaigns c
		WHERE d.id = $4 AND d.organization_id = $2::ulid
		  AND c.id = $3 AND c.organization_id = $2::ulid
		RETURNING id`,
		id, orgID, campaignID, in.DomainID, in.Slug,
	).Scan(&insertedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return TrackingLink{}, apierror.Validation("invalid tracking link", map[string]string{"domainId": "campaign or domain not found"})
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return TrackingLink{}, apierror.Conflict("this slug is already used on that domain")
		}
		return TrackingLink{}, fmt.Errorf("trackinglink: inserting: %w", err)
	}
	return r.GetByID(ctx, orgID, insertedID)
}

func (r *Repository) UpdateStatus(ctx context.Context, orgID, id string, status Status) (TrackingLink, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE tracking_links SET status = $1 WHERE id = $2 AND organization_id = $3`,
		status, id, orgID,
	)
	if err != nil {
		return TrackingLink{}, fmt.Errorf("trackinglink: updating: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return TrackingLink{}, apierror.NotFound("tracking link not found")
	}
	return r.GetByID(ctx, orgID, id)
}

func (r *Repository) Delete(ctx context.Context, orgID, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM tracking_links WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		return fmt.Errorf("trackinglink: deleting: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("tracking link not found")
	}
	return nil
}
