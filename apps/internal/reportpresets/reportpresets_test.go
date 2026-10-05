package reportpresets_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/reportpresets"
)

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

func validInput(name string) reportpresets.CreateInput {
	days := 14
	return reportpresets.CreateInput{
		Name:       name,
		Dimensions: []string{"campaign", "source"},
		Metrics:    []string{"clicks", "revenue"},
		GroupBy:    "campaign",
		Period:     reportpresets.Period{Type: reportpresets.PeriodRelative, Days: &days},
		Timezone:   "UTC",
	}
}

func TestListSeedsDefaultPreset(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, _ := seedOrgAndUser(t, ctx, pool)
	svc := reportpresets.NewService(reportpresets.NewRepository(pool))

	list, err := svc.List(ctx, orgID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || !list[0].IsDefault {
		t.Fatalf("List = %+v, want exactly one default preset", list)
	}

	// Calling List again must not create a second default.
	list2, err := svc.List(ctx, orgID)
	if err != nil {
		t.Fatalf("second List: %v", err)
	}
	if len(list2) != 1 || list2[0].ID != list[0].ID {
		t.Fatalf("second List = %+v, want the same single default preset", list2)
	}
}

func TestCreateUpdateDelete(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, ctx, pool)
	svc := reportpresets.NewService(reportpresets.NewRepository(pool))

	created, err := svc.Create(ctx, orgID, userID, validInput("Source Performance"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.IsDefault {
		t.Fatal("a caller-created preset must never be the default")
	}

	newName := "Source Performance — 7d"
	updated, err := svc.Update(ctx, orgID, created.ID, reportpresets.UpdateInput{Name: &newName})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != newName {
		t.Fatalf("Name = %q, want %q", updated.Name, newName)
	}
	// Unrelated fields must survive a partial update untouched.
	if len(updated.Dimensions) != 2 || updated.GroupBy != "campaign" {
		t.Fatalf("partial update clobbered unrelated fields: %+v", updated)
	}

	if err := svc.Delete(ctx, orgID, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, orgID, created.ID); err == nil {
		t.Fatal("Get after Delete succeeded, want not-found")
	}
}

func TestDefaultPresetCannotBeEditedOrDeleted(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, _ := seedOrgAndUser(t, ctx, pool)
	svc := reportpresets.NewService(reportpresets.NewRepository(pool))

	list, err := svc.List(ctx, orgID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	defaultID := list[0].ID

	newName := "Renamed"
	if _, err := svc.Update(ctx, orgID, defaultID, reportpresets.UpdateInput{Name: &newName}); err == nil {
		t.Fatal("renamed the default preset, want rejected")
	}
	if err := svc.Delete(ctx, orgID, defaultID); err == nil {
		t.Fatal("deleted the default preset, want rejected")
	}
}

func TestCreateRejectsInvalidInput(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, ctx, pool)
	svc := reportpresets.NewService(reportpresets.NewRepository(pool))

	cases := []struct {
		name  string
		build func() reportpresets.CreateInput
	}{
		{"empty name", func() reportpresets.CreateInput { in := validInput(""); return in }},
		{"unknown dimension", func() reportpresets.CreateInput {
			in := validInput("X")
			in.Dimensions = []string{"pwa"} // real, but not yet backed by chstore — must be rejected
			return in
		}},
		{"too many dimensions", func() reportpresets.CreateInput {
			in := validInput("X")
			in.Dimensions = []string{"campaign", "source", "country", "region", "city"}
			return in
		}},
		{"unknown group by", func() reportpresets.CreateInput {
			in := validInput("X")
			in.GroupBy = "not_a_dimension"
			return in
		}},
		{"empty metrics", func() reportpresets.CreateInput {
			in := validInput("X")
			in.Metrics = nil
			return in
		}},
		{"unknown metric", func() reportpresets.CreateInput {
			in := validInput("X")
			in.Metrics = []string{"not_a_metric"}
			return in
		}},
		{"relative days out of range", func() reportpresets.CreateInput {
			in := validInput("X")
			days := 0
			in.Period = reportpresets.Period{Type: reportpresets.PeriodRelative, Days: &days}
			return in
		}},
		{"custom period missing dates", func() reportpresets.CreateInput {
			in := validInput("X")
			in.Period = reportpresets.Period{Type: reportpresets.PeriodCustom}
			return in
		}},
		{"custom period inverted range", func() reportpresets.CreateInput {
			in := validInput("X")
			from, to := "2026-02-01", "2026-01-01"
			in.Period = reportpresets.Period{Type: reportpresets.PeriodCustom, From: &from, To: &to}
			return in
		}},
		{"invalid timezone", func() reportpresets.CreateInput {
			in := validInput("X")
			in.Timezone = "Not/A_Zone"
			return in
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.Create(ctx, orgID, userID, tc.build()); err == nil {
				t.Errorf("%s: accepted, want rejected", tc.name)
			}
		})
	}
}

func TestCustomPeriodRoundTrips(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, ctx, pool)
	svc := reportpresets.NewService(reportpresets.NewRepository(pool))

	in := validInput("Custom Range")
	from, to := "2026-01-01", "2026-01-31"
	in.Period = reportpresets.Period{Type: reportpresets.PeriodCustom, From: &from, To: &to}

	created, err := svc.Create(ctx, orgID, userID, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Period.Type != reportpresets.PeriodCustom || created.Period.From == nil || *created.Period.From != from || created.Period.To == nil || *created.Period.To != to {
		t.Fatalf("Period = %+v, want custom %s..%s", created.Period, from, to)
	}

	fetched, err := svc.Get(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fetched.Period.From == nil || *fetched.Period.From != from {
		t.Fatalf("Get: Period.From = %v, want %s", fetched.Period.From, from)
	}
}

func TestCrossOrgIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA, userA := seedOrgAndUser(t, ctx, pool)
	orgB, _ := seedOrgAndUser(t, ctx, pool)
	svc := reportpresets.NewService(reportpresets.NewRepository(pool))

	created, err := svc.Create(ctx, orgA, userA, validInput("Org A Preset"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := svc.Get(ctx, orgB, created.ID); err == nil {
		t.Fatal("org B fetched org A's preset, want not-found")
	}
	if err := svc.Delete(ctx, orgB, created.ID); err == nil {
		t.Fatal("org B deleted org A's preset, want not-found")
	}

	// Org B's own default must be independent of org A's.
	listB, err := svc.List(ctx, orgB)
	if err != nil {
		t.Fatalf("List (org B): %v", err)
	}
	if len(listB) != 1 || !listB[0].IsDefault || listB[0].OrganizationID != orgB {
		t.Fatalf("org B List = %+v, want its own independent default", listB)
	}
}
