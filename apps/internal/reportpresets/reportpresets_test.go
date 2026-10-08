package reportpresets_test

import (
	"context"
	"os"
	"slices"
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
	if _, err := pool.Exec(ctx, `UPDATE organizations SET timezone = 'Europe/Moscow' WHERE id = $1`, orgID); err != nil {
		t.Fatal(err)
	}
	svc := reportpresets.NewService(reportpresets.NewRepository(pool))

	list, err := svc.List(ctx, orgID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || !list[0].IsDefault {
		t.Fatalf("List = %+v, want exactly one default preset", list)
	}
	// The organization's timezone, not UTC (audit #2), and the funnel
	// metrics in the column set (audit #12).
	if list[0].Timezone != "Europe/Moscow" || !slices.Contains(list[0].Metrics, "pwaInstalls") || !slices.Contains(list[0].Metrics, "costPerPurchase") {
		t.Fatalf("default preset = timezone %q, metrics %v", list[0].Timezone, list[0].Metrics)
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
			in.Dimensions = []string{"postlanding"} // real, but not yet backed by chstore — must be rejected
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

// TestPresetAcceptsTheDayDimension: "day" is the report builder's per-day
// pseudo-dimension, valid in a preset's dimensions and as its group-by.
func TestPresetAcceptsTheDayDimension(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, ctx, pool)
	svc := reportpresets.NewService(reportpresets.NewRepository(pool))

	in := validInput("Daily by campaign")
	in.Dimensions = []string{"campaign", "day"}
	in.GroupBy = "day"
	created, err := svc.Create(ctx, orgID, userID, in)
	if err != nil {
		t.Fatalf("Create with the day dimension: %v", err)
	}
	if len(created.Dimensions) != 2 || created.Dimensions[1] != "day" || created.GroupBy != "day" {
		t.Fatalf("preset = %+v, want dimensions [campaign day] grouped by day", created)
	}
}

// TestPresetFiltersRoundTripUpdateAndClear: filters are saved with the preset,
// come back on read, survive a rename (nil = unchanged), can be replaced and can
// be cleared with an empty list; a preset made without any reads back as [].
func TestPresetFiltersRoundTripUpdateAndClear(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, ctx, pool)
	svc := reportpresets.NewService(reportpresets.NewRepository(pool))

	plain, err := svc.Create(ctx, orgID, userID, validInput("No filters"))
	if err != nil {
		t.Fatalf("Create (no filters): %v", err)
	}
	if plain.Filters == nil || len(plain.Filters) != 0 {
		t.Fatalf("Filters = %#v, want an empty non-nil list (serialized as [], not null)", plain.Filters)
	}

	in := validInput("Facebook, Linux")
	in.Filters = []reportpresets.Filter{{Dimension: "source", Value: "src-1"}, {Dimension: "os", Value: "linux"}}
	created, err := svc.Create(ctx, orgID, userID, in)
	if err != nil {
		t.Fatalf("Create (filters): %v", err)
	}
	if len(created.Filters) != 2 || created.Filters[0].Dimension != "source" || created.Filters[1].Value != "linux" {
		t.Fatalf("created filters = %+v", created.Filters)
	}

	renamed := "Renamed"
	got, err := svc.Update(ctx, orgID, created.ID, reportpresets.UpdateInput{Name: &renamed})
	if err != nil {
		t.Fatalf("Update (rename): %v", err)
	}
	if len(got.Filters) != 2 {
		t.Fatalf("a rename dropped the filters: %+v", got.Filters)
	}

	got, err = svc.Update(ctx, orgID, created.ID, reportpresets.UpdateInput{Filters: []reportpresets.Filter{{Dimension: "country", Value: "US"}}})
	if err != nil || len(got.Filters) != 1 || got.Filters[0].Dimension != "country" {
		t.Fatalf("Update (replace) = %+v, %v", got.Filters, err)
	}

	got, err = svc.Update(ctx, orgID, created.ID, reportpresets.UpdateInput{Filters: []reportpresets.Filter{}})
	if err != nil || len(got.Filters) != 0 {
		t.Fatalf("Update (clear) = %+v, %v, want no filters", got.Filters, err)
	}

	fetched, err := svc.Get(ctx, orgID, created.ID)
	if err != nil || len(fetched.Filters) != 0 {
		t.Fatalf("Get after clear = %+v, %v", fetched.Filters, err)
	}
}

func TestPresetRejectsInvalidFilters(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, ctx, pool)
	svc := reportpresets.NewService(reportpresets.NewRepository(pool))

	for name, filters := range map[string][]reportpresets.Filter{
		"unknown dimension": {{Dimension: "nope", Value: "x"}},
		"day is a bucket":   {{Dimension: "day", Value: "2026-08-01"}},
		"value too long":    {{Dimension: "os", Value: string(make([]byte, 201))}},
	} {
		in := validInput("Bad " + name)
		in.Filters = filters
		if _, err := svc.Create(ctx, orgID, userID, in); err == nil {
			t.Fatalf("%s: Create succeeded, want a validation error", name)
		}
	}
	tooMany := make([]reportpresets.Filter, 11)
	for i := range tooMany {
		tooMany[i] = reportpresets.Filter{Dimension: "os", Value: "x"}
	}
	in := validInput("Too many")
	in.Filters = tooMany
	if _, err := svc.Create(ctx, orgID, userID, in); err == nil {
		t.Fatal("11 filters: Create succeeded, want a validation error")
	}
}

