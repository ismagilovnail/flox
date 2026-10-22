package main

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/campaign"
	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// scopedReads are the reads of campaign data a Buyer is limited to their
// own campaigns on (audit TEAM-1b). {campaign}, {click}, {streamSet},
// {flow} and {network} are Ann's.
var scopedReads = []struct {
	method, route, path string
	body                any
}{
	{"GET", "/campaigns", "/campaigns", nil},
	{"GET", "/campaigns/{id}", "/campaigns/{campaign}", nil},
	{"GET", "/campaigns/{campaignId}/tracking-links", "/campaigns/{campaign}/tracking-links", nil},
	{"GET", "/campaigns/{campaignId}/stream-sets", "/campaigns/{campaign}/stream-sets", nil},
	{"GET", "/campaigns/{campaignId}/stream-sets/{id}", "/campaigns/{campaign}/stream-sets/{streamSet}", nil},
	{"GET", "/campaigns/{campaignId}/cost-entries", "/campaigns/{campaign}/cost-entries", nil},
	{"GET", "/campaigns/{campaignId}/cost-entries/daily", "/campaigns/{campaign}/cost-entries/daily?from={from}&to={to}", nil},
	{"POST", "/campaigns/{campaignId}/routing/simulate", "/campaigns/{campaign}/routing/simulate", map[string]any{}},
	{"GET", "/tracking-links", "/tracking-links", nil},
	{"GET", "/flows", "/flows", nil},
	{"GET", "/analytics/campaigns/{campaignId}/daily", "/analytics/campaigns/{campaign}/daily?from={from}&to={to}", nil},
	{"GET", "/analytics/campaigns/{campaignId}/daily-revenue", "/analytics/campaigns/{campaign}/daily-revenue?from={from}&to={to}", nil},
	{"POST", "/analytics/report", "/analytics/report", map[string]any{"dims": []string{"campaign"}, "from": "{from}", "to": "{to}"}},
	{"POST", "/analytics/timeseries", "/analytics/timeseries", map[string]any{"from": "{from}", "to": "{to}"}},
	{"POST", "/analytics/breakdown", "/analytics/breakdown", map[string]any{"dim": "campaign", "metric": "clicks", "from": "{from}", "to": "{to}"}},
	{"POST", "/analytics/funnel", "/analytics/funnel", map[string]any{"from": "{from}", "to": "{to}"}},
	{"POST", "/analytics/dashboard", "/analytics/dashboard", map[string]any{"from": "{from}", "to": "{to}"}},
	{"POST", "/analytics/dimension-values", "/analytics/dimension-values", map[string]any{"dim": "campaign", "from": "{from}", "to": "{to}"}},
	{"POST", "/analytics/flow-summary", "/analytics/flow-summary", map[string]any{"flowIds": []string{"{flow}"}, "from": "{from}", "to": "{to}"}},
	{"GET", "/analytics/buyers", "/analytics/buyers", nil},
	{"GET", "/analytics/ltv/ftd-cohorts", "/analytics/ltv/ftd-cohorts?from={from}&to={to}&groupBy=campaign", nil},
	{"GET", "/analytics/ltv/reg-cohorts", "/analytics/ltv/reg-cohorts?from={from}&to={to}&groupBy=campaign", nil},
	{"GET", "/conversions", "/conversions?from={from}&to={to}", nil},
	{"GET", "/conversions/{clickId}", "/conversions/{click}", nil},
	{"GET", "/click-log", "/click-log?from={from}&to={to}", nil},
	{"GET", "/postback-logs", "/postback-logs?from={from}&to={to}", nil},
	{"POST", "/postback-logs/check", "/postback-logs/check", map[string]any{"url": "https://track.example/postback/{network}?secret=x&click_id={click}&status=sale"}},
}

// notCampaignReads: routes under the campaign-data prefixes that are not
// reads of one buyer's campaigns, each with why.
var notCampaignReads = map[string]string{
	"POST /campaigns":                     "creates a campaign, which its creator then owns",
	"POST /analytics/cost-timezones":      "the ad accounts' time zones, no campaign data",
	"POST /postback-logs/replay-incoming": "a write gated on offer.write, which a Buyer lacks",
	"POST /postback-logs/replay-outgoing": "a write gated on offer.write, which a Buyer lacks",
}

var campaignDataPrefix = []string{"/campaigns", "/tracking-links", "/flows", "/analytics", "/conversions", "/click-log", "/postback-logs"}

// sharedWithTheTeam: parameterless reads that may name another buyer's
// campaign by design (docs/team.md "Not limited").
var sharedWithTheTeam = map[string]string{
	"/push/subscriptions": "push is shared: subscribers carry the campaign they came from",
	"/push/stats":         "push is shared",
	"/push/summary":       "push is shared",
	"/push/broadcasts":    "push is shared: a broadcast can target campaigns",
	"/notifications":      "team alerts",
	"/alert-rules":        "team alerts",
}

