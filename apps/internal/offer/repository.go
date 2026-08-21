package offer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const offerColumns = `id, organization_id, network_id, name, countries, payout, currency, cap, status, created_at, updated_at`

func scanOffer(row pgx.Row) (Offer, error) {
	var o Offer
	err := row.Scan(&o.ID, &o.OrganizationID, &o.NetworkID, &o.Name, &o.Countries, &o.Payout, &o.Currency, &o.Cap, &o.Status, &o.CreatedAt, &o.UpdatedAt)
	return o, err
}

// List returns every offer for organizationID with its links attached —
// two queries (offers, then every link for those offer ids) merged in Go
// rather than one query.Rows()-per-offer link fetch or a JSON-aggregating
// SQL query, since this stays simple and the per-org offer count is small
// (same "unpaginated, small n" reasoning as trafficsource.List).
func (r *Repository) List(ctx context.Context, organizationID string) ([]Offer, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+offerColumns+`
		FROM offers
		WHERE organization_id = $1
		ORDER BY name`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("offer: listing: %w", err)
	}
	defer rows.Close()

	offers := []Offer{}
	ids := []string{}
	for rows.Next() {
		o, err := scanOffer(rows)
		if err != nil {
			return nil, fmt.Errorf("offer: scanning: %w", err)
		}
		o.Links = []Link{}
		offers = append(offers, o)
		ids = append(ids, o.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return offers, nil
	}

	linksByOffer, err := r.linksByOfferID(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range offers {
		offers[i].Links = linksByOffer[offers[i].ID]
	}
	return offers, nil
}

func (r *Repository) linksByOfferID(ctx context.Context, offerIDs []string) (map[string][]Link, error) {
	rows, err := r.db.Query(ctx, `
		SELECT offer_id, id, label, url
		FROM offer_links
		WHERE offer_id = ANY($1)
		ORDER BY offer_id, label`,
		offerIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("offer: listing links: %w", err)
	}
	defer rows.Close()

	out := map[string][]Link{}
	for rows.Next() {
		var offerID string
		var l Link
		if err := rows.Scan(&offerID, &l.ID, &l.Label, &l.URL); err != nil {
			return nil, fmt.Errorf("offer: scanning link: %w", err)
		}
		out[offerID] = append(out[offerID], l)
	}
	return out, rows.Err()
}

func (r *Repository) GetByID(ctx context.Context, orgID, id string) (Offer, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+offerColumns+`
		FROM offers
		WHERE id = $1 AND organization_id = $2`,
		id, orgID,
	)
	o, err := scanOffer(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Offer{}, apierror.NotFound("offer not found")
	}
	if err != nil {
		return Offer{}, err
	}

	links, err := r.linksByOfferID(ctx, []string{id})
	if err != nil {
		return Offer{}, err
	}
	o.Links = links[id]
	if o.Links == nil {
		o.Links = []Link{}
	}
	return o, nil
}

// Create inserts the offer row and its links in one transaction — an
// offer with a partial link set (insert succeeded, links didn't) would
// violate the service layer's own "at least one link" invariant.
func (r *Repository) Create(ctx context.Context, id, orgID string, in CreateInput) (Offer, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Offer{}, fmt.Errorf("offer: beginning create tx: %w", err)
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		INSERT INTO offers (id, organization_id, network_id, name, countries, payout, currency, cap)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+offerColumns,
		id, orgID, in.NetworkID, in.Name, in.Countries, in.Payout, in.Currency, in.Cap,
	)
	o, err := scanOffer(row)
	if err != nil {
		return Offer{}, fmt.Errorf("offer: inserting: %w", err)
	}

	links, err := insertLinks(ctx, tx, orgID, id, in.Links)
	if err != nil {
		return Offer{}, err
	}
	o.Links = links

	if err := tx.Commit(ctx); err != nil {
		return Offer{}, fmt.Errorf("offer: committing create tx: %w", err)
	}
	return o, nil
}

func insertLinks(ctx context.Context, tx pgx.Tx, orgID, offerID string, in []LinkInput) ([]Link, error) {
	links := make([]Link, len(in))
	for i, l := range in {
		linkID := idgen.New()
		_, err := tx.Exec(ctx,
			`INSERT INTO offer_links (id, organization_id, offer_id, label, url) VALUES ($1, $2, $3, $4, $5)`,
			linkID, orgID, offerID, l.Label, l.URL,
		)
		if err != nil {
			return nil, fmt.Errorf("offer: inserting link: %w", err)
		}
		links[i] = Link{ID: linkID, Label: l.Label, URL: l.URL}
	}
	return links, nil
}

