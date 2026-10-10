package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/ismagilovnail/flox/apps/internal/clientip"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/ratelimit"
)

// countingNetworks counts lookups, to show a blocked IP never reaches
// Postgres.
type countingNetworks struct {
	staticNetworks
	calls *int
}

func (c countingNetworks) ByID(ctx context.Context, id string) (conversion.Network, error) {
	*c.calls++
	return c.staticNetworks.ByID(ctx, id)
}

// Tech audit 2026-10-06, P4: refused postbacks are limited per client IP;
// other outcomes do not count, and a blocked IP is answered without a
// network lookup.
func TestPostbackRefusalsAreLimitedPerIP(t *testing.T) {
	networkID := idgen.New()
	calls := 0
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver, err := clientip.New(nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	h := &PostbackHandler{
		networks: countingNetworks{staticNetworks{conversion.Network{ID: networkID, OrganizationID: idgen.New(), PostbackSecretHash: hashFor("s3cret")}}, &calls},
		failures: ratelimit.New(nil, logger).Failures("postback-failures", 3, time.Minute),
		clientIP: resolver,
		logger:   logger,
	}
	r := chi.NewRouter()
	h.Register(r)
	get := func(ip, query string) int {
		req := httptest.NewRequest(http.MethodGet, "/postback/"+networkID+"?"+query, nil)
		req.RemoteAddr = ip + ":1234"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	// A 400 from a network with the right secret is not a refusal.
	for i := 0; i < 5; i++ {
		if code := get("10.0.0.1", "secret=s3cret&click_id=c&status=sale&revenue=NaN"); code != http.StatusBadRequest {
			t.Fatalf("bad revenue #%d = %d, want 400", i+1, code)
		}
	}
	for i := 0; i < 2; i++ {
		if code := get("10.0.0.1", "secret=wrong"); code != http.StatusUnauthorized {
			t.Fatalf("wrong secret #%d = %d, want 401", i+1, code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/postback/not-an-id", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	r.ServeHTTP(httptest.NewRecorder(), req)

	before := testutil.ToFloat64(metrics.PostbackIncomingTotal.WithLabelValues("rate_limited"))
	lookups := calls
	if code := get("10.0.0.1", "secret=s3cret&click_id=c&status=sale&revenue=NaN"); code != http.StatusTooManyRequests {
		t.Fatalf("after 3 refusals = %d, want 429", code)
	}
	if calls != lookups {
		t.Fatal("a blocked IP still looked the network up")
	}
	if got := testutil.ToFloat64(metrics.PostbackIncomingTotal.WithLabelValues("rate_limited")) - before; got != 1 {
		t.Fatalf("postback_incoming_total{result=rate_limited} grew by %v, want 1", got)
	}
	if code := get("10.0.0.2", "secret=wrong"); code != http.StatusUnauthorized {
		t.Fatalf("another IP = %d, want 401 (own count)", code)
	}
}
