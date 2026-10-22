package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

const paymentColumns = `id, organization_id, plan_code, months, amount, currency, coalesce(provider_payment_id, ''),
	confirmation_url, status, cancellation_reason, applied_at, created_at`

func scanPayment(row pgx.Row) (Payment, error) {
	var p Payment
	err := row.Scan(&p.ID, &p.OrganizationID, &p.PlanCode, &p.Months, &p.Amount, &p.Currency, &p.ProviderPaymentID,
		&p.ConfirmationURL, &p.Status, &p.CancellationReason, &p.AppliedAt, &p.CreatedAt)
	return p, err
}

// Plan reads one plan by code, public or not.
func (r *Repository) Plan(ctx context.Context, code string) (Plan, error) {
	p, err := scanPlan(r.db.QueryRow(ctx, `SELECT `+planColumns+` FROM plans p WHERE p.code = $1`, code))
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, apierror.Validation("unknown plan", map[string]string{"plan": code})
	}
	if err != nil {
		return Plan{}, fmt.Errorf("billing: reading plan %s: %w", code, err)
	}
	return p, nil
}

// OwnerEmail is userID's email when they are an active Owner of the
// organization; ok false otherwise.
func (r *Repository) OwnerEmail(ctx context.Context, orgID, userID string) (email string, ok bool, err error) {
	err = r.db.QueryRow(ctx, `
		SELECT u.email FROM memberships m
		JOIN users u ON u.id = m.user_id
		JOIN roles ro ON ro.id = m.role_id
		WHERE m.organization_id = $1 AND m.user_id = $2 AND m.status = 'active' AND ro.key = 'Owner'`,
		orgID, userID).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("billing: reading the payer: %w", err)
	}
	return email, true, nil
}

// OwnerEmails are the active Owners' addresses, for reminders.
func (r *Repository) OwnerEmails(ctx context.Context, orgID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT u.email FROM memberships m
		JOIN users u ON u.id = m.user_id
		JOIN roles ro ON ro.id = m.role_id
		WHERE m.organization_id = $1 AND m.status = 'active' AND ro.key = 'Owner'`, orgID)
	if err != nil {
		return nil, fmt.Errorf("billing: listing owners: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CreatePayment records a checkout before the provider is called, so its
// id can be the provider's idempotence key.
func (r *Repository) CreatePayment(ctx context.Context, p Payment, userID string) (Payment, error) {
	return scanPayment(r.db.QueryRow(ctx, `
		INSERT INTO billing_payments (id, organization_id, plan_code, months, amount, currency, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+paymentColumns, p.ID, p.OrganizationID, p.PlanCode, p.Months, p.Amount, p.Currency, userID))
}

// AttachProvider stores the provider's payment id and payment page.
func (r *Repository) AttachProvider(ctx context.Context, id, provider, providerPaymentID, confirmationURL string) error {
	_, err := r.db.Exec(ctx, `UPDATE billing_payments SET provider = $2, provider_payment_id = $3, confirmation_url = $4 WHERE id = $1`,
		id, provider, providerPaymentID, confirmationURL)
	if err != nil {
		return fmt.Errorf("billing: storing the provider payment: %w", err)
	}
	return nil
}

