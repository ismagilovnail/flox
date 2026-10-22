package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// ReceiptConfig: whether a 54-FZ receipt goes with each payment, and its
// codes (config.YooKassaConfig).
type ReceiptConfig struct {
	Enabled       bool
	VATCode       int
	TaxSystemCode int
}

// Payments sells plans: a prepaid number of months, paid on the
// provider's page. The provider's word is the only one that applies a
// payment — a notification is a hint to read it back (HandleNotification).
type Payments struct {
	repo     *Repository
	provider PaymentProvider // nil: plans are not sold online
	appURL   string
	receipt  ReceiptConfig
	now      func() time.Time
	logger   *slog.Logger
}

func NewPayments(repo *Repository, provider PaymentProvider, appURL string, receipt ReceiptConfig, logger *slog.Logger) *Payments {
	return &Payments{repo: repo, provider: provider, appURL: strings.TrimRight(appURL, "/"), receipt: receipt, now: time.Now, logger: logger}
}

// WithClock replaces time.Now, for tests.
func (s *Payments) WithClock(now func() time.Time) *Payments {
	s.now = now
	return s
}

// Available reports whether plans can be paid online.
func (s *Payments) Available() bool { return s != nil && s.provider != nil }

// Checkout starts paying for months of plan: an Owner only. The payment
// row is written first and its id is the provider's idempotence key, so a
// retried create cannot charge twice.
func (s *Payments) Checkout(ctx context.Context, orgID, userID, planCode string, months int) (Payment, error) {
	if !s.Available() {
		return Payment{}, apierror.Unavailable("online payment is not set up on this server")
	}
	email, owner, err := s.repo.OwnerEmail(ctx, orgID, userID)
	if err != nil {
		return Payment{}, err
	}
	if !owner {
		return Payment{}, apierror.Forbidden("only an owner of the organization can pay for its plan")
	}
	if !PayableMonths[months] {
		return Payment{}, apierror.Validation("pay for 1, 3 or 12 months", map[string]string{"months": "want 1, 3 or 12"})
	}
	plan, err := s.repo.Plan(ctx, planCode)
	if err != nil {
		return Payment{}, err
	}
	if plan.PriceRub == nil {
		return Payment{}, apierror.Validation("this plan is not sold online", map[string]string{"plan": planCode})
	}

	p, err := s.repo.CreatePayment(ctx, Payment{
		ID: idgen.New(), OrganizationID: orgID, PlanCode: plan.Code, Months: months,
		Amount: *plan.PriceRub * int64(months), Currency: "RUB",
	}, userID)
	if err != nil {
		return Payment{}, fmt.Errorf("billing: recording the checkout: %w", err)
	}
	desc := fmt.Sprintf("FLOX %s, %d мес.", plan.Name, months)
	req := PaymentRequest{
		IdempotenceKey: p.ID, Amount: p.Amount, Currency: p.Currency, Description: desc,
		ReturnURL: s.appURL + "/settings?tab=billing&payment=" + p.ID,
		Metadata:  map[string]string{"payment_id": p.ID, "organization_id": orgID},
	}
	if s.receipt.Enabled {
		req.Receipt = &Receipt{Email: email, Item: "Подписка " + desc, VATCode: s.receipt.VATCode, TaxSystemCode: s.receipt.TaxSystemCode}
	}
	pp, err := s.provider.CreatePayment(ctx, req)
	if err != nil {
		s.logger.Error("billing: creating the provider payment", "payment_id", p.ID, "error", err)
		if _, cerr := s.repo.ApplyStatus(ctx, p.ID, ProviderPayment{Status: PaymentCanceled, CancellationReason: "not created"}, s.now(), Extend); cerr != nil {
			s.logger.Error("billing: cancelling a failed checkout", "payment_id", p.ID, "error", cerr)
		}
		if errors.Is(err, ErrProvider) {
			return Payment{}, apierror.Unavailable("the payment service is not answering; try again in a few minutes")
		}
		return Payment{}, err
	}
	if err := s.repo.AttachProvider(ctx, p.ID, s.provider.Name(), pp.ID, pp.ConfirmationURL); err != nil {
		return Payment{}, err
	}
	p.ProviderPaymentID, p.ConfirmationURL = pp.ID, pp.ConfirmationURL
	return p, nil
}

