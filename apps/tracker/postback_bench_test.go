package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/eventbuf"
	"github.com/ismagilovnail/flox/apps/internal/eventmapping"
	"github.com/ismagilovnail/flox/apps/internal/network"
	"github.com/ismagilovnail/flox/apps/internal/postback"
	"github.com/ismagilovnail/flox/apps/internal/postbacklog"
)

// BenchmarkPostback measures apps/tracker's incoming POST /postback/{id}
// end to end (secret auth, network lookup, status mapping, progression
// check, attribution, FX, durable write, delivery enqueue) — everything
// tracker/postback.go's own doc comment says is deliberately NOT on the
// §41 redirect budget, so unlike BenchmarkTrack this has no p50/p95
// target to compare against. It exists so a future regression here (e.g.
// an accidentally-synchronous call added to Record) is visible in
// docs/performance.md's numbers rather than only showing up as a slow
// worker queue in production.
func BenchmarkPostback(b *testing.B) {
	pool := benchPool(b)
	ctx := context.Background()
	fx := seedBenchFixture(b, ctx, pool, 1)

	netSvc := network.NewService(network.NewRepository(pool))
	netResult, err := netSvc.Create(ctx, fx.OrgID, network.CreateInput{
		Name:        "Bench Network",
		PostbackURL: "https://network.example/postback?click_id={click_id}",
	})
	if err != nil {
		b.Fatalf("seeding network: %v", err)
	}

	mapSvc := eventmapping.NewService(eventmapping.NewRepository(pool))
	if _, err := mapSvc.Create(ctx, fx.OrgID, eventmapping.CreateInput{
		NetworkID:     netResult.Network.ID,
		NetworkStatus: "approved",
		FloxStatus:    "CPA_ACCEPT",
	}); err != nil {
		b.Fatalf("seeding event mapping: %v", err)
	}

	events := eventbuf.New(eventbuf.DiscardSink{}, slog.New(slog.NewTextHandler(io.Discard, nil)), eventbuf.Config{})
	b.Cleanup(events.Close)

	deliveries := postback.NewEnqueuer(postback.NewPostgresStore(pool), slog.New(slog.NewTextHandler(io.Discard, nil)))
	attemptLog := postbacklog.NewConversionAttemptLogger(postbacklog.NewPostgresQueue(pool, slog.New(slog.NewTextHandler(io.Discard, nil))))
	attributionSvc := attribution.NewService(attribution.NewMemoryResolver())

	handler := &PostbackHandler{
		networks: conversion.NewPostgresNetworkLookup(pool),
		service: conversion.NewService(
			conversion.NewPostgresMapper(pool),
			conversion.NewPostgresStore(pool),
			conversion.NewPostgresFX(pool),
			attributionSvc,
			events,
			deliveries,
			attemptLog,
		),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	r := chi.NewRouter()
	handler.Register(r)

	target := "http://tracker.example/postback/" + netResult.Network.ID

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Every call is a distinct click_id so the progression check
		// (§45) always takes the "first postback for this click" branch —
		// the common case in real traffic, where a HOLD->ACCEPT->REDEP
		// chain is a small minority of clicks. AcceptDuplicates on the
		// seeded network isn't set, but a fresh click_id per iteration
		// means dedup is never actually exercised, which is fine: dedup's
		// cost is one more indexed lookup already covered by LastStatus.
		form := url.Values{
			"secret":   {netResult.PostbackSecret},
			"click_id": {"bench-click-" + strconv.Itoa(i)},
			"status":   {"approved"},
			"revenue":  {"12.34"},
			"currency": {"USD"},
		}
		req := httptest.NewRequest(http.MethodPost, target, nil)
		req.URL.RawQuery = form.Encode()
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	}
}
