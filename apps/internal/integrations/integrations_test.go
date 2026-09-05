package integrations_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/integrations"
)

// fakeVerifier lets tests control whether a provider's real API call
// would have succeeded, without ever making a real HTTP request — this
// project has no live Namecheap/Cloudflare/Slack credential to exercise
// the real adapters against.
type fakeVerifier struct {
	err        error
	calls      int
	lastFields map[string]string
}

func (f *fakeVerifier) Verify(_ context.Context, fields map[string]string) error {
	f.calls++
	f.lastFields = fields
	return f.err
}

type fakeAdPlatforms struct {
	summary []adaccount.PlatformCount
}

func (f *fakeAdPlatforms) AdPlatformSummary(_ context.Context, _ string) ([]adaccount.PlatformCount, error) {
	return f.summary, nil
}

var testKey = mustTestKey()

func mustTestKey() []byte {
	key, err := integrations.ParseKey("MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE=")
	if err != nil {
		panic(err)
	}
	return key
}

func TestConnectRequiresSuccessfulVerification(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := integrations.NewService(integrations.NewRepository(pool), testKey, &fakeAdPlatforms{})
	fake := &fakeVerifier{err: errors.New("invalid api key")}
	svc.SetAdapter(integrations.ProviderCloudflare, fake)

	_, err := svc.Connect(ctx, orgID, integrations.ConnectInput{
		Provider: integrations.ProviderCloudflare,
		Fields:   map[string]string{"apiToken": "bad-token"},
	})
	if err == nil {
		t.Fatal("Connect with a failing Verify succeeded, want an error")
	}
	if fake.calls != 1 {
		t.Fatalf("Verify called %d times, want 1", fake.calls)
	}

	sum, err := svc.Summary(ctx, orgID)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if got := statusOf(sum, integrations.ProviderCloudflare); got != integrations.StatusNotConnected {
		t.Fatalf("status after a failed Connect = %q, want not_connected (no row should have been written)", got)
	}
}

func TestConnectValidatesRequiredFields(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := integrations.NewService(integrations.NewRepository(pool), testKey, &fakeAdPlatforms{})
	svc.SetAdapter(integrations.ProviderNamecheap, &fakeVerifier{})

	_, err := svc.Connect(ctx, orgID, integrations.ConnectInput{
		Provider: integrations.ProviderNamecheap,
		Fields:   map[string]string{"apiUser": "someone"}, // missing apiKey, clientIp
	})
	if err == nil {
		t.Fatal("Connect with missing required fields succeeded, want a validation error")
	}
}

