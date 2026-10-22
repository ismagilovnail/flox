package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/billing"
)

// fakeYooKassa is YooKassa's API v3 as FLOX uses it: payments created by
// idempotence key, read back by id, statuses set by the test.
type fakeYooKassa struct {
	mu     sync.Mutex
	status map[string]string // payment id -> status
	amount map[string]string
	gets   int
}

func newFakeYooKassa(t *testing.T) (*fakeYooKassa, string) {
	f := &fakeYooKassa{status: map[string]string{}, amount: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, _ := r.BasicAuth(); user != "test-shop" || pass != "test_key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		var id string
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/payments":
			var body struct {
				Amount struct{ Value string } `json:"amount"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			id = "yk-" + r.Header.Get("Idempotence-Key")
			if _, ok := f.status[id]; !ok {
				f.status[id], f.amount[id] = "pending", body.Amount.Value
			}
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/payments/"):
			f.gets++
			id = strings.TrimPrefix(r.URL.Path, "/payments/")
			if _, ok := f.status[id]; !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = fmt.Fprintf(w, `{"id":%q,"status":%q,"amount":{"value":%q,"currency":"RUB"},
			"confirmation":{"type":"redirect","confirmation_url":"https://yoomoney.example/checkout/%s"}}`,
			id, f.status[id], f.amount[id], id)
	}))
	t.Cleanup(srv.Close)
	return f, srv.URL
}

func (f *fakeYooKassa) set(id, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status[id] = status
}

// BIZ-1 part 2: an owner pays through the real router; YooKassa's
// notification body is never believed — the plan changes only once
// YooKassa itself says the payment succeeded.
func TestNotificationBodyIsNotTrusted(t *testing.T) {
	yk, url := newFakeYooKassa(t)
	t.Setenv("YOOKASSA_SHOP_ID", "test-shop")
	t.Setenv("YOOKASSA_SECRET_KEY", "test_key")
	t.Setenv("YOOKASSA_API_URL", url)
	env := newIsolationEnv(t)
	c := env.c
	c.signIn(env.orgA)

	code, body := c.do(http.MethodGet, "/billing")
	if code != http.StatusOK || !strings.Contains(body, `"canPay":true`) {
		t.Fatalf("GET /billing = %d %s, want canPay", code, body)
	}
	code, body, _ = c.request(http.MethodPost, "/billing/checkout", map[string]any{"plan": "pro", "months": 1})
	if code != http.StatusCreated {
		t.Fatalf("checkout = %d %s", code, body)
	}
	var p billing.Payment
	if err := json.Unmarshal([]byte(body), &p); err != nil || p.ConfirmationURL == "" || p.Amount != 1490000 {
		t.Fatalf("checkout answer %s: %v", body, err)
	}
	ykID := "yk-" + p.ID

	notify := func(raw string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/v1/payment-notifications/yookassa", strings.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "[" + randomTestIP() + "]:443"
		rec := httptest.NewRecorder()
		env.srv.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	plan := func() string {
		t.Helper()
		var code string
		if err := env.pool.QueryRow(env.ctx, `SELECT plan_code FROM organization_plans WHERE organization_id = $1 AND ended_at IS NULL`, env.orgA).Scan(&code); err != nil {
			return billing.FreePlan
		}
		return code
	}

	// A forged "succeeded" while YooKassa says pending: nothing changes.
	forged := fmt.Sprintf(`{"type":"notification","event":"payment.succeeded","object":{"id":%q,"status":"succeeded","paid":true,
		"amount":{"value":"14900.00","currency":"RUB"}}}`, ykID)
	if status := notify(forged); status != http.StatusOK {
		t.Fatalf("notification = %d", status)
	}
	if got := plan(); got != billing.FreePlan {
		t.Fatalf("plan after a forged notification = %s, want free", got)
	}
	if yk.gets == 0 {
		t.Fatal("the notification was not read back from YooKassa")
	}

	// YooKassa says succeeded: the same notification now applies, once.
	yk.set(ykID, "succeeded")
	for i := 0; i < 2; i++ {
		if status := notify(forged); status != http.StatusOK {
			t.Fatalf("notification = %d", status)
		}
	}
	if got := plan(); got != "pro" {
		t.Fatalf("plan after a real payment = %s, want pro", got)
	}
	code, body = c.do(http.MethodGet, "/billing/payments/"+p.ID)
	if code != http.StatusOK || !strings.Contains(body, `"status":"succeeded"`) {
		t.Errorf("payment = %d %s", code, body)
	}

	// Not ours, a refund event, or no id at all.
	if status := notify(`{"event":"payment.succeeded","object":{"id":"someone-elses"}}`); status != http.StatusOK {
		t.Errorf("unknown payment = %d, want 200", status)
	}
	if status := notify(`{"event":"refund.succeeded","object":{"id":"r-1","payment_id":"` + ykID + `"}}`); status != http.StatusOK {
		t.Errorf("refund event = %d, want 200", status)
	}
	if status := notify(`not json`); status != http.StatusUnprocessableEntity {
		t.Errorf("garbage = %d, want 422", status)
	}

	// Organization B neither sees A's payment nor its history.
	c.signIn(env.orgB)
	if code, _ := c.do(http.MethodGet, "/billing/payments/"+p.ID); code != http.StatusNotFound {
		t.Errorf("B reading A's payment = %d, want 404", code)
	}
	code, body = c.do(http.MethodGet, "/billing/payments")
	if code != http.StatusOK || strings.Contains(body, p.ID) {
		t.Errorf("B's payments = %d %s", code, body)
	}
}
