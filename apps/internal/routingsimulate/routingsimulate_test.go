package routingsimulate_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/routing"
	"github.com/ismagilovnail/flox/apps/internal/routingsimulate"
	"github.com/ismagilovnail/flox/apps/internal/routingstore"
	"github.com/ismagilovnail/flox/apps/internal/streamset"
)

func redirectFlow(name string, weight int, url string) streamset.FlowInput {
	return streamset.FlowInput{
		Name: name, Active: true, Weight: weight,
		Destination: streamset.Destination{Kind: routing.DestinationRedirect, URL: url},
	}
}

func countryIsGroup(code string) streamset.FilterNode {
	return streamset.FilterNode{
		Kind:   streamset.NodeGroup,
		Joiner: routing.JoinAND,
		Children: []streamset.FilterNode{
			{Kind: streamset.NodeCondition, Field: routing.FieldCountry, Operator: routing.OpIs, Value: code},
		},
	}
}

func emptyGroup() streamset.FilterNode {
	return streamset.FilterNode{Kind: streamset.NodeGroup, Joiner: routing.JoinAND}
}

func newService(pool *pgxpool.Pool) *routingsimulate.Service {
	return routingsimulate.NewService(routingstore.New(pool), &routing.Engine{})
}

func TestSimulateMatchesFilterAndSelectsFlow(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID)

	streamSetSvc := streamset.NewService(streamset.NewRepository(pool))
	created, err := streamSetSvc.Create(ctx, orgID, campaignID, streamset.CreateInput{
		Name:       "US traffic",
		RootFilter: countryIsGroup("US"),
		Flows:      []streamset.FlowInput{redirectFlow("Primary", 100, "https://example.com/us")},
	})
	if err != nil {
		t.Fatalf("seeding stream set: %v", err)
	}

	svc := newService(pool)
	resp, err := svc.Simulate(ctx, orgID, campaignID, map[string]string{"country": "US"})
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}

	if resp.MatchedStreamSet == nil || resp.MatchedStreamSet.StreamSetID != created.ID {
		t.Fatalf("MatchedStreamSet = %+v, want stream set %s", resp.MatchedStreamSet, created.ID)
	}
	if len(resp.FlowCandidates) != 1 || !resp.FlowCandidates[0].Selected {
		t.Fatalf("FlowCandidates = %+v, want one selected flow", resp.FlowCandidates)
	}
	if resp.Destination.URL != "https://example.com/us" || resp.Destination.Label != "Redirect" {
		t.Fatalf("Destination = %+v, want the flow's redirect URL labeled Redirect", resp.Destination)
	}
}

func TestSimulateNoMatchFallsBackToCampaignFallback(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID) // fallback_url = https://example.com/fallback

	streamSetSvc := streamset.NewService(streamset.NewRepository(pool))
	if _, err := streamSetSvc.Create(ctx, orgID, campaignID, streamset.CreateInput{
		Name:       "US traffic",
		RootFilter: countryIsGroup("US"),
		Flows:      []streamset.FlowInput{redirectFlow("Primary", 100, "https://example.com/us")},
	}); err != nil {
		t.Fatalf("seeding stream set: %v", err)
	}

	svc := newService(pool)
	resp, err := svc.Simulate(ctx, orgID, campaignID, map[string]string{"country": "CA"})
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}

	if resp.MatchedStreamSet != nil {
		t.Fatalf("MatchedStreamSet = %+v, want nil (no stream set should match CA)", resp.MatchedStreamSet)
	}
	if len(resp.StreamSetEvaluations) != 1 || resp.StreamSetEvaluations[0].Matched {
		t.Fatalf("StreamSetEvaluations = %+v, want one unmatched evaluation", resp.StreamSetEvaluations)
	}
	if resp.Destination.URL != "https://example.com/fallback" || resp.Destination.Label != "Campaign fallback" {
		t.Fatalf("Destination = %+v, want the campaign fallback labeled Campaign fallback", resp.Destination)
	}
	// A real bug caught during manual browser verification: no stream set
	// matching leaves routing.Explanation.FlowCandidates nil (no draw ever
	// happens), which encodes as JSON null and crashed the frontend's
	// unconditional flowCandidates.some(...) call. The response must carry
	// a real, if empty, array here — nil is not an acceptable substitute.
	if resp.FlowCandidates == nil {
		t.Fatalf("FlowCandidates = nil, want a non-nil empty slice (encodes as JSON null otherwise)")
	}
	if len(resp.FlowCandidates) != 0 {
		t.Fatalf("FlowCandidates = %+v, want empty", resp.FlowCandidates)
	}
}

