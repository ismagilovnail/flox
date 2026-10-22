package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const planColumns = `p.code, p.name, p.monthly_clicks, p.max_campaigns, p.max_domains, p.max_members, p.price_cents, p.currency, p.price_rub`

func scanPlan(row pgx.Row, extra ...any) (Plan, error) {
	var p Plan
	err := row.Scan(append([]any{&p.Code, &p.Name, &p.MonthlyClicks, &p.MaxCampaigns, &p.MaxDomains, &p.MaxMembers, &p.PriceCents, &p.Currency, &p.PriceRub}, extra...)...)
	return p, err
}

// Plans lists the public plans, cheapest first.
func (r *Repository) Plans(ctx context.Context) ([]Plan, error) {
	rows, err := r.db.Query(ctx, `SELECT `+planColumns+` FROM plans p WHERE p.public ORDER BY p.sort_order, p.code`)
	if err != nil {
		return nil, fmt.Errorf("billing: listing plans: %w", err)
	}
	defer rows.Close()
	out := []Plan{}
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, fmt.Errorf("billing: reading a plan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Current is the organization's plan and since when: its open
// organization_plans row, or Free since the organization was created.
func (r *Repository) Current(ctx context.Context, orgID string) (Plan, time.Time, error) {
	s, err := r.Subscription(ctx, orgID)
	return s.Plan, s.Since, err
}

// Subscription is Current with the paid period.
func (r *Repository) Subscription(ctx context.Context, orgID string) (Subscription, error) {
	var s Subscription
	p, err := scanPlan(r.db.QueryRow(ctx, `
		SELECT `+planColumns+`, coalesce(op.started_at, o.created_at), op.paid_until
		FROM organizations o
		LEFT JOIN organization_plans op ON op.organization_id = o.id AND op.ended_at IS NULL
		JOIN plans p ON p.code = coalesce(op.plan_code, '`+FreePlan+`')
		WHERE o.id = $1`, orgID), &s.Since, &s.PaidUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return Subscription{}, apierror.NotFound("organization not found")
	}
	if err != nil {
		return Subscription{}, fmt.Errorf("billing: reading the plan: %w", err)
	}
	s.Plan = p
	return s, nil
}

// Counts reads what the count limits are checked against.
func (r *Repository) Counts(ctx context.Context, orgID string) (Counts, error) {
	var c Counts
	err := r.db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM campaigns WHERE organization_id = $1 AND status <> 'archived'),
			(SELECT count(*) FROM domains WHERE organization_id = $1),
			(SELECT count(*) FROM memberships WHERE organization_id = $1 AND status <> 'suspended')`,
		orgID,
	).Scan(&c.Campaigns, &c.Domains, &c.Members)
	if err != nil {
		return Counts{}, fmt.Errorf("billing: counting: %w", err)
	}
	return c, nil
}

// SetPlan closes the organization's current plan and opens code, with an
// audit_logs row naming both. The organization row is locked so two
// changes at once cannot both open a plan.
func (r *Repository) SetPlan(ctx context.Context, orgID, code, note string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("billing: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT true FROM organizations WHERE id = $1 FOR UPDATE`, orgID).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.NotFound("organization not found")
		}
		return fmt.Errorf("billing: locking the organization: %w", err)
	}
	if err := tx.QueryRow(ctx, `SELECT true FROM plans WHERE code = $1`, code).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.Validation("unknown plan", map[string]string{"plan": code})
		}
		return fmt.Errorf("billing: reading the plan: %w", err)
	}

	from := FreePlan
	if err := tx.QueryRow(ctx, `SELECT plan_code FROM organization_plans WHERE organization_id = $1 AND ended_at IS NULL`, orgID).Scan(&from); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("billing: reading the current plan: %w", err)
	}
	if from == code {
		return apierror.Conflict("the organization is already on this plan")
	}

	if _, err := tx.Exec(ctx, `UPDATE organization_plans SET ended_at = now() WHERE organization_id = $1 AND ended_at IS NULL`, orgID); err != nil {
		return fmt.Errorf("billing: closing the current plan: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO organization_plans (id, organization_id, plan_code, note) VALUES ($1, $2, $3, $4)`,
		idgen.New(), orgID, code, note); err != nil {
		return fmt.Errorf("billing: opening the plan: %w", err)
	}
	meta, _ := json.Marshal(map[string]string{"from": from, "to": code, "note": note}) // strings cannot fail to marshal
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs (id, organization_id, actor_user_id, action, entity_type, entity_id, metadata)
		VALUES ($1, $2, NULL, 'billing.plan_changed', 'organization', $3, $4)`, idgen.New(), orgID, orgID, meta); err != nil {
		return fmt.Errorf("billing: auditing the change: %w", err)
	}
	return tx.Commit(ctx)
}

// History is every plan the organization has been on, newest first.
func (r *Repository) History(ctx context.Context, orgID string) ([]Change, error) {
	rows, err := r.db.Query(ctx, `SELECT plan_code, started_at, ended_at, note FROM organization_plans
		WHERE organization_id = $1 ORDER BY started_at DESC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("billing: reading the history: %w", err)
	}
	defer rows.Close()
	var out []Change
	for rows.Next() {
		var c Change
		if err := rows.Scan(&c.PlanCode, &c.StartedAt, &c.EndedAt, &c.Note); err != nil {
			return nil, fmt.Errorf("billing: reading the history: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
