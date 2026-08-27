// Command loadtestseed seeds (and tears down) a real campaign for Phase
// 31's manual load test against a running apps/tracker binary (§56):
// "Load test: sustained clicks with zero event loss." It exists because
// that load test needs a real (domain, slug) resolving through a real
// multi-stream-set routing config in Postgres — an httptest-in-process
// benchmark (apps/tracker/bench_test.go) can't exercise the actual
// listening HTTP server, TCP stack, or a real load generator (vegeta).
//
// Not wired into any service; run manually against DATABASE_URL:
//
//	go run ./cmd/loadtestseed seed              # prints {"orgId","host","slug"} JSON
//	go run ./cmd/loadtestseed cleanup <orgId>    # deletes everything the seed created
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/campaign"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/routing"
	"github.com/ismagilovnail/flox/apps/internal/streamset"
	"github.com/ismagilovnail/flox/apps/internal/trafficsource"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: loadtestseed seed | loadtestseed cleanup <orgId>")
		os.Exit(2)
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL not set")
		os.Exit(1)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "connecting:", err)
		os.Exit(1)
	}
	defer pool.Close()

	switch os.Args[1] {
	case "seed":
		if err := seed(ctx, pool); err != nil {
			fmt.Fprintln(os.Stderr, "seed:", err)
			os.Exit(1)
		}
	case "cleanup":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: loadtestseed cleanup <orgId>")
			os.Exit(2)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
			os.Exit(1)
		}
		fmt.Println("cleaned up", os.Args[2])
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", os.Args[1])
		os.Exit(2)
	}
}

type fixture struct {
	OrgID string `json:"orgId"`
	Host  string `json:"host"`
	Slug  string `json:"slug"`
}

// seed builds the same shape apps/tracker/bench_test.go's own
// seedBenchFixture does — 5 real stream sets (nested AND/OR filters,
// weighted flows) plus a catch-all, through the real write path
// (trafficsource/campaign/streamset services) — so the load test's
// routing decision cost matches what BenchmarkTrack already measured
// in-process, and the two numbers (in-process benchmark vs. real HTTP
// load test) are actually comparable.
func seed(ctx context.Context, pool *pgxpool.Pool) error {
	orgID := idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, orgID, "Loadtest Org "+orgID); err != nil {
		return fmt.Errorf("seeding organization: %w", err)
	}

	tsSvc := trafficsource.NewService(trafficsource.NewRepository(pool))
	ts, err := tsSvc.Create(ctx, orgID, trafficsource.CreateInput{
		Name:             "Loadtest Source",
		Type:             "Facebook",
		TrackingTemplate: "https://track.example.com/click?clickid={click_id}",
		CostIntegration:  trafficsource.CostIntegrationNone,
	})
	if err != nil {
		return fmt.Errorf("seeding traffic source: %w", err)
	}

	campSvc := campaign.NewService(campaign.NewRepository(pool))
	camp, err := campSvc.Create(ctx, orgID, campaign.CreateInput{
		TrafficSourceID: ts.ID,
		Name:            "Loadtest Campaign",
		FallbackURL:     "https://fallback.example",
	})
	if err != nil {
		return fmt.Errorf("seeding campaign: %w", err)
	}
	if _, err := campSvc.Activate(ctx, orgID, camp.ID); err != nil {
		return fmt.Errorf("activating campaign: %w", err)
	}

	ssSvc := streamset.NewService(streamset.NewRepository(pool))
	const streamSetCount = 5
	for i := 0; i < streamSetCount; i++ {
		_, err := ssSvc.Create(ctx, orgID, camp.ID, streamset.CreateInput{
			Name:        "Loadtest Set",
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
			return fmt.Errorf("seeding stream set %d: %w", i, err)
		}
	}
	if _, err := ssSvc.Create(ctx, orgID, camp.ID, streamset.CreateInput{
		Name:        "Loadtest Catch-All",
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
		return fmt.Errorf("seeding catch-all stream set: %w", err)
	}

	host := "loadtest-" + idgen.New() + ".example"
	slug := "loadtest"
	domainID := idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO domains (id, organization_id, domain, status, purpose) VALUES ($1, $2, $3, 'active', '{tracking}')`,
		domainID, orgID, host); err != nil {
		return fmt.Errorf("seeding domain: %w", err)
	}
	trackingLinkID := idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO tracking_links (id, organization_id, campaign_id, domain_id, slug) VALUES ($1, $2, $3, $4, $5)`,
		trackingLinkID, orgID, camp.ID, domainID, slug); err != nil {
		return fmt.Errorf("seeding tracking link: %w", err)
	}

	return json.NewEncoder(os.Stdout).Encode(fixture{OrgID: orgID, Host: host, Slug: slug})
}
