// Package shopify takes a Shopify store's order webhooks to an own-site
// conversion source (audit UNI-4 part 3; docs/shop-connections.md).
//
// The store sends "Order payment" (orders/paid) and "Refund create"
// (refunds/create) to apps/tracker's /shopify/{networkId}, signed with the
// store's key. The tracker checks the signature (Verify), reads the click
// id the order carries (ClickID: the cart attribute flox.js sets, else the
// click_id on the order's landing page) and records, through the
// conversion engine, a purchase — a repeat purchase when the click already
// bought — or a refund of the order (part of it, by the refund's own id).
//
// The connection settings and the order lookups are internal/shopconn's;
// this package is the webhook format and the store's domain.
package shopify

import (
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

// Platform is a Shopify store's shop_connections platform: the
// settings at /networks/{id}/shopify, the webhooks at the tracker's
// /shopify/{networkId}.
var Platform = shopconn.Platform{
	Name:            "shopify",
	Title:           "Shopify",
	NormalizeDomain: NormalizeShopDomain,
	ValidDomain:     shopDomainPattern.MatchString,
	DomainHint:      "the store's myshopify.com domain, e.g. my-shop.myshopify.com",
}

var shopDomainPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,99}\.myshopify\.com$`)

// NormalizeShopDomain takes "https://Shop.myshopify.com/" and the like to
// "shop.myshopify.com".
func NormalizeShopDomain(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
	return strings.TrimSuffix(strings.SplitN(d, "/", 2)[0], ".")
}

// Topics the tracker acts on; every other topic is acknowledged and left.
const (
	TopicOrderPaid    = "orders/paid"
	TopicRefundCreate = "refunds/create"
)

// CartAttribute is the cart attribute flox.js sets on a Shopify storefront
// (Cart AJAX API); Shopify keeps cart attributes on the order as
// note_attributes.
const CartAttribute = "flox_click_id"

// Verify checks a webhook's X-Shopify-Hmac-SHA256: base64 of HMAC-SHA256
// over the raw body, keyed with the store's secret.
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

// Sign is Verify's counterpart, for tests and the settings' self-check.
func Sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// Order is the part of an orders/paid payload FLOX reads.
type Order struct {
	ID          json.Number `json:"id"`
	Name        string      `json:"name"`
	Currency    string      `json:"currency"`
	TotalPrice  string      `json:"total_price"`
	ProcessedAt string      `json:"processed_at"`
	CreatedAt   string      `json:"created_at"`
	Test        bool        `json:"test"`
	LandingSite string      `json:"landing_site"`
	NoteAttrs   []struct {
		Name  string `json:"name"`
		Value any    `json:"value"`
	} `json:"note_attributes"`
}

// Refund is the part of a refunds/create payload FLOX reads.
type Refund struct {
	ID           json.Number `json:"id"`
	OrderID      json.Number `json:"order_id"`
	CreatedAt    string      `json:"created_at"`
	ProcessedAt  string      `json:"processed_at"`
	Transactions []struct {
		Amount   string `json:"amount"`
		Currency string `json:"currency"`
		Kind     string `json:"kind"`
		Status   string `json:"status"`
	} `json:"transactions"`
}

func ParseOrder(body []byte) (Order, error) {
	var o Order
	d := json.NewDecoder(strings.NewReader(string(body)))
	d.UseNumber()
	if err := d.Decode(&o); err != nil {
		return Order{}, fmt.Errorf("shopify: reading the order: %w", err)
	}
	if o.ID == "" {
		return Order{}, fmt.Errorf("shopify: the order has no id")
	}
	return o, nil
}

func ParseRefund(body []byte) (Refund, error) {
	var r Refund
	d := json.NewDecoder(strings.NewReader(string(body)))
	d.UseNumber()
	if err := d.Decode(&r); err != nil {
		return Refund{}, fmt.Errorf("shopify: reading the refund: %w", err)
	}
	if r.ID == "" || r.OrderID == "" {
		return Refund{}, fmt.Errorf("shopify: the refund has no id or order id")
	}
	return r, nil
}

// ClickID is the FLOX click the order came from: the cart attribute
// flox.js set, else click_id on the page the visit landed on (an offer link
// straight to the store carries it); "" when neither.
func (o Order) ClickID() string {
	for _, a := range o.NoteAttrs {
		if a.Name != CartAttribute || a.Value == nil {
			continue
		}
		if v := strings.TrimSpace(fmt.Sprint(a.Value)); v != "" {
			return v
		}
	}
	if o.LandingSite == "" {
		return ""
	}
	u, err := url.Parse(o.LandingSite)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(u.Query().Get("click_id"))
}

// At is when the order was paid, as Shopify gives it (RFC 3339).
func (o Order) At() string {
	if o.ProcessedAt != "" {
		return o.ProcessedAt
	}
	return o.CreatedAt
}

// Amount is the money refunded: the refund's successful refund
// transactions — what was actually paid back (line items, shipping and
// adjustments can differ from it). currency is theirs, "" when Shopify
// left it out; ok false when nothing was paid back.
func (r Refund) Amount() (amount float64, currency string, ok bool) {
	for _, t := range r.Transactions {
		if t.Kind != "refund" || t.Status != "success" {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(t.Amount), 64)
		if err != nil || v <= 0 {
			continue
		}
		amount += v
		if currency == "" {
			currency = strings.ToUpper(strings.TrimSpace(t.Currency))
		}
		ok = true
	}
	return amount, currency, ok
}

// At is when the refund was made.
func (r Refund) At() string {
	if r.ProcessedAt != "" {
		return r.ProcessedAt
	}
	return r.CreatedAt
}
