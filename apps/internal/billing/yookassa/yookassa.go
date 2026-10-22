// Package yookassa is billing.PaymentProvider over the YooKassa API v3
// (https://yookassa.ru/developers/api): one-stage payments (capture:
// true) confirmed by a redirect to YooKassa's payment page.
//
// Requests are authenticated with HTTP Basic shopId:secretKey and carry an
// Idempotence-Key, so a retried create returns the same payment. YooKassa
// does not sign its notifications: internal/billing reads every payment
// back through GetPayment before believing it.
package yookassa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/billing"
	"github.com/ismagilovnail/flox/apps/internal/httpclient"
)

const DefaultAPIURL = "https://api.yookassa.ru/v3"

type Client struct {
	APIURL     string
	ShopID     string
	SecretKey  string
	HTTPClient *http.Client
}

func New(apiURL, shopID, secretKey string) *Client {
	if apiURL == "" {
		apiURL = DefaultAPIURL
	}
	return &Client{APIURL: strings.TrimRight(apiURL, "/"), ShopID: shopID, SecretKey: secretKey,
		HTTPClient: httpclient.New(httpclient.ControlAPI)}
}

func (c *Client) Name() string { return "yookassa" }

type amount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type receiptItem struct {
	Description    string `json:"description"`
	Quantity       string `json:"quantity"`
	Amount         amount `json:"amount"`
	VATCode        int    `json:"vat_code"`
	PaymentSubject string `json:"payment_subject"`
	PaymentMode    string `json:"payment_mode"`
}

type receipt struct {
	Customer struct {
		Email string `json:"email"`
	} `json:"customer"`
	Items         []receiptItem `json:"items"`
	TaxSystemCode int           `json:"tax_system_code,omitempty"`
}

type createRequest struct {
	Amount       amount `json:"amount"`
	Capture      bool   `json:"capture"`
	Confirmation struct {
		Type      string `json:"type"`
		ReturnURL string `json:"return_url"`
	} `json:"confirmation"`
	Description string            `json:"description"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Receipt     *receipt          `json:"receipt,omitempty"`
}

type payment struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	Amount       amount `json:"amount"`
	Confirmation struct {
		ConfirmationURL string `json:"confirmation_url"`
	} `json:"confirmation"`
	CancellationDetails struct {
		Party  string `json:"party"`
		Reason string `json:"reason"`
	} `json:"cancellation_details"`
	Metadata map[string]string `json:"metadata"`
}

type apiError struct {
	Code        string `json:"code"`
	Description string `json:"description"`
	Parameter   string `json:"parameter"`
}

func (c *Client) CreatePayment(ctx context.Context, req billing.PaymentRequest) (billing.ProviderPayment, error) {
	body := createRequest{
		Amount:      amount{Value: formatAmount(req.Amount), Currency: req.Currency},
		Capture:     true,
		Description: truncate(req.Description, 128),
		Metadata:    req.Metadata,
	}
	body.Confirmation.Type = "redirect"
	body.Confirmation.ReturnURL = req.ReturnURL
	if r := req.Receipt; r != nil {
		rc := &receipt{TaxSystemCode: r.TaxSystemCode, Items: []receiptItem{{
			Description: truncate(r.Item, 128), Quantity: "1.00",
			Amount:  amount{Value: formatAmount(req.Amount), Currency: req.Currency},
			VATCode: r.VATCode, PaymentSubject: "service", PaymentMode: "full_payment",
		}}}
		rc.Customer.Email = r.Email
		body.Receipt = rc
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return billing.ProviderPayment{}, fmt.Errorf("yookassa: %w", err)
	}
	return c.do(ctx, http.MethodPost, "/payments", req.IdempotenceKey, raw)
}

func (c *Client) GetPayment(ctx context.Context, id string) (billing.ProviderPayment, error) {
	if id == "" || strings.ContainsAny(id, "/?#") {
		return billing.ProviderPayment{}, fmt.Errorf("yookassa: invalid payment id %q", id)
	}
	return c.do(ctx, http.MethodGet, "/payments/"+id, "", nil)
}

func (c *Client) do(ctx context.Context, method, path, idempotenceKey string, body []byte) (billing.ProviderPayment, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.APIURL+path, bytes.NewReader(body))
	if err != nil {
		return billing.ProviderPayment{}, fmt.Errorf("yookassa: %w", err)
	}
	req.SetBasicAuth(c.ShopID, c.SecretKey)
	req.Header.Set("Content-Type", "application/json")
	if idempotenceKey != "" {
		req.Header.Set("Idempotence-Key", idempotenceKey)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return billing.ProviderPayment{}, fmt.Errorf("%w: yookassa: %v", billing.ErrProvider, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return billing.ProviderPayment{}, fmt.Errorf("%w: yookassa: reading the answer: %v", billing.ErrProvider, err)
	}
	if resp.StatusCode != http.StatusOK {
		var e apiError
		_ = json.Unmarshal(raw, &e)
		err := fmt.Errorf("yookassa: %s %s = %d %s: %s %s", method, path, resp.StatusCode, e.Code, e.Description, e.Parameter)
		// 5xx and 429 pass; anything else is this request's fault.
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
			return billing.ProviderPayment{}, fmt.Errorf("%w: %v", billing.ErrProvider, err)
		}
		return billing.ProviderPayment{}, err
	}
	var p payment
	if err := json.Unmarshal(raw, &p); err != nil {
		return billing.ProviderPayment{}, fmt.Errorf("yookassa: decoding the payment: %w", err)
	}
	minor, err := parseAmount(p.Amount.Value)
	if err != nil {
		return billing.ProviderPayment{}, fmt.Errorf("yookassa: payment %s: %w", p.ID, err)
	}
	status := billing.PaymentPending // pending, waiting_for_capture
	switch p.Status {
	case "succeeded":
		status = billing.PaymentSucceeded
	case "canceled":
		status = billing.PaymentCanceled
	}
	reason := p.CancellationDetails.Reason
	if reason != "" && p.CancellationDetails.Party != "" {
		reason = p.CancellationDetails.Party + ": " + reason
	}
	return billing.ProviderPayment{
		ID: p.ID, Status: status, Amount: minor, Currency: p.Amount.Currency,
		ConfirmationURL: p.Confirmation.ConfirmationURL, CancellationReason: reason, Metadata: p.Metadata,
	}, nil
}

// formatAmount is minor units as YooKassa's "4900.00".
func formatAmount(minor int64) string {
	return fmt.Sprintf("%d.%02d", minor/100, minor%100)
}

// parseAmount is YooKassa's "4900.00" (or "4900", "4900.5") in minor units.
func parseAmount(v string) (int64, error) {
	whole, frac, _ := strings.Cut(v, ".")
	if len(frac) > 2 {
		return 0, fmt.Errorf("amount %q has more than two decimals", v)
	}
	frac += strings.Repeat("0", 2-len(frac))
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("amount %q: %w", v, err)
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil || w < 0 {
		return 0, fmt.Errorf("amount %q is not a positive decimal", v)
	}
	return w*100 + f, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
