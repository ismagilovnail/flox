// Package tilda takes a Tilda site's form webhooks to an own-site
// conversion source (audit UNI-4 part 5; docs/shop-connections.md).
//
// A Tilda form, the cart's included, sends each submission to a "Webhook"
// receiver (Site settings › Forms) as a POST of its fields, with tranid
// (the submission's id), formid and — with "Send cookies" ticked — the
// visitor's cookies in COOKIES, where flox.js keeps the click. Tilda does
// not sign it: the receiver's "API key", a name and value of the owner's
// choosing sent as a POST field or a header, is the secret (KeyName). A
// submission with an order (the cart's payment block) is a purchase; any
// other is a lead. Tilda sends no refunds.
//
// The connection settings and the order lookups are internal/shopconn's.
package tilda

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/shopconn"
)

// Platform is a Tilda site's shop_connections platform: the settings at
// /networks/{id}/tilda, the webhooks at the tracker's /tilda/{networkId}.
// The site address is the owner's note only — a Tilda webhook does not
// say which site sent it — so it may be empty.
var Platform = shopconn.Platform{
	Name:            "tilda",
	Title:           "Tilda",
	NormalizeDomain: shopconn.NormalizeSite,
	ValidDomain:     func(d string) bool { return d == "" || shopconn.ValidSite(d) },
	DomainHint:      "the site's address, e.g. example.tilda.ws or shop.example.com (optional)",
}

// KeyName is the API key's name the owner sets on the Tilda receiver —
// "API Name" — sent as a POST field or a header (no "-" or "_": proxies
// drop underscored headers, and it must read the same both ways).
const KeyName = "floxkey"

// CookieName is flox.js's first-party cookie with the click id.
const CookieName = "flox_click_id"

// Submission is a form submission as Tilda sends it.
type Submission struct {
	// Fields are the form's own fields and Tilda's (tranid, formid,
	// COOKIES), flattened: payment[...] keys stay as sent.
	Fields url.Values
	// Payment is the cart's order; nil for a form without one.
	Payment *Payment
}

// Payment is the part of a cart order FLOX reads.
type Payment struct {
	OrderID  string
	Amount   string
	Currency string
}

// Parse reads a submission: a form body (what Tilda documents) or a JSON
// object, its payment either as payment[...] fields, a JSON string in
// "payment", or an object.
func Parse(r *http.Request, body []byte) (Submission, error) {
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	sub := Submission{Fields: url.Values{}}
	var paymentObj map[string]any
	if ct == "application/json" {
		var obj map[string]any
		if err := decode(body, &obj); err != nil {
			return Submission{}, fmt.Errorf("tilda: reading the submission: %w", err)
		}
		for k, v := range obj {
			if k == "payment" {
				if m, ok := v.(map[string]any); ok {
					paymentObj = m
					continue
				}
			}
			sub.Fields.Set(k, scalar(v))
		}
	} else {
		form, err := url.ParseQuery(string(body))
		if err != nil {
			return Submission{}, fmt.Errorf("tilda: reading the submission: %w", err)
		}
		sub.Fields = form
	}
	if paymentObj == nil {
		if raw := strings.TrimSpace(sub.Fields.Get("payment")); strings.HasPrefix(raw, "{") {
			_ = decode([]byte(raw), &paymentObj)
		}
	}
	p := Payment{}
	if paymentObj != nil {
		p.OrderID, p.Amount, p.Currency = scalar(paymentObj["orderid"]), scalar(paymentObj["amount"]), scalar(paymentObj["currency"])
	} else {
		p.OrderID, p.Amount, p.Currency = sub.Fields.Get("payment[orderid]"), sub.Fields.Get("payment[amount]"), sub.Fields.Get("payment[currency]")
	}
	p.OrderID, p.Amount, p.Currency = strings.TrimSpace(p.OrderID), strings.TrimSpace(p.Amount), strings.ToUpper(strings.TrimSpace(p.Currency))
	if p.OrderID != "" {
		sub.Payment = &p
	}
	return sub, nil
}

// decode keeps numbers as written: an order id 1234567 must not come
// back 1.234567e+06.
func decode(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	return d.Decode(v)
}

func scalar(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool, json.Number:
		return fmt.Sprint(x)
	}
	return ""
}

// Authorized: the request carries the receiver's API key — as the POST
// field or the header KeyName — and it is secret.
func Authorized(r *http.Request, sub Submission, secret string) bool {
	if secret == "" {
		return false
	}
	got := r.Header.Get(KeyName)
	if got == "" {
		got = sub.Fields.Get(KeyName)
	}
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(secret)) == 1
}

// IsTest is the request Tilda sends when the receiver is connected —
// test=test, with the API key when it goes in the body — and expects a 200
// for.
func IsTest(sub Submission) bool {
	if sub.Fields.Get("test") != "test" {
		return false
	}
	for k := range sub.Fields {
		if k != "test" && k != KeyName {
			return false
		}
	}
	return true
}

// TranID is Tilda's id of the submission ("467251:8442970").
func (s Submission) TranID() string { return strings.TrimSpace(s.Fields.Get("tranid")) }

// ClickID is the FLOX click the submission came from: flox.js's cookie in
// COOKIES (the receiver's "Send cookies"), else a form field named click_id
// (a hidden field; Tilda capitalizes the first letter of a field's name
// unless a variable name is set), else flox_click_id; "" when none.
func (s Submission) ClickID() string {
	for _, c := range strings.Split(s.Fields.Get("COOKIES"), ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(c), "=")
		if ok && name == CookieName {
			if v, err := url.QueryUnescape(value); err == nil && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	for _, name := range []string{"click_id", "Click_id", CookieName, "Flox_click_id"} {
		if v := strings.TrimSpace(s.Fields.Get(name)); v != "" {
			return v
		}
	}
	return ""
}
