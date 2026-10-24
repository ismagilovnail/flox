package network

import (
	"reflect"
	"testing"
)

// What an operator types as their site becomes a host name (audit UNI-1
// part 2): no scheme, path or www., lower case, once.
func TestNormalizeDomains(t *testing.T) {
	got, bad := normalizeDomains([]string{" Shop.Example.com ", "https://www.shop.example.com/cart?x=1", "", "promo.shop.example.com."})
	if bad != "" || !reflect.DeepEqual(got, []string{"shop.example.com", "promo.shop.example.com"}) {
		t.Fatalf("got %v %q", got, bad)
	}
	for _, in := range []string{"localhost", "shop..com", "-shop.com", "shop_x.com", "https://", "*.shop.com"} {
		if _, bad := normalizeDomains([]string{in}); bad == "" {
			t.Errorf("%q accepted, want refused", in)
		}
	}
	many := make([]string, maxBrowserDomains+1)
	for i := range many {
		many[i] = string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".example.com"
	}
	if _, bad := normalizeDomains(many); bad == "" {
		t.Error("too many domains accepted")
	}
	if got, bad := normalizeDomains([]string{}); bad != "" || got == nil || len(got) != 0 {
		t.Errorf("empty list = %v %q, want an empty, non-nil list (clears them)", got, bad)
	}
}
