package macro_test

import (
	"net/url"
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

// A postlanding's onward link is a whole URL dropped into a query
// parameter: it must arrive as one escaped value, and nowhere else
// set it resolves to nothing.
func TestResolveURLNextURL(t *testing.T) {
	next := "https://t.example/pc/abc.def?x=1&y=2"
	got := macro.ResolveURL("https://post.example/?go={next_url}&cid={click_id}", macro.Values{ClickID: "c1", NextURL: next})
	want := "https://post.example/?go=https%3A%2F%2Ft.example%2Fpc%2Fabc.def%3Fx%3D1%26y%3D2&cid=c1"
	if got != want {
		t.Fatalf("ResolveURL =\n  %s\nwant\n  %s", got, want)
	}
	if got := macro.ResolveURL("https://offer.example/?n={next_url}", macro.Values{}); got != "https://offer.example/?n=" {
		t.Fatalf("unset next_url = %s, want it blank", got)
	}
}

// {pwa_link} resolves (escaped) only where it is set: an external PWA's URL.
func TestResolveURLPwaLink(t *testing.T) {
	link := "https://t.example/pwx/tok/"
	if got := macro.ResolveURL("https://pwa.example/?f={pwa_link}", macro.Values{PwaLink: link}); got != "https://pwa.example/?f="+url.QueryEscape(link) {
		t.Fatalf("got %q", got)
	}
	if got := macro.ResolveURL("https://offer.example/?f={pwa_link}", macro.Values{}); got != "https://offer.example/?f=" {
		t.Fatalf("unset = %q, want empty", got)
	}
}

// Audit UNI-2: {gclid}, {yclid}, … resolve to the click's network click id
// when it came in that parameter, to "" otherwise — never left literal.
func TestResolveAdClickIDs(t *testing.T) {
	tmpl := "https://shop.example/?g={gclid}&y={yclid}&m={msclkid}&b={gbraid}&w={wbraid}"
	got := macro.ResolveURL(tmpl, macro.Values{AdClickParam: "yclid", AdClickID: "73 64"})
	if want := "https://shop.example/?g=&y=73+64&m=&b=&w="; got != want {
		t.Fatalf("ResolveURL =\n  %s\nwant\n  %s", got, want)
	}
	if got := macro.Resolve(tmpl, macro.Values{}); got != "https://shop.example/?g=&y=&m=&b=&w=" {
		t.Fatalf("without an ad click id = %q, want every token blank", got)
	}
}
