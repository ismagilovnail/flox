package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/billing"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// BIZ-1: a create past the plan's count limit answers 402 plan_limit
// through the real router, a plan change lifts it, and GET /billing
// counts the caller's organization only.
func TestPlanLimitsThroughTheAPI(t *testing.T) {
	env := newIsolationEnv(t)
	ctx, pool, c := env.ctx, env.pool, env.c

	status := func(orgID string) billing.Status {
		t.Helper()
		c.signIn(orgID)
		code, body := c.do(http.MethodGet, "/billing")
		if code != http.StatusOK {
			t.Fatalf("GET /billing = %d: %s", code, body)
		}
		var st billing.Status
		if err := json.Unmarshal([]byte(body), &st); err != nil {
			t.Fatal(err)
		}
		return st
	}
	countCampaigns := func(orgID string) int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM campaigns WHERE organization_id = $1 AND status <> 'archived'`, orgID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	a := status(env.orgA)
	if a.Plan.Code != billing.FreePlan || a.Plan.MaxCampaigns == nil || a.Usage.Campaigns != countCampaigns(env.orgA) {
		t.Fatalf("org A status = %+v, want Free and its own %d campaigns", a, countCampaigns(env.orgA))
	}

	// Fill A up to the Free limit, then one more is refused.
	for n := countCampaigns(env.orgA); n < *a.Plan.MaxCampaigns; n++ {
		if _, err := pool.Exec(ctx, `INSERT INTO campaigns (id, organization_id, traffic_source_id, name, fallback_url)
			VALUES ($1, $2, $3, 'Filler', 'https://example.com')`, idgen.New(), env.orgA, env.idsA["trafficSource"]); err != nil {
			t.Fatalf("filling campaigns: %v", err)
		}
	}
	create := map[string]any{"trafficSourceId": env.idsA["trafficSource"], "name": "Over the limit", "fallbackUrl": "https://example.com/f"}
	c.signIn(env.orgA)
	code, body, _ := c.request(http.MethodPost, "/campaigns", create)
	if code != http.StatusPaymentRequired || !strings.Contains(body, `"code":"plan_limit"`) {
		t.Fatalf("campaign past the limit = %d %s, want 402 plan_limit", code, body)
	}
	code, body, _ = c.request(http.MethodPost, "/campaigns/"+env.idsA["campaign"]+"/duplicate", nil)
	if code != http.StatusPaymentRequired {
		t.Errorf("duplicate past the limit = %d %s, want 402", code, body)
	}

	// B has its own count: A being full does not stop B.
	if b := status(env.orgB); b.Usage.Campaigns != countCampaigns(env.orgB) || b.Usage.Campaigns == countCampaigns(env.orgA) {
		t.Errorf("org B counts %d campaigns, want its own %d", b.Usage.Campaigns, countCampaigns(env.orgB))
	}

	if err := billing.NewRepository(pool).SetPlan(ctx, env.orgA, "business", "api test"); err != nil {
		t.Fatal(err)
	}
	c.signIn(env.orgA)
	if code, body, _ := c.request(http.MethodPost, "/campaigns", create); code != http.StatusCreated {
		t.Errorf("campaign on Business = %d %s, want 201", code, body)
	}
	if a := status(env.orgA); a.Plan.Code != "business" || a.Plan.MaxCampaigns != nil {
		t.Errorf("org A after the change = %+v, want Business, unlimited", a.Plan)
	}

	code, body = c.do(http.MethodGet, "/billing/plans")
	if code != http.StatusOK || !strings.Contains(body, `"code":"free"`) {
		t.Errorf("GET /billing/plans = %d %s", code, body)
	}
}
