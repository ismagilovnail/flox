package woocommerce_test

import (
	"reflect"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/shopconn"
	"github.com/ismagilovnail/flox/apps/internal/woocommerce"
)

// Audit UNI-4 part 4: the signature over the raw body, the activation
// ping, the store's address, when an order counts as paid, the click it
// carries and the money its refunds paid back.
func TestVerify(t *testing.T) {
	body := []byte(`{"id":1}`)
	sig := woocommerce.Sign(body, "key")
	if !woocommerce.Verify(body, sig, "key") {
		t.Fatal("own signature refused")
	}
	for name, tc := range map[string]struct {
		body     []byte
		sig, key string
	}{
		"other key":  {body, sig, "other"},
		"other body": {[]byte(`{"id":2}`), sig, "key"},
		"no key":     {body, sig, ""},
		"no sig":     {body, "", "key"},
		"not base64": {body, "%%%", "key"},
	} {
		if woocommerce.Verify(tc.body, tc.sig, tc.key) {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestIsPing(t *testing.T) {
	if !woocommerce.IsPing([]byte("webhook_id=12"), "") {
		t.Fatal("the activation ping not recognised")
	}
	for _, c := range []struct{ body, sig string }{
		{"webhook_id=12", "c2ln"}, // signed: a real delivery
		{"webhook_id=12&x=1", ""},
		{`{"id":12}`, ""},
		{"", ""},
	} {
		if woocommerce.IsPing([]byte(c.body), c.sig) {
			t.Errorf("%q (signature %q) taken for a ping", c.body, c.sig)
		}
	}
}

func TestNormalizeSite(t *testing.T) {
	for in, want := range map[string]string{
		"https://Shop.Example.com/":        "shop.example.com",
		"http://example.com/store/?a=1#x":  "example.com/store",
		" shop.example.com. ":              "shop.example.com",
		"https://example.com:8443/wp/shop": "example.com:8443/wp/shop",
	} {
		got := shopconn.NormalizeSite(in)
		if got != want {
			t.Errorf("NormalizeSite(%q) = %q, want %q", in, got, want)
		}
		if !woocommerce.Platform.ValidDomain(got) {
			t.Errorf("%q refused", got)
		}
	}
	for _, bad := range []string{"localhost", "shop", "user@example.com", "example.com/a b", ""} {
		if woocommerce.Platform.ValidDomain(shopconn.NormalizeSite(bad)) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestOrder(t *testing.T) {
	cases := []struct {
		json        string
		paid        bool
		at, clickID string
	}{
		{`{"id":1,"status":"processing","date_paid_gmt":"2026-10-26T10:00:00",
			"meta_data":[{"id":5,"key":"flox_click_id","value":"01A"},{"key":"_wc_order_attribution_session_entry","value":"https://s.example/?click_id=01B"}]}`,
			true, "2026-10-26T10:00:00Z", "01A"},
		{`{"id":1,"status":"completed","date_paid_gmt":"2026-10-26T10:00:00+03:00",
			"meta_data":[{"key":"flox_click_id","value":""},{"key":"_wc_order_attribution_session_entry","value":"https://s.example/p?x=1&click_id=01B"}]}`,
			true, "2026-10-26T10:00:00+03:00", "01B"},
		{`{"id":1,"status":"refunded","date_paid_gmt":"2026-10-26T10:00:00","meta_data":[{"key":"flox_click_id","value":{"nested":1}}]}`,
			true, "2026-10-26T10:00:00Z", ""},
		{`{"id":1,"status":"pending","date_paid_gmt":null,"meta_data":[]}`, false, "", ""},
		{`{"id":1,"status":"processing","date_paid_gmt":null}`, false, "", ""}, // cash on delivery
		{`{"id":1,"status":"cancelled","date_paid_gmt":"2026-10-26T10:00:00"}`, false, "2026-10-26T10:00:00Z", ""},
	}
	for _, c := range cases {
		o, err := woocommerce.ParseOrder([]byte(c.json))
		if err != nil {
			t.Fatal(err)
		}
		if o.Paid() != c.paid || o.At() != c.at || o.ClickID() != c.clickID {
			t.Errorf("%s: paid %v at %q click %q, want %v %q %q", c.json, o.Paid(), o.At(), o.ClickID(), c.paid, c.at, c.clickID)
		}
	}
	if _, err := woocommerce.ParseOrder([]byte(`{"status":"processing"}`)); err == nil {
		t.Error("an order without an id accepted")
	}
	if _, err := woocommerce.ParseOrder([]byte(`webhook_id=1`)); err == nil {
		t.Error("a form body accepted as an order")
	}
}

func TestPaidBack(t *testing.T) {
	o, err := woocommerce.ParseOrder([]byte(`{"id":1,"refunds":[
		{"id":31,"reason":"","total":"-490.00"},
		{"id":30,"reason":"","total":"0.00"},
		{"id":29,"reason":"","total":"-10.50"},
		{"id":28,"total":"x"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []woocommerce.Refund{{ID: "29", Amount: 10.5}, {ID: "31", Amount: 490}}
	if got := o.PaidBack(); !reflect.DeepEqual(got, want) {
		t.Fatalf("PaidBack = %+v, want %+v", got, want)
	}
}
