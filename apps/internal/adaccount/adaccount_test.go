package adaccount_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

func TestConnectGetDisconnect(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	sourceID := seedTrafficSource(t, ctx, pool, orgID, "facebook_ads")

	svc := adaccount.NewService(adaccount.NewRepository(pool))

	if _, err := svc.Get(ctx, orgID, sourceID); err == nil {
		t.Fatal("Get before Connect succeeded, want not-found")
	}

	created, err := svc.Connect(ctx, orgID, sourceID, adaccount.ConnectInput{
		AdAccountID: "act_123456789", AccessToken: "EAAGtokenlonglivedvalue1234567890",
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if created.AdAccountID != "act_123456789" {
		t.Fatalf("AdAccountID = %q, want act_123456789", created.AdAccountID)
	}
	if created.TokenPreview != "7890" {
		t.Fatalf("TokenPreview = %q, want the last 4 characters of the token (7890)", created.TokenPreview)
	}

	got, err := svc.Get(ctx, orgID, sourceID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID || got.AdAccountID != created.AdAccountID {
		t.Fatalf("Get returned %+v, want the just-connected row", got)
	}

	// Re-connecting (a fresh token) replaces the row in place — same id,
	// new account/token, not a second row (traffic_source_id is UNIQUE).
	reconnected, err := svc.Connect(ctx, orgID, sourceID, adaccount.ConnectInput{
		AdAccountID: "act_987654321", AccessToken: "EAAGdifferenttokenvalue0987654321",
	})
	if err != nil {
		t.Fatalf("re-Connect: %v", err)
	}
	if reconnected.ID != created.ID {
		t.Fatalf("re-Connect id = %q, want the same id as the original connection %q", reconnected.ID, created.ID)
	}
	if reconnected.AdAccountID != "act_987654321" {
		t.Fatalf("AdAccountID after reconnect = %q, want act_987654321", reconnected.AdAccountID)
	}

	if err := svc.Disconnect(ctx, orgID, sourceID); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if _, err := svc.Get(ctx, orgID, sourceID); err == nil {
		t.Fatal("Get after Disconnect succeeded, want not-found")
	}
	if err := svc.Disconnect(ctx, orgID, sourceID); err == nil {
		t.Fatal("Disconnect on an already-disconnected source succeeded, want not-found")
	}
}

func TestConnectRejectsSourceWithoutAdCostIntegration(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := adaccount.NewService(adaccount.NewRepository(pool))

	t.Run("none", func(t *testing.T) {
		sourceID := seedTrafficSource(t, ctx, pool, orgID, "none")
		if _, err := svc.Connect(ctx, orgID, sourceID, adaccount.ConnectInput{AdAccountID: "act_1", AccessToken: "sometoken1234567890"}); err == nil {
			t.Fatal("Connect on a source with cost_integration=none succeeded, want a validation error")
		}
	})

	t.Run("manual", func(t *testing.T) {
		sourceID := seedTrafficSource(t, ctx, pool, orgID, "manual")
		if _, err := svc.Connect(ctx, orgID, sourceID, adaccount.ConnectInput{AdAccountID: "act_1", AccessToken: "sometoken1234567890"}); err == nil {
			t.Fatal("Connect on a source with cost_integration=manual succeeded, want a validation error")
		}
	})

	t.Run("tiktok_ads is allowed", func(t *testing.T) {
		sourceID := seedTrafficSource(t, ctx, pool, orgID, "tiktok_ads")
		if _, err := svc.Connect(ctx, orgID, sourceID, adaccount.ConnectInput{AdAccountID: "act_1", AccessToken: "sometoken1234567890"}); err != nil {
			t.Fatalf("Connect on a source with cost_integration=tiktok_ads failed: %v", err)
		}
	})
}

func TestConnectRejectsInvalidShapes(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	sourceID := seedTrafficSource(t, ctx, pool, orgID, "facebook_ads")

	svc := adaccount.NewService(adaccount.NewRepository(pool))

	if _, err := svc.Connect(ctx, orgID, sourceID, adaccount.ConnectInput{AdAccountID: "", AccessToken: "sometoken1234567890"}); err == nil {
		t.Fatal("Connect with an empty adAccountId succeeded, want a validation error")
	}
	if _, err := svc.Connect(ctx, orgID, sourceID, adaccount.ConnectInput{AdAccountID: "act_1", AccessToken: ""}); err == nil {
		t.Fatal("Connect with an empty accessToken succeeded, want a validation error")
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)
	sourceA := seedTrafficSource(t, ctx, pool, orgA, "facebook_ads")

	svc := adaccount.NewService(adaccount.NewRepository(pool))
	if _, err := svc.Connect(ctx, orgA, sourceA, adaccount.ConnectInput{AdAccountID: "act_1", AccessToken: "sometoken1234567890"}); err != nil {
		t.Fatalf("connecting for org A: %v", err)
	}

	t.Run("get", func(t *testing.T) {
		if _, err := svc.Get(ctx, orgB, sourceA); err == nil {
			t.Fatal("org B fetched org A's connection, want not-found")
		}
	})
	t.Run("disconnect", func(t *testing.T) {
		if err := svc.Disconnect(ctx, orgB, sourceA); err == nil {
			t.Fatal("org B disconnected org A's connection, want not-found")
		}
	})
	t.Run("connect against another org's traffic source", func(t *testing.T) {
		if _, err := svc.Connect(ctx, orgB, sourceA, adaccount.ConnectInput{AdAccountID: "act_2", AccessToken: "someothertoken0987654321"}); err == nil {
			t.Fatal("org B connected an account to org A's traffic source, want not-found")
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

func seedTrafficSource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID, costIntegration string) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO traffic_sources (id, organization_id, name, type, cost_integration) VALUES ($1, $2, $3, $4, $5)`,
		id, orgID, "Test Source", "Facebook", costIntegration,
	)
	if err != nil {
		t.Fatalf("seeding traffic source: %v", err)
	}
	return id
}
