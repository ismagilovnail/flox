package billing_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/billing"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// carry is d of a plan at oldPrice in days of one at newPrice.
func carry(d time.Duration, oldPrice, newPrice float64) time.Duration {
	return time.Duration(float64(d) * oldPrice / newPrice).Truncate(time.Second)
}

func TestExtend(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	start := billing.Plan{Code: "start", PriceRub: ptr(int64(490000))}
	pro := billing.Plan{Code: "pro", PriceRub: ptr(int64(1490000))}
	pay := func(code string, months int, monthly int64) billing.Payment {
		return billing.Payment{PlanCode: code, Months: months, Amount: monthly * int64(months)}
	}
	in := func(d time.Duration) *time.Time { t := now.Add(d); return &t }

	for _, c := range []struct {
		name string
		sub  billing.Subscription
		p    billing.Payment
		want time.Time
	}{
		{"from Free", billing.Subscription{Plan: billing.Plan{Code: "free"}}, pay("start", 1, 490000), now.AddDate(0, 1, 0)},
		{"same plan, paid: from its end", billing.Subscription{Plan: start, PaidUntil: in(10 * 24 * time.Hour)}, pay("start", 3, 490000),
			now.Add(10*24*time.Hour).AddDate(0, 3, 0)},
		{"same plan, in grace: from now", billing.Subscription{Plan: start, PaidUntil: in(-24 * time.Hour)}, pay("start", 1, 490000), now.AddDate(0, 1, 0)},
		{"same plan, open-ended: from now", billing.Subscription{Plan: start}, pay("start", 12, 490000), now.AddDate(0, 12, 0)},
		// 30 days of 4 900 ₽ carried to 14 900 ₽: 30 × 4900/14900 ≈ 9.866 days.
		{"upgrade carries the unused days", billing.Subscription{Plan: start, PaidUntil: in(30 * 24 * time.Hour)}, pay("pro", 1, 1490000),
			now.AddDate(0, 1, 0).Add(carry(30*24*time.Hour, 4900, 14900))},
		{"downgrade carries more days", billing.Subscription{Plan: pro, PaidUntil: in(10 * 24 * time.Hour)}, pay("start", 1, 490000),
			now.AddDate(0, 1, 0).Add(carry(10*24*time.Hour, 14900, 4900))},
		{"switch from a plan not sold online: nothing carried", billing.Subscription{Plan: billing.Plan{Code: "custom"}, PaidUntil: in(5 * 24 * time.Hour)},
			pay("pro", 1, 1490000), now.AddDate(0, 1, 0)},
	} {
		if got := billing.Extend(c.sub, c.p, now); !got.Equal(c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// fakeProvider is YooKassa as the tests need it: one payment per
// idempotence key, statuses set by the test.
type fakeProvider struct {
	mu        sync.Mutex
	payments  map[string]billing.ProviderPayment
	requests  []billing.PaymentRequest
	createErr error
	getErr    error
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{payments: map[string]billing.ProviderPayment{}}
}

func (f *fakeProvider) Name() string { return "yookassa" }

func (f *fakeProvider) CreatePayment(_ context.Context, req billing.PaymentRequest) (billing.ProviderPayment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
	if f.createErr != nil {
		return billing.ProviderPayment{}, f.createErr
	}
	id := "yk-" + req.IdempotenceKey
	p, ok := f.payments[id]
	if !ok {
		p = billing.ProviderPayment{ID: id, Status: billing.PaymentPending, Amount: req.Amount, Currency: req.Currency,
			ConfirmationURL: "https://yoomoney.example/checkout/" + id}
		f.payments[id] = p
	}
	return p, nil
}

func (f *fakeProvider) GetPayment(_ context.Context, id string) (billing.ProviderPayment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return billing.ProviderPayment{}, f.getErr
	}
	p, ok := f.payments[id]
	if !ok {
		return billing.ProviderPayment{}, errors.New("404 not found")
	}
	return p, nil
}

func (f *fakeProvider) set(id, status string, mutate ...func(*billing.ProviderPayment)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.payments[id]
	p.Status = status
	for _, m := range mutate {
		m(&p)
	}
	f.payments[id] = p
}

const (
	ownerRole = "3JDXP6ABBFEXQ6AWHG60R30C1G"
	adminRole = "3JDXP6ABB1CHPPJVHG60R30C1G"
)

// seedMember adds a user with role to orgID and returns the user id.
func seedMember(t *testing.T, pool *pgxpool.Pool, orgID, role string) string {
	t.Helper()
	ctx := context.Background()
	id := idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name) VALUES ($1, $2, 'Member')`, id, strings.ToLower(id)+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO memberships (id, organization_id, user_id, role_id, status) VALUES ($1, $2, $3, $4, 'active')`,
		idgen.New(), orgID, id, role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id) })
	return id
}