// Get is one of the organization's payments; a pending one is read back
// from the provider first, so the page the payer returns to is current.
func (s *Payments) Get(ctx context.Context, orgID, id string) (Payment, error) {
	p, err := s.repo.Payment(ctx, orgID, id)
	if err != nil {
		return Payment{}, err
	}
	if p.Status == PaymentPending && p.ProviderPaymentID != "" && s.Available() {
		if err := s.Refresh(ctx, p); err != nil {
			s.logger.Warn("billing: refreshing a payment", "payment_id", p.ID, "error", err)
		} else if p, err = s.repo.Payment(ctx, orgID, id); err != nil {
			return Payment{}, err
		}
	}
	// Only a payment still payable keeps its page ("Continue payment").
	if p.Status != PaymentPending {
		p.ConfirmationURL = ""
	}
	return p, nil
}

func (s *Payments) List(ctx context.Context, orgID string) ([]Payment, error) {
	return s.repo.Payments(ctx, orgID, 50)
}

// HandleNotification reads providerPaymentID back from the provider and
// applies what it says. A payment that is not ours is ignored (nil): the
// sender gets its 200 and stops retrying.
func (s *Payments) HandleNotification(ctx context.Context, providerPaymentID string) error {
	if !s.Available() {
		return apierror.Unavailable("online payment is not set up on this server")
	}
	p, ok, err := s.repo.PaymentByProviderID(ctx, s.provider.Name(), providerPaymentID)
	if err != nil {
		return err
	}
	if !ok {
		s.logger.Warn("billing: notification for an unknown payment", "provider_payment_id", providerPaymentID)
		return nil
	}
	return s.Refresh(ctx, p)
}

// Refresh reads p back from the provider and applies its status.
func (s *Payments) Refresh(ctx context.Context, p Payment) error {
	pp, err := s.provider.GetPayment(ctx, p.ProviderPaymentID)
	if err != nil {
		return err
	}
	if pp.ID != p.ProviderPaymentID {
		return fmt.Errorf("billing: asked for payment %s, provider answered %s", p.ProviderPaymentID, pp.ID)
	}
	applied, err := s.repo.ApplyStatus(ctx, p.ID, pp, s.now(), Extend)
	if err != nil {
		return err
	}
	if applied {
		s.logger.Info("billing: payment applied", "payment_id", p.ID, "organization_id", p.OrganizationID, "plan", p.PlanCode, "months", p.Months)
	}
	return nil
}

// Extend is the end of the paid period after p applies to sub.
//
// The same plan, still paid: the months are added to its end. Otherwise
// the months start now — and when the organization moves off a plan that
// is still paid, its unused time is carried over at the two plans'
// monthly prices: ten days left of a 4 900 ₽ plan become about 3.3 days
// of a 14 900 ₽ one. No money is refunded.
func Extend(sub Subscription, p Payment, now time.Time) time.Time {
	paid := sub.PaidUntil != nil && sub.PaidUntil.After(now)
	start := now
	if sub.Plan.Code == p.PlanCode && paid {
		start = *sub.PaidUntil
	}
	end := start.AddDate(0, p.Months, 0)
	if sub.Plan.Code != p.PlanCode && paid && sub.Plan.PriceRub != nil && p.Months > 0 && p.Amount > 0 {
		newMonthly := float64(p.Amount) / float64(p.Months)
		credit := time.Duration(float64(sub.PaidUntil.Sub(now)) * float64(*sub.Plan.PriceRub) / newMonthly)
		end = end.Add(credit.Truncate(time.Second))
	}
	return end
}
