package yookassa_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/billing"
	"github.com/ismagilovnail/flox/apps/internal/billing/yookassa"
)

type seen struct {
	method, path, user, pass, key string
	body                          map[string]any
}

func server(t *testing.T, status int, answer string, got *seen) *yookassa.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path = r.Method, r.URL.Path
		got.user, got.pass, _ = r.BasicAuth()
		got.key = r.Header.Get("Idempotence-Key")
		raw, _ := io.ReadAll(r.Body)
		got.body = nil
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &got.body)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, answer)
	}))
	t.Cleanup(srv.Close)
	return yookassa.New(srv.URL, "shop-1", "test_secret")
}

const pendingAnswer = `{"id":"2f2b1c8e-000f-5000-8000-1a2b3c4d5e6f","status":"pending","paid":false,
	"amount":{"value":"14900.00","currency":"RUB"},
	"confirmation":{"type":"redirect","confirmation_url":"https://yoomoney.ru/checkout/payments/v2/contract?orderId=2f2b"},
	"metadata":{"payment_id":"P1"}}`

func TestCreatePayment(t *testing.T) {
	var got seen
	c := server(t, http.StatusOK, pendingAnswer, &got)
	p, err := c.CreatePayment(context.Background(), billing.PaymentRequest{
		IdempotenceKey: "P1", Amount: 1490000, Currency: "RUB", Description: "FLOX Pro, 1 мес.",
		ReturnURL: "https://app.example/settings?tab=billing&payment=P1", Metadata: map[string]string{"payment_id": "P1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.method != http.MethodPost || got.path != "/payments" || got.user != "shop-1" || got.pass != "test_secret" || got.key != "P1" {
		t.Errorf("request = %s %s as %s:%s key %q", got.method, got.path, got.user, got.pass, got.key)
	}
	amount, _ := got.body["amount"].(map[string]any)
	conf, _ := got.body["confirmation"].(map[string]any)
	if amount["value"] != "14900.00" || amount["currency"] != "RUB" || got.body["capture"] != true ||
		conf["type"] != "redirect" || conf["return_url"] != "https://app.example/settings?tab=billing&payment=P1" {
		t.Errorf("body = %v", got.body)
	}
	if _, ok := got.body["receipt"]; ok {
		t.Error("a receipt was sent without one asked for")
	}
	if p.ID != "2f2b1c8e-000f-5000-8000-1a2b3c4d5e6f" || p.Status != billing.PaymentPending || p.Amount != 1490000 ||
		p.Currency != "RUB" || p.ConfirmationURL == "" {
		t.Errorf("payment = %+v", p)
	}
}

func TestCreatePaymentWithReceipt(t *testing.T) {
	var got seen
	c := server(t, http.StatusOK, pendingAnswer, &got)
	_, err := c.CreatePayment(context.Background(), billing.PaymentRequest{
		IdempotenceKey: "P1", Amount: 490005, Currency: "RUB", Description: "FLOX Start, 1 мес.", ReturnURL: "https://app.example/",
		Receipt: &billing.Receipt{Email: "owner@example.com", Item: "Подписка FLOX Start, 1 мес.", VATCode: 1, TaxSystemCode: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	r, _ := got.body["receipt"].(map[string]any)
	customer, _ := r["customer"].(map[string]any)
	items, _ := r["items"].([]any)
	if customer["email"] != "owner@example.com" || r["tax_system_code"] != float64(2) || len(items) != 1 {
		t.Fatalf("receipt = %v", r)
	}
	item := items[0].(map[string]any)
	amount := item["amount"].(map[string]any)
	if amount["value"] != "4900.05" || item["vat_code"] != float64(1) || item["quantity"] != "1.00" ||
		item["payment_subject"] != "service" || item["payment_mode"] != "full_payment" {
		t.Errorf("receipt item = %v", item)
	}
}

func TestGetPaymentStatuses(t *testing.T) {
	for answer, want := range map[string]billing.ProviderPayment{
		`{"id":"a","status":"succeeded","amount":{"value":"4900.00","currency":"RUB"}}`:                                                                                {ID: "a", Status: billing.PaymentSucceeded, Amount: 490000, Currency: "RUB"},
		`{"id":"a","status":"waiting_for_capture","amount":{"value":"4900","currency":"RUB"}}`:                                                                         {ID: "a", Status: billing.PaymentPending, Amount: 490000, Currency: "RUB"},
		`{"id":"a","status":"canceled","amount":{"value":"4900.5","currency":"RUB"},"cancellation_details":{"party":"payment_network","reason":"insufficient_funds"}}`: {ID: "a", Status: billing.PaymentCanceled, Amount: 490050, Currency: "RUB", CancellationReason: "payment_network: insufficient_funds"},
	} {
		var got seen
		p, err := server(t, http.StatusOK, answer, &got).GetPayment(context.Background(), "a")
		if err != nil {
			t.Fatal(err)
		}
		if got.method != http.MethodGet || got.path != "/payments/a" || got.key != "" {
			t.Errorf("request = %s %s key %q", got.method, got.path, got.key)
		}
		if p.ID != want.ID || p.Status != want.Status || p.Amount != want.Amount || p.CancellationReason != want.CancellationReason {
			t.Errorf("%s = %+v, want %+v", answer, p, want)
		}
	}
	var got seen
	if _, err := server(t, http.StatusOK, `{}`, &got).GetPayment(context.Background(), "../refunds"); err == nil || got.path != "" {
		t.Errorf("a path in the id = %v, reached %q", err, got.path)
	}
}

func TestErrors(t *testing.T) {
	var got seen
	_, err := server(t, http.StatusBadRequest, `{"type":"error","code":"invalid_request","description":"Bad amount","parameter":"amount.value"}`, &got).
		CreatePayment(context.Background(), billing.PaymentRequest{Amount: 1, Currency: "RUB"})
	if err == nil || errors.Is(err, billing.ErrProvider) {
		t.Errorf("400 = %v, want an error that is not a provider outage", err)
	}
	for _, status := range []int{http.StatusInternalServerError, http.StatusTooManyRequests} {
		_, err := server(t, status, `{}`, &got).GetPayment(context.Background(), "a")
		if !errors.Is(err, billing.ErrProvider) {
			t.Errorf("%d = %v, want ErrProvider", status, err)
		}
	}
	unreachable := yookassa.New("http://127.0.0.1:1", "s", "k")
	if _, err := unreachable.GetPayment(context.Background(), "a"); !errors.Is(err, billing.ErrProvider) {
		t.Errorf("unreachable = %v, want ErrProvider", err)
	}
	if _, err := server(t, http.StatusOK, `{"id":"a","status":"succeeded","amount":{"value":"1.234","currency":"RUB"}}`, &got).
		GetPayment(context.Background(), "a"); err == nil {
		t.Error("an amount with three decimals was accepted")
	}
}