type payEnv struct {
	pool     *pgxpool.Pool
	repo     *billing.Repository
	provider *fakeProvider
	payments *billing.Payments
	now      time.Time
	org      string
	owner    string
}

func newPayEnv(t *testing.T) *payEnv {
	pool := mustPool(t)
	e := &payEnv{pool: pool, repo: billing.NewRepository(pool), provider: newFakeProvider(), now: time.Now().UTC().Truncate(time.Second)}
	e.payments = billing.NewPayments(e.repo, e.provider, "https://app.example/", billing.ReceiptConfig{}, quiet).
		WithClock(func() time.Time { return e.now })
	e.org = seedOrg(t, pool)
	e.owner = seedMember(t, pool, e.org, ownerRole)
	return e
}

func (e *payEnv) subscription(t *testing.T) billing.Subscription {
	t.Helper()
	s, err := e.repo.Subscription(context.Background(), e.org)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (e *payEnv) audits(t *testing.T, action string) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE organization_id = $1 AND action = $2`, e.org, action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCheckoutRules(t *testing.T) {
	e := newPayEnv(t)
	ctx := context.Background()
	admin := seedMember(t, e.pool, e.org, adminRole)

	for name, c := range map[string]struct {
		user, plan string
		months     int
		status     int
	}{
		"an admin":         {admin, "pro", 1, http.StatusForbidden},
		"a stranger":       {seedMember(t, e.pool, seedOrg(t, e.pool), ownerRole), "pro", 1, http.StatusForbidden},
		"two months":       {e.owner, "pro", 2, http.StatusUnprocessableEntity},
		"free is not sold": {e.owner, "free", 1, http.StatusUnprocessableEntity},
		"an unknown plan":  {e.owner, "platinum", 1, http.StatusUnprocessableEntity},
	} {
		if _, err := e.payments.Checkout(ctx, e.org, c.user, c.plan, c.months); !isStatus(err, c.status) {
			t.Errorf("%s: %v, want %d", name, err, c.status)
		}
	}
	if len(e.provider.requests) != 0 {
		t.Fatalf("refused checkouts reached the provider: %+v", e.provider.requests)
	}

	p, err := e.payments.Checkout(ctx, e.org, e.owner, "pro", 3)
	if err != nil {
		t.Fatal(err)
	}
	req := e.provider.requests[0]
	if p.Amount != 3*1490000 || p.Currency != "RUB" || p.Status != billing.PaymentPending || p.ConfirmationURL == "" ||
		req.IdempotenceKey != p.ID || req.Amount != p.Amount || req.Receipt != nil ||
		req.ReturnURL != "https://app.example/settings?tab=billing&payment="+p.ID || req.Metadata["organization_id"] != e.org {
		t.Errorf("payment %+v, request %+v", p, req)
	}

	// Receipts on: the owner's email and the configured codes.
	withReceipt := billing.NewPayments(e.repo, e.provider, "https://app.example", billing.ReceiptConfig{Enabled: true, VATCode: 1, TaxSystemCode: 6}, quiet)
	if _, err := withReceipt.Checkout(ctx, e.org, e.owner, "start", 1); err != nil {
		t.Fatal(err)
	}
	r := e.provider.requests[1].Receipt
	if r == nil || !strings.HasSuffix(r.Email, "@example.test") || r.VATCode != 1 || r.TaxSystemCode != 6 || !strings.Contains(r.Item, "Start") {
		t.Errorf("receipt = %+v", r)
	}

	// The provider down: 503 and the row is cancelled, not left pending.
	e.provider.createErr = billing.ErrProvider
	if _, err := e.payments.Checkout(ctx, e.org, e.owner, "pro", 1); !isStatus(err, http.StatusServiceUnavailable) {
		t.Errorf("provider down = %v, want 503", err)
	}
	list, _ := e.payments.List(ctx, e.org)
	if len(list) != 3 || list[0].Status != billing.PaymentCanceled || list[0].ConfirmationURL != "" {
		t.Errorf("payments = %+v, want the failed one cancelled first", list)
	}

	// No provider: unavailable.
	if _, err := billing.NewPayments(e.repo, nil, "", billing.ReceiptConfig{}, quiet).Checkout(ctx, e.org, e.owner, "pro", 1); !isStatus(err, http.StatusServiceUnavailable) {
		t.Errorf("without a provider = %v, want 503", err)
	}
}

func TestPaymentAppliesOnceFromTheProvidersWord(t *testing.T) {
	e := newPayEnv(t)
	ctx := context.Background()
	p, err := e.payments.Checkout(ctx, e.org, e.owner, "pro", 1)
	if err != nil {
		t.Fatal(err)
	}
	yk := "yk-" + p.ID

	// Still pending at the provider: a notification changes nothing.
	if err := e.payments.HandleNotification(ctx, yk); err != nil {
		t.Fatal(err)
	}
	if s := e.subscription(t); s.Plan.Code != billing.FreePlan {
		t.Fatalf("plan after a pending notification = %s", s.Plan.Code)
	}

	e.provider.set(yk, billing.PaymentSucceeded)
	for i := 0; i < 3; i++ { // the notification, its retry, the worker's read-back
		if err := e.payments.HandleNotification(ctx, yk); err != nil {
			t.Fatal(err)
		}
	}
	s := e.subscription(t)
	want := e.now.AddDate(0, 1, 0)
	if s.Plan.Code != "pro" || s.PaidUntil == nil || !s.PaidUntil.Equal(want) {
		t.Fatalf("subscription = %+v, want pro until %v", s, want)
	}
	if n := e.audits(t, "billing.payment_applied"); n != 1 {
		t.Errorf("payment applied %d times", n)
	}
	got, err := e.payments.Get(ctx, e.org, p.ID)
	if err != nil || got.Status != billing.PaymentSucceeded || got.AppliedAt == nil || got.ConfirmationURL != "" {
		t.Errorf("payment = %+v, %v", got, err)
	}

	// A second month of the same plan: from the end of the first.
	p2, _ := e.payments.Checkout(ctx, e.org, e.owner, "pro", 3)
	e.provider.set("yk-"+p2.ID, billing.PaymentSucceeded)
	if _, err := e.payments.Get(ctx, e.org, p2.ID); err != nil { // the payer's return reads it back
		t.Fatal(err)
	}
	if s := e.subscription(t); !s.PaidUntil.Equal(want.AddDate(0, 3, 0)) {
		t.Errorf("after renewing = %v, want %v", s.PaidUntil, want.AddDate(0, 3, 0))
	}

	// Unknown payment: ignored, nil (the sender stops retrying).
	if err := e.payments.HandleNotification(ctx, "yk-not-ours"); err != nil {
		t.Errorf("unknown payment = %v", err)
	}
	// The provider down: an error, so the sender retries.
	e.provider.getErr = billing.ErrProvider
	if err := e.payments.HandleNotification(ctx, yk); !errors.Is(err, billing.ErrProvider) {
		t.Errorf("provider down = %v", err)
	}
}

func TestPaymentNotAppliedOnAMismatchOrCancel(t *testing.T) {
	e := newPayEnv(t)
	ctx := context.Background()

	p, _ := e.payments.Checkout(ctx, e.org, e.owner, "business", 1)
	e.provider.set("yk-"+p.ID, billing.PaymentSucceeded, func(pp *billing.ProviderPayment) { pp.Amount = 100 })
	if err := e.payments.HandleNotification(ctx, "yk-"+p.ID); err == nil {
		t.Error("a payment for another amount applied")
	}
	if s := e.subscription(t); s.Plan.Code != billing.FreePlan {
		t.Errorf("plan = %s after a mismatched payment", s.Plan.Code)
	}

	c, _ := e.payments.Checkout(ctx, e.org, e.owner, "start", 1)
	e.provider.set("yk-"+c.ID, billing.PaymentCanceled, func(pp *billing.ProviderPayment) { pp.CancellationReason = "payment_network: insufficient_funds" })
	if err := e.payments.HandleNotification(ctx, "yk-"+c.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := e.payments.Get(ctx, e.org, c.ID)
	if got.Status != billing.PaymentCanceled || got.CancellationReason != "payment_network: insufficient_funds" || got.AppliedAt != nil {
		t.Errorf("cancelled payment = %+v", got)
	}
	// Another organization cannot read it.
	if _, err := e.payments.Get(ctx, seedOrg(t, e.pool), c.ID); !isStatus(err, http.StatusNotFound) {
		t.Errorf("another organization's payment = %v, want 404", err)
	}
}

func TestStatusShowsThePaidPeriod(t *testing.T) {
	e := newPayEnv(t)
	ctx := context.Background()
	p, _ := e.payments.Checkout(ctx, e.org, e.owner, "start", 1)
	e.provider.set("yk-"+p.ID, billing.PaymentSucceeded)
	if err := e.payments.HandleNotification(ctx, "yk-"+p.ID); err != nil {
		t.Fatal(err)
	}
	paidUntil := e.now.AddDate(0, 1, 0)
	for _, c := range []struct {
		at    time.Time
		renew bool
	}{
		{e.now, false},
		{paidUntil.Add(-billing.RemindBefore - time.Minute), false},
		{paidUntil.Add(-billing.RemindBefore), true},
		{paidUntil.Add(time.Hour), true}, // in grace, still on Start
	} {
		svc := billing.NewService(e.repo, nil, quiet).WithPayments(true).WithClock(func() time.Time { return c.at })
		st, err := svc.Status(ctx, e.org)
		if err != nil {
			t.Fatal(err)
		}
		if st.Plan.Code != "start" || st.PaidUntil == nil || !st.PaidUntil.Equal(paidUntil) ||
			st.GraceUntil == nil || !st.GraceUntil.Equal(paidUntil.Add(billing.GracePeriod)) || st.Renew != c.renew || !st.CanPay {
			t.Errorf("at %v: %+v, want renew %v", c.at, st, c.renew)
		}
	}
}

type sentMail struct {
	mu   sync.Mutex
	sent []string
}

func (m *sentMail) Send(_ context.Context, to, subject, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, to+"|"+subject)
	return nil
}

func TestRunnerRemindsAndExpires(t *testing.T) {
	e := newPayEnv(t)
	ctx := context.Background()
	p, _ := e.payments.Checkout(ctx, e.org, e.owner, "pro", 1)
	e.provider.set("yk-"+p.ID, billing.PaymentSucceeded)
	paidUntil := e.now.AddDate(0, 1, 0)

	// A pending payment whose notification was lost: the runner reads it
	// back once it is a few minutes old.
	mail := &sentMail{}
	at := e.now.Add(10 * time.Minute)
	runner := &billing.Runner{Repo: e.repo, Only: e.org, Payments: e.payments, Mail: mail, AppURL: "https://app.example", Logger: quiet,
		Now: func() time.Time { return at }}
	if err := runner.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if s := e.subscription(t); s.Plan.Code != "pro" {
		t.Fatalf("runner did not apply the lost payment: %+v", s)
	}
	if len(mail.sent) != 0 {
		t.Fatalf("emails right after paying: %v", mail.sent)
	}

	// Inside the reminder window: one email, once.
	at = paidUntil.Add(-24 * time.Hour)
	for i := 0; i < 2; i++ {
		if err := runner.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if len(mail.sent) != 1 || !strings.Contains(mail.sent[0], "продлите") {
		t.Fatalf("reminders = %v, want one", mail.sent)
	}

	// In grace: still Pro.
	at = paidUntil.Add(billing.GracePeriod - time.Minute)
	if err := runner.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if s := e.subscription(t); s.Plan.Code != "pro" {
		t.Fatalf("plan in grace = %s", s.Plan.Code)
	}

	// Grace over: Free, audited, owners told — once.
	at = paidUntil.Add(billing.GracePeriod + time.Minute)
	for i := 0; i < 2; i++ {
		if err := runner.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	s := e.subscription(t)
	if s.Plan.Code != billing.FreePlan || s.PaidUntil != nil {
		t.Errorf("after grace = %+v, want open-ended Free", s)
	}
	if n := e.audits(t, "billing.plan_expired"); n != 1 {
		t.Errorf("expiry audited %d times", n)
	}
	if len(mail.sent) != 2 || !strings.Contains(mail.sent[1], "закончился") {
		t.Errorf("emails = %v, want the reminder and the expiry notice", mail.sent)
	}
}

func TestRunnerCancelsAbandonedPayments(t *testing.T) {
	e := newPayEnv(t)
	ctx := context.Background()
	p, _ := e.payments.Checkout(ctx, e.org, e.owner, "start", 1)
	runner := &billing.Runner{Repo: e.repo, Only: e.org, Payments: e.payments, Logger: quiet, Now: func() time.Time { return e.now.Add(49 * time.Hour) }}
	if err := runner.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := e.repo.Payment(ctx, e.org, p.ID)
	if got.Status != billing.PaymentCanceled || got.CancellationReason != "expired" {
		t.Fatalf("abandoned payment = %+v", got)
	}
	// Paid after all (a late notification): the money is in, the plan applies.
	e.provider.set("yk-"+p.ID, billing.PaymentSucceeded)
	if err := e.payments.HandleNotification(ctx, "yk-"+p.ID); err != nil {
		t.Fatal(err)
	}
	if s := e.subscription(t); s.Plan.Code != "start" {
		t.Errorf("late payment not applied: %+v", s)
	}
}
