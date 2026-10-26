package main

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/shopconn"
	"github.com/ismagilovnail/flox/apps/internal/tilda"
)

// Audit UNI-4 part 5: a connected Tilda site's form submissions, on the
// real Postgres and conversion engine. The connect test is answered; only
// a submission with the receiver's API key (body or header) is taken. A
// form is a lead, a cart order a purchase (the click's next one a repeat
// purchase); Tilda's retries record nothing more.
func TestTildaWebhooks(t *testing.T) {
	f := newShopFixture(t)
	ctx, orgID, networkID, partnerID, click, other := f.ctx, f.orgID, f.networkID, f.partnerID, f.click, f.other
	settings := shopconn.NewService(f.store, tilda.Platform, "https://t.example")
	if _, err := settings.Put(ctx, orgID, partnerID, shopconn.Input{Secret: "k"}); err == nil {
		t.Fatal("connected a site to a partner network")
	}
	if _, err := settings.Put(ctx, orgID, networkID, shopconn.Input{ShopDomain: "not a site", Secret: "k"}); err == nil {
		t.Fatal("took a site address that is not one")
	}
	view, err := settings.Put(ctx, orgID, networkID, shopconn.Input{Secret: "tilda_api_key"})
	if err != nil {
		t.Fatal(err)
	}
	if !view.Connected || view.ShopDomain != "" || view.SecretPreview != "_key" || view.WebhookURL != "https://t.example/tilda/"+networkID {
		t.Fatalf("settings = %+v", view)
	}
	if _, err := settings.Get(ctx, idgen.New(), networkID); err == nil {
		t.Fatal("another organization read the site connection")
	}

	const key = "tilda_api_key"
	send := func(network string, fields url.Values, header string) (int, postbackResponse) {
		t.Helper()
		h := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
		if header != "" {
			h[tilda.KeyName] = header
		}
		return f.send("/tilda/"+network, []byte(fields.Encode()), h)
	}
	withKey := func(v url.Values) url.Values { v.Set(tilda.KeyName, key); return v }
	cookies := func(c string) string { return "_ga=GA1.2.3; flox_click_id=" + c + "; _ym_uid=1" }

	if code, res := send(networkID, withKey(url.Values{"test": {"test"}}), ""); code != http.StatusOK || res.Message != "ping" {
		t.Fatalf("connect test: %d %+v", code, res)
	}
	lead := url.Values{"Name": {"Ann"}, "Phone": {"+7900"}, "tranid": {"467251:1"}, "formid": {"form1"}, "COOKIES": {cookies(click)}}
	if code, _ := send(networkID, lead, ""); code != http.StatusUnauthorized {
		t.Fatalf("no key: %d", code)
	}
	if code, _ := send(networkID, lead, "wrong"); code != http.StatusUnauthorized {
		t.Fatalf("a wrong key: %d", code)
	}
	if code, _ := send(partnerID, withKey(url.Values{"tranid": {"1:1"}}), ""); code != http.StatusNotFound {
		t.Fatalf("a source without a site: %d", code)
	}

	// A form with the key in a header is a lead; Tilda's retry of it
	// records nothing.
	if code, res := send(networkID, lead, key); code != 200 || res.Result != "success" || res.Status != "CPA_HOLD" {
		t.Fatalf("lead: %d %+v", code, res)
	}
	if code, res := send(networkID, lead, key); code != 200 || res.Result != "duplicate" {
		t.Fatalf("lead retried: %d %+v", code, res)
	}
	// A cart order (key in the body) is a purchase, once.
	order := withKey(url.Values{"Name": {"Ann"}, "tranid": {"467251:2"}, "COOKIES": {cookies(click)},
		"payment[orderid]": {"1234567"}, "payment[amount]": {"1990"}, "payment[products][0]": {"Tee - 1x1990 = 1990"}})
	if code, res := send(networkID, order, ""); code != 200 || res.Result != "success" || res.Status != "CPA_ACCEPT" {
		t.Fatalf("order: %d %+v", code, res)
	}
	if code, res := send(networkID, order, ""); code != 200 || res.Message != "duplicate" {
		t.Fatalf("order retried: %d %+v", code, res)
	}
	if n, rev := f.rows("CPA_ACCEPT"); n != 1 || rev != 1990 {
		t.Fatalf("purchases: %d, %v", n, rev)
	}
	// The click's next order, its click in a hidden field, is a repeat
	// purchase; an order of another click is that click's purchase.
	next := withKey(url.Values{"tranid": {"467251:3"}, "Click_id": {click}, "payment[orderid]": {"1234568"}, "payment[amount]": {"490"}})
	if code, res := send(networkID, next, ""); code != 200 || res.Status != "CPA_REDEP" {
		t.Fatalf("second order: %d %+v", code, res)
	}
	asJSON := withKey(url.Values{"tranid": {"467251:4"}, "COOKIES": {cookies(other)}, "payment": {`{"orderid":"1234569","amount":"990"}`}})
	if code, res := send(networkID, asJSON, ""); code != 200 || res.Status != "CPA_ACCEPT" {
		t.Fatalf("order with the payment as JSON: %d %+v", code, res)
	}
	if n, rev := f.rows("CPA_ACCEPT"); n != 2 || rev != 2980 {
		t.Fatalf("purchases: %d, %v", n, rev)
	}
	if n, _ := f.rows("CPA_HOLD"); n != 1 {
		t.Fatalf("leads: %d", n)
	}
}
