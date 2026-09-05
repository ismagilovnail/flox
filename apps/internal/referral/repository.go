package referral

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

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

// ErrCodeNotFound means the code doesn't resolve to any org — expected and
// silent for an invalid/garbage referral code typed by a signing-up user,
// never surfaced to them as an error (see Service.RecordSignup).
var ErrCodeNotFound = errors.New("referral: code not found")

// codeAlphabet skips 0/1/i/l/o — visually ambiguous characters a person
// might mistype copying a short code by hand.
const codeAlphabet = "23456789abcdefghjkmnpqrstuvwxyz"
const codeLength = 8
const maxCodeAttempts = 5

func newCode() (string, error) {
	b := make([]byte, codeLength)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("referral: generating code: %w", err)
	}
	out := make([]byte, codeLength)
	for i, v := range b {
		out[i] = codeAlphabet[int(v)%len(codeAlphabet)]
	}
	return string(out), nil
}

// EnsureCode returns the org's referral code, lazily creating one on first
// call — no admin action or signup-time seed step needed, same bar Report
// Presets' own EnsureDefault already held itself to. Race-safe against
// concurrent first-callers via ON CONFLICT (organization_id) DO NOTHING;
// a genuine code collision (astronomically unlikely at 32^8 combinations)
// is retried with a fresh code up to maxCodeAttempts times.
func (r *Repository) EnsureCode(ctx context.Context, orgID string) (string, error) {
	var existing string
	err := r.db.QueryRow(ctx, `SELECT code FROM referral_codes WHERE organization_id = $1`, orgID).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("referral: reading code: %w", err)
	}

	for attempt := 0; attempt < maxCodeAttempts; attempt++ {
		code, err := newCode()
		if err != nil {
			return "", err
		}
		_, err = r.db.Exec(ctx, `
			INSERT INTO referral_codes (organization_id, code) VALUES ($1, $2)
			ON CONFLICT (organization_id) DO NOTHING`, orgID, code)
		if err == nil {
			var final string
			if err := r.db.QueryRow(ctx, `SELECT code FROM referral_codes WHERE organization_id = $1`, orgID).Scan(&final); err != nil {
				return "", fmt.Errorf("referral: reading code after insert: %w", err)
			}
			return final, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "referral_codes_code_key" {
			continue
		}
		return "", fmt.Errorf("referral: creating code: %w", err)
	}
	return "", fmt.Errorf("referral: exhausted attempts generating a unique code")
}

// ResolveCode looks up which org owns a code — used only at signup time to
// attribute a new org's signup to its referrer.
func (r *Repository) ResolveCode(ctx context.Context, code string) (string, error) {
	var orgID string
	err := r.db.QueryRow(ctx, `SELECT organization_id FROM referral_codes WHERE code = $1`, code).Scan(&orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrCodeNotFound
	}
	if err != nil {
		return "", fmt.Errorf("referral: resolving code: %w", err)
	}
	return orgID, nil
}

const signupColumns = `id, organization_id, referred_organization_id, name, email, status, signed_up_at`

func scanSignup(row pgx.Row) (Signup, error) {
	var s Signup
	err := row.Scan(&s.ID, &s.OrganizationID, &s.ReferredOrganizationID, &s.Name, &s.Email, &s.Status, &s.SignedUpAt)
	return s, err
}

func (r *Repository) CreateSignup(ctx context.Context, referrerOrgID, referredOrgID, name, email string) (Signup, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO referral_signups (id, organization_id, referred_organization_id, name, email)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+signupColumns,
		idgen.New(), referrerOrgID, referredOrgID, name, email,
	)
	return scanSignup(row)
}

func (r *Repository) ListSignups(ctx context.Context, orgID string) ([]Signup, error) {
	rows, err := r.db.Query(ctx, `SELECT `+signupColumns+` FROM referral_signups WHERE organization_id = $1 ORDER BY signed_up_at DESC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("referral: listing signups: %w", err)
	}
	defer rows.Close()

	out := []Signup{}
	for rows.Next() {
		s, err := scanSignup(rows)
		if err != nil {
			return nil, fmt.Errorf("referral: scanning signup: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) SignupExists(ctx context.Context, orgID, id string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM referral_signups WHERE id = $1 AND organization_id = $2)`, id, orgID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("referral: checking signup: %w", err)
	}
	return exists, nil
}

const transactionColumns = `id, organization_id, type, amount, currency, description, related_signup_id, related_payout_id, created_by_user_id, created_at`

func scanTransaction(row pgx.Row) (Transaction, error) {
	var t Transaction
	err := row.Scan(
		&t.ID, &t.OrganizationID, &t.Type, &t.Amount, &t.Currency, &t.Description,
		&t.RelatedSignupID, &t.RelatedPayoutID, &t.CreatedByUserID, &t.CreatedAt,
	)
	return t, err
}

func (r *Repository) ListTransactions(ctx context.Context, orgID string) ([]Transaction, error) {
	rows, err := r.db.Query(ctx, `SELECT `+transactionColumns+` FROM referral_transactions WHERE organization_id = $1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("referral: listing transactions: %w", err)
	}
	defer rows.Close()

	out := []Transaction{}
	for rows.Next() {
		t, err := scanTransaction(rows)
		if err != nil {
			return nil, fmt.Errorf("referral: scanning transaction: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) CreateTransaction(ctx context.Context, orgID, createdByUserID string, in AddTransactionInput) (Transaction, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO referral_transactions (id, organization_id, type, amount, currency, description, related_signup_id, created_by_user_id)
		VALUES ($1, $2, $3, $4, 'USD', $5, $6, $7)
		RETURNING `+transactionColumns,
		idgen.New(), orgID, in.Type, in.Amount, in.Description, in.RelatedSignupID, createdByUserID,
	)
	return scanTransaction(row)
}

