package main

import (
	"math/rand/v2"
	"net"
	"net/url"
	"testing"
)

// The clicks costbench sends must reach the tracker as distinct public
// visitors: a private or loopback X-Forwarded-For is a trusted proxy, so
// the tracker would see one client and its per-IP limit would cut the run.
func TestTrafficLooksLikePaidSocialClicks(t *testing.T) {
	gen := newTraffic(rand.New(rand.NewPCG(1, 2)))
	clickIDs := map[string]bool{}
	for range 2000 {
		tg := gen.next("http://localhost:8081/t/loadtest", "loadtest.example")
		u, err := url.Parse(tg.URL)
		if err != nil {
			t.Fatalf("target URL %q: %v", tg.URL, err)
		}
		if tg.Header["Host"][0] != "loadtest.example" {
			t.Fatalf("Host = %v", tg.Header["Host"])
		}
		ip := net.ParseIP(tg.Header["X-Forwarded-For"][0])
		if ip == nil || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			t.Fatalf("X-Forwarded-For %v is not a public address", tg.Header["X-Forwarded-For"])
		}
		id := u.Query().Get("fbclid") + u.Query().Get("ttclid")
		if id == "" || clickIDs[id] {
			t.Fatalf("click id %q missing or repeated", id)
		}
		clickIDs[id] = true
	}
}
