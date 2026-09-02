package domain_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/domain"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

func TestCreateGetUpdateDelete(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := domain.NewService(domain.NewRepository(pool))

	created, err := svc.Create(ctx, orgID, domain.CreateInput{
		Domain:      "Track.Example.com",
		Purpose:     []string{"tracking"},
		Registrar:   domain.RegistrarUnmanaged,
		DNSProvider: domain.DNSProviderUnmanaged,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Domain != "track.example.com" {
		t.Fatalf("Domain = %q, want lowercased", created.Domain)
	}
	if created.Status != domain.StatusActive {
		t.Fatalf("Status = %q, want active default", created.Status)
	}
	if created.SSL != domain.SSLNone {
		t.Fatalf("SSL = %q, want none (nothing issues certs yet)", created.SSL)
	}
	if created.VerifiedAt != nil || created.ExpiresAt != nil {
		t.Fatalf("VerifiedAt/ExpiresAt should stay nil, got %+v", created)
	}

	got, err := svc.Get(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("Get returned a different domain")
	}

	status := domain.StatusPending
	updated, err := svc.Update(ctx, orgID, created.ID, domain.UpdateInput{Status: &status})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Status != domain.StatusPending {
		t.Fatalf("Status after update = %q, want pending", updated.Status)
	}

	if err := svc.Delete(ctx, orgID, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, orgID, created.ID); !isNotFound(err) {
		t.Fatalf("Get after delete: err = %v, want not_found", err)
	}
}

func TestCreateRejectsDuplicateDomainAcrossOrgs(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)
	svc := domain.NewService(domain.NewRepository(pool))

	if _, err := svc.Create(ctx, orgA, domain.CreateInput{Domain: "shared.example.com", Purpose: []string{"tracking"}}); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := svc.Create(ctx, orgB, domain.CreateInput{Domain: "shared.example.com", Purpose: []string{"tracking"}})
	apiErr, ok := err.(*apierror.Error)
	if !ok || apiErr.Code != "conflict" {
		t.Fatalf("second Create err = %v, want a conflict (one hostname can't belong to two orgs)", err)
	}
}

func TestCreateRejectsInvalidDomainAndEmptyPurpose(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := domain.NewService(domain.NewRepository(pool))

	if _, err := svc.Create(ctx, orgID, domain.CreateInput{Domain: "not a domain", Purpose: []string{"tracking"}}); err == nil {
		t.Fatal("expected an error for an invalid domain string")
	}
	if _, err := svc.Create(ctx, orgID, domain.CreateInput{Domain: "valid.example.com", Purpose: nil}); err == nil {
		t.Fatal("expected an error for empty purpose")
	}
}

func TestListReturnsOrgsOwnDomainsOnly(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)
	svc := domain.NewService(domain.NewRepository(pool))

	if _, err := svc.Create(ctx, orgA, domain.CreateInput{Domain: "a.example.com", Purpose: []string{"tracking"}}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Create(ctx, orgB, domain.CreateInput{Domain: "b.example.com", Purpose: []string{"tracking"}}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	domains, err := svc.List(ctx, orgA)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(domains) != 1 || domains[0].Domain != "a.example.com" {
		t.Fatalf("domains = %+v, want only org A's own", domains)
	}
}

func isNotFound(err error) bool {
	apiErr, ok := err.(*apierror.Error)
	return ok && apiErr.Code == "not_found"
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
