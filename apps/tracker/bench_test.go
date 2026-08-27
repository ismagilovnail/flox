package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/campaign"
	"github.com/ismagilovnail/flox/apps/internal/classifier"
	"github.com/ismagilovnail/flox/apps/internal/eventbuf"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/routing"
	"github.com/ismagilovnail/flox/apps/internal/routingstore"
	"github.com/ismagilovnail/flox/apps/internal/streamset"
	"github.com/ismagilovnail/flox/apps/internal/trafficsource"
)

// benchFixture mirrors internal/routingstore/bench_test.go's own
// seedBenchFixture — duplicated rather than shared, matching this repo's
// existing per-package mustPool/seedOrg convention (see e.g.
// internal/network/network_test.go), since apps/tracker (package main)
// cannot import another package's _test.go helpers anyway.
type benchFixture struct {
	Host       string
	Slug       string
	OrgID      string
	CampaignID string
}

func seedBenchFixture(tb testing.TB, ctx context.Context, pool *pgxpool.Pool, streamSetCount int) benchFixture {
	tb.Helper()

	orgID := idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, orgID, "Bench Org "+orgID); err != nil {
		tb.Fatalf("seeding organization: %v", err)
	}
	tb.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID)
	})

	tsSvc := trafficsource.NewService(trafficsource.NewRepository(pool))
	ts, err := tsSvc.Create(ctx, orgID, trafficsource.CreateInput{
		Name:             "Bench Source",
		Type:             "Facebook",
		TrackingTemplate: "https://track.example.com/click?clickid={click_id}",
		CostIntegration:  trafficsource.CostIntegrationNone,
	})
	if err != nil {
		tb.Fatalf("seeding traffic source: %v", err)
	}

	campSvc := campaign.NewService(campaign.NewRepository(pool))
	camp, err := campSvc.Create(ctx, orgID, campaign.CreateInput{
		TrafficSourceID: ts.ID,
		Name:            "Bench Campaign",
		FallbackURL:     "https://fallback.example",
	})
	if err != nil {
		tb.Fatalf("seeding campaign: %v", err)
	}
	if _, err := campSvc.Activate(ctx, orgID, camp.ID); err != nil {
		tb.Fatalf("activating campaign: %v", err)
	}

	ssSvc := streamset.NewService(streamset.NewRepository(pool))
	for i := 0; i < streamSetCount; i++ {
		_, err := ssSvc.Create(ctx, orgID, camp.ID, streamset.CreateInput{
			Name:        "Bench Set",
			Priority:    i + 1,
			FallbackURL: "https://set-fallback.example",
			RootFilter: streamset.FilterNode{
				Kind:   streamset.NodeGroup,
				Joiner: routing.JoinAND,
				Children: []streamset.FilterNode{
					{Kind: streamset.NodeCondition, Field: routing.FieldCountry, Operator: routing.OpIn, Value: "US,CA,GB,DE,FR"},
					{
						Kind:   streamset.NodeGroup,
						Joiner: routing.JoinOR,
						Children: []streamset.FilterNode{
							{Kind: streamset.NodeCondition, Field: routing.FieldDevice, Operator: routing.OpIs, Value: "mobile"},
							{Kind: streamset.NodeCondition, Field: routing.FieldOS, Operator: routing.OpIs, Value: "android"},
						},
					},
					{Kind: streamset.NodeCondition, Field: routing.FieldBot, Operator: routing.OpIs, Value: "0"},
				},
			},
			Flows: []streamset.FlowInput{
				{Name: "Flow A", Active: true, Weight: 50, Destination: streamset.Destination{Kind: routing.DestinationRedirect, URL: "https://a.example"}},
				{Name: "Flow B", Active: true, Weight: 30, Destination: streamset.Destination{Kind: routing.DestinationRedirect, URL: "https://b.example"}},
				{Name: "Flow C", Active: true, Weight: 20, Destination: streamset.Destination{Kind: routing.DestinationRedirect, URL: "https://c.example"}},
			},
		})
		if err != nil {
			tb.Fatalf("seeding stream set %d: %v", i, err)
		}
	}
	if _, err := ssSvc.Create(ctx, orgID, camp.ID, streamset.CreateInput{
		Name:        "Bench Catch-All",
		Priority:    streamSetCount + 1,
		FallbackURL: "https://set-fallback.example",
		RootFilter: streamset.FilterNode{
			Kind:   streamset.NodeGroup,
			Joiner: routing.JoinOR,
			Children: []streamset.FilterNode{
				{Kind: streamset.NodeCondition, Field: routing.FieldCountry, Operator: routing.OpExists},
			},
		},
		Flows: []streamset.FlowInput{
			{Name: "Catch-All Flow", Active: true, Weight: 100, Destination: streamset.Destination{Kind: routing.DestinationRedirect, URL: "https://catchall.example"}},
		},
	}); err != nil {
		tb.Fatalf("seeding catch-all stream set: %v", err)
	}

	host := "bench-" + idgen.New() + ".example"
	slug := "bench"
	domainID := idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO domains (id, organization_id, domain, status, purpose) VALUES ($1, $2, $3, 'active', '{tracking}')`,
		domainID, orgID, host); err != nil {
		tb.Fatalf("seeding domain: %v", err)
	}
	trackingLinkID := idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO tracking_links (id, organization_id, campaign_id, domain_id, slug) VALUES ($1, $2, $3, $4, $5)`,
		trackingLinkID, orgID, camp.ID, domainID, slug); err != nil {
		tb.Fatalf("seeding tracking link: %v", err)
	}

	return benchFixture{Host: host, Slug: slug, OrgID: orgID, CampaignID: camp.ID}
}

