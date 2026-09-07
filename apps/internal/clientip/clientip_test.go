package clientip_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/clientip"
)

func TestNewRejectsMalformedEntries(t *testing.T) {
	cases := []string{"not-an-ip", "10.0.0.0/33", "1.2.3.4/abc"}
	for _, entry := range cases {
		if _, err := clientip.New([]string{entry}, nil); err == nil {
			t.Errorf("New(%q) = nil error, want an error for a malformed entry", entry)
		}
	}
}

func TestAddrIgnoresXFFFromAnUntrustedPeer(t *testing.T) {
	// A public peer is never in DefaultTrustedProxies, so its claimed
	// X-Forwarded-For must be ignored entirely — this is the exact bypass
	// audit finding #2 describes: trusting the leftmost (client-written)
	// entry unconditionally.
	rs, err := clientip.New(nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.9:54321"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")

	if got := rs.String(req); got != "203.0.113.9" {
		t.Fatalf("String = %q, want the untrusted peer's own address, not the spoofed header", got)
	}
}

func TestAddrWalksTrustedChainFromTheRight(t *testing.T) {
	rs, err := clientip.New([]string{"10.0.0.0/8"}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:1"
	// Written left to right as each proxy appends its upstream peer: the
	// real client is leftmost, 10.0.0.2 and 10.0.0.5 are our own trusted
	// hops closest to this server.
	req.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.2, 10.0.0.5")

	if got := rs.String(req); got != "198.51.100.7" {
		t.Fatalf("String = %q, want the first non-trusted entry from the right", got)
	}
}

func TestAddrFallsBackToLeftmostWhenEveryHopIsTrusted(t *testing.T) {
	rs, err := clientip.New([]string{"10.0.0.0/8"}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:1"
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")

	if got := rs.String(req); got != "10.0.0.1" {
		t.Fatalf("String = %q, want the leftmost entry of an all-internal chain", got)
	}
}

func TestTrustNothingIgnoresXFFEvenFromAKnownPeer(t *testing.T) {
	rs, err := clientip.New([]string{clientip.TrustNothing}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:1"
	req.Header.Set("X-Forwarded-For", "198.51.100.7")

	if got := rs.String(req); got != "10.0.0.5" {
		t.Fatalf("String = %q, want RemoteAddr with TRUSTED_PROXIES=none, header ignored regardless of peer", got)
	}
}

func TestParseListSplitsAndTrims(t *testing.T) {
	got := clientip.ParseList(" 10.0.0.0/8 , 192.168.0.0/16,, fc00::/7 ")
	want := []string{"10.0.0.0/8", "192.168.0.0/16", "fc00::/7"}
	if len(got) != len(want) {
		t.Fatalf("ParseList = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ParseList = %v, want %v", got, want)
		}
	}
	if got := clientip.ParseList("   "); got != nil {
		t.Fatalf("ParseList(blank) = %v, want nil", got)
	}
}

func TestUnmapsIPv4MappedIPv6(t *testing.T) {
	// ::ffff:1.2.3.4 and 1.2.3.4 must resolve to one identity, otherwise
	// they'd get two separate rate-limit buckets for the same client.
	rs, err := clientip.New(nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "[::ffff:203.0.113.9]:54321"

	if got := rs.String(req); got != "203.0.113.9" {
		t.Fatalf("String = %q, want the unmapped IPv4 address", got)
	}
}
