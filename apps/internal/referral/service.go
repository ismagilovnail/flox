package referral

import (
	"context"
	"errors"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

const (
	descriptionMaxLen = 300
	noteMaxLen        = 300
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// Dashboard hydrates the whole page in one call — code/link are ensured
// (lazily created) rather than merely read, so a brand-new org's first
// visit to the page already has a real, shareable link.
func (s *Service) Dashboard(ctx context.Context, orgID string) (Dashboard, error) {
	code, err := s.repo.EnsureCode(ctx, orgID)
	if err != nil {
		return Dashboard{}, err
	}
	signups, err := s.repo.ListSignups(ctx, orgID)
	if err != nil {
		return Dashboard{}, err
	}
	transactions, err := s.repo.ListTransactions(ctx, orgID)
	if err != nil {
		return Dashboard{}, err
	}
	payouts, err := s.repo.ListPayouts(ctx, orgID)
	if err != nil {
		return Dashboard{}, err
	}
	balance, err := s.repo.ComputeBalance(ctx, orgID)
	if err != nil {
		return Dashboard{}, err
	}
	return Dashboard{
		Code:         code,
		Link:         "https://flox.io/r/" + code,
		Balance:      balance,
		Signups:      signups,
		Transactions: transactions,
		Payouts:      payouts,
	}, nil
}

// RecordSignup implements auth.ReferralRecorder — called best-effort from
// auth.Service.Signup after a brand-new org is created. An unknown/invalid
// code is silently treated as "no referrer," never an error: the caller
// (a person signing up) typed or pasted whatever string they had, and a
// typo here must never affect their own signup.
func (s *Service) RecordSignup(ctx context.Context, code, referredOrgID, name, email string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil
	}
	referrerOrgID, err := s.repo.ResolveCode(ctx, code)
	if err != nil {
		if errors.Is(err, ErrCodeNotFound) {
			return nil
		}
		return err
	}
	if referrerOrgID == referredOrgID {
		// Structurally shouldn't happen (a fresh org has no code of its
		// own yet at the moment it signs up), but costs nothing to guard.
		return nil
	}
	_, err = s.repo.CreateSignup(ctx, referrerOrgID, referredOrgID, name, email)
	return err
}

func (s *Service) RequestPayout(ctx context.Context, orgID, requestedByUserID string, amount float64) (Payout, error) {
	if amount <= 0 {
		return Payout{}, apierror.Validation("amount must be greater than zero", map[string]string{"amount": "invalid"})
	}
	balance, err := s.repo.ComputeBalance(ctx, orgID)
	if err != nil {
		return Payout{}, err
	}
	if amount > balance.AvailableBalance {
		return Payout{}, apierror.Validation("amount exceeds available balance", map[string]string{"amount": "exceeds available balance"})
	}
	return s.repo.CreatePayout(ctx, orgID, requestedByUserID, amount)
}

func (s *Service) ApprovePayout(ctx context.Context, orgID, id, resolvedByUserID string) (Payout, error) {
	return s.repo.ApprovePayout(ctx, orgID, id, resolvedByUserID)
}

func (s *Service) RejectPayout(ctx context.Context, orgID, id, resolvedByUserID, note string) (Payout, error) {
	note = strings.TrimSpace(note)
	if note == "" {
		return Payout{}, apierror.Validation("a reason is required", map[string]string{"note": "required"})
	}
	if len(note) > noteMaxLen {
		return Payout{}, apierror.Validation("reason is too long", map[string]string{"note": "too long"})
	}
	return s.repo.RejectPayout(ctx, orgID, id, resolvedByUserID, note)
}

func (s *Service) MarkPayoutPaid(ctx context.Context, orgID, id, actingUserID string) (Payout, error) {
	return s.repo.MarkPayoutPaid(ctx, orgID, id, actingUserID)
}

func (s *Service) AddTransaction(ctx context.Context, orgID, createdByUserID string, in AddTransactionInput) (Transaction, error) {
	if !in.Type.mutable() {
		return Transaction{}, apierror.Validation("type must be accrual or adjustment", map[string]string{"type": "invalid"})
	}
	if in.Amount == 0 {
		return Transaction{}, apierror.Validation("amount must not be zero", map[string]string{"amount": "required"})
	}
	in.Description = strings.TrimSpace(in.Description)
	if in.Description == "" {
		return Transaction{}, apierror.Validation("a description is required", map[string]string{"description": "required"})
	}
	if len(in.Description) > descriptionMaxLen {
		return Transaction{}, apierror.Validation("description is too long", map[string]string{"description": "too long"})
	}
	if in.RelatedSignupID != nil {
		ok, err := s.repo.SignupExists(ctx, orgID, *in.RelatedSignupID)
		if err != nil {
			return Transaction{}, err
		}
		if !ok {
			return Transaction{}, apierror.Validation("unknown signup", map[string]string{"relatedSignupId": *in.RelatedSignupID})
		}
	}
	return s.repo.CreateTransaction(ctx, orgID, createdByUserID, in)
}