// Update applies only the non-nil fields in in. Links, if present,
// replaces the offer's entire link set (delete-all, insert-all) inside
// the same transaction as the scalar-field update — matching the
// frontend form's own whole-array submission (offer-form-sheet.tsx's
// useFieldArray sends every link on every save, not a diff).
func (r *Repository) Update(ctx context.Context, orgID, id string, in UpdateInput) (Offer, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Offer{}, fmt.Errorf("offer: beginning update tx: %w", err)
	}
	defer tx.Rollback(ctx)

	sets := []string{}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if in.NetworkID != nil {
		sets = append(sets, "network_id = "+arg(*in.NetworkID))
	}
	if in.Name != nil {
		sets = append(sets, "name = "+arg(*in.Name))
	}
	if in.Countries != nil {
		sets = append(sets, "countries = "+arg(*in.Countries))
	}
	if in.Payout != nil {
		sets = append(sets, "payout = "+arg(*in.Payout))
	}
	if in.Currency != nil {
		sets = append(sets, "currency = "+arg(*in.Currency))
	}
	if in.Cap != nil {
		sets = append(sets, "cap = "+arg(in.Cap.Value))
	}
	if in.Status != nil {
		sets = append(sets, "status = "+arg(*in.Status))
	}

	var o Offer
	if len(sets) > 0 {
		query := fmt.Sprintf(
			`UPDATE offers SET %s WHERE id = %s AND organization_id = %s RETURNING %s`,
			strings.Join(sets, ", "), arg(id), arg(orgID), offerColumns,
		)
		row := tx.QueryRow(ctx, query, args...)
		o, err = scanOffer(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return Offer{}, apierror.NotFound("offer not found")
		}
		if err != nil {
			return Offer{}, fmt.Errorf("offer: updating: %w", err)
		}
	} else {
		row := tx.QueryRow(ctx, `SELECT `+offerColumns+` FROM offers WHERE id = $1 AND organization_id = $2`, id, orgID)
		o, err = scanOffer(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return Offer{}, apierror.NotFound("offer not found")
		}
		if err != nil {
			return Offer{}, err
		}
	}

	if in.Links != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM offer_links WHERE offer_id = $1`, id); err != nil {
			return Offer{}, fmt.Errorf("offer: clearing links: %w", err)
		}
		links, err := insertLinks(ctx, tx, orgID, id, *in.Links)
		if err != nil {
			return Offer{}, err
		}
		o.Links = links
	} else {
		linksByOffer, err := r.linksByOfferIDTx(ctx, tx, id)
		if err != nil {
			return Offer{}, err
		}
		o.Links = linksByOffer
	}

	if err := tx.Commit(ctx); err != nil {
		return Offer{}, fmt.Errorf("offer: committing update tx: %w", err)
	}
	return o, nil
}

func (r *Repository) linksByOfferIDTx(ctx context.Context, tx pgx.Tx, offerID string) ([]Link, error) {
	rows, err := tx.Query(ctx, `SELECT id, label, url FROM offer_links WHERE offer_id = $1 ORDER BY label`, offerID)
	if err != nil {
		return nil, fmt.Errorf("offer: reading links in tx: %w", err)
	}
	defer rows.Close()

	links := []Link{}
	for rows.Next() {
		var l Link
		if err := rows.Scan(&l.ID, &l.Label, &l.URL); err != nil {
			return nil, fmt.Errorf("offer: scanning link in tx: %w", err)
		}
		links = append(links, l)
	}
	return links, rows.Err()
}

// Delete: offer_links CASCADEs (00003) — no conflict from there. §36-
// flows.destination_offer_id (00006) has no ON DELETE clause (RESTRICT)
// — no Flow CRUD exists yet to populate it, but the 23503 catch is here
// defensively for when it does, same shape as network/trafficsource's
// own Delete.
func (r *Repository) Delete(ctx context.Context, orgID, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM offers WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return apierror.Conflict("offer is still referenced by one or more flows")
		}
		return fmt.Errorf("deleting offer: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("offer not found")
	}
	return nil
}

// NetworkBelongsToOrg guards the same cross-tenant gap
// campaign.Repository.TrafficSourceBelongsToOrg documents (§36-TENANCY).
func (r *Repository) NetworkBelongsToOrg(ctx context.Context, orgID, networkID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM networks WHERE id = $1 AND organization_id = $2)`,
		networkID, orgID,
	).Scan(&exists)
	return exists, err
}
