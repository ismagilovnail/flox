package custommetrics_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/custommetrics"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
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

func seedOrg(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	id := idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, id, "Test Org "+id); err != nil {
		t.Fatalf("seeding organization: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, id) })
	return id
}

// seedMember creates a real user + an active membership at roleKey
// ("Owner"/"Admin"/"Manager"/"Buyer"/"Analyst"/"Viewer") — real rows
// against the real roles table (migration 00001), not a fabricated
// role string, so Service.callerTier's own role -> tier logic is
// exercised for real.
func seedMember(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID, roleKey string) string {
	t.Helper()
	userID := idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name) VALUES ($1, $2, 'Test User')`, userID, userID+"@example.com"); err != nil {
		t.Fatalf("seeding user: %v", err)
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO memberships (id, organization_id, user_id, role_id, status)
		SELECT $1, $2, $3, r.id, 'active' FROM roles r WHERE r.key = $4`,
		idgen.New(), orgID, userID, roleKey,
	)
	if err != nil {
		t.Fatalf("seeding membership: %v", err)
	}
	// The organization first: its rows may reference the user.
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return userID
}

func validInput(formula string) custommetrics.CreateInput {
	return custommetrics.CreateInput{
		Name: "Margin per Click", GroupName: "Profitability", Formula: formula,
		Format: custommetrics.FormatCurrency, Status: custommetrics.StatusDraft,
	}
}

