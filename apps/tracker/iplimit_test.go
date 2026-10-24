package main

import (
	"context"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/clicklimit"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/landinglink"
)

func TestClickLimiterCountsPerIPPerWindow(t *testing.T) {
	l := newClickLimiter(3, time.Minute)
	at := time.Date(2026, 10, 22, 12, 0, 10, 0, time.UTC)
	a, b := net.ParseIP("198.51.100.1"), net.ParseIP("198.51.100.2")

	for i := 1; i <= 3; i++ {
		if !l.allow(a, at) {
			t.Fatalf("click %d of 3 refused", i)
		}
	}
	if l.allow(a, at) {
		t.Fatal("the 4th click in the minute was allowed")
	}
	if !l.allow(b, at) {
		t.Fatal("another IP was refused for the first one's clicks")
	}
	// The next minute starts from zero.
	if !l.allow(a, at.Add(50*time.Second)) {
		t.Fatal("the next minute did not reset the count")
	}
}

func TestClickLimiterGroupsIPv6By64(t *testing.T) {
	l := newClickLimiter(2, time.Minute)
	at := time.Now()
	same1, same2 := net.ParseIP("2001:db8:aa:1::1"), net.ParseIP("2001:db8:aa:1:ffff::9")
	other := net.ParseIP("2001:db8:aa:2::1")
	if !l.allow(same1, at) || !l.allow(same2, at) {
		t.Fatal("first two clicks of a /64 refused")
	}
	if l.allow(same1, at) {
		t.Error("a /64 got past its limit by changing the last 64 bits")
	}
	if !l.allow(other, at) {
		t.Error("the next /64 was refused")
	}
	// An IPv4-mapped IPv6 address is the IPv4 address.
	if limitKey(net.ParseIP("::ffff:198.51.100.1")) != limitKey(net.ParseIP("198.51.100.1")) {
		t.Error("IPv4-mapped IPv6 counted apart from its IPv4 address")
	}
}

func TestClickLimiterOff(t *testing.T) {
	var nilLimiter *clickLimiter
	off := newClickLimiter(0, time.Minute)
	on := newClickLimiter(1, time.Minute)
	ip := net.ParseIP("198.51.100.1")
	for i := 0; i < 5; i++ {
		if !nilLimiter.allow(ip, time.Now()) || !off.allow(ip, time.Now()) || !on.allow(nil, time.Now()) {
			t.Fatal("a nil limiter, a zero limit or an unknown IP limited a click")
		}
	}
}

func TestClickLimiterConcurrent(t *testing.T) {
	l := newClickLimiter(1000, time.Minute)
	at := time.Now()
	ip := net.ParseIP("198.51.100.1")
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 250; i++ {
				if l.allow(ip, at) {
					mu.Lock()
					allowed++
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	if allowed != 1000 {
		t.Errorf("allowed %d of 2000 concurrent clicks, want exactly 1000", allowed)
	}
}

func BenchmarkClickLimiter(b *testing.B) {
	l := newClickLimiter(600, time.Minute)
	ips := make([]net.IP, 1024)
	for i := range ips {
		ips[i] = net.IPv4(198, 51, byte(i>>8), byte(i))
	}
	now := time.Now()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			l.allow(ips[i%len(ips)], now)
			i++
		}
	})
}

// SEC-4 through the real handlers: over the limit, /t/ sends the visitor
// to the campaign's fallback and /lc/, /pc/ to the destination their token
// carries — and nothing is recorded but a count per campaign for the
// campaign page (REL-2). Under it, nothing changes.
func TestOverTheClickLimitNothingIsRecorded(t *testing.T) {
	pool := testPool(t)
	fx := seedFlowFixture(t, pool, nil, nil)
	limited := &clicklimit.Counter{DB: pool}
	h, sink, closeEvents := newLandingHandler(t, pool, true, func(h *Handler) {
		h.clicks = newClickLimiter(1, time.Minute)
		h.limited = limited
	})

	first := get(h, fx.host, "/t/"+fx.slug)
	if first.Code != http.StatusFound || first.Header().Get("Location") != landingTestOffer {
		t.Fatalf("first click: %d → %q, want the flow's destination", first.Code, first.Header().Get("Location"))
	}
	second := get(h, fx.host, "/t/"+fx.slug)
	if second.Code != http.StatusFound || second.Header().Get("Location") != "https://fallback.example" {
		t.Fatalf("over the limit: %d → %q, want the campaign fallback", second.Code, second.Header().Get("Location"))
	}

	signer := landinglink.NewSigner(testLinkKey)
	claims := landinglink.Claims{OrganizationID: fx.orgID, CampaignID: idgen.New(), ClickID: "click-1", LandingID: idgen.New(),
		Destination: "https://offer.example/x?c=click-1", Host: fx.host, IssuedAt: time.Now().Unix()}
	lc := get(h, fx.host, landClickPath+signer.Sign(claims))
	claims.PostlandingID = idgen.New()
	pc := get(h, fx.host, postClickPath+signer.Sign(claims))
	for name, rec := range map[string]int{"/lc/": lc.Code, "/pc/": pc.Code} {
		if rec != http.StatusFound {
			t.Errorf("%s over the limit = %d, want 302", name, rec)
		}
	}
	if lc.Header().Get("Location") != claims.Destination || pc.Header().Get("Location") != claims.Destination {
		t.Errorf("over the limit: /lc/ → %q, /pc/ → %q, want the token's destination", lc.Header().Get("Location"), pc.Header().Get("Location"))
	}

	if err := limited.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int64{}
	rows, err := pool.Query(context.Background(), `SELECT campaign_id, clicks FROM ip_limited_clicks WHERE organization_id = $1 AND day = (now() AT TIME ZONE 'UTC')::date`, fx.orgID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var c string
		var n int64
		if err := rows.Scan(&c, &n); err != nil {
			t.Fatal(err)
		}
		counts[c] = n
	}
	rows.Close()
	if len(counts) != 2 || counts[fx.campaignID] != 1 || counts[claims.CampaignID] != 2 {
		t.Errorf("limited clicks = %v, want 1 for the campaign (/t/) and 2 for the token's (/lc/, /pc/)", counts)
	}

	closeEvents()
	if len(sink.events) != 1 {
		types := []string{}
		for _, e := range sink.events {
			types = append(types, string(e.Type))
		}
		t.Fatalf("recorded %v, want only the first click", types)
	}
}
