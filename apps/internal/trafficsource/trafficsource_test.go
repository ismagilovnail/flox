package trafficsource_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/trafficsource"
)

func TestListReturnsOrgsOwnSourcesOrderedByName(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)

	seedSource(t, ctx, pool, orgA, "Zulu Ads", "Facebook")
	seedSource(t, ctx, pool, orgA, "Alpha Ads", "TikTok")
	seedSource(t, ctx, pool, orgB, "Org B Source", "Google")

	repo := trafficsource.NewRepository(pool)
	sources, err := repo.List(ctx, orgA)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("sources = %d, want 2 (org A's own only)", len(sources))
	}
	if sources[0].Name != "Alpha Ads" || sources[1].Name != "Zulu Ads" {
		t.Fatalf("sources = %+v, want alphabetical order", sources)
	}
	for _, s := range sources {
		if s.ID == "" || s.Type == "" || s.Status == "" {
			t.Fatalf("source missing a field: %+v", s)
		}
	}
}

func TestListEmptyIsEmptySliceNotNil(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	repo := trafficsource.NewRepository(pool)
	sources, err := repo.List(ctx, orgID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if sources == nil {
		t.Fatal("List returned nil, want an empty (non-nil) slice so JSON encodes [] not null")
	}
	if len(sources) != 0 {
		t.Fatalf("sources = %d, want 0", len(sources))
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

func seedSource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID, name, typ string) {
	t.Helper()
	_, err := pool.Exec(ctx,
		`INSERT INTO traffic_sources (id, organization_id, name, type) VALUES ($1, $2, $3, $4)`,
		idgen.New(), orgID, name, typ,
	)
	if err != nil {
		t.Fatalf("seeding traffic source: %v", err)
	}
}