func benchPool(b *testing.B) *pgxpool.Pool {
	b.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		b.Skip("DATABASE_URL not set, skipping DB-backed benchmark")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		b.Fatalf("connecting to database: %v", err)
	}
	b.Cleanup(pool.Close)
	return pool
}

// BenchmarkTrack is the §41/§56 benchmark itself: the whole
// Handler.track hot path — resolve tracking link, load routing config,
// classify, route, enqueue (discarded, not written — isolates this from
// Postgres event-queue write cost, which Phase 25's worker already owns
// independently) — through a real net/http mux against a real seeded
// Postgres. This is the number docs/performance.md compares against the
// tracking p50 < 20ms / p95 < 50ms target.
func BenchmarkTrack(b *testing.B) {
	pool := benchPool(b)
	ctx := context.Background()
	fx := seedBenchFixture(b, ctx, pool, 5)

	events := eventbuf.New(eventbuf.DiscardSink{}, slog.New(slog.NewTextHandler(io.Discard, nil)), eventbuf.Config{})
	b.Cleanup(events.Close)

	handler := &Handler{
		store:      routingstore.New(pool),
		classifier: classifier.New(nil, nil, nil),
		engine:     &routing.Engine{},
		events:     events,
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	r := chi.NewRouter()
	handler.Register(r)

	target := "http://" + fx.Host + "/t/" + fx.Slug

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Host = fx.Host
		req.RemoteAddr = "198.51.100.7:12345"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusFound {
			b.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
		}
	}
}

// BenchmarkTrack_Parallel runs the same request concurrently across
// GOMAXPROCS goroutines — real traffic is concurrent, not serial, and the
// pgxpool connection pool (internal/postgres.NewPool, default-sized) is
// the resource most likely to make p95 diverge from p50 under load, which
// a strictly serial b.N loop can never surface.
func BenchmarkTrack_Parallel(b *testing.B) {
	pool := benchPool(b)
	ctx := context.Background()
	fx := seedBenchFixture(b, ctx, pool, 5)

	events := eventbuf.New(eventbuf.DiscardSink{}, slog.New(slog.NewTextHandler(io.Discard, nil)), eventbuf.Config{})
	b.Cleanup(events.Close)

	handler := &Handler{
		store:      routingstore.New(pool),
		classifier: classifier.New(nil, nil, nil),
		engine:     &routing.Engine{},
		events:     events,
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	r := chi.NewRouter()
	handler.Register(r)

	target := "http://" + fx.Host + "/t/" + fx.Slug

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			req.Host = fx.Host
			req.RemoteAddr = "198.51.100.7:12345"
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusFound {
				b.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
			}
		}
	})
}