func TestSimulateAmbiguousTieReturnsValidationError(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID)

	streamSetSvc := streamset.NewService(streamset.NewRepository(pool))
	if _, err := streamSetSvc.Create(ctx, orgID, campaignID, streamset.CreateInput{
		Name:       "Catch-all",
		RootFilter: emptyGroup(),
		Flows: []streamset.FlowInput{
			redirectFlow("A", 50, "https://example.com/a"),
			redirectFlow("B", 50, "https://example.com/b"),
		},
	}); err != nil {
		t.Fatalf("seeding stream set: %v", err)
	}

	svc := newService(pool)
	_, err := svc.Simulate(ctx, orgID, campaignID, map[string]string{})

	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "validation" {
		t.Fatalf("Simulate with no attributes and two tied weighted flows: err = %v, want a validation apierror", err)
	}
}

func TestSimulateStickyNoteReflectsCampaignConfig(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID)

	streamSetSvc := streamset.NewService(streamset.NewRepository(pool))
	if _, err := streamSetSvc.Create(ctx, orgID, campaignID, streamset.CreateInput{
		Name:       "Catch-all",
		RootFilter: emptyGroup(),
		Flows:      []streamset.FlowInput{redirectFlow("A", 100, "https://example.com/a")},
	}); err != nil {
		t.Fatalf("seeding stream set: %v", err)
	}

	svc := newService(pool)

	resp, err := svc.Simulate(ctx, orgID, campaignID, map[string]string{})
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	if resp.StickyNote == "" || resp.StickyNote[:len("Sticky flow isn't enabled")] != "Sticky flow isn't enabled" {
		t.Fatalf("StickyNote = %q, want the sticky-disabled message", resp.StickyNote)
	}

	if _, err := pool.Exec(ctx, `UPDATE campaigns SET sticky_flow = true WHERE id = $1`, campaignID); err != nil {
		t.Fatalf("enabling sticky flow: %v", err)
	}

	resp, err = svc.Simulate(ctx, orgID, campaignID, map[string]string{})
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	if resp.StickyNote == "" || resp.StickyNote[:len("This campaign has sticky flow enabled")] != "This campaign has sticky flow enabled" {
		t.Fatalf("StickyNote = %q, want the sticky-enabled message", resp.StickyNote)
	}
}

func TestSimulateCrossTenantIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgA)

	svc := newService(pool)
	_, err := svc.Simulate(ctx, orgB, campaignID, map[string]string{})

	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "not_found" {
		t.Fatalf("Simulate against another org's campaign: err = %v, want a not_found apierror", err)
	}
}

func mustPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connecting to database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func seedOrg(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, id, "Test Org "+id)
	if err != nil {
		t.Fatalf("seeding organization: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, id)
	})
	return id
}

func seedCampaign(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID string) string {
	t.Helper()
	sourceID := idgen.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO traffic_sources (id, organization_id, name, type) VALUES ($1, $2, $3, $4)`,
		sourceID, orgID, "Test Source", "Facebook",
	)
	if err != nil {
		t.Fatalf("seeding traffic source: %v", err)
	}
	id := idgen.New()
	_, err = pool.Exec(ctx,
		`INSERT INTO campaigns (id, organization_id, traffic_source_id, name, fallback_url) VALUES ($1, $2, $3, $4, $5)`,
		id, orgID, sourceID, "Test Campaign", "https://example.com/fallback",
	)
	if err != nil {
		t.Fatalf("seeding campaign: %v", err)
	}
	return id
}