func TestConnectGetVerifyDisconnect(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := integrations.NewService(integrations.NewRepository(pool), testKey, &fakeAdPlatforms{})
	fake := &fakeVerifier{}
	svc.SetAdapter(integrations.ProviderSlack, fake)

	connected, err := svc.Connect(ctx, orgID, integrations.ConnectInput{
		Provider: integrations.ProviderSlack,
		Fields:   map[string]string{"webhookUrl": "https://hooks.slack.com/services/T000/B000/xxxx"},
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if connected.Status != integrations.StatusConnected {
		t.Fatalf("status after Connect = %q, want connected", connected.Status)
	}
	if fake.lastFields["webhookUrl"] != "https://hooks.slack.com/services/T000/B000/xxxx" {
		t.Fatalf("Verify received fields %+v, want the connected webhookUrl", fake.lastFields)
	}

	sum, err := svc.Summary(ctx, orgID)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if got := statusOf(sum, integrations.ProviderSlack); got != integrations.StatusConnected {
		t.Fatalf("Summary status = %q, want connected", got)
	}

	// Re-verifying an already-connected credential that now fails (e.g. the
	// webhook was revoked in Slack) records the failure as an "error"
	// status WITHOUT deleting the stored credential.
	fake.err = errors.New("channel_not_found")
	reverified, err := svc.Verify(ctx, orgID, integrations.ProviderSlack)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if reverified.Status != integrations.StatusError {
		t.Fatalf("status after a failing re-verify = %q, want error", reverified.Status)
	}
	if reverified.LastError == nil || *reverified.LastError != "channel_not_found" {
		t.Fatalf("LastError = %v, want the real verify error message", reverified.LastError)
	}

	sum, err = svc.Summary(ctx, orgID)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if got := statusOf(sum, integrations.ProviderSlack); got != integrations.StatusError {
		t.Fatalf("Summary status after failed re-verify = %q, want error (row must not be deleted)", got)
	}

	// A subsequent successful re-verify clears the error back to connected.
	fake.err = nil
	recovered, err := svc.Verify(ctx, orgID, integrations.ProviderSlack)
	if err != nil {
		t.Fatalf("Verify (recovery): %v", err)
	}
	if recovered.Status != integrations.StatusConnected || recovered.LastError != nil {
		t.Fatalf("status after a successful re-verify = %+v, want connected with no error", recovered)
	}

	if err := svc.Disconnect(ctx, orgID, integrations.ProviderSlack); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	sum, err = svc.Summary(ctx, orgID)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if got := statusOf(sum, integrations.ProviderSlack); got != integrations.StatusNotConnected {
		t.Fatalf("status after Disconnect = %q, want not_connected", got)
	}
}

func TestVerifyAndDisconnectRequireAnExistingConnection(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := integrations.NewService(integrations.NewRepository(pool), testKey, &fakeAdPlatforms{})

	if _, err := svc.Verify(ctx, orgID, integrations.ProviderCloudflare); err == nil {
		t.Fatal("Verify with no stored connection succeeded, want not-found")
	}
	if err := svc.Disconnect(ctx, orgID, integrations.ProviderCloudflare); err == nil {
		t.Fatal("Disconnect with no stored connection succeeded, want not-found")
	}
}

func TestConnectRejectsUnknownProvider(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := integrations.NewService(integrations.NewRepository(pool), testKey, &fakeAdPlatforms{})

	_, err := svc.Connect(ctx, orgID, integrations.ConnectInput{Provider: "google_ads", Fields: map[string]string{}})
	if err == nil {
		t.Fatal("Connect with an ad-platform provider succeeded, want a validation error (those aren't org-level integrations)")
	}
}

func TestSummaryIncludesRealAdPlatformAggregate(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	fake := &fakeAdPlatforms{summary: []adaccount.PlatformCount{
		{CostIntegration: "facebook_ads", ConnectedCount: 2, TotalCount: 3},
	}}
	svc := integrations.NewService(integrations.NewRepository(pool), testKey, fake)

	sum, err := svc.Summary(ctx, orgID)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if len(sum.AdPlatforms) != 1 || sum.AdPlatforms[0].Provider != "facebook_ads" ||
		sum.AdPlatforms[0].ConnectedCount != 2 || sum.AdPlatforms[0].TotalCount != 3 {
		t.Fatalf("AdPlatforms = %+v, want the fake summary passed through unchanged", sum.AdPlatforms)
	}
}

func TestCrossOrgIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)

	svc := integrations.NewService(integrations.NewRepository(pool), testKey, &fakeAdPlatforms{})
	svc.SetAdapter(integrations.ProviderCloudflare, &fakeVerifier{})

	if _, err := svc.Connect(ctx, orgA, integrations.ConnectInput{
		Provider: integrations.ProviderCloudflare,
		Fields:   map[string]string{"apiToken": "org-a-token"},
	}); err != nil {
		t.Fatalf("Connect (org A): %v", err)
	}

	sumB, err := svc.Summary(ctx, orgB)
	if err != nil {
		t.Fatalf("Summary (org B): %v", err)
	}
	if got := statusOf(sumB, integrations.ProviderCloudflare); got != integrations.StatusNotConnected {
		t.Fatalf("org B sees status %q for org A's integration, want not_connected", got)
	}

	if err := svc.Disconnect(ctx, orgB, integrations.ProviderCloudflare); err == nil {
		t.Fatal("org B disconnecting org A's integration succeeded, want not-found")
	}
}

func statusOf(sum integrations.Summary, provider integrations.Provider) integrations.Status {
	for _, i := range sum.Integrations {
		if i.Provider == provider {
			return i.Status
		}
	}
	return ""
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
