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

const selectColumns = `id, organization_id, campaign_id, traffic_source_id, entry_date, amount, currency, amount_usd, source, created_by_user_id, created_at, updated_at`

func scanEntry(row pgx.Row) (Entry, error) {
	var e Entry
	err := row.Scan(
		&e.ID, &e.OrganizationID, &e.CampaignID, &e.TrafficSourceID, &e.EntryDate,
		&e.Amount, &e.Currency, &e.AmountUSD, &e.Source, &e.CreatedByUserID, &e.CreatedAt, &e.UpdatedAt,
	)
	return e, err
}

// Upsert inserts a new (campaign, source, day) entry or overwrites the
// existing one, tagged with the given Source. Two statements, chosen by
// whether TrafficSourceID is set, because cost_entries' identity is
// defined by two separate partial unique indexes (00009) — traffic_source_id
// IS NULL and IS NOT NULL can't share one ON CONFLICT target. source is
// a parameter here (not read off Entry/UpsertInput) so every caller has
// to say explicitly what it's writing — see UpsertInput's own doc
// comment for why Service.Upsert/UpsertFromSync split on this instead of
// sharing one struct field.
func (r *Repository) Upsert(ctx context.Context, id, orgID, campaignID string, in UpsertInput, amountUSD *float64, source Source) (Entry, error) {
	var row pgx.Row
	if in.TrafficSourceID != nil {
		row = r.db.QueryRow(ctx, `
			INSERT INTO cost_entries (id, organization_id, campaign_id, traffic_source_id, entry_date, amount, currency, amount_usd, source)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (campaign_id, traffic_source_id, entry_date) WHERE traffic_source_id IS NOT NULL
			DO UPDATE SET amount = EXCLUDED.amount, currency = EXCLUDED.currency, amount_usd = EXCLUDED.amount_usd, source = EXCLUDED.source, updated_at = now()
			RETURNING `+selectColumns,
			id, orgID, campaignID, *in.TrafficSourceID, in.EntryDate, in.Amount, in.Currency, amountUSD, source,
		)
	} else {
		row = r.db.QueryRow(ctx, `
			INSERT INTO cost_entries (id, organization_id, campaign_id, traffic_source_id, entry_date, amount, currency, amount_usd, source)
			VALUES ($1, $2, $3, NULL, $4, $5, $6, $7, $8)
			ON CONFLICT (campaign_id, entry_date) WHERE traffic_source_id IS NULL
			DO UPDATE SET amount = EXCLUDED.amount, currency = EXCLUDED.currency, amount_usd = EXCLUDED.amount_usd, source = EXCLUDED.source, updated_at = now()
			RETURNING `+selectColumns,
			id, orgID, campaignID, in.EntryDate, in.Amount, in.Currency, amountUSD, source,
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

// SpendByCampaign sums amount_usd per campaign, across every traffic
// source and every day in [from, to] — the report builder and Overview
// dashboard need this (cost is only ever computable when a
// report's grouping resolves to campaign granularity or coarser; see
// apps/internal/analytics's own doc comment on why) and DailyCampaignSpend
// above doesn't fit: that's one campaign's spend broken out by day, this
// is every campaign's spend for the org collapsed to one total each.
func (r *Repository) SpendByCampaign(ctx context.Context, orgID string, from, to time.Time) ([]CampaignSpend, error) {
	rows, err := r.db.Query(ctx, `
		SELECT campaign_id, COALESCE(SUM(amount_usd), 0), bool_and(amount_usd IS NOT NULL)
		FROM cost_entries
		WHERE organization_id = $1 AND entry_date >= $2 AND entry_date <= $3
		GROUP BY campaign_id`,
		orgID, from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("querying spend by campaign: %w", err)
	}
	defer rows.Close()

	var out []CampaignSpend
	for rows.Next() {
		var c CampaignSpend
		if err := rows.Scan(&c.CampaignID, &c.AmountUSD, &c.AllConverted); err != nil {
			return nil, fmt.Errorf("scanning spend-by-campaign row: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SpendBySource sums amount_usd per traffic source — each cost entry counts
// toward the source its campaign is on (campaigns.traffic_source_id), the
// same attribution the report builder's `source` dimension uses for clicks.
func (r *Repository) SpendBySource(ctx context.Context, orgID string, from, to time.Time) ([]SourceSpend, error) {
	rows, err := r.db.Query(ctx, `
		SELECT c.traffic_source_id, COALESCE(SUM(ce.amount_usd), 0), bool_and(ce.amount_usd IS NOT NULL)
		FROM cost_entries ce
		JOIN campaigns c ON c.id = ce.campaign_id AND c.organization_id = ce.organization_id
		WHERE ce.organization_id = $1 AND ce.entry_date >= $2 AND ce.entry_date <= $3
		GROUP BY c.traffic_source_id`,
		orgID, from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("querying spend by source: %w", err)
	}
	defer rows.Close()

	var out []SourceSpend
	for rows.Next() {
		var c SourceSpend
		if err := rows.Scan(&c.TrafficSourceID, &c.AmountUSD, &c.AllConverted); err != nil {
			return nil, fmt.Errorf("scanning spend-by-source row: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CampaignSpendByDay sums amount_usd per (campaign, day).
func (r *Repository) CampaignSpendByDay(ctx context.Context, orgID string, from, to time.Time) ([]CampaignDaySpend, error) {
	rows, err := r.db.Query(ctx, `
		SELECT campaign_id, entry_date, COALESCE(SUM(amount_usd), 0)
		FROM cost_entries
		WHERE organization_id = $1 AND entry_date >= $2 AND entry_date <= $3
		GROUP BY campaign_id, entry_date`,
		orgID, from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("querying spend by campaign and day: %w", err)
	}
	defer rows.Close()
	var out []CampaignDaySpend
	for rows.Next() {
		var c CampaignDaySpend
		if err := rows.Scan(&c.CampaignID, &c.Day, &c.AmountUSD); err != nil {
			return nil, fmt.Errorf("scanning spend-by-campaign-day row: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SourceSpendByDay is SpendBySource per day (each entry counts toward its
// campaign's traffic source).
func (r *Repository) SourceSpendByDay(ctx context.Context, orgID string, from, to time.Time) ([]SourceDaySpend, error) {
	rows, err := r.db.Query(ctx, `
		SELECT c.traffic_source_id, ce.entry_date, COALESCE(SUM(ce.amount_usd), 0)
		FROM cost_entries ce
		JOIN campaigns c ON c.id = ce.campaign_id AND c.organization_id = ce.organization_id
		WHERE ce.organization_id = $1 AND ce.entry_date >= $2 AND ce.entry_date <= $3
		GROUP BY c.traffic_source_id, ce.entry_date`,
		orgID, from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("querying spend by source and day: %w", err)
	}
	defer rows.Close()
	var out []SourceDaySpend
	for rows.Next() {
		var c SourceDaySpend
		if err := rows.Scan(&c.TrafficSourceID, &c.Day, &c.AmountUSD); err != nil {
			return nil, fmt.Errorf("scanning spend-by-source-day row: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DailySpendForOrg sums amount_usd per day across every campaign and
// traffic source for the whole org — the Overview dashboard's Spend
// chart/KPI, the org-wide counterpart to DailyCampaignSpend.
func (r *Repository) DailySpendForOrg(ctx context.Context, orgID string, from, to time.Time) ([]DailySpend, error) {
	rows, err := r.db.Query(ctx, `
		SELECT entry_date, COALESCE(SUM(amount_usd), 0), bool_and(amount_usd IS NOT NULL)
		FROM cost_entries
		WHERE organization_id = $1 AND entry_date >= $2 AND entry_date <= $3
		GROUP BY entry_date
		ORDER BY entry_date`,
		orgID, from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("querying daily org spend: %w", err)
	}
	defer rows.Close()

	var out []DailySpend
	for rows.Next() {
		var d DailySpend
		if err := rows.Scan(&d.Day, &d.AmountUSD, &d.AllConverted); err != nil {
			return nil, fmt.Errorf("scanning daily org spend row: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// AccountTimezones lists the ad-account zones of the synced spend in
// [from, to]: each entry's traffic source's connection zone, where a sync
// has learned it (tech audit AN4). Manual entries have none.
func (r *Repository) AccountTimezones(ctx context.Context, orgID string, from, to time.Time) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT ac.timezone
		FROM cost_entries ce
		JOIN ad_account_connections ac
		  ON ac.organization_id = ce.organization_id AND ac.traffic_source_id = ce.traffic_source_id
		WHERE ce.organization_id = $1 AND ce.entry_date >= $2 AND ce.entry_date <= $3
		  AND ce.source <> 'manual' AND ac.timezone <> ''
		ORDER BY 1`,
		orgID, from, to)
	if err != nil {
		return nil, fmt.Errorf("cost: listing ad account timezones: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var tz string
		if err := rows.Scan(&tz); err != nil {
			return nil, fmt.Errorf("cost: listing ad account timezones: %w", err)
		}
		out = append(out, tz)
	}
	return out, rows.Err()
}