// TestPresetColumnOrderRoundTripUpdateAndClear: the dragged column order is
// saved with the preset, survives a rename (nil = unchanged), can be replaced
// and reset with an empty list; a preset made without one reads back as [].
func TestPresetColumnOrderRoundTripUpdateAndClear(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, ctx, pool)
	svc := reportpresets.NewService(reportpresets.NewRepository(pool))

	plain, err := svc.Create(ctx, orgID, userID, validInput("Natural order"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if plain.ColumnOrder == nil || len(plain.ColumnOrder) != 0 {
		t.Fatalf("ColumnOrder = %#v, want an empty non-nil list (serialized as [], not null)", plain.ColumnOrder)
	}

	in := validInput("Dragged")
	in.ColumnOrder = []string{"revenue", "campaign", "clicks"}
	created, err := svc.Create(ctx, orgID, userID, in)
	if err != nil {
		t.Fatalf("Create (order): %v", err)
	}
	if len(created.ColumnOrder) != 3 || created.ColumnOrder[0] != "revenue" || created.ColumnOrder[2] != "clicks" {
		t.Fatalf("created order = %v", created.ColumnOrder)
	}

	renamed := "Renamed"
	got, err := svc.Update(ctx, orgID, created.ID, reportpresets.UpdateInput{Name: &renamed})
	if err != nil || len(got.ColumnOrder) != 3 {
		t.Fatalf("a rename must keep the order: %v, %v", got.ColumnOrder, err)
	}

	got, err = svc.Update(ctx, orgID, created.ID, reportpresets.UpdateInput{ColumnOrder: []string{"clicks", "revenue"}})
	if err != nil || len(got.ColumnOrder) != 2 || got.ColumnOrder[0] != "clicks" {
		t.Fatalf("Update (replace) = %v, %v", got.ColumnOrder, err)
	}

	got, err = svc.Update(ctx, orgID, created.ID, reportpresets.UpdateInput{ColumnOrder: []string{}})
	if err != nil || len(got.ColumnOrder) != 0 {
		t.Fatalf("Update (reset) = %v, %v, want the natural order", got.ColumnOrder, err)
	}

	bad := validInput("Bad order")
	bad.ColumnOrder = []string{"clicks", ""}
	if _, err := svc.Create(ctx, orgID, userID, bad); err == nil {
		t.Fatal("an empty column id was accepted, want a validation error")
	}
	bad.ColumnOrder = make([]string, 61)
	for i := range bad.ColumnOrder {
		bad.ColumnOrder[i] = "clicks"
	}
	if _, err := svc.Create(ctx, orgID, userID, bad); err == nil {
		t.Fatal("61 columns were accepted, want a validation error")
	}
}

// TestPresetColumnWidthsRoundTripUpdateAndClear: dragged column widths are
// saved with the preset, survive a rename (nil = unchanged), can be replaced and
// reset with an empty map; a preset made without any reads back as {}.
func TestPresetColumnWidthsRoundTripUpdateAndClear(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, ctx, pool)
	svc := reportpresets.NewService(reportpresets.NewRepository(pool))

	plain, err := svc.Create(ctx, orgID, userID, validInput("Natural widths"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if plain.ColumnWidths == nil || len(plain.ColumnWidths) != 0 {
		t.Fatalf("ColumnWidths = %#v, want an empty non-nil map (serialized as {}, not null)", plain.ColumnWidths)
	}

	in := validInput("Wide")
	in.ColumnWidths = map[string]int{"campaign": 320, "clicks": 90}
	created, err := svc.Create(ctx, orgID, userID, in)
	if err != nil {
		t.Fatalf("Create (widths): %v", err)
	}
	if created.ColumnWidths["campaign"] != 320 || created.ColumnWidths["clicks"] != 90 {
		t.Fatalf("created widths = %v", created.ColumnWidths)
	}

	renamed := "Renamed"
	got, err := svc.Update(ctx, orgID, created.ID, reportpresets.UpdateInput{Name: &renamed})
	if err != nil || len(got.ColumnWidths) != 2 {
		t.Fatalf("a rename must keep the widths: %v, %v", got.ColumnWidths, err)
	}

	got, err = svc.Update(ctx, orgID, created.ID, reportpresets.UpdateInput{ColumnWidths: map[string]int{"revenue": 150}})
	if err != nil || len(got.ColumnWidths) != 1 || got.ColumnWidths["revenue"] != 150 {
		t.Fatalf("Update (replace) = %v, %v", got.ColumnWidths, err)
	}

	got, err = svc.Update(ctx, orgID, created.ID, reportpresets.UpdateInput{ColumnWidths: map[string]int{}})
	if err != nil || len(got.ColumnWidths) != 0 {
		t.Fatalf("Update (reset) = %v, %v, want natural widths", got.ColumnWidths, err)
	}

	for name, widths := range map[string]map[string]int{
		"too narrow": {"clicks": 10},
		"too wide":   {"clicks": 5000},
		"empty id":   {"": 100},
	} {
		bad := validInput("Bad " + name)
		bad.ColumnWidths = widths
		if _, err := svc.Create(ctx, orgID, userID, bad); err == nil {
			t.Fatalf("%s: Create succeeded, want a validation error", name)
		}
	}
}

func TestPresetHiddenColumnsRoundTripUpdateAndClear(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, ctx, pool)
	svc := reportpresets.NewService(reportpresets.NewRepository(pool))

	plain, err := svc.Create(ctx, orgID, userID, validInput("All shown"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if plain.HiddenColumns == nil || len(plain.HiddenColumns) != 0 {
		t.Fatalf("HiddenColumns = %#v, want an empty non-nil slice (serialized as [], not null)", plain.HiddenColumns)
	}

	in := validInput("Trimmed")
	in.HiddenColumns = []string{"uniqueClicks", "cm_abc"}
	created, err := svc.Create(ctx, orgID, userID, in)
	if err != nil {
		t.Fatalf("Create (hidden): %v", err)
	}
	if len(created.HiddenColumns) != 2 || created.HiddenColumns[0] != "uniqueClicks" {
		t.Fatalf("created hidden = %v", created.HiddenColumns)
	}

	renamed := "Renamed"
	got, err := svc.Update(ctx, orgID, created.ID, reportpresets.UpdateInput{Name: &renamed})
	if err != nil || len(got.HiddenColumns) != 2 {
		t.Fatalf("a rename must keep the hidden columns: %v, %v", got.HiddenColumns, err)
	}

	got, err = svc.Update(ctx, orgID, created.ID, reportpresets.UpdateInput{HiddenColumns: []string{"cpa"}})
	if err != nil || len(got.HiddenColumns) != 1 || got.HiddenColumns[0] != "cpa" {
		t.Fatalf("Update (replace) = %v, %v", got.HiddenColumns, err)
	}

	got, err = svc.Update(ctx, orgID, created.ID, reportpresets.UpdateInput{HiddenColumns: []string{}})
	if err != nil || len(got.HiddenColumns) != 0 {
		t.Fatalf("Update (reset) = %v, %v, want every column shown", got.HiddenColumns, err)
	}

	bad := validInput("Bad hidden")
	bad.HiddenColumns = []string{""}
	if _, err := svc.Create(ctx, orgID, userID, bad); err == nil {
		t.Fatal("empty hidden column id: Create succeeded, want a validation error")
	}
}
