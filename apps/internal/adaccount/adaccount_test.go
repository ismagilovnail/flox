package adaccount_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

var testKey = mustTestKey()

func mustTestKey() []byte {
	key, err := adaccount.ParseKey("MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE=")
	if err != nil {
		panic(err)
	}
	return key
}

func TestConnectGetDisconnect(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	sourceID := seedTrafficSource(t, ctx, pool, orgID, "facebook_ads")

	svc := adaccount.NewService(adaccount.NewRepository(pool, testKey))

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

	svc := adaccount.NewService(adaccount.NewRepository(pool, testKey))

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

	t.Run("google_ads is allowed", func(t *testing.T) {
		sourceID := seedTrafficSource(t, ctx, pool, orgID, "google_ads")
		_, err := svc.Connect(ctx, orgID, sourceID, adaccount.ConnectInput{
			AdAccountID: "123-456-7890", DeveloperToken: "dev-token", ClientID: "client-id",
			ClientSecret: "client-secret", RefreshToken: "refresh-token",
		})
		if err != nil {
			t.Fatalf("Connect on a source with cost_integration=google_ads failed: %v", err)
		}
	})

	t.Run("yandex_direct is allowed with a login and a token", func(t *testing.T) {
		sourceID := seedTrafficSource(t, ctx, pool, orgID, "yandex_direct")
		if _, err := svc.Connect(ctx, orgID, sourceID, adaccount.ConnectInput{AdAccountID: "shop-client"}); err == nil {
			t.Fatal("Connect on a yandex_direct source without a token succeeded")
		}
		if _, err := svc.Connect(ctx, orgID, sourceID, adaccount.ConnectInput{AdAccountID: "shop-client", AccessToken: "y0_token1234567890"}); err != nil {
			t.Fatalf("Connect on a source with cost_integration=yandex_direct failed: %v", err)
		}
	})
}

func TestConnectGoogleAdsRequiresGoogleAdsFields(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	sourceID := seedTrafficSource(t, ctx, pool, orgID, "google_ads")

	svc := adaccount.NewService(adaccount.NewRepository(pool, testKey))

	// A plain accessToken (the facebook_ads/tiktok_ads shape) is not
	// enough for google_ads — it has no long-lived access token to
	// accept, only the means to mint short-lived ones via a refresh
	// token.
	if _, err := svc.Connect(ctx, orgID, sourceID, adaccount.ConnectInput{AdAccountID: "123", AccessToken: "sometoken1234567890"}); err == nil {
		t.Fatal("Connect on a google_ads source with only accessToken succeeded, want a validation error")
	}
	if _, err := svc.Connect(ctx, orgID, sourceID, adaccount.ConnectInput{
		AdAccountID: "123", DeveloperToken: "dev", ClientID: "id", ClientSecret: "secret", // missing refreshToken
	}); err == nil {
		t.Fatal("Connect on a google_ads source with a missing refreshToken succeeded, want a validation error")
	}
}