// Audit TEAM-1b: a Buyer reads only their own campaigns. Ann owns
// organization A's campaign, with a click, a conversion and a postback;
// Bob owns nothing. No read of campaign data answers Bob anything that
// names Ann's campaign or click — while Ann's own answers do (so the check
// is not vacuous). Every route under the campaign-data prefixes must be
// listed here, and a sweep of every other parameterless read finds Ann's
// ids nowhere outside what the team shares on purpose.
func TestBuyersReadOnlyTheirCampaigns(t *testing.T) {
	if os.Getenv("CLICKHOUSE_URL") == "" {
		t.Skip("CLICKHOUSE_URL not set: the reports, conversions and logs need ClickHouse")
	}
	env := newIsolationEnv(t)
	ctx := env.ctx
	annID, ann := addTeamMember(t, env, "Buyer", "Ann")
	_, bob := addTeamMember(t, env, "Buyer", "Bob")
	_, viewer := addTeamMember(t, env, "Viewer", "Vic")

	camp := env.idsA["campaign"]
	if _, err := campaign.NewRepository(env.pool).SetOwner(ctx, env.orgA, camp, annID); err != nil {
		t.Fatal(err)
	}
	var flowID string
	if err := env.pool.QueryRow(ctx, `SELECT f.id FROM flows f JOIN stream_sets s ON s.id = f.stream_set_id WHERE s.id = $1 LIMIT 1`, env.idsA["streamSet"]).Scan(&flowID); err != nil {
		flowID = idgen.New()
	}

	// Ann's click, conversion and postback in ClickHouse.
	ch, err := chconn.NewConn(ctx, config.ClickHouseConfig{URL: os.Getenv("CLICKHOUSE_URL"), Database: envOr("CLICKHOUSE_DATABASE", "flox"),
		User: envOr("CLICKHOUSE_USER", "flox"), Password: os.Getenv("CLICKHOUSE_PASSWORD")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ch.Close() })
	store := chstore.NewEventStore(ch)
	click := "scope-" + idgen.New()
	now := time.Now().UTC().Add(-time.Hour)
	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: now, OrganizationID: env.orgA, CampaignID: camp, ClickID: click, FlowID: flowID, Country: "US"},
		{Type: event.CpaAccept, EventAt: now.Add(time.Minute), OrganizationID: env.orgA, CampaignID: camp, ClickID: click, FlowID: flowID,
			NetworkID: env.idsA["network"], USDValue: 10, HasUSDValue: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertPostbackAttempts(ctx, []chstore.PostbackAttempt{{OrganizationID: env.orgA, EventAt: now.Add(time.Minute),
		Direction: "incoming", NetworkID: env.idsA["network"], ClickID: click, Status: "CPA_ACCEPT", Result: "success"}}); err != nil {
		t.Fatal(err)
	}

	from, to := now.AddDate(0, 0, -1).Format(time.DateOnly), now.AddDate(0, 0, 1).Format(time.DateOnly)
	fill := func(s string) string {
		return strings.NewReplacer("{campaign}", camp, "{click}", click, "{streamSet}", env.idsA["streamSet"], "{flow}", flowID,
			"{network}", env.idsA["network"], "{from}", from, "{to}", to).Replace(s)
	}
	fillBody := func(b any) any {
		m, ok := b.(map[string]any)
		if !ok {
			return b
		}
		out := map[string]any{}
		for k, v := range m {
			switch x := v.(type) {
			case string:
				out[k] = fill(x)
			case []string:
				ys := make([]string, len(x))
				for i, y := range x {
					ys[i] = fill(y)
				}
				out[k] = ys
			default:
				out[k] = v
			}
		}
		return out
	}
	leaks := func(body string) bool { return strings.Contains(body, camp) || strings.Contains(body, click) }

	listed := map[string]bool{}
	seenByAnn := 0
	for _, rd := range scopedReads {
		listed[rd.method+" "+rd.route] = true
		path := fill(rd.path)
		status, body, _ := bob.request(rd.method, path, fillBody(rd.body))
		if leaks(body) {
			t.Errorf("Bob %s %s = %d, names Ann's campaign or click: %.300s", rd.method, rd.route, status, body)
		}
		if status >= 500 {
			t.Errorf("Bob %s %s = %d: %s", rd.method, rd.route, status, body)
		}
		// Ann's campaign or click named in the path: as if it did not exist.
		if (strings.Contains(rd.path, "{campaign}") || strings.Contains(rd.path, "/{click}")) && status != http.StatusNotFound {
			t.Errorf("Bob %s %s = %d, want 404", rd.method, rd.route, status)
		}
		status, body, _ = ann.request(rd.method, path, fillBody(rd.body))
		if status >= 400 && status != http.StatusNotFound {
			t.Errorf("Ann %s %s = %d: %.300s", rd.method, rd.route, status, body)
		}
		if leaks(body) {
			seenByAnn++
		}
		// A Viewer (campaign.read_all) still sees everything.
		if _, vbody, _ := viewer.request(rd.method, path, fillBody(rd.body)); rd.route == "/campaigns" && !strings.Contains(vbody, camp) {
			t.Errorf("a Viewer's campaign list lacks Ann's campaign")
		}
	}
	if seenByAnn < 15 {
		t.Errorf("Ann saw her own campaign or click in only %d of %d reads — the check is near vacuous", seenByAnn, len(scopedReads))
	}

	// Every route under the campaign-data prefixes is decided.
	for rk := range routerRoutes(t, env.srv.Mux()) {
		key := rk.method + " " + rk.path
		inScope := false
		for _, p := range campaignDataPrefix {
			if rk.path == p || strings.HasPrefix(rk.path, p+"/") {
				inScope = true
			}
		}
		if !inScope || listed[key] || notCampaignReads[key] != "" {
			continue
		}
		if rk.method != http.MethodGet && campaignWriteRoute.MatchString(rk.path) {
			continue // a write to one campaign: TestBuyersChangeOnlyTheirCampaigns
		}
		t.Errorf("%s is not in scopedReads or notCampaignReads — decide whether a Buyer may read it", key)
	}

	// Every other parameterless read: Ann's ids appear only where the team
	// shares them on purpose.
	for rk := range routerRoutes(t, env.srv.Mux()) {
		if rk.method != http.MethodGet || strings.Contains(rk.path, "{") || sharedWithTheTeam[rk.path] != "" || listed["GET "+rk.path] {
			continue
		}
		if _, body, _ := bob.request(http.MethodGet, rk.path, nil); leaks(body) {
			t.Errorf("Bob GET %s names Ann's campaign or click: %.300s", rk.path, body)
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
