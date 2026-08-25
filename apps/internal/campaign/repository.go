package campaign

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

const selectColumns = `id, organization_id, traffic_source_id, name, status, fallback_url, notes, external_campaign_id, created_at, updated_at`

func scanCampaign(row pgx.Row) (Campaign, error) {
	var c Campaign
	err := row.Scan(&c.ID, &c.OrganizationID, &c.TrafficSourceID, &c.Name, &c.Status, &c.FallbackURL, &c.Notes, &c.ExternalCampaignID, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (r *Repository) Create(ctx context.Context, id, orgID string, in CreateInput) (Campaign, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO campaigns (id, organization_id, traffic_source_id, name, fallback_url, notes, external_campaign_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+selectColumns,
		id, orgID, in.TrafficSourceID, in.Name, in.FallbackURL, in.Notes, in.ExternalCampaignID,
	)
	return scanCampaign(row)
}

func (r *Repository) GetByID(ctx context.Context, orgID, id string) (Campaign, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+selectColumns+`
		FROM campaigns
		WHERE id = $1 AND organization_id = $2`,
		id, orgID,
	)
	c, err := scanCampaign(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Campaign{}, apierror.NotFound("campaign not found")
	}
	return c, err
}

func (r *Repository) List(ctx context.Context, orgID string, filter ListFilter) (ListResult, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+selectColumns+`
		FROM campaigns
		WHERE organization_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`,
		orgID, filter.Limit, filter.Offset,
	)
	if err != nil {
		return ListResult{}, fmt.Errorf("querying campaigns: %w", err)
	}
	defer rows.Close()

	var campaigns []Campaign
	for rows.Next() {
		c, err := scanCampaign(rows)
		if err != nil {
			return ListResult{}, fmt.Errorf("scanning campaign row: %w", err)
		}
		campaigns = append(campaigns, c)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, fmt.Errorf("iterating campaign rows: %w", err)
	}

	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM campaigns WHERE organization_id = $1`, orgID).Scan(&total); err != nil {
		return ListResult{}, fmt.Errorf("counting campaigns: %w", err)
	}

	return ListResult{Campaigns: campaigns, Total: total}, nil
}

// Update applies only the non-nil fields in in, via a dynamically built SET
// clause — a real partial update (PATCH semantics), not a full-row replace
// that would clobber fields the caller never mentioned.
func (r *Repository) Update(ctx context.Context, orgID, id string, in UpdateInput) (Campaign, error) {
	sets := []string{}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if in.Name != nil {
		sets = append(sets, "name = "+arg(*in.Name))
	}
	if in.TrafficSourceID != nil {
		sets = append(sets, "traffic_source_id = "+arg(*in.TrafficSourceID))
	}
	if in.FallbackURL != nil {
		sets = append(sets, "fallback_url = "+arg(*in.FallbackURL))
	}
	if in.Notes != nil {
		sets = append(sets, "notes = "+arg(*in.Notes))
	}
	if in.Status != nil {
		sets = append(sets, "status = "+arg(*in.Status))
	}
	if in.ExternalCampaignID != nil {
		sets = append(sets, "external_campaign_id = "+arg(*in.ExternalCampaignID))
	}

	if len(sets) == 0 {
		return r.GetByID(ctx, orgID, id)
	}

	query := fmt.Sprintf(
		`UPDATE campaigns SET %s WHERE id = %s AND organization_id = %s RETURNING %s`,
		strings.Join(sets, ", "), arg(id), arg(orgID), selectColumns,
	)

	row := r.db.QueryRow(ctx, query, args...)
	c, err := scanCampaign(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Campaign{}, apierror.NotFound("campaign not found")
	}
	return c, err
}

func (r *Repository) Delete(ctx context.Context, orgID, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM campaigns WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		return fmt.Errorf("deleting campaign: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("campaign not found")
	}
	return nil
}

// ListByExternalID finds every campaign under trafficSourceID whose
// ExternalCampaignID matches — the match step a later ad-spend sync
// (§74/§27-COST) uses to attribute a day's ad-platform-reported spend to
// specific FLOX campaigns. Scoped to trafficSourceID (not just orgID)
// since a connection's own DailySpendByCampaign results only ever cover
// that one traffic source's ad account — matching a bare externalID
// across every campaign in the org would risk attributing spend to a
// campaign under a completely different, unrelated traffic source that
// happens to share the same platform campaign id string by coincidence.
// Returns a slice, not a single Campaign: external_campaign_id has no
// uniqueness constraint (migration 00019's own comment), so a caller who
// genuinely maps two FLOX campaigns to one ad-platform campaign gets the
// full day's spend attributed to both, rather than an arbitrary pick
// silently dropping one.
func (r *Repository) ListByExternalID(ctx context.Context, orgID, trafficSourceID, externalCampaignID string) ([]Campaign, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+selectColumns+`
		FROM campaigns
		WHERE organization_id = $1 AND traffic_source_id = $2 AND external_campaign_id = $3`,
		orgID, trafficSourceID, externalCampaignID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying campaigns by external id: %w", err)
	}
	defer rows.Close()

	var campaigns []Campaign
	for rows.Next() {
		c, err := scanCampaign(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning campaign row: %w", err)
		}
		campaigns = append(campaigns, c)
	}
	return campaigns, rows.Err()
}

// TrafficSourceBelongsToOrg guards against a cross-tenant reference: the
// traffic_sources FK only proves the row exists somewhere, not that it
// belongs to the caller's organization (§36-TENANCY — a forgotten check
// here would let org A silently attach its campaign to org B's source).
func (r *Repository) TrafficSourceBelongsToOrg(ctx context.Context, orgID, trafficSourceID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM traffic_sources WHERE id = $1 AND organization_id = $2)`,
		trafficSourceID, orgID,
	).Scan(&exists)
	return exists, err
}