// Payment reads one of the organization's payments.
func (r *Repository) Payment(ctx context.Context, orgID, id string) (Payment, error) {
	p, err := scanPayment(r.db.QueryRow(ctx, `SELECT `+paymentColumns+` FROM billing_payments WHERE organization_id = $1 AND id = $2`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, apierror.NotFound("payment not found")
	}
	if err != nil {
		return Payment{}, fmt.Errorf("billing: reading a payment: %w", err)
	}
	return p, nil
}

// PaymentByProviderID finds a payment from a provider notification; ok
// false when it is not one of ours.
func (r *Repository) PaymentByProviderID(ctx context.Context, provider, providerPaymentID string) (Payment, bool, error) {
	p, err := scanPayment(r.db.QueryRow(ctx, `SELECT `+paymentColumns+` FROM billing_payments WHERE provider = $1 AND provider_payment_id = $2`,
		provider, providerPaymentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, false, nil
	}
	if err != nil {
		return Payment{}, false, fmt.Errorf("billing: finding a payment: %w", err)
	}
	return p, true, nil
}

// Payments lists the organization's latest payments.
func (r *Repository) Payments(ctx context.Context, orgID string, limit int) ([]Payment, error) {
	rows, err := r.db.Query(ctx, `SELECT `+paymentColumns+` FROM billing_payments WHERE organization_id = $1
		ORDER BY created_at DESC LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, fmt.Errorf("billing: listing payments: %w", err)
	}
	defer rows.Close()
	out := []Payment{}
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, fmt.Errorf("billing: reading a payment: %w", err)
		}
		p.ConfirmationURL = "" // only the checkout answer carries it
		out = append(out, p)
	}
	return out, rows.Err()
}

// The worker's all-organization queries below take only, an organization
// to limit them to; "" is every organization (apps/worker). Tests pass
// their own: the dev database is shared with other packages' tests.
const onlyOrg = `($%[1]d::text = '' OR %[2]s::text = $%[1]d::text)`

// PendingPayments are payments still pending that were created between
// from and to, oldest first — apps/worker reads them back from the
// provider in case a notification was lost.
func (r *Repository) PendingPayments(ctx context.Context, only string, from, to time.Time, limit int) ([]Payment, error) {
	rows, err := r.db.Query(ctx, `SELECT `+paymentColumns+` FROM billing_payments
		WHERE status = 'pending' AND provider_payment_id IS NOT NULL AND created_at >= $1 AND created_at < $2
		  AND `+fmt.Sprintf(onlyOrg, 4, "organization_id")+`
		ORDER BY created_at LIMIT $3`, from, to, limit, only)
	if err != nil {
		return nil, fmt.Errorf("billing: listing pending payments: %w", err)
	}
	defer rows.Close()
	var out []Payment
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CancelStale marks pending payments created before before as canceled
// without asking the provider: a payment page abandoned for that long is
// not coming back (YooKassa cancels unpaid payments itself well before).
func (r *Repository) CancelStale(ctx context.Context, only string, before time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx, `UPDATE billing_payments SET status = 'canceled', cancellation_reason = 'expired'
		WHERE status = 'pending' AND created_at < $1 AND `+fmt.Sprintf(onlyOrg, 2, "organization_id"), before, only)
	if err != nil {
		return 0, fmt.Errorf("billing: cancelling stale payments: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Extender decides the new end of the paid period when a payment applies:
// sub is the organization's plan right now, read inside the transaction.
type Extender func(sub Subscription, p Payment, now time.Time) time.Time

// ApplyStatus brings the payment to the provider's status, once. A
// succeeded payment extends (or switches) the organization's plan in the
// same transaction, under a lock on the payment and the organization, and
// sets applied_at: applying it again changes nothing. applied reports
// whether this call changed the plan.
func (r *Repository) ApplyStatus(ctx context.Context, paymentID string, pp ProviderPayment, now time.Time, extend Extender) (applied bool, err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("billing: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	p, err := scanPayment(tx.QueryRow(ctx, `SELECT `+paymentColumns+` FROM billing_payments WHERE id = $1 FOR UPDATE`, paymentID))
	if err != nil {
		return false, fmt.Errorf("billing: locking payment %s: %w", paymentID, err)
	}
	if p.AppliedAt != nil || p.Status == PaymentCanceled && pp.Status == PaymentCanceled {
		return false, nil
	}
	switch pp.Status {
	case PaymentPending:
		return false, nil
	case PaymentCanceled:
		if p.Status == PaymentSucceeded {
			// A succeeded payment does not go back; a refund is handled by hand.
			return false, fmt.Errorf("billing: provider reports payment %s canceled after it succeeded", p.ID)
		}
		if _, err := tx.Exec(ctx, `UPDATE billing_payments SET status = 'canceled', cancellation_reason = $2 WHERE id = $1`,
			p.ID, pp.CancellationReason); err != nil {
			return false, fmt.Errorf("billing: cancelling payment %s: %w", p.ID, err)
		}
		return false, tx.Commit(ctx)
	case PaymentSucceeded:
	default:
		return false, fmt.Errorf("billing: unknown provider status %q", pp.Status)
	}
	if pp.Amount != p.Amount || pp.Currency != p.Currency {
		return false, fmt.Errorf("billing: payment %s: provider says %d %s, checkout was %d %s", p.ID, pp.Amount, pp.Currency, p.Amount, p.Currency)
	}

	if _, err := tx.Exec(ctx, `SELECT 1 FROM organizations WHERE id = $1 FOR UPDATE`, p.OrganizationID); err != nil {
		return false, fmt.Errorf("billing: locking the organization: %w", err)
	}
	var sub Subscription
	var openID *string
	sub.Plan, err = scanPlan(tx.QueryRow(ctx, `
		SELECT `+planColumns+`, coalesce(op.started_at, o.created_at), op.paid_until, op.id
		FROM organizations o
		LEFT JOIN organization_plans op ON op.organization_id = o.id AND op.ended_at IS NULL
		JOIN plans p ON p.code = coalesce(op.plan_code, '`+FreePlan+`')
		WHERE o.id = $1`, p.OrganizationID), &sub.Since, &sub.PaidUntil, &openID)
	if err != nil {
		return false, fmt.Errorf("billing: reading the plan: %w", err)
	}
	paidUntil := extend(sub, p, now)
	note := "payment " + p.ID

	if openID != nil && sub.Plan.Code == p.PlanCode {
		if _, err := tx.Exec(ctx, `UPDATE organization_plans SET paid_until = $2, reminded_at = NULL WHERE id = $1`, *openID, paidUntil); err != nil {
			return false, fmt.Errorf("billing: extending the plan: %w", err)
		}
	} else {
		if openID != nil {
			if _, err := tx.Exec(ctx, `UPDATE organization_plans SET ended_at = $2 WHERE id = $1`, *openID, now); err != nil {
				return false, fmt.Errorf("billing: closing the plan: %w", err)
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO organization_plans (id, organization_id, plan_code, started_at, paid_until, note)
			VALUES ($1, $2, $3, $4, $5, $6)`, idgen.New(), p.OrganizationID, p.PlanCode, now, paidUntil, note); err != nil {
			return false, fmt.Errorf("billing: opening the plan: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE billing_payments SET status = 'succeeded', applied_at = $2 WHERE id = $1`, p.ID, now); err != nil {
		return false, fmt.Errorf("billing: marking payment %s: %w", p.ID, err)
	}
	meta, _ := json.Marshal(map[string]any{"from": sub.Plan.Code, "to": p.PlanCode, "months": p.Months,
		"amount": p.Amount, "currency": p.Currency, "paidUntil": paidUntil.UTC().Format(time.RFC3339)})
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs (id, organization_id, actor_user_id, action, entity_type, entity_id, metadata)
		VALUES ($1, $2, NULL, 'billing.payment_applied', 'billing_payment', $3, $4)`, idgen.New(), p.OrganizationID, p.ID, meta); err != nil {
		return false, fmt.Errorf("billing: auditing the payment: %w", err)
	}
	return true, tx.Commit(ctx)
}

// Lapsed is a paid plan whose grace period is over.
type Lapsed struct {
	OrganizationID string
	PlanName       string
}

// ExpireLapsed moves organizations whose paid period ended more than
// GracePeriod before now to Free, each with an audit row.
func (r *Repository) ExpireLapsed(ctx context.Context, only string, now time.Time) ([]Lapsed, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("billing: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		UPDATE organization_plans op SET ended_at = $1
		FROM plans p
		WHERE op.ended_at IS NULL AND op.paid_until IS NOT NULL AND op.paid_until < $2 AND p.code = op.plan_code
		  AND `+fmt.Sprintf(onlyOrg, 3, "op.organization_id")+`
		RETURNING op.organization_id, op.plan_code, p.name`, now, now.Add(-GracePeriod), only)
	if err != nil {
		return nil, fmt.Errorf("billing: expiring plans: %w", err)
	}
	type row struct{ org, code, name string }
	var lapsed []row
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.org, &x.code, &x.name); err != nil {
			rows.Close()
			return nil, err
		}
		lapsed = append(lapsed, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Lapsed, 0, len(lapsed))
	for _, x := range lapsed {
		if _, err := tx.Exec(ctx, `INSERT INTO organization_plans (id, organization_id, plan_code, started_at, note)
			VALUES ($1, $2, $3, $4, 'paid period ended')`, idgen.New(), x.org, FreePlan, now); err != nil {
			return nil, fmt.Errorf("billing: moving %s to Free: %w", x.org, err)
		}
		meta, _ := json.Marshal(map[string]string{"from": x.code, "to": FreePlan})
		if _, err := tx.Exec(ctx, `INSERT INTO audit_logs (id, organization_id, actor_user_id, action, entity_type, entity_id, metadata)
			VALUES ($1, $2, NULL, 'billing.plan_expired', 'organization', $3, $4)`, idgen.New(), x.org, x.org, meta); err != nil {
			return nil, fmt.Errorf("billing: auditing the expiry: %w", err)
		}
		out = append(out, Lapsed{OrganizationID: x.org, PlanName: x.name})
	}
	return out, tx.Commit(ctx)
}

// Due is a paid plan whose owners are to be asked to renew.
type Due struct {
	OrganizationID string
	PlanName       string
	PaidUntil      time.Time
}

// ClaimReminders marks and returns the paid plans ending within
// RemindBefore of now (or already ended) whose owners were not reminded
// for this period yet. Marking first means a failed email is not retried
// — one missed reminder is better than a reminder every run.
func (r *Repository) ClaimReminders(ctx context.Context, only string, now time.Time) ([]Due, error) {
	rows, err := r.db.Query(ctx, `
		UPDATE organization_plans op SET reminded_at = $1
		FROM plans p
		WHERE op.ended_at IS NULL AND op.paid_until IS NOT NULL AND op.reminded_at IS NULL
		  AND op.paid_until < $2 AND p.code = op.plan_code AND `+fmt.Sprintf(onlyOrg, 3, "op.organization_id")+`
		RETURNING op.organization_id, p.name, op.paid_until`, now, now.Add(RemindBefore), only)
	if err != nil {
		return nil, fmt.Errorf("billing: claiming reminders: %w", err)
	}
	defer rows.Close()
	var out []Due
	for rows.Next() {
		var d Due
		if err := rows.Scan(&d.OrganizationID, &d.PlanName, &d.PaidUntil); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
