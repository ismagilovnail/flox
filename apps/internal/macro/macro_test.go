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
	got := macro.Resolve("?click_id={click_id}&payout={payout}&offer={offer_id}&typo={bogus_token}",
		macro.Values{ClickID: "c1"})
	want := "?click_id=c1&payout={payout}&offer={offer_id}&typo={bogus_token}"
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