func TestCreateGetUpdateDelete(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	owner := seedMember(t, ctx, pool, orgID, "Owner")
	svc := custommetrics.NewService(custommetrics.NewRepository(pool))

	created, err := svc.Create(ctx, orgID, owner, validInput("({revenue} - {cost}) / {clicks}"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Status != custommetrics.StatusDraft || created.Active {
		t.Fatalf("new draft metric = %+v, want draft/inactive", created)
	}

	got, err := svc.Get(ctx, orgID, owner, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Formula != created.Formula {
		t.Fatalf("Get returned %+v, want the just-created row", got)
	}

	newName := "Renamed"
	updated, err := svc.Update(ctx, orgID, owner, created.ID, custommetrics.UpdateInput{Name: &newName})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != newName {
		t.Fatalf("Name = %q, want %q", updated.Name, newName)
	}

	if err := svc.Delete(ctx, orgID, owner, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, orgID, owner, created.ID); err == nil {
		t.Fatal("Get after Delete succeeded, want not-found")
	}
}

func TestCreateRejectsInvalidFormulas(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	owner := seedMember(t, ctx, pool, orgID, "Owner")
	svc := custommetrics.NewService(custommetrics.NewRepository(pool))

	cases := []struct {
		name    string
		formula string
	}{
		{"empty", ""},
		{"unknown metric", "{not_a_real_metric}"},
		{"unterminated brace", "{clicks"},
		{"unknown function", "NOPE({clicks})"},
		{"wrong arity", "DIV({clicks})"},
		{"ltv forbidden", "{ltv_total} / {clicks}"},
		{"mixed data sources", "{clicks} + {push_sent}"},
		{"trailing garbage", "{clicks} + {cost} )"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.Create(ctx, orgID, owner, validInput(tc.formula)); err == nil {
				t.Fatalf("Create(%q) succeeded, want a validation error", tc.formula)
			}
		})
	}
}

func TestSafeDivisionSyntaxAccepted(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	owner := seedMember(t, ctx, pool, orgID, "Owner")
	svc := custommetrics.NewService(custommetrics.NewRepository(pool))

	// Formula validity only — actual safe-division arithmetic runs
	// client-side (features/analytics/report-table.tsx), already covered
	// by the frontend's own formula-engine. This proves the server-side
	// parser accepts the same real-world formulas the UI does.
	for _, formula := range []string{
		"DIV({revenue}, {cost})",
		"EMPTYIF({cost}, 0)",
		"IF({clicks} > 0, {revenue}, 0)",
		"ROUND({roi}, 1)",
		"MIN({cpc}, {cpa})",
		"MAX({cpc}, {cpa}, {epc})",
		"{bots} / ({bots} + {click_all})",
	} {
		if _, err := svc.Create(ctx, orgID, owner, validInput(formula)); err != nil {
			t.Errorf("Create(%q): %v", formula, err)
		}
	}
}

func TestPublishFiltersUncomputableTargets(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	owner := seedMember(t, ctx, pool, orgID, "Owner")
	svc := custommetrics.NewService(custommetrics.NewRepository(pool))

	in := validInput("{clicks} + {cost}")
	in.Status = custommetrics.StatusPublished
	// offers_table can't compute anything yet (no backed offer dimension)
	// — report_builder and campaigns_table both can for this formula.
	in.Targets = []string{"report_builder", "campaigns_table", "offers_table"}

	created, err := svc.Create(ctx, orgID, owner, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	want := map[string]bool{"report_builder": true, "campaigns_table": true}
	if len(created.Targets) != len(want) {
		t.Fatalf("targets = %v, want exactly %v (offers_table silently dropped)", created.Targets, want)
	}
	for _, target := range created.Targets {
		if !want[target] {
			t.Errorf("unexpected target %q survived filtering", target)
		}
	}
	if !created.Active {
		t.Error("published metric should be active on creation")
	}
}

func TestUnpublishClearsTargets(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	owner := seedMember(t, ctx, pool, orgID, "Owner")
	svc := custommetrics.NewService(custommetrics.NewRepository(pool))

	in := validInput("{clicks}")
	in.Status = custommetrics.StatusPublished
	in.Targets = []string{"report_builder"}
	created, err := svc.Create(ctx, orgID, owner, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(created.Targets) == 0 {
		t.Fatal("expected the published metric to start with a target")
	}

	draft := custommetrics.StatusDraft
	updated, err := svc.Update(ctx, orgID, owner, created.ID, custommetrics.UpdateInput{Status: &draft})
	if err != nil {
		t.Fatalf("Update (unpublish): %v", err)
	}
	if len(updated.Targets) != 0 {
		t.Fatalf("targets after unpublish = %v, want empty (§30.5: drafts are invisible everywhere)", updated.Targets)
	}
}

func TestDeleteBlockedWhilePublishedOrExposed(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	owner := seedMember(t, ctx, pool, orgID, "Owner")
	svc := custommetrics.NewService(custommetrics.NewRepository(pool))

	in := validInput("{clicks}")
	in.Status = custommetrics.StatusPublished
	in.Targets = []string{"report_builder"}
	created, err := svc.Create(ctx, orgID, owner, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.Delete(ctx, orgID, owner, created.ID); err == nil {
		t.Fatal("Delete on a published, exposed metric succeeded, want conflict")
	}

	draft := custommetrics.StatusDraft
	if _, err := svc.Update(ctx, orgID, owner, created.ID, custommetrics.UpdateInput{Status: &draft}); err != nil {
		t.Fatalf("Update (unpublish): %v", err)
	}
	if err := svc.Delete(ctx, orgID, owner, created.ID); err != nil {
		t.Fatalf("Delete after unpublish (targets now clear): %v", err)
	}
}

func TestManagerCanManageOwnNotOthers(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	owner := seedMember(t, ctx, pool, orgID, "Owner")
	managerA := seedMember(t, ctx, pool, orgID, "Manager")
	managerB := seedMember(t, ctx, pool, orgID, "Manager")
	svc := custommetrics.NewService(custommetrics.NewRepository(pool))

	ownedByA, err := svc.Create(ctx, orgID, managerA, validInput("{clicks}"))
	if err != nil {
		t.Fatalf("Create as managerA: %v", err)
	}

	newName := "Hijacked"
	if _, err := svc.Update(ctx, orgID, managerB, ownedByA.ID, custommetrics.UpdateInput{Name: &newName}); err == nil {
		t.Fatal("managerB updated managerA's metric, want forbidden")
	}
	if err := svc.Delete(ctx, orgID, managerB, ownedByA.ID); err == nil {
		t.Fatal("managerB deleted managerA's metric, want forbidden")
	}

	// Owner can manage anyone's metric.
	if _, err := svc.Update(ctx, orgID, owner, ownedByA.ID, custommetrics.UpdateInput{Name: &newName}); err != nil {
		t.Fatalf("Owner managing managerA's metric: %v", err)
	}
}

func TestDuplicateOwnedByCaller(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	owner := seedMember(t, ctx, pool, orgID, "Owner")
	svc := custommetrics.NewService(custommetrics.NewRepository(pool))

	original, err := svc.Create(ctx, orgID, owner, validInput("{clicks}"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	dup, err := svc.Duplicate(ctx, orgID, owner, original.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if dup.Status != custommetrics.StatusDraft || len(dup.Targets) != 0 {
		t.Fatalf("duplicate = %+v, want a targetless draft", dup)
	}
	if dup.Name == original.Name {
		t.Fatalf("duplicate name %q should differ from original", dup.Name)
	}
}

func TestListVisibilityByRole(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	owner := seedMember(t, ctx, pool, orgID, "Owner")
	buyer := seedMember(t, ctx, pool, orgID, "Buyer")
	svc := custommetrics.NewService(custommetrics.NewRepository(pool))

	draftIn := validInput("{clicks}")
	if _, err := svc.Create(ctx, orgID, owner, draftIn); err != nil {
		t.Fatalf("Create draft: %v", err)
	}
	pubIn := validInput("{cost}")
	pubIn.Status = custommetrics.StatusPublished
	if _, err := svc.Create(ctx, orgID, owner, pubIn); err != nil {
		t.Fatalf("Create published: %v", err)
	}

	ownerList, err := svc.List(ctx, orgID, owner)
	if err != nil {
		t.Fatalf("List as owner: %v", err)
	}
	if len(ownerList) != 2 {
		t.Fatalf("owner sees %d metrics, want 2 (draft + published)", len(ownerList))
	}

	buyerList, err := svc.List(ctx, orgID, buyer)
	if err != nil {
		t.Fatalf("List as buyer: %v", err)
	}
	if len(buyerList) != 1 || buyerList[0].Status != custommetrics.StatusPublished {
		t.Fatalf("buyer sees %+v, want exactly the one published+active metric", buyerList)
	}
}

func TestCrossOrgIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)
	ownerA := seedMember(t, ctx, pool, orgA, "Owner")
	ownerB := seedMember(t, ctx, pool, orgB, "Owner")
	svc := custommetrics.NewService(custommetrics.NewRepository(pool))

	created, err := svc.Create(ctx, orgA, ownerA, validInput("{clicks}"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := svc.Get(ctx, orgB, ownerB, created.ID); err == nil {
		t.Fatal("org B fetched org A's metric, want not-found")
	}
	listB, err := svc.List(ctx, orgB, ownerB)
	if err != nil {
		t.Fatalf("List (org B): %v", err)
	}
	if len(listB) != 0 {
		t.Fatalf("org B sees %+v, want no metrics from org A", listB)
	}
}
