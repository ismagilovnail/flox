package routingstore_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/campaign"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/routing"
	"github.com/ismagilovnail/flox/apps/internal/routingstore"
	"github.com/ismagilovnail/flox/apps/internal/streamset"
	"github.com/ismagilovnail/flox/apps/internal/trafficsource"
)

// benchFixture is a realistically-shaped campaign seeded through the real
// write paths (trafficsource/campaign/streamset services), not raw SQL for
// the routing objects — so the shape on disk (row count, index usage)
// matches what an actual operator's campaign produces. Only domains/
// tracking_links, which have no service layer yet (Phase 14 territory),
// are inserted directly.
type benchFixture struct {
	Host       string
	Slug       string
	OrgID      string
	CampaignID string
}

func seedBenchFixture(tb testing.TB, ctx context.Context, pool *pgxpool.Pool, streamSetCount int) benchFixture {
	tb.Helper()

	orgID := idgen.New()
	mustExec(tb, ctx, pool, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, orgID, "Bench Org "+orgID)
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
	// A last, catch-all set so a benchmark request actually reaches the
	// weighted draw instead of always falling through to the fallback.
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
	mustExec(tb, ctx, pool, `INSERT INTO domains (id, organization_id, domain, status, purpose) VALUES ($1, $2, $3, 'active', '{tracking}')`,
		domainID, orgID, host)
	trackingLinkID := idgen.New()
	mustExec(tb, ctx, pool, `INSERT INTO tracking_links (id, organization_id, campaign_id, domain_id, slug) VALUES ($1, $2, $3, $4, $5)`,
		trackingLinkID, orgID, camp.ID, domainID, slug)

	return benchFixture{Host: host, Slug: slug, OrgID: orgID, CampaignID: camp.ID}
}

func mustExec(tb testing.TB, ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) {
	tb.Helper()
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		tb.Fatalf("exec %q: %v", sql, err)
	}
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

// BenchmarkLoadRoutingConfig measures routingstore.Store.LoadRoutingConfig
// against a real Postgres — the four sequential queries (campaign, stream
// sets, filter groups+conditions, flows) apps/tracker's Handler.track runs
// on every single click, with no cache in front of them today. This is the
// number Phase 31's benchmark list (§56) actually cares about: it is the
// single biggest piece of the §41 hot-path budget that isn't pure CPU.
func BenchmarkLoadRoutingConfig(b *testing.B) {
	pool := benchPool(b)
	ctx := context.Background()
	fx := seedBenchFixture(b, ctx, pool, 5)
	store := routingstore.New(pool)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := store.LoadRoutingConfig(ctx, fx.OrgID, fx.CampaignID); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkResolveTrackingLink measures the other Postgres round trip on
// the hot path: the (domain, slug) -> campaign lookup, a single indexed
// query (tracking_links_domain_id_slug_key + domains_domain_key).
func BenchmarkResolveTrackingLink(b *testing.B) {
	pool := benchPool(b)
	ctx := context.Background()
	fx := seedBenchFixture(b, ctx, pool, 5)
	store := routingstore.New(pool)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := store.ResolveTrackingLink(ctx, fx.Host, fx.Slug); err != nil {
			b.Fatal(err)
		}
	}
}
