package eventmapping_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/eventmapping"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

func TestCreateListDelete(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID)

	svc := eventmapping.NewService(eventmapping.NewRepository(pool))

	created, err := svc.Create(ctx, orgID, eventmapping.CreateInput{
		NetworkID: networkID, NetworkStatus: "ftd", FloxStatus: event.CpaAccept,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.NetworkStatus != "ftd" || created.FloxStatus != event.CpaAccept {
		t.Fatalf("Create result = %+v, want networkStatus=ftd floxStatus=CPA_ACCEPT", created)
	}

	list, err := svc.List(ctx, orgID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("List = %+v, want exactly the created mapping", list)
	}

	if err := svc.Delete(ctx, orgID, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	list, err = svc.List(ctx, orgID)
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("List after delete = %+v, want empty", list)
	}
}

func TestCreateRejectsInvalidFloxStatus(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID)

	svc := eventmapping.NewService(eventmapping.NewRepository(pool))
	_, err := svc.Create(ctx, orgID, eventmapping.CreateInput{
		NetworkID: networkID, NetworkStatus: "ftd", FloxStatus: "NOT_A_REAL_STATUS",
	})

	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "validation" {
		t.Fatalf("Create with an invalid floxStatus: err = %v, want a validation apierror", err)
	}
}

func TestCreateRejectsDuplicateCaseInsensitively(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID)

	svc := eventmapping.NewService(eventmapping.NewRepository(pool))
	if _, err := svc.Create(ctx, orgID, eventmapping.CreateInput{
		NetworkID: networkID, NetworkStatus: "FTD", FloxStatus: event.CpaAccept,
	}); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	_, err := svc.Create(ctx, orgID, eventmapping.CreateInput{
		NetworkID: networkID, NetworkStatus: "ftd", FloxStatus: event.CpaHold,
	})
	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "conflict" {
		t.Fatalf("Create with a case-insensitive duplicate status: err = %v, want a conflict apierror", err)
	}
}

func TestCreateRejectsUnknownNetwork(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := eventmapping.NewService(eventmapping.NewRepository(pool))
	_, err := svc.Create(ctx, orgID, eventmapping.CreateInput{
		NetworkID: idgen.New(), NetworkStatus: "ftd", FloxStatus: event.CpaAccept,
	})

	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "validation" {
		t.Fatalf("Create with an unknown network id: err = %v, want a validation apierror", err)
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)
	networkA := seedNetwork(t, ctx, pool, orgA)

	svc := eventmapping.NewService(eventmapping.NewRepository(pool))
	created, err := svc.Create(ctx, orgA, eventmapping.CreateInput{
		NetworkID: networkA, NetworkStatus: "ftd", FloxStatus: event.CpaAccept,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// org A's network doesn't belong to org B — creating against it fails.
	if _, err := svc.Create(ctx, orgB, eventmapping.CreateInput{
		NetworkID: networkA, NetworkStatus: "ftd", FloxStatus: event.CpaAccept,
	}); err == nil {
		t.Fatal("Create against another org's network: want an error")
	}

	listB, err := svc.List(ctx, orgB)
	if err != nil {
		t.Fatalf("List (org B): %v", err)
	}
	if len(listB) != 0 {
		t.Fatalf("org B's List = %+v, want empty (org A's mapping must not leak)", listB)
	}

	if err := svc.Delete(ctx, orgB, created.ID); err == nil {
		t.Fatal("Delete org A's mapping as org B: want a not-found error")
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

func seedNetwork(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID string) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx, `INSERT INTO networks (id, organization_id, name) VALUES ($1, $2, $3)`, id, orgID, "Test Network")
	if err != nil {
		t.Fatalf("seeding network: %v", err)
	}
	return id
}