// ComputeBalance aggregates the ledger + payouts — the balance itself is
// never stored. Formula mirrors the mock's own computeBalances exactly:
// netEarned already nets every accrual and adjustment (positive or
// negative) together, matching what the mock's own "totalEarned" field
// actually returns (totalEarned + totalAdjustedDown in the mock's terms).
func (r *Repository) ComputeBalance(ctx context.Context, orgID string) (Balance, error) {
	var netEarned, totalPaid float64
	err := r.db.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(amount) FILTER (WHERE type IN ('accrual', 'adjustment')), 0),
			COALESCE(SUM(-amount) FILTER (WHERE type = 'payout_paid'), 0)
		FROM referral_transactions WHERE organization_id = $1`, orgID,
	).Scan(&netEarned, &totalPaid)
	if err != nil {
		return Balance{}, fmt.Errorf("referral: aggregating transactions: %w", err)
	}

	var pendingPayout float64
	err = r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0) FROM referral_payouts
		WHERE organization_id = $1 AND status IN ('pending', 'approved')`, orgID,
	).Scan(&pendingPayout)
	if err != nil {
		return Balance{}, fmt.Errorf("referral: aggregating pending payouts: %w", err)
	}

	return Balance{
		TotalEarned:      netEarned,
		TotalPaid:        totalPaid,
		PendingPayout:    pendingPayout,
		AvailableBalance: netEarned - totalPaid - pendingPayout,
	}, nil
}

const payoutColumns = `id, organization_id, amount, currency, status, requested_at, requested_by_user_id, resolved_at, resolved_by_user_id, note`

func scanPayout(row pgx.Row) (Payout, error) {
	var p Payout
	err := row.Scan(
		&p.ID, &p.OrganizationID, &p.Amount, &p.Currency, &p.Status,
		&p.RequestedAt, &p.RequestedByUserID, &p.ResolvedAt, &p.ResolvedByUserID, &p.Note,
	)
	return p, err
}

func (r *Repository) ListPayouts(ctx context.Context, orgID string) ([]Payout, error) {
	rows, err := r.db.Query(ctx, `SELECT `+payoutColumns+` FROM referral_payouts WHERE organization_id = $1 ORDER BY requested_at DESC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("referral: listing payouts: %w", err)
	}
	defer rows.Close()

	out := []Payout{}
	for rows.Next() {
		p, err := scanPayout(rows)
		if err != nil {
			return nil, fmt.Errorf("referral: scanning payout: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) CreatePayout(ctx context.Context, orgID, requestedByUserID string, amount float64) (Payout, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO referral_payouts (id, organization_id, amount, currency, requested_by_user_id)
		VALUES ($1, $2, $3, 'USD', $4)
		RETURNING `+payoutColumns,
		idgen.New(), orgID, amount, requestedByUserID,
	)
	return scanPayout(row)
}

func (r *Repository) ApprovePayout(ctx context.Context, orgID, id, resolvedByUserID string) (Payout, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE referral_payouts SET status = 'approved', resolved_at = now(), resolved_by_user_id = $3
		WHERE id = $1 AND organization_id = $2 AND status = 'pending'
		RETURNING `+payoutColumns,
		id, orgID, resolvedByUserID,
	)
	p, err := scanPayout(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Payout{}, apierror.Conflict("payout is not pending")
	}
	return p, err
}

func (r *Repository) RejectPayout(ctx context.Context, orgID, id, resolvedByUserID, note string) (Payout, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE referral_payouts SET status = 'rejected', resolved_at = now(), resolved_by_user_id = $3, note = $4
		WHERE id = $1 AND organization_id = $2 AND status = 'pending'
		RETURNING `+payoutColumns,
		id, orgID, resolvedByUserID, note,
	)
	p, err := scanPayout(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Payout{}, apierror.Conflict("payout is not pending")
	}
	return p, err
}

// MarkPayoutPaid updates the payout AND inserts the payout_paid ledger
// transaction in one DB transaction — the only place a transaction is
// ever auto-generated from a payout event, matching the mock exactly
// (including the literal "Payout paid — bank transfer" description; no
// real payment processor is called anywhere in this codebase).
func (r *Repository) MarkPayoutPaid(ctx context.Context, orgID, id, actingUserID string) (Payout, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Payout{}, fmt.Errorf("referral: beginning mark-paid tx: %w", err)
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		UPDATE referral_payouts SET status = 'paid'
		WHERE id = $1 AND organization_id = $2 AND status = 'approved'
		RETURNING `+payoutColumns,
		id, orgID,
	)
	payout, err := scanPayout(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Payout{}, apierror.Conflict("payout is not approved")
	}
	if err != nil {
		return Payout{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO referral_transactions (id, organization_id, type, amount, currency, description, related_payout_id, created_by_user_id)
		VALUES ($1, $2, 'payout_paid', $3, 'USD', $4, $5, $6)`,
		idgen.New(), orgID, -payout.Amount, "Payout paid — bank transfer", payout.ID, actingUserID,
	); err != nil {
		return Payout{}, fmt.Errorf("referral: recording payout-paid transaction: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Payout{}, fmt.Errorf("referral: committing mark-paid: %w", err)
	}
	return payout, nil
}
