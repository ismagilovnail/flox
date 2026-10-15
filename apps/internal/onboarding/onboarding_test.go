package onboarding_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/network"
	"github.com/ismagilovnail/flox/apps/internal/onboarding"
	"github.com/ismagilovnail/flox/apps/internal/trafficsource"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

type fakeSteps map[string]bool

func (f fakeSteps) Done(context.Context, string) (map[string]bool, error) {
	out := map[string]bool{}
	for k, v := range f {
		out[k] = v
	}
	return out, nil
}

type fakeClicks struct {
	has bool
	err error
}

func (f fakeClicks) HasClick(context.Context, string) (bool, error) { return f.has, f.err }

func step(t *testing.T, c onboarding.Checklist, id string) onboarding.Step {
	t.Helper()
	for _, s := range c.Steps {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no step %q in %+v", id, c.Steps)
	return onboarding.Step{}
}

func TestChecklistOrderAndCounts(t *testing.T) {
	svc := onboarding.NewService(fakeSteps{onboarding.StepSource: true, onboarding.StepNetwork: true}, fakeClicks{has: true}, quiet)
	c, err := svc.Checklist(context.Background(), "org")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"source", "network", "mapping", "offer", "campaign", "click", "conversion", "cost"}
	if len(c.Steps) != len(want) || c.Total != len(want) {
		t.Fatalf("steps = %+v", c.Steps)
	}
	for i, id := range want {
		if c.Steps[i].ID != id {
			t.Errorf("step %d = %q, want %q", i, c.Steps[i].ID, id)
		}
	}
	if c.Done != 3 || !step(t, c, "click").Done {
		t.Errorf("done = %d, click %+v; want 3 with the click", c.Done, step(t, c, "click"))
	}
}

func TestClickStepWithoutClickHouse(t *testing.T) {
	ctx := context.Background()
	// No ClickHouse at all, or ClickHouse failing: unknown, not done.
	for name, probe := range map[string]onboarding.ClickProbe{
		"none":    nil,
		"failing": fakeClicks{err: errors.New("down")},
	} {
		c, err := onboarding.NewService(fakeSteps{}, probe, quiet).Checklist(ctx, "org")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if s := step(t, c, "click"); s.Done || !s.Unknown {
			t.Errorf("%s: click = %+v, want unknown and not done", name, s)
		}
	}
	// A recorded conversion proves a click even when ClickHouse cannot.
	c, err := onboarding.NewService(fakeSteps{onboarding.StepConversion: true}, nil, quiet).Checklist(ctx, "org")
	if err != nil {
		t.Fatal(err)
	}
	if s := step(t, c, "click"); !s.Done || s.Unknown {
		t.Errorf("click with a conversion = %+v, want done", s)
	}
}

func TestChecklistNeedsAnOrganization(t *testing.T) {
	if _, err := onboarding.NewService(fakeSteps{}, nil, quiet).Checklist(context.Background(), ""); err == nil {
		t.Fatal("Checklist without an organization succeeded")
	}
}

// Every step query is scoped to the organization (CLAUDE.md #5): one
// organization's network and source never tick another's checklist.
func TestRepositoryIsScopedToTheOrganization(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	orgA, orgB := seedOrg(t, pool), seedOrg(t, pool)

	if _, err := trafficsource.NewService(trafficsource.NewRepository(pool)).Create(ctx, orgA, trafficsource.CreateInput{Name: "Source A", Type: "facebook", TrackingTemplate: "https://example.com/?c={click_id}", CostIntegration: trafficsource.CostIntegrationNone}); err != nil {
		t.Fatalf("creating a traffic source: %v", err)
	}
	if _, err := network.NewService(network.NewRepository(pool)).Create(ctx, orgA, network.CreateInput{Name: "Network A"}); err != nil {
		t.Fatalf("creating a network: %v", err)
	}

	repo := onboarding.NewRepository(pool)
	a, err := repo.Done(ctx, orgA)
	if err != nil {
		t.Fatal(err)
	}
	if !a["source"] || !a["network"] || a["mapping"] || a["offer"] || a["campaign"] || a["conversion"] || a["cost"] {
		t.Errorf("org A = %v, want only source and network", a)
	}
	b, err := repo.Done(ctx, orgB)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range b {
		if v {
			t.Errorf("org B step %q is done from org A's data", k)
		}
	}
}

func seedOrg(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := idgen.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO organizations (id, name) VALUES ($1, $2)`, id, "Test Org "+id); err != nil {
		t.Fatalf("seeding organization: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, id) })
	return id
}
