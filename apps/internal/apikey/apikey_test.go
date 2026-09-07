package apikey_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apikey"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/logging"
)

func newService(pool *pgxpool.Pool) *apikey.Service {
	return apikey.NewService(apikey.NewRepository(pool), logging.New("error"))
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

func seedOrgAndUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (orgID, userID string) {
	t.Helper()
	orgID = idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, orgID, "Test Org "+orgID); err != nil {
		t.Fatalf("seeding organization: %v", err)
	}
	userID = idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name) VALUES ($1, $2, 'Test User')`, userID, userID+"@example.com"); err != nil {
		t.Fatalf("seeding user: %v", err)
	}
	return orgID, userID
}

func TestCreateListRevoke(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, ctx, pool)
	svc := newService(pool)

	result, err := svc.Create(ctx, orgID, userID, apikey.CreateInput{Name: "Reporting export", Scope: apikey.ScopeRead})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(result.FullKey, "flx_live_") {
		t.Fatalf("FullKey = %q, want flx_live_ prefix", result.FullKey)
	}
	if !strings.HasSuffix(result.ApiKey.Prefix, "...") || !strings.HasPrefix(result.ApiKey.Prefix, "flx_live_") {
		t.Fatalf("Prefix = %q, want flx_live_XXXX... shape", result.ApiKey.Prefix)
	}
	if result.ApiKey.Prefix == result.FullKey || len(result.ApiKey.Prefix) >= len(result.FullKey) {
		t.Fatalf("Prefix (%q) must be strictly shorter than and never equal to the full key (%q)", result.ApiKey.Prefix, result.FullKey)
	}

	list, err := svc.List(ctx, orgID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != result.ApiKey.ID {
		t.Fatalf("List = %+v, want exactly the created key", list)
	}

	if err := svc.Revoke(ctx, orgID, result.ApiKey.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	list, err = svc.List(ctx, orgID)
	if err != nil {
		t.Fatalf("List after revoke: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("List after revoke = %+v, want empty (revoked keys are hidden, matching the mock's own revoke-removes-from-list behavior)", list)
	}
}

func TestCreateRejectsInvalidInput(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, ctx, pool)
	svc := newService(pool)

	if _, err := svc.Create(ctx, orgID, userID, apikey.CreateInput{Name: "A", Scope: apikey.ScopeRead}); err == nil {
		t.Error("too-short name accepted")
	}
	if _, err := svc.Create(ctx, orgID, userID, apikey.CreateInput{Name: "Valid Name", Scope: "superuser"}); err == nil {
		t.Error("invalid scope accepted")
	}
}

func TestRevokeIsIdempotentlyRejectedTwice(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, ctx, pool)
	svc := newService(pool)

	result, err := svc.Create(ctx, orgID, userID, apikey.CreateInput{Name: "Once", Scope: apikey.ScopeWrite})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Revoke(ctx, orgID, result.ApiKey.ID); err != nil {
		t.Fatalf("first Revoke: %v", err)
	}
	if err := svc.Revoke(ctx, orgID, result.ApiKey.ID); err == nil {
		t.Fatal("second Revoke on an already-revoked key succeeded, want not-found")
	}
}

func TestCrossOrgIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA, userA := seedOrgAndUser(t, ctx, pool)
	orgB, _ := seedOrgAndUser(t, ctx, pool)
	svc := newService(pool)

	created, err := svc.Create(ctx, orgA, userA, apikey.CreateInput{Name: "Org A Key", Scope: apikey.ScopeRead})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	listB, err := svc.List(ctx, orgB)
	if err != nil {
		t.Fatalf("List (org B): %v", err)
	}
	if len(listB) != 0 {
		t.Fatalf("org B sees %+v, want no keys from org A", listB)
	}
	if err := svc.Revoke(ctx, orgB, created.ApiKey.ID); err == nil {
		t.Fatal("org B revoked org A's key, want not-found")
	}
}

func TestAuthenticateValidKey(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, ctx, pool)
	svc := newService(pool)

	created, err := svc.Create(ctx, orgID, userID, apikey.CreateInput{Name: "Integration", Scope: apikey.ScopeWrite})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	gotOrg, gotUser, gotScope, err := svc.Authenticate(ctx, created.FullKey)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if gotOrg != orgID {
		t.Errorf("organizationID = %q, want %q", gotOrg, orgID)
	}
	if gotUser != userID {
		t.Errorf("userID = %q, want the key's creator %q", gotUser, userID)
	}
	if gotScope != string(apikey.ScopeWrite) {
		t.Errorf("scope = %q, want %q", gotScope, apikey.ScopeWrite)
	}

	// last_used_at was a dead column before this phase (finding #4) —
	// confirm Authenticate actually wrote it, not just returned identity.
	list, err := svc.List(ctx, orgID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].LastUsedAt == nil {
		t.Fatalf("List = %+v, want the authenticated key's LastUsedAt set", list)
	}
}

func TestAuthenticateRejectsRevokedKey(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, ctx, pool)
	svc := newService(pool)

	created, err := svc.Create(ctx, orgID, userID, apikey.CreateInput{Name: "To be revoked", Scope: apikey.ScopeAdmin})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Revoke(ctx, orgID, created.ApiKey.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	if _, _, _, err := svc.Authenticate(ctx, created.FullKey); err == nil {
		t.Fatal("Authenticate accepted a revoked key")
	}
}

func TestAuthenticateRejectsUnknownOrGarbageKey(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	svc := newService(pool)

	cases := []string{
		"flx_live_" + strings.Repeat("z", 32), // right shape, never issued
		"not-even-the-right-prefix",
		"",
	}
	for _, raw := range cases {
		if _, _, _, err := svc.Authenticate(ctx, raw); err == nil {
			t.Errorf("Authenticate(%q) accepted, want rejected", raw)
		}
	}
}
