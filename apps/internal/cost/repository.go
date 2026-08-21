package cost

import (
	"context"
	"fmt"
	"time"

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

const selectColumns = `id, organization_id, campaign_id, traffic_source_id, entry_date, amount, currency, amount_usd, created_by_user_id, created_at, updated_at`

func scanEntry(row pgx.Row) (Entry, error) {
	var e Entry
	err := row.Scan(
		&e.ID, &e.OrganizationID, &e.CampaignID, &e.TrafficSourceID, &e.EntryDate,
		&e.Amount, &e.Currency, &e.AmountUSD, &e.CreatedByUserID, &e.CreatedAt, &e.UpdatedAt,
	)
	return e, err
}

// Upsert inserts a new (campaign, source, day) entry or overwrites the
// existing one — always "manual" source (the only writer this phase has;
// FB/TikTok import is later work per §27-COST). Two statements, chosen by
// whether TrafficSourceID is set, because cost_entries' identity is
// defined by two separate partial unique indexes (00009) — traffic_source_id
// IS NULL and IS NOT NULL can't share one ON CONFLICT target.
func (r *Repository) Upsert(ctx context.Context, id, orgID, campaignID string, in UpsertInput, amountUSD *float64) (Entry, error) {
	var row pgx.Row
	if in.TrafficSourceID != nil {
		row = r.db.QueryRow(ctx, `
			INSERT INTO cost_entries (id, organization_id, campaign_id, traffic_source_id, entry_date, amount, currency, amount_usd, source)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'manual')
			ON CONFLICT (campaign_id, traffic_source_id, entry_date) WHERE traffic_source_id IS NOT NULL
			DO UPDATE SET amount = EXCLUDED.amount, currency = EXCLUDED.currency, amount_usd = EXCLUDED.amount_usd, updated_at = now()
			RETURNING `+selectColumns,
			id, orgID, campaignID, *in.TrafficSourceID, in.EntryDate, in.Amount, in.Currency, amountUSD,
		)
	} else {
		row = r.db.QueryRow(ctx, `
			INSERT INTO cost_entries (id, organization_id, campaign_id, traffic_source_id, entry_date, amount, currency, amount_usd, source)
			VALUES ($1, $2, $3, NULL, $4, $5, $6, $7, 'manual')
			ON CONFLICT (campaign_id, entry_date) WHERE traffic_source_id IS NULL
			DO UPDATE SET amount = EXCLUDED.amount, currency = EXCLUDED.currency, amount_usd = EXCLUDED.amount_usd, updated_at = now()
			RETURNING `+selectColumns,
			id, orgID, campaignID, in.EntryDate, in.Amount, in.Currency, amountUSD,
		)
	}
	return scanEntry(row)
}

func (r *Repository) List(ctx context.Context, orgID, campaignID string, filter ListFilter) ([]Entry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+selectColumns+`
		FROM cost_entries
		WHERE organization_id = $1 AND campaign_id = $2 AND entry_date >= $3 AND entry_date <= $4
		ORDER BY entry_date DESC`,
		orgID, campaignID, filter.From, filter.To,
	)
	if err != nil {
		return nil, fmt.Errorf("querying cost entries: %w", err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning cost entry row: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (r *Repository) Delete(ctx context.Context, orgID, campaignID, id string) error {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM cost_entries WHERE id = $1 AND organization_id = $2 AND campaign_id = $3`,
		id, orgID, campaignID,
	)
	if err != nil {
		return fmt.Errorf("deleting cost entry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("cost entry not found")
	}
	return nil
}

// CampaignBelongsToOrg guards the same cross-tenant gap
// campaign.Repository.TrafficSourceBelongsToOrg documents (§36-TENANCY):
// a syntactically valid campaign id proves nothing about which org owns
// it.
func (r *Repository) CampaignBelongsToOrg(ctx context.Context, orgID, campaignID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM campaigns WHERE id = $1 AND organization_id = $2)`,
		campaignID, orgID,
	).Scan(&exists)
	return exists, err
}

// TrafficSourceBelongsToOrg mirrors campaign.Repository's own method of
// the same name (§36-TENANCY) — a syntactically valid traffic source id
// proves nothing about which org owns it.
func (r *Repository) TrafficSourceBelongsToOrg(ctx context.Context, orgID, trafficSourceID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM traffic_sources WHERE id = $1 AND organization_id = $2)`,
		trafficSourceID, orgID,
	).Scan(&exists)
	return exists, err
}

// DailyCampaignSpend sums amount_usd per day for one campaign over
// [from, to], across every traffic source. bool_and(amount_usd IS NOT
// NULL) surfaces a day where at least one entry still has no FX rate on
// file, rather than letting SUM() silently skip it and understate spend.
func (r *Repository) DailyCampaignSpend(ctx context.Context, orgID, campaignID string, from, to time.Time) ([]DailySpend, error) {
	rows, err := r.db.Query(ctx, `
		SELECT entry_date, COALESCE(SUM(amount_usd), 0), bool_and(amount_usd IS NOT NULL)
		FROM cost_entries
		WHERE organization_id = $1 AND campaign_id = $2 AND entry_date >= $3 AND entry_date <= $4
		GROUP BY entry_date
		ORDER BY entry_date`,
		orgID, campaignID, from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("querying daily campaign spend: %w", err)
	}
	defer rows.Close()

	var out []DailySpend
	for rows.Next() {
		var d DailySpend
		if err := rows.Scan(&d.Day, &d.AmountUSD, &d.AllConverted); err != nil {
			return nil, fmt.Errorf("scanning daily campaign spend row: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
