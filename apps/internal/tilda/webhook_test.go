package tilda_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/tilda"
)

func parse(t *testing.T, contentType, body string) (tilda.Submission, *http.Request) {
	t.Helper()
	r := httptest.NewRequest("POST", "/tilda/x", strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	sub, err := tilda.Parse(r, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	return sub, r
}

// Audit UNI-4 part 5: the API key in the body or a header, the connect
// test, the order in each shape it may come in, the click from the cookies
// or a hidden field.
func TestAuthorized(t *testing.T) {
	sub, r := parse(t, "application/x-www-form-urlencoded", "Name=x&floxkey=s3cret")
	if !tilda.Authorized(r, sub, "s3cret") {
		t.Fatal("key in the body refused")
	}
	if tilda.Authorized(r, sub, "other") || tilda.Authorized(r, sub, "") {
		t.Fatal("wrong or empty secret accepted")
	}
	sub, r = parse(t, "", "Name=x")
	if tilda.Authorized(r, sub, "s3cret") {
		t.Fatal("no key accepted")
	}
	r.Header.Set("floxkey", "s3cret")
	if !tilda.Authorized(r, sub, "s3cret") {
		t.Fatal("key in the header refused")
	}
}

func TestIsTest(t *testing.T) {
	for body, want := range map[string]bool{
		"test=test":                true,
		"test=test&floxkey=k":      true,
		"test=test&Name=x":         false,
		"Name=test":                false,
		"tranid=1%3A2&formid=f123": false,
	} {
		sub, _ := parse(t, "", body)
		if got := tilda.IsTest(sub); got != want {
			t.Errorf("IsTest(%q) = %v", body, got)
		}
	}
}

func TestPayment(t *testing.T) {
	cases := []struct {
		name, ct, body string
		order, amount  string
		currency, tran string
		noPayment      bool
	}{
		{name: "form fields", body: "Name=x&tranid=467251%3A8442970&payment%5Borderid%5D=1234567&payment%5Bamount%5D=1990&payment%5Bproducts%5D%5B0%5D=Tee",
			order: "1234567", amount: "1990", tran: "467251:8442970"},
		{name: "json string", body: "tranid=1%3A2&payment=" + `%7B%22orderid%22%3A%221234567%22%2C%22amount%22%3A%22990.50%22%7D`,
			order: "1234567", amount: "990.50", tran: "1:2"},
		{name: "json body", ct: "application/json", body: `{"tranid":"1:3","payment":{"orderid":1234567,"amount":1990,"currency":"rub"}}`,
			order: "1234567", amount: "1990", currency: "RUB", tran: "1:3"},
		{name: "plain form", body: "Name=x&Phone=1&tranid=1%3A4", tran: "1:4", noPayment: true},
	}
	for _, c := range cases {
		sub, _ := parse(t, c.ct, c.body)
		if sub.TranID() != c.tran {
			t.Errorf("%s: tranid %q", c.name, sub.TranID())
		}
		if c.noPayment {
			if sub.Payment != nil {
				t.Errorf("%s: payment %+v on a plain form", c.name, sub.Payment)
			}
			continue
		}
		if sub.Payment == nil || sub.Payment.OrderID != c.order || sub.Payment.Amount != c.amount || sub.Payment.Currency != c.currency {
			t.Errorf("%s: payment %+v", c.name, sub.Payment)
		}
	}
}

func TestClickID(t *testing.T) {
	for body, want := range map[string]string{
		"COOKIES=" + "_ga%3DGA1.2.1%3B+flox_click_id%3D01ABC%3B+_ym_uid%3D2": "01ABC",
		"COOKIES=_ga%3D1&Click_id=01HID":                                     "01HID",
		"click_id=01LOW":                                                     "01LOW",
		"COOKIES=TILDAUTM%3Dutm_source%253Dx":                                "",
	} {
		sub, _ := parse(t, "", body)
		if got := sub.ClickID(); got != want {
			t.Errorf("ClickID(%q) = %q, want %q", body, got, want)
		}
	}
}
