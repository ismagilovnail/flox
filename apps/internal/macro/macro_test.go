package macro_test

import (
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/macro"
)

func TestResolveSubstitutesKnownTokens(t *testing.T) {
	got := macro.Resolve(
		"https://net.example/pb?click_id={click_id}&status={status}&revenue={revenue}&s1={sub1}",
		macro.Values{ClickID: "c1", Status: "CPA_ACCEPT", Revenue: "50.00", Subs: [10]string{"a"}},
	)
	want := "https://net.example/pb?click_id=c1&status=CPA_ACCEPT&revenue=50.00&s1=a"
	if got != want {
		t.Fatalf("Resolve() = %q, want %q", got, want)
	}
}

func TestResolveBlanksRecognizedButEmptyField(t *testing.T) {
	got := macro.Resolve("?click_id={click_id}&currency={currency}", macro.Values{ClickID: "c1"})
	if got != "?click_id=c1&currency=" {
		t.Fatalf("Resolve() = %q, want the empty currency substituted, not left literal", got)
	}
}

func TestResolveLeavesUnrecognizedTokensLiteral(t *testing.T) {
	got := macro.Resolve("?click_id={click_id}&payout={payout}&source={source}&typo={bogus_token}",
		macro.Values{ClickID: "c1"})
	want := "?click_id=c1&payout={payout}&source={source}&typo={bogus_token}"
	if got != want {
		t.Fatalf("Resolve() = %q, want unrecognized tokens left literal: %q", got, want)
	}
}

func TestResolveAllTenSubs(t *testing.T) {
	var subs [10]string
	for i := range subs {
		subs[i] = string(rune('a' + i))
	}
	got := macro.Resolve("{sub1}{sub2}{sub10}", macro.Values{Subs: subs})
	if got != "abj" {
		t.Fatalf("Resolve() = %q, want %q", got, "abj")
	}
}

func TestResolveNoTemplateTokens(t *testing.T) {
	if got := macro.Resolve("https://net.example/static", macro.Values{}); got != "https://net.example/static" {
		t.Fatalf("Resolve() = %q, want the template unchanged", got)
	}
}

// Audit 2026-10-06, T7: a URL template's values are query-escaped — sub
// values come from the click's own query string and must not be able to
// add parameters, cut the URL short or change its path.
func TestResolveURLEscapesValues(t *testing.T) {
	v := macro.Values{ClickID: "01CLICK", OfferID: "01OFFER", Subs: [10]string{"a&click_id=evil#frag", "x y"}}
	got := macro.ResolveURL("https://offer.example/go?cid={click_id}&o={offer_id}&s1={sub1}&s2={sub2}&p={payout}", v)
	want := "https://offer.example/go?cid=01CLICK&o=01OFFER&s1=a%26click_id%3Devil%23frag&s2=x+y&p={payout}"
	if got != want {
		t.Fatalf("ResolveURL =\n  %s\nwant\n  %s", got, want)
	}
	// Resolve itself stays unescaped (payload templates, not URLs).
	if got := macro.Resolve("{sub1}", v); got != "a&click_id=evil#frag" {
		t.Fatalf("Resolve = %q, want the raw value", got)
	}
}
