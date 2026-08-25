package streamset_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/routing"
	"github.com/ismagilovnail/flox/apps/internal/streamset"
)

func redirectFlow(name string, weight int) streamset.FlowInput {
	return streamset.FlowInput{
		Name: name, Active: true, Weight: weight,
		Destination: streamset.Destination{Kind: routing.DestinationRedirect, URL: "https://example.com/redirect"},
	}
}

func TestCreateGetUpdateDelete(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID)

	svc := streamset.NewService(streamset.NewRepository(pool))

	root := streamset.FilterNode{
		Kind:   streamset.NodeGroup,
		Joiner: routing.JoinAND,
		Children: []streamset.FilterNode{
			{Kind: streamset.NodeCondition, Field: routing.FieldCountry, Operator: routing.OpIs, Value: "US"},
			{
				Kind: streamset.NodeGroup, Joiner: routing.JoinOR,
				Children: []streamset.FilterNode{
					{Kind: streamset.NodeCondition, Field: routing.FieldOS, Operator: routing.OpIs, Value: "android"},
					{Kind: streamset.NodeCondition, Field: routing.FieldOS, Operator: routing.OpIs, Value: "ios"},
				},
			},
		},
	}

	created, err := svc.Create(ctx, orgID, campaignID, streamset.CreateInput{
		Name:       "Mobile — Tier 1",
		RootFilter: root,
		Flows:      []streamset.FlowInput{redirectFlow("Primary", 100)},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Status != streamset.StatusActive {
		t.Fatalf("Status = %q, want active", created.Status)
	}
	if created.Priority != 1 {
		t.Fatalf("Priority = %d, want 1 (first stream set for this campaign)", created.Priority)
	}
	if len(created.RootFilter.Children) != 2 {
		t.Fatalf("RootFilter.Children = %d, want 2", len(created.RootFilter.Children))
	}
	nested := created.RootFilter.Children[1]
	if nested.Kind != streamset.NodeGroup || len(nested.Children) != 2 {
		t.Fatalf("nested group = %+v, want a 2-child OR group", nested)
	}
	if len(created.Flows) != 1 || created.Flows[0].ID == "" {
		t.Fatalf("Flows = %+v, want one flow with a generated id", created.Flows)
	}

	got, err := svc.Get(ctx, orgID, campaignID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.RootFilter.Children) != 2 {
		t.Fatalf("Get round-trip RootFilter.Children = %d, want 2", len(got.RootFilter.Children))
	}

	newName := "Mobile — Tier 1 (renamed)"
	updated, err := svc.Update(ctx, orgID, campaignID, created.ID, streamset.UpdateInput{Name: &newName})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != newName {
		t.Fatalf("Name = %q, want %q", updated.Name, newName)
	}
	if len(updated.RootFilter.Children) != 2 || len(updated.Flows) != 1 {
		t.Fatalf("a name-only update touched the tree/flows: %+v", updated)
	}

	if err := svc.Delete(ctx, orgID, campaignID, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, orgID, campaignID, created.ID); err == nil {
		t.Fatal("Get after Delete succeeded, want not-found")
	}

	var groupCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM filter_groups WHERE stream_set_id = $1`, created.ID).Scan(&groupCount); err != nil {
		t.Fatalf("counting filter_groups after delete: %v", err)
	}
	if groupCount != 0 {
		t.Fatalf("filter_groups count after delete = %d, want 0 (CASCADE)", groupCount)
	}
}

// TestEmptyRootGroupChildrenIsNeverNilOnTheWire guards a real bug caught
// during manual browser verification of the routing simulator phase: an
// empty top-level filter group ("no filters — matches all traffic", a
// normal, UI-supported configuration per stream-set-form-sheet.tsx's own
// help text) round-tripped through the repository's build() (the
// List/Get read path) with a nil Children slice. Combined with the JSON
// struct tag's now-removed `omitempty`, that encoded as `"children":
// null` — and the frontend's hydrateFilterNode calls
// node.children.map(...) unconditionally, so loading any campaign with
// such a stream set crashed the whole detail page, not just the Routing
// Simulator tab. Children is a real (JSON-decoded) non-nil empty slice
// here, matching what dehydrateFilterNode always sends on the wire — the
// bug was specifically in the re-read path, not the create/update
// echo-back.
func TestEmptyRootGroupChildrenIsNeverNilOnTheWire(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID)

	svc := streamset.NewService(streamset.NewRepository(pool))
	created, err := svc.Create(ctx, orgID, campaignID, streamset.CreateInput{
		Name:       "Catch-all",
		RootFilter: streamset.FilterNode{Kind: streamset.NodeGroup, Joiner: routing.JoinAND, Children: []streamset.FilterNode{}},
		Flows:      []streamset.FlowInput{redirectFlow("Primary", 100)},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.RootFilter.Children == nil {
		t.Fatal("Create response RootFilter.Children is nil, want a non-nil empty slice")
	}

	got, err := svc.Get(ctx, orgID, campaignID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.RootFilter.Children == nil {
		t.Fatal("Get response RootFilter.Children is nil, want a non-nil empty slice (this is the read path the real bug was in)")
	}
}

func TestCreateRejectsEmptyFlowsAndBadFilters(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID)

	svc := streamset.NewService(streamset.NewRepository(pool))
	emptyRoot := streamset.FilterNode{Kind: streamset.NodeGroup, Joiner: routing.JoinAND}

	t.Run("no flows", func(t *testing.T) {
		_, err := svc.Create(ctx, orgID, campaignID, streamset.CreateInput{Name: "No Flows", RootFilter: emptyRoot})
		if err == nil {
			t.Fatal("Create with no flows succeeded, want a validation error")
		}
	})

	t.Run("condition as root", func(t *testing.T) {
		badRoot := streamset.FilterNode{Kind: streamset.NodeCondition, Field: routing.FieldCountry, Operator: routing.OpIs, Value: "US"}
		_, err := svc.Create(ctx, orgID, campaignID, streamset.CreateInput{
			Name: "Bad Root", RootFilter: badRoot, Flows: []streamset.FlowInput{redirectFlow("Primary", 100)},
		})
		if err == nil {
			t.Fatal("Create with a condition as root succeeded, want a validation error")
		}
	})

	t.Run("invalid country code", func(t *testing.T) {
		root := streamset.FilterNode{
			Kind: streamset.NodeGroup, Joiner: routing.JoinAND,
			Children: []streamset.FilterNode{{Kind: streamset.NodeCondition, Field: routing.FieldCountry, Operator: routing.OpIs, Value: "UK"}},
		}
		_, err := svc.Create(ctx, orgID, campaignID, streamset.CreateInput{
			Name: "Bad Country", RootFilter: root, Flows: []streamset.FlowInput{redirectFlow("Primary", 100)},
		})
		if err == nil {
			t.Fatal(`Create with country "UK" succeeded, want a validation error (use GB)`)
		}
	})

	t.Run("invalid RE2 pattern", func(t *testing.T) {
		root := streamset.FilterNode{
			Kind: streamset.NodeGroup, Joiner: routing.JoinAND,
			Children: []streamset.FilterNode{{Kind: streamset.NodeCondition, Field: routing.FieldReferrer, Operator: routing.OpMatches, Value: "(?<=foo)bar"}},
		}
		_, err := svc.Create(ctx, orgID, campaignID, streamset.CreateInput{
			Name: "Bad Regex", RootFilter: root, Flows: []streamset.FlowInput{redirectFlow("Primary", 100)},
		})
		if err == nil {
			t.Fatal("Create with a PCRE-only lookbehind succeeded, want a validation error (RE2 only, CLAUDE.md #8)")
		}
	})

	t.Run("BETWEEN missing bound", func(t *testing.T) {
		root := streamset.FilterNode{
			Kind: streamset.NodeGroup, Joiner: routing.JoinAND,
			Children: []streamset.FilterNode{{Kind: streamset.NodeCondition, Field: routing.FieldOSVersion, Operator: routing.OpBetween, Value: "10"}},
		}
		_, err := svc.Create(ctx, orgID, campaignID, streamset.CreateInput{
			Name: "Bad Range", RootFilter: root, Flows: []streamset.FlowInput{redirectFlow("Primary", 100)},
		})
		if err == nil {
			t.Fatal("Create with a BETWEEN missing valueTo succeeded, want a validation error")
		}
	})
}

func TestOfferDestinationDerivesNetworkFromOffer(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID)
	realNetworkID := seedNetwork(t, ctx, pool, orgID)
	offerID := seedOffer(t, ctx, pool, orgID, realNetworkID)

	svc := streamset.NewService(streamset.NewRepository(pool))
	root := streamset.FilterNode{Kind: streamset.NodeGroup, Joiner: routing.JoinAND}

	created, err := svc.Create(ctx, orgID, campaignID, streamset.CreateInput{
		Name:       "Offer Destination",
		RootFilter: root,
		Flows: []streamset.FlowInput{{
			Name: "Primary", Active: true, Weight: 100,
			Destination: streamset.Destination{
				Kind: routing.DestinationOffer,
				// Deliberately wrong — the service must derive the real
				// network from the offer, not trust this.
				NetworkID: "01AAAAAAAAAAAAAAAAAAAAAAAA",
				OfferID:   offerID,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Flows[0].Destination.NetworkID != realNetworkID {
		t.Fatalf("Destination.NetworkID = %q, want the offer's real network %q (client-supplied value must be ignored)",
			created.Flows[0].Destination.NetworkID, realNetworkID)
	}
}

// TestFlowFunnelStagesRoundTrip covers the phase this test file's own
// commit added: Landing/PWA/Postlanding stages wired onto a Flow on top of
// its Destination, now that internal/landing, internal/pwa, and
// internal/postlanding all exist for real — see docs/stream-sets.md's
// "Landing/PWA/Postlanding stages" section.
func TestFlowFunnelStagesRoundTrip(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID)
	landingID := seedLanding(t, ctx, pool, orgID)
	pwaID := seedPwa(t, ctx, pool, orgID)
	postlandingID := seedPostlanding(t, ctx, pool, orgID)

	svc := streamset.NewService(streamset.NewRepository(pool))
	root := streamset.FilterNode{Kind: streamset.NodeGroup, Joiner: routing.JoinAND}

	flow := redirectFlow("Primary", 100)
	flow.Landing = streamset.FlowLanding{Enabled: true, LandingID: landingID, AsPwa: true}
	flow.Pwa = streamset.FlowPwa{Enabled: true, PwaID: pwaID, PwaType: streamset.PwaTypeExternal}
	flow.Postlanding = streamset.FlowPostlanding{Enabled: true, PostlandingID: postlandingID}

	created, err := svc.Create(ctx, orgID, campaignID, streamset.CreateInput{
		Name: "Full Funnel", RootFilter: root, Flows: []streamset.FlowInput{flow},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got := created.Flows[0]
	if !got.Landing.Enabled || got.Landing.LandingID != landingID || !got.Landing.AsPwa {
		t.Fatalf("Landing = %+v, want enabled with id %q and asPwa true", got.Landing, landingID)
	}
	if !got.Pwa.Enabled || got.Pwa.PwaID != pwaID || got.Pwa.PwaType != streamset.PwaTypeExternal {
		t.Fatalf("Pwa = %+v, want enabled with id %q and type external", got.Pwa, pwaID)
	}
	if !got.Postlanding.Enabled || got.Postlanding.PostlandingID != postlandingID {
		t.Fatalf("Postlanding = %+v, want enabled with id %q", got.Postlanding, postlandingID)
	}

	fetched, err := svc.Get(ctx, orgID, campaignID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fetched.Flows[0].Landing != got.Landing || fetched.Flows[0].Pwa != got.Pwa || fetched.Flows[0].Postlanding != got.Postlanding {
		t.Fatalf("Get round-trip stages = %+v, want to match Create's response %+v", fetched.Flows[0], got)
	}
}

func TestFlowFunnelStageValidation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID)

	svc := streamset.NewService(streamset.NewRepository(pool))
	root := streamset.FilterNode{Kind: streamset.NodeGroup, Joiner: routing.JoinAND}

	t.Run("landing enabled without an id", func(t *testing.T) {
		flow := redirectFlow("Primary", 100)
		flow.Landing = streamset.FlowLanding{Enabled: true}
		_, err := svc.Create(ctx, orgID, campaignID, streamset.CreateInput{Name: "X", RootFilter: root, Flows: []streamset.FlowInput{flow}})
		if err == nil {
			t.Fatal("Create with landing enabled but no landingId succeeded, want a validation error")
		}
	})

	t.Run("pwa enabled with an invalid type", func(t *testing.T) {
		pwaID := seedPwa(t, ctx, pool, orgID)
		flow := redirectFlow("Primary", 100)
		flow.Pwa = streamset.FlowPwa{Enabled: true, PwaID: pwaID, PwaType: "desktop"}
		_, err := svc.Create(ctx, orgID, campaignID, streamset.CreateInput{Name: "X", RootFilter: root, Flows: []streamset.FlowInput{flow}})
		if err == nil {
			t.Fatal("Create with an invalid pwaType succeeded, want a validation error")
		}
	})

	t.Run("postlanding id from another org is rejected even when disabled", func(t *testing.T) {
		otherOrg := seedOrg(t, ctx, pool)
		foreignPostlandingID := seedPostlanding(t, ctx, pool, otherOrg)
		flow := redirectFlow("Primary", 100)
		flow.Postlanding = streamset.FlowPostlanding{Enabled: false, PostlandingID: foreignPostlandingID}
		_, err := svc.Create(ctx, orgID, campaignID, streamset.CreateInput{Name: "X", RootFilter: root, Flows: []streamset.FlowInput{flow}})
		if err == nil {
			t.Fatal("Create referencing another org's postlanding id succeeded, want a not-found/validation error")
		}
	})
}

func TestReorderRewritesPriority(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID)

	svc := streamset.NewService(streamset.NewRepository(pool))
	root := streamset.FilterNode{Kind: streamset.NodeGroup, Joiner: routing.JoinAND}
	flows := []streamset.FlowInput{redirectFlow("Primary", 100)}

	a, err := svc.Create(ctx, orgID, campaignID, streamset.CreateInput{Name: "Set A", RootFilter: root, Flows: flows})
	if err != nil {
		t.Fatalf("creating A: %v", err)
	}
	b, err := svc.Create(ctx, orgID, campaignID, streamset.CreateInput{Name: "Set B", RootFilter: root, Flows: flows})
	if err != nil {
		t.Fatalf("creating B: %v", err)
	}
	if a.Priority != 1 || b.Priority != 2 {
		t.Fatalf("initial priorities = %d, %d, want 1, 2", a.Priority, b.Priority)
	}

	reordered, err := svc.Reorder(ctx, orgID, campaignID, []string{b.ID, a.ID})
	if err != nil {
		t.Fatalf("Reorder: %v", err)
	}
	if len(reordered) != 2 || reordered[0].ID != b.ID || reordered[0].Priority != 1 || reordered[1].ID != a.ID || reordered[1].Priority != 2 {
		t.Fatalf("Reorder result = %+v, want B first at priority 1, A second at priority 2", reordered)
	}
}

func TestDuplicateKeepsStatusAndCopiesTreeAndFlows(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID)

	svc := streamset.NewService(streamset.NewRepository(pool))
	root := streamset.FilterNode{
		Kind: streamset.NodeGroup, Joiner: routing.JoinAND,
		Children: []streamset.FilterNode{{Kind: streamset.NodeCondition, Field: routing.FieldCountry, Operator: routing.OpIs, Value: "US"}},
	}
	created, err := svc.Create(ctx, orgID, campaignID, streamset.CreateInput{
		Name: "Original", RootFilter: root, Flows: []streamset.FlowInput{redirectFlow("Primary", 100)},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	paused := streamset.StatusPaused
	if _, err := svc.Update(ctx, orgID, campaignID, created.ID, streamset.UpdateInput{Status: &paused}); err != nil {
		t.Fatalf("pausing: %v", err)
	}

	dup, err := svc.Duplicate(ctx, orgID, campaignID, created.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if dup.Name != "Original (Copy)" {
		t.Fatalf("Name = %q, want %q", dup.Name, "Original (Copy)")
	}
	if dup.Status != streamset.StatusPaused {
		t.Fatalf("Status = %q, want paused (copied, not reset)", dup.Status)
	}
	if len(dup.RootFilter.Children) != 1 {
		t.Fatalf("RootFilter.Children = %d, want 1 (copied)", len(dup.RootFilter.Children))
	}
	if len(dup.Flows) != 1 || dup.Flows[0].ID == created.Flows[0].ID {
		t.Fatalf("Flows = %+v, want one copied flow with a fresh id", dup.Flows)
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)
	campaignA := seedCampaign(t, ctx, pool, orgA)

	svc := streamset.NewService(streamset.NewRepository(pool))
	root := streamset.FilterNode{Kind: streamset.NodeGroup, Joiner: routing.JoinAND}
	created, err := svc.Create(ctx, orgA, campaignA, streamset.CreateInput{
		Name: "Org A Set", RootFilter: root, Flows: []streamset.FlowInput{redirectFlow("Primary", 100)},
	})
	if err != nil {
		t.Fatalf("creating for org A: %v", err)
	}

	t.Run("get", func(t *testing.T) {
		if _, err := svc.Get(ctx, orgB, campaignA, created.ID); err == nil {
			t.Fatal("org B fetched org A's stream set, want not-found")
		}
	})
	t.Run("update", func(t *testing.T) {
		name := "hijacked"
		if _, err := svc.Update(ctx, orgB, campaignA, created.ID, streamset.UpdateInput{Name: &name}); err == nil {
			t.Fatal("org B updated org A's stream set, want not-found")
		}
	})
	t.Run("delete", func(t *testing.T) {
		if err := svc.Delete(ctx, orgB, campaignA, created.ID); err == nil {
			t.Fatal("org B deleted org A's stream set, want not-found")
		}
	})
	t.Run("list", func(t *testing.T) {
		sets, err := svc.List(ctx, orgB, campaignA)
		if err != nil {
			t.Fatalf("listing as org B: %v", err)
		}
		if len(sets) != 0 {
			t.Fatalf("org B saw %d of org A's stream sets, want 0", len(sets))
		}
	})
	t.Run("create against another org's campaign", func(t *testing.T) {
		_, err := svc.Create(ctx, orgB, campaignA, streamset.CreateInput{
			Name: "Cross Org", RootFilter: root, Flows: []streamset.FlowInput{redirectFlow("Primary", 100)},
		})
		if err == nil {
			t.Fatal("org B created a stream set against org A's campaign, want not-found")
		}
	})
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

func seedNetwork(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID string) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx, `INSERT INTO networks (id, organization_id, name) VALUES ($1, $2, $3)`, id, orgID, "Test Network")
	if err != nil {
		t.Fatalf("seeding network: %v", err)
	}
	return id
}

func seedOffer(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID, networkID string) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO offers (id, organization_id, network_id, name, payout) VALUES ($1, $2, $3, $4, $5)`,
		id, orgID, networkID, "Test Offer", 10,
	)
	if err != nil {
		t.Fatalf("seeding offer: %v", err)
	}
	return id
}

func seedLanding(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID string) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO landings (id, organization_id, name, type, url) VALUES ($1, $2, $3, $4, $5)`,
		id, orgID, "Test Landing", "external", "https://example.com/landing",
	)
	if err != nil {
		t.Fatalf("seeding landing: %v", err)
	}
	return id
}

func seedPwa(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID string) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO pwas (id, organization_id, name, short_name, theme_color, background_color, icon_url, start_url)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		id, orgID, "Test PWA", "TPWA", "#000000", "#ffffff", "https://example.com/icon.png", "/",
	)
	if err != nil {
		t.Fatalf("seeding pwa: %v", err)
	}
	return id
}

func seedPostlanding(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID string) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO postlandings (id, organization_id, name, url) VALUES ($1, $2, $3, $4)`,
		id, orgID, "Test Postlanding", "https://example.com/postlanding",
	)
	if err != nil {
		t.Fatalf("seeding postlanding: %v", err)
	}
	return id
}
