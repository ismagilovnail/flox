package shopify_test

import (
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/shopify"
)

// Audit UNI-4 part 3: the signature over the raw body, the click id the
// order carries, and the money a refund paid back.
func TestVerify(t *testing.T) {
	body := []byte(`{"id":1}`)
	sig := shopify.Sign(body, "key")
	if !shopify.Verify(body, sig, "key") {
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
		if shopify.Verify(tc.body, tc.sig, tc.key) {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestOrderClickID(t *testing.T) {
	cases := []struct {
		json, want string
	}{
		{`{"id":1,"note_attributes":[{"name":"flox_click_id","value":"01A"}],"landing_site":"/?click_id=01B"}`, "01A"},
		{`{"id":1,"note_attributes":[{"name":"flox_click_id","value":null}],"landing_site":"/p?x=1&click_id=01B"}`, "01B"},
		{`{"id":1,"landing_site":"https://shop.example/?click_id=01C#top"}`, "01C"},
		{`{"id":1,"landing_site":"/?utm_source=x"}`, ""},
		{`{"id":1}`, ""},
	}
	for _, c := range cases {
		o, err := shopify.ParseOrder([]byte(c.json))
		if err != nil {
			t.Fatal(err)
		}
		if got := o.ClickID(); got != c.want {
			t.Errorf("%s: click %q, want %q", c.json, got, c.want)
		}
	}
	if _, err := shopify.ParseOrder([]byte(`{"name":"#1"}`)); err == nil {
		t.Error("an order without an id was read")
	}
	o, _ := shopify.ParseOrder([]byte(`{"id":820982911946154508}`))
	if o.ID.String() != "820982911946154508" {
		t.Errorf("a large order id read as %s", o.ID)
	}
}

func TestRefundAmount(t *testing.T) {
	r, err := shopify.ParseRefund([]byte(`{"id":5,"order_id":9,"transactions":[
		{"amount":"75.00","kind":"refund","status":"success","currency":null},
		{"amount":"10.00","kind":"refund","status":"success","currency":"rub"},
		{"amount":"99.00","kind":"refund","status":"failure"},
		{"amount":"50.00","kind":"sale","status":"success"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if amount, currency, ok := r.Amount(); !ok || amount != 85 || currency != "RUB" {
		t.Fatalf("amount %v %q %v, want 85 RUB", amount, currency, ok)
	}
	none, _ := shopify.ParseRefund([]byte(`{"id":6,"order_id":9,"transactions":[]}`))
	if _, _, ok := none.Amount(); ok {
		t.Fatal("a refund that paid nothing back has an amount")
	}
}

func TestNormalizeShopDomain(t *testing.T) {
	for in, want := range map[string]string{
		"https://My-Shop.myshopify.com/admin": "my-shop.myshopify.com",
		" my-shop.myshopify.com. ":            "my-shop.myshopify.com",
	} {
		if got := shopify.NormalizeShopDomain(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}