// TestGoogleAdsCredentialsRoundTripEncryption confirms developer_token/
// client_secret/refresh_token really are encrypted at rest — not just
// that Connect/CredentialsByTrafficSourceID happen to agree — by reading
// the raw column value directly and asserting it isn't the plaintext.
func TestGoogleAdsCredentialsRoundTripEncryption(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	sourceID := seedTrafficSource(t, ctx, pool, orgID, "google_ads")

	repo := adaccount.NewRepository(pool, testKey)
	if _, err := repo.Connect(ctx, idgen.New(), orgID, sourceID, adaccount.ConnectInput{
		AdAccountID: "123-456-7890", DeveloperToken: "real-dev-token", ClientID: "real-client-id",
		ClientSecret: "real-client-secret", RefreshToken: "real-refresh-token",
	}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	var rawRefreshToken string
	if err := pool.QueryRow(ctx, `SELECT refresh_token FROM ad_account_connections WHERE traffic_source_id = $1`, sourceID).Scan(&rawRefreshToken); err != nil {
		t.Fatalf("reading raw refresh_token column: %v", err)
	}
	if rawRefreshToken == "real-refresh-token" {
		t.Fatal("refresh_token is stored in plain text, want it AES-256-GCM encrypted")
	}

	creds, err := repo.CredentialsByTrafficSourceID(ctx, orgID, sourceID)
	if err != nil {
		t.Fatalf("CredentialsByTrafficSourceID: %v", err)
	}
	if creds.AdAccountID != "123-456-7890" || creds.DeveloperToken != "real-dev-token" ||
		creds.ClientID != "real-client-id" || creds.ClientSecret != "real-client-secret" || creds.RefreshToken != "real-refresh-token" {
		t.Fatalf("CredentialsByTrafficSourceID = %+v, want the exact plaintext round-tripped back", creds)
	}
	if creds.AccessToken != "" {
		t.Fatalf("AccessToken = %q, want empty for a google_ads connection", creds.AccessToken)
	}

	conn, err := repo.GetByTrafficSourceID(ctx, orgID, sourceID)
	if err != nil {
		t.Fatalf("GetByTrafficSourceID: %v", err)
	}
	if conn.TokenPreview != "oken" {
		t.Fatalf("TokenPreview = %q, want the last 4 characters of the refresh token (oken)", conn.TokenPreview)
	}
}

// TestAccessTokenRoundTripEncryption confirms facebook_ads/tiktok_ads'
// own access_token is now real AES-256-GCM ciphertext at rest — not just
// that Connect/CredentialsByTrafficSourceID happen to agree — by reading
// the raw column value directly and asserting it isn't the plaintext.
// Mirrors TestGoogleAdsCredentialsRoundTripEncryption above; this column
// used to be the one deliberately-flagged plain-text exception.
func TestAccessTokenRoundTripEncryption(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	sourceID := seedTrafficSource(t, ctx, pool, orgID, "facebook_ads")

	repo := adaccount.NewRepository(pool, testKey)
	if _, err := repo.Connect(ctx, idgen.New(), orgID, sourceID, adaccount.ConnectInput{
		AdAccountID: "act_123456789", AccessToken: "real-plaintext-access-token",
	}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	var rawAccessToken string
	if err := pool.QueryRow(ctx, `SELECT access_token FROM ad_account_connections WHERE traffic_source_id = $1`, sourceID).Scan(&rawAccessToken); err != nil {
		t.Fatalf("reading raw access_token column: %v", err)
	}
	if rawAccessToken == "real-plaintext-access-token" {
		t.Fatal("access_token is stored in plain text, want it AES-256-GCM encrypted")
	}

	creds, err := repo.CredentialsByTrafficSourceID(ctx, orgID, sourceID)
	if err != nil {
		t.Fatalf("CredentialsByTrafficSourceID: %v", err)
	}
	if creds.AccessToken != "real-plaintext-access-token" {
		t.Fatalf("CredentialsByTrafficSourceID.AccessToken = %q, want the exact plaintext round-tripped back", creds.AccessToken)
	}

	conn, err := repo.GetByTrafficSourceID(ctx, orgID, sourceID)
	if err != nil {
		t.Fatalf("GetByTrafficSourceID: %v", err)
	}
	if conn.TokenPreview != "oken" {
		t.Fatalf("TokenPreview = %q, want the last 4 characters of the access token (oken)", conn.TokenPreview)
	}
}

func TestConnectRejectsInvalidShapes(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	sourceID := seedTrafficSource(t, ctx, pool, orgID, "facebook_ads")

	svc := adaccount.NewService(adaccount.NewRepository(pool, testKey))

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

	svc := adaccount.NewService(adaccount.NewRepository(pool, testKey))
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

func TestListAllConnections(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)
	sourceA := seedTrafficSource(t, ctx, pool, orgA, "facebook_ads")
	sourceB := seedTrafficSource(t, ctx, pool, orgB, "tiktok_ads")
	sourceUnconnected := seedTrafficSource(t, ctx, pool, orgA, "facebook_ads")

	repo := adaccount.NewRepository(pool, testKey)
	if _, err := repo.Connect(ctx, idgen.New(), orgA, sourceA, adaccount.ConnectInput{AdAccountID: "act_a", AccessToken: "token-a-1234567890"}); err != nil {
		t.Fatalf("connecting org A: %v", err)
	}
	if _, err := repo.Connect(ctx, idgen.New(), orgB, sourceB, adaccount.ConnectInput{AdAccountID: "adv_b", AccessToken: "token-b-1234567890"}); err != nil {
		t.Fatalf("connecting org B: %v", err)
	}

	refs, err := repo.ListAllConnections(ctx)
	if err != nil {
		t.Fatalf("ListAllConnections: %v", err)
	}

	want := map[string]bool{sourceA: false, sourceB: false}
	for _, ref := range refs {
		if ref.TrafficSourceID == sourceA {
			if ref.OrganizationID != orgA {
				t.Fatalf("connection for sourceA has organization_id %q, want %q", ref.OrganizationID, orgA)
			}
			want[sourceA] = true
		}
		if ref.TrafficSourceID == sourceB {
			if ref.OrganizationID != orgB {
				t.Fatalf("connection for sourceB has organization_id %q, want %q", ref.OrganizationID, orgB)
			}
			want[sourceB] = true
		}
		if ref.TrafficSourceID == sourceUnconnected {
			t.Fatalf("ListAllConnections returned the unconnected traffic source %q", sourceUnconnected)
		}
	}
	if !want[sourceA] || !want[sourceB] {
		t.Fatalf("ListAllConnections = %+v, want both org A's and org B's connections present", refs)
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
