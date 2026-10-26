// Package woocommerce takes a WooCommerce store's order webhooks to an
// own-site conversion source (audit UNI-4 part 4; docs/shop-connections.md).
//
// The store sends "Order created" and "Order updated" (order.created,
// order.updated: the order as the REST API v3 has it) to apps/tracker's
// /woocommerce/{networkId}, signed with the secret set on the webhook. The
// tracker checks the signature (Verify), reads the click the order carries
// (ClickID: the flox_click_id meta field, else the click_id on the page
// WooCommerce's order attribution saw the visit enter by) and records,
// through the conversion engine, a paid order as a purchase — a repeat
// purchase when the click already bought — and each refund on it as a
// refund of that amount. WooCommerce has no refund webhook: a refund
// arrives as an update of its order, with every refund the order has.
//
// The connection settings and the order lookups are internal/shopconn's.
package woocommerce

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/shopconn"
)

// Platform is a WooCommerce store's shop_connections platform: the
// settings at /networks/{id}/woocommerce, the webhooks at the tracker's
// /woocommerce/{networkId}.
var Platform = shopconn.Platform{
	Name:            "woocommerce",
	Title:           "WooCommerce",
	NormalizeDomain: shopconn.NormalizeSite,
	ValidDomain:     shopconn.ValidSite,
	DomainHint:      "the store's site address as WordPress has it (Settings › General), e.g. shop.example.com or example.com/shop",
}

// Topics the tracker acts on (X-WC-Webhook-Topic); every other topic is
// acknowledged and left.
const (
	TopicOrderCreated = "order.created"
	TopicOrderUpdated = "order.updated"
)

// MetaKey is the order meta field the store's checkout copies flox.js's
// click cookie into (the snippet in docs/shop-connections.md).
const MetaKey = "flox_click_id"

// attributionEntry is the order meta WooCommerce's order attribution
// (8.5+, on by default) keeps the visit's first page in.
const attributionEntry = "_wc_order_attribution_session_entry"

// Verify checks a webhook's X-WC-Webhook-Signature: base64 of HMAC-SHA256
// over the raw body, keyed with the webhook's secret.
func Verify(body []byte, signature, secret string) bool {
	if secret == "" || signature == "" {
		return false
	}
	got, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signature))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

// Sign is Verify's counterpart, for tests.
func Sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

var pingBody = regexp.MustCompile(`^webhook_id=[0-9]+$`)

// IsPing is the request WooCommerce sends when a webhook is made active:
// an unsigned form body "webhook_id=N". Anything but a 200 shows the
// owner a delivery error, so it is answered before the signature check —
// it records nothing.
func IsPing(body []byte, signature string) bool {
	return signature == "" && pingBody.Match(bytes.TrimSpace(body))
}

// Order is the part of an order webhook FLOX reads.
type Order struct {
	ID          json.Number `json:"id"`
	Status      string      `json:"status"`
	Currency    string      `json:"currency"`
	Total       string      `json:"total"`
	DatePaidGMT *string     `json:"date_paid_gmt"`
	MetaData    []struct {
		Key   string `json:"key"`
		Value any    `json:"value"`
	} `json:"meta_data"`
	Refunds []struct {
		ID    json.Number `json:"id"`
		Total string      `json:"total"`
	} `json:"refunds"`
}

func ParseOrder(body []byte) (Order, error) {
	var o Order
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	if err := d.Decode(&o); err != nil {
		return Order{}, fmt.Errorf("woocommerce: reading the order: %w", err)
	}
	if o.ID == "" || o.ID == "0" {
		return Order{}, fmt.Errorf("woocommerce: the order has no id")
	}
	return o, nil
}

// Paid: the order has been paid for. WooCommerce sets date_paid when the
// payment completes (for cash on delivery: when the order is completed)
// and keeps it through a refund; an order that failed or was cancelled
// before paying has none.
func (o Order) Paid() bool {
	if o.DatePaidGMT == nil || strings.TrimSpace(*o.DatePaidGMT) == "" {
		return false
	}
	switch o.Status {
	case "pending", "failed", "cancelled", "checkout-draft", "trash":
		return false
	}
	return true
}

// At is when the order was paid, RFC 3339: date_paid_gmt is UTC without a
// zone.
func (o Order) At() string {
	if o.DatePaidGMT == nil {
		return ""
	}
	at := strings.TrimSpace(*o.DatePaidGMT)
	if at == "" || strings.HasSuffix(at, "Z") || strings.ContainsAny(at[min(len(at), 19):], "+-") {
		return at
	}
	return at + "Z"
}

// ClickID is the FLOX click the order came from: the flox_click_id meta
// field the checkout copied from flox.js's cookie, else click_id on the
// page the visit entered the store by (an offer link straight to the
// store carries it); "" when neither.
func (o Order) ClickID() string {
	if v := o.meta(MetaKey); v != "" {
		return v
	}
	entry := o.meta(attributionEntry)
	if entry == "" {
		return ""
	}
	u, err := url.Parse(entry)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(u.Query().Get("click_id"))
}

func (o Order) meta(key string) string {
	for _, m := range o.MetaData {
		if m.Key != key || m.Value == nil {
			continue
		}
		if v, ok := m.Value.(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// Refund is money paid back on the order.
type Refund struct {
	ID     string
	Amount float64
}

// PaidBack is the order's refunds that paid money back, oldest first
// (WooCommerce lists them newest first; each refund's total is negative).
func (o Order) PaidBack() []Refund {
	var out []Refund
	for i := len(o.Refunds) - 1; i >= 0; i-- {
		r := o.Refunds[i]
		v, err := strconv.ParseFloat(strings.TrimSpace(r.Total), 64)
		if err != nil || v == 0 || r.ID == "" {
			continue
		}
		if v < 0 {
			v = -v
		}
		out = append(out, Refund{ID: r.ID.String(), Amount: v})
	}
	return out
}
