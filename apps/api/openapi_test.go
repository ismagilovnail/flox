package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/openapi"
)

// notInPublicSpec: the routes the public API reference (API-1) leaves out,
// each with why. "METHOD /path" names one route, "/prefix/*" every route
// under it. A new route must land in openapi.yaml or here.
var notInPublicSpec = map[string]string{
	"/openapi.json":                      "the reference itself",
	"/openapi.yaml":                      "the reference itself",
	"/auth/*":                            "browser sign-in and account security, session only",
	"/team/*":                            "team administration, done in the app",
	"/organization/*":                    "organization settings, deletion and exports, done in the app",
	"/organization":                      "organization settings, done in the app",
	"/security":                          "organization security settings, done in the app",
	"/privacy/*":                         "visitor data requests, done in the app",
	"/api-keys/*":                        "keys are managed in the app, never by a key",
	"/integrations/*":                    "connected accounts, set up in the app",
	"/telegram-bots/*":                   "connected accounts, set up in the app",
	"/telegram-channels/*":               "connected accounts, set up in the app",
	"/traffic-sources/{id}/connection/*": "connected ad accounts, set up in the app",
	"/networks/{id}/shopify":             "a connected Shopify store, set up in the app",
	"/networks/{id}/woocommerce":         "a connected WooCommerce store, set up in the app",
	"/landings/*":                        "content — a later version of the reference",
	"/pwas/*":                            "content — a later version of the reference",
	"/postlandings/*":                    "content — a later version of the reference",
	"/content-gallery/*":                 "content — a later version of the reference",
	"/gallery-system/*":                  "static assets for the app",
	"/pixels/*":                          "content — a later version of the reference",
	"/push/*":                            "push — a later version of the reference",
	"/tags/*":                            "tags — a later version of the reference",
	"/event-mappings/*":                  "network status mapping — a later version of the reference",
	"GET /offer-caps":                    "the app's cap gauges",
	"/alert-rules/*":                     "app alerts",
	"/notifications/*":                   "the app's notification bell",
	"/custom-metrics/*":                  "the app's custom metrics",
	"/report-presets/*":                  "the app's saved reports",
	"/referral/*":                        "the referral program, in the app",
	"/onboarding/*":                      "the app's onboarding checklist",
	"PUT /campaigns/{id}/owner":          "assigning campaigns to buyers is done in the app",
	"GET /analytics/buyers":              "the report builder's buyer names",
	"GET /analytics/unique-check":        "a diagnostic: unique clicks by cookie vs IP + user agent",
	"/billing/payments/*":                "paying for a plan is done in the app, by an owner",
	"GET /billing/payments":              "paying for a plan is done in the app, by an owner",
	"POST /billing/checkout":             "paying for a plan is done in the app, by an owner",
	"/payment-notifications/*":           "YooKassa calls it",
	"/analytics/ltv/*":                   "LTV cohorts — a later version of the reference",
	"POST /analytics/dashboard":          "the Overview screen's bundle; report/timeseries/breakdown carry the data",
	"POST /analytics/funnel":             "a later version of the reference",
	"POST /analytics/flow-summary":       "the flow builder's node counts",
	"POST /analytics/dimension-values":   "the report builder's filter suggestions",
	"POST /analytics/cost-timezones":     "the report builder's time-zone warning",
	"GET /analytics/campaigns/{campaignId}/daily":         "superseded by POST /analytics/timeseries",
	"GET /analytics/campaigns/{campaignId}/daily-revenue": "superseded by POST /analytics/timeseries",
	"POST /domains":                       "domains are added and verified in the app (DNS records)",
	"GET /domains/{id}":                   "domains are added and verified in the app (DNS records)",
	"PATCH /domains/{id}":                 "domains are added and verified in the app (DNS records)",
	"DELETE /domains/{id}":                "domains are added and verified in the app (DNS records)",
	"/domains/{id}/*":                     "domains are added and verified in the app (DNS records)",
	"POST /postback-logs/check":           "the app's postback tester",
	"POST /postback-logs/replay-incoming": "operator tool in the app",
	"POST /postback-logs/replay-outgoing": "operator tool in the app",
}

type routeKey struct{ method, path string }

func routerRoutes(t *testing.T, mux chi.Router) map[routeKey]bool {
	t.Helper()
	routes := map[routeKey]bool{}
	if err := chi.Walk(mux, func(method, path string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if p := strings.TrimSuffix(path, "/"); p != "" {
			path = p
		}
		routes[routeKey{method, path}] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return routes
}

// excludedBy is the notInPublicSpec entry covering a route, "" if none.
func excludedBy(rk routeKey) string {
	if _, ok := notInPublicSpec[rk.method+" "+rk.path]; ok {
		return rk.method + " " + rk.path
	}
	if _, ok := notInPublicSpec[rk.path]; ok {
		return rk.path
	}
	for entry := range notInPublicSpec {
		if prefix, ok := strings.CutSuffix(entry, "/*"); ok && strings.HasPrefix(rk.path+"/", prefix+"/") {
			return entry
		}
	}
	return ""
}

// API-1: the reference and the router agree. Every operation it documents
// is a real route, every route is documented or left out with a reason,
// and nothing is both.
func TestOpenAPIMatchesTheRouter(t *testing.T) {
	env := newIsolationEnv(t)
	routes := routerRoutes(t, env.srv.Mux())
	doc, err := openapi.Spec()
	if err != nil {
		t.Fatal(err)
	}

	documented := map[routeKey]bool{}
	for _, op := range openapi.Operations(doc) {
		rk := routeKey{op.Method, op.Path}
		documented[rk] = true
		if !routes[rk] {
			t.Errorf("openapi.yaml documents %s %s (%s), the router has no such route", op.Method, op.Path, op.ID)
		}
		if by := excludedBy(rk); by != "" {
			t.Errorf("%s %s is documented and also listed in notInPublicSpec as %q", op.Method, op.Path, by)
		}
	}
	used := map[string]bool{}
	var missing []string
	for rk := range routes {
		if documented[rk] {
			continue
		}
		if by := excludedBy(rk); by != "" {
			used[by] = true
			continue
		}
		missing = append(missing, rk.method+" "+rk.path)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("routes neither in openapi.yaml nor in notInPublicSpec:\n  %s", strings.Join(missing, "\n  "))
	}
	for entry := range notInPublicSpec {
		if !used[entry] {
			t.Errorf("notInPublicSpec entry %q matches no route — remove it", entry)
		}
	}
}

// keyClient calls the in-process API as an integration does: a Bearer key,
// no cookie, no Origin.
type keyClient struct {
	t   *testing.T
	h   http.Handler
	key string
	ip  string
}

func (c keyClient) call(method, path string, body any) (int, []byte) {
	c.t.Helper()
	var r *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		r = bytes.NewReader(b)
	} else {
		r = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, "/v1"+path, r)
	req.RemoteAddr = "[" + c.ip + "]:1234"
	req.Header.Set("Authorization", "Bearer "+c.key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

// API-1: every operation of the reference, called with an API key, answers
// with the status and the body the reference documents. Response schemas
// are strict (every field required unless Go omits it when empty, no field
// the schema does not name), so a field added to or dropped from a Go
// response type fails here until openapi.yaml says so. The requests sent
// are checked against the request schemas too, and a read-scope key must
// get 403 from every write operation.
func TestOpenAPIOperationsAnswerAsDocumented(t *testing.T) {
	if os.Getenv("CLICKHOUSE_URL") == "" {
		t.Skip("CLICKHOUSE_URL not set: the reports, conversions and logs need ClickHouse")
	}
	env := newIsolationEnv(t)
	ctx := env.ctx
	doc, err := openapi.Spec()
	if err != nil {
		t.Fatal(err)
	}
	v := openapi.NewValidator(doc)
	ops := map[string]openapi.Operation{}
	for _, op := range openapi.Operations(doc) {
		ops[op.ID] = op
	}
	errorSchema, err := v.Resolve("#/components/schemas/Error")
	if err != nil {
		t.Fatal(err)
	}

	newKey := func(scope string) string {
		status, body, _ := env.c.request(http.MethodPost, "/api-keys", map[string]string{"name": "OpenAPI " + scope, "scope": scope})
		if status != http.StatusCreated {
			t.Fatalf("creating a %s key = %d: %s", scope, status, body)
		}
		var resp struct {
			FullKey string `json:"fullKey"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil || resp.FullKey == "" {
			t.Fatalf("api key response %s: %v", body, err)
		}
		return resp.FullKey
	}
	writer := keyClient{t: t, h: env.srv.Handler(), key: newKey("write"), ip: randomTestIP()}
	reader := keyClient{t: t, h: env.srv.Handler(), key: newKey("read"), ip: randomTestIP()}

	type call struct{ method, path string }
	called := map[string]call{}
	decode := func(raw []byte) any {
		var x any
		if err := json.Unmarshal(raw, &x); err != nil {
			t.Fatalf("not JSON: %s", raw)
		}
		return x
	}
	// do runs one operation and checks it against the reference; out, when
	// set, receives the decoded answer.
	do := func(id string, path string, body any, want int, out any) {
		t.Helper()
		op, ok := ops[id]
		if !ok {
			t.Fatalf("no operation %q in openapi.yaml", id)
		}
		if body != nil {
			if s := op.RequestSchema(); s == nil {
				t.Errorf("%s: sends a body, the reference documents none", id)
			} else {
				b, _ := json.Marshal(body)
				for _, e := range v.Validate(s, decode(b)) {
					t.Errorf("%s request: %s", id, e)
				}
			}
		}
		client := writer
		if op.Scope == "read" {
			client = reader
		}
		status, raw := client.call(op.Method, path, body)
		called[id] = call{op.Method, path}
		if status != want {
			t.Fatalf("%s %s %s = %d, want %d: %s", id, op.Method, path, status, want, raw)
		}
		schema, ok := op.ResponseSchema(status)
		if !ok {
			t.Fatalf("%s: status %d is not documented", id, status)
		}
		if schema == nil {
			if len(bytes.TrimSpace(raw)) != 0 {
				t.Errorf("%s: documented without a body, answered %s", id, raw)
			}
			return
		}
		for _, e := range v.Validate(schema, decode(raw)) {
			t.Errorf("%s response: %s", id, e)
		}
		if out != nil {
			if err := json.Unmarshal(raw, out); err != nil {
				t.Fatal(err)
			}
		}
	}
	type withID struct {
		ID string `json:"id"`
	}
	tag := idgen.New()[18:]

	// Traffic sources.
	var src withID
	do("listTrafficSources", "/traffic-sources?limit=10&offset=0", nil, http.StatusOK, nil)
	do("createTrafficSource", "/traffic-sources", map[string]any{
		"name": "API source " + tag, "type": "Facebook", "trackingTemplate": "https://track.example.com/c?cid={click_id}",
		"costIntegration": "manual", "postbackStatuses": []string{"CPA_ACCEPT", "CPA_REDEP"},
	}, http.StatusCreated, &src)
	do("getTrafficSource", "/traffic-sources/"+src.ID, nil, http.StatusOK, nil)
	do("updateTrafficSource", "/traffic-sources/"+src.ID, map[string]any{"name": "API source renamed " + tag}, http.StatusOK, nil)
	do("pauseTrafficSource", "/traffic-sources/"+src.ID+"/pause", nil, http.StatusOK, nil)
	do("activateTrafficSource", "/traffic-sources/"+src.ID+"/activate", nil, http.StatusOK, nil)
	var srcCopy withID
	do("duplicateTrafficSource", "/traffic-sources/"+src.ID+"/duplicate", nil, http.StatusCreated, &srcCopy)
	do("deleteTrafficSource", "/traffic-sources/"+srcCopy.ID, nil, http.StatusNoContent, nil)

	// Networks.
	var net withID
	do("listNetworks", "/networks", nil, http.StatusOK, nil)
	do("createNetwork", "/networks", map[string]any{"name": "API network " + tag, "currency": "EUR", "acceptDuplicates": false}, http.StatusCreated, &net)
	do("getNetwork", "/networks/"+net.ID, nil, http.StatusOK, nil)
	do("updateNetwork", "/networks/"+net.ID, map[string]any{"postbackUrl": "https://partner.example/pb?cid={click_id}"}, http.StatusOK, nil)
	do("pauseNetwork", "/networks/"+net.ID+"/pause", nil, http.StatusOK, nil)
	do("activateNetwork", "/networks/"+net.ID+"/activate", nil, http.StatusOK, nil)
	do("regenerateNetworkPostbackSecret", "/networks/"+net.ID+"/regenerate-postback-secret", nil, http.StatusOK, nil)
	do("networkDiagnostics", "/networks/"+net.ID+"/diagnostics", nil, http.StatusOK, nil)
	var netCopy withID
	do("duplicateNetwork", "/networks/"+net.ID+"/duplicate", nil, http.StatusCreated, &netCopy)
	do("deleteNetwork", "/networks/"+netCopy.ID, nil, http.StatusNoContent, nil)

	// Offers.
	var off withID
	do("createOffer", "/offers", map[string]any{
		"networkId": net.ID, "name": "API offer " + tag, "countries": []string{"DE", "AT"}, "payout": 25.5, "currency": "EUR", "cap": 100,
		"links": []map[string]string{{"label": "Main", "url": "https://offer.example.com/?cid={click_id}"}},
	}, http.StatusCreated, &off)
	do("listOffers", "/offers", nil, http.StatusOK, nil)
	do("getOffer", "/offers/"+off.ID, nil, http.StatusOK, nil)
	do("updateOffer", "/offers/"+off.ID, map[string]any{"cap": nil, "payout": 30}, http.StatusOK, nil)
	do("pauseOffer", "/offers/"+off.ID+"/pause", nil, http.StatusOK, nil)
	do("activateOffer", "/offers/"+off.ID+"/activate", nil, http.StatusOK, nil)
	var offCopy withID
	do("duplicateOffer", "/offers/"+off.ID+"/duplicate", nil, http.StatusCreated, &offCopy)
	do("deleteOffer", "/offers/"+offCopy.ID, nil, http.StatusNoContent, nil)

	// Campaigns.
	var camp withID
	do("createCampaign", "/campaigns", map[string]any{
		"trafficSourceId": src.ID, "name": "API campaign " + tag, "fallbackUrl": "https://example.com/fallback", "notes": "made by the API test",
	}, http.StatusCreated, &camp)
	do("listCampaigns", "/campaigns?limit=5", nil, http.StatusOK, nil)
	do("getCampaign", "/campaigns/"+camp.ID, nil, http.StatusOK, nil)
	do("updateCampaign", "/campaigns/"+camp.ID, map[string]any{"externalCampaignId": "238400001"}, http.StatusOK, nil)
	do("activateCampaign", "/campaigns/"+camp.ID+"/activate", nil, http.StatusOK, nil)
	do("pauseCampaign", "/campaigns/"+camp.ID+"/pause", nil, http.StatusOK, nil)
	var campCopy withID
	do("duplicateCampaign", "/campaigns/"+camp.ID+"/duplicate", nil, http.StatusCreated, &campCopy)
	do("deleteCampaign", "/campaigns/"+campCopy.ID, nil, http.StatusNoContent, nil)

	// Routing.
	sets := "/campaigns/" + camp.ID + "/stream-sets"
	var set, set2 struct {
		ID    string `json:"id"`
		Flows []struct {
			ID string `json:"id"`
		} `json:"flows"`
	}
	do("createStreamSet", sets, map[string]any{
		"name": "DACH mobile",
		"rootFilter": map[string]any{"type": "group", "joiner": "AND", "children": []any{
			map[string]any{"type": "condition", "field": "country", "operator": "IN", "value": "DE,AT"},
			map[string]any{"type": "group", "joiner": "OR", "children": []any{
				map[string]any{"type": "condition", "field": "device", "operator": "IS", "value": "mobile"},
				map[string]any{"type": "condition", "field": "sub1", "operator": "EXISTS"},
			}},
		}},
		"flows": []any{
			map[string]any{"name": "Offer", "active": true, "weight": 70, "destination": map[string]any{"kind": "offer", "offerId": off.ID}},
			map[string]any{"name": "Redirect", "active": true, "weight": 30, "destination": map[string]any{"kind": "redirect", "url": "https://example.com/alt"}},
		},
	}, http.StatusCreated, &set)
	do("createStreamSet", sets, map[string]any{
		"name": "Everyone else", "rootFilter": map[string]any{"type": "group", "joiner": "AND", "children": []any{}},
		"flows": []any{map[string]any{"name": "Default", "active": true, "weight": 1, "destination": map[string]any{"kind": "redirect", "url": "https://example.com/default"}}},
	}, http.StatusCreated, &set2)
	do("listStreamSets", sets, nil, http.StatusOK, nil)
	do("getStreamSet", sets+"/"+set.ID, nil, http.StatusOK, nil)
	do("updateStreamSet", sets+"/"+set.ID, map[string]any{"name": "DACH mobile + sub1"}, http.StatusOK, nil)
	do("reorderStreamSets", sets+"/reorder", map[string]any{"orderedIds": []string{set2.ID, set.ID}}, http.StatusOK, nil)
	do("reorderStreamSets", sets+"/reorder", map[string]any{"orderedIds": []string{set.ID, set2.ID}}, http.StatusOK, nil)
	var setCopy withID
	do("duplicateStreamSet", sets+"/"+set.ID+"/duplicate", nil, http.StatusCreated, &setCopy)
	do("deleteStreamSet", sets+"/"+setCopy.ID, nil, http.StatusNoContent, nil)
	do("listFlows", "/flows", nil, http.StatusOK, nil)
	var sim struct {
		MatchedStreamSet *struct {
			StreamSetID string `json:"streamSetId"`
		} `json:"matchedStreamSet"`
	}
	do("simulateRouting", "/campaigns/"+camp.ID+"/routing/simulate", map[string]any{"request": map[string]string{"country": "DE", "device": "mobile"}}, http.StatusOK, &sim)
	if sim.MatchedStreamSet == nil || sim.MatchedStreamSet.StreamSetID != set.ID {
		t.Errorf("simulate: a German mobile visit should match %s, got %+v", set.ID, sim.MatchedStreamSet)
	}
	do("simulateRouting", "/campaigns/"+camp.ID+"/routing/simulate", map[string]any{"request": map[string]string{"country": "US"}}, http.StatusOK, nil)

	// Domains and tracking links.
	do("listDomains", "/domains", nil, http.StatusOK, nil)
	links := "/campaigns/" + camp.ID + "/tracking-links"
	var link withID
	do("createTrackingLink", links, map[string]any{"domainId": env.idsA["domain"], "slug": "api-" + strings.ToLower(tag)}, http.StatusCreated, &link)
	do("listCampaignTrackingLinks", links, nil, http.StatusOK, nil)
	do("listTrackingLinks", "/tracking-links?status=active", nil, http.StatusOK, nil)
	do("updateTrackingLinkStatus", links+"/"+link.ID, map[string]any{"status": "paused"}, http.StatusOK, nil)
	do("deleteTrackingLink", links+"/"+link.ID, nil, http.StatusNoContent, nil)

	// Spend.
	today := time.Now().UTC().Format(time.DateOnly)
	weekAgo := time.Now().UTC().AddDate(0, 0, -7).Format(time.DateOnly)
	do("campaignDiagnostics", "/campaigns/"+camp.ID+"/diagnostics", nil, http.StatusOK, nil)
	costs := "/campaigns/" + camp.ID + "/cost-entries"
	var entry withID
	do("upsertCostEntry", costs, map[string]any{"entryDate": today, "amount": 120.5, "currency": "USD", "country": "DE", "trafficSourceId": src.ID}, http.StatusOK, &entry)
	do("importCostEntries", costs+"/import", map[string]any{"csv": "date,amount,currency,country\n" + weekAgo + ",80,USD,AT\n"}, http.StatusOK, nil)
	do("listCostEntries", costs+"?from="+weekAgo+"&to="+today, nil, http.StatusOK, nil)
	do("dailyCampaignSpend", costs+"/daily?from="+weekAgo+"&to="+today, nil, http.StatusOK, nil)

	// Events for the reports and logs: one routed click that converted, one
	// blocked click, and a postback each way.
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	conn, err := chconn.NewConn(ctx, cfg.ClickHouse)
	if err != nil {
		t.Fatalf("connecting to clickhouse: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	events := chstore.NewEventStore(conn)
	clickID := "api" + strings.ToLower(idgen.New())
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	base := event.Event{
		OrganizationID: env.orgA, CampaignID: camp.ID, TrafficSourceID: src.ID, ClickID: clickID,
		StreamSetID: set.ID, FlowID: set.Flows[0].ID, OfferID: off.ID, NetworkID: net.ID,
		Country: "DE", Device: "mobile", OS: "Android", Browser: "Chrome", UTMSource: "fb",
		Subs: event.Subs{"creative-7"},
	}
	click := base
	click.Type, click.EventAt, click.Destination = event.SourceClick, at, "https://offer.example.com/?cid="+clickID
	land := base
	land.Type, land.EventAt = event.LandView, at.Add(time.Second)
	accept := base
	accept.Type, accept.EventAt = event.CpaAccept, at.Add(time.Minute)
	accept.Revenue, accept.Currency, accept.USDValue, accept.NetworkTxnID = 25.5, "EUR", 27.8, "txn-1"
	blocked := event.Event{
		Type: event.SourceFilter, EventAt: at, OrganizationID: env.orgA, CampaignID: camp.ID, TrafficSourceID: src.ID,
		ClickID: "api" + strings.ToLower(idgen.New()), Country: "US", IsBot: true, FilterReason: "bot",
	}
	if err := events.InsertBatch(ctx, []event.Event{click, land, accept, blocked}); err != nil {
		t.Fatalf("inserting events: %v", err)
	}
	if err := events.InsertPostbackAttempts(ctx, []chstore.PostbackAttempt{
		{OrganizationID: env.orgA, EventAt: accept.EventAt, Direction: "incoming", NetworkID: net.ID, ClickID: clickID,
			Status: "CPA_ACCEPT", RawStatus: "approved", Result: "success", Message: "recorded", Revenue: 25.5, Currency: "EUR"},
		{OrganizationID: env.orgA, EventAt: accept.EventAt.Add(time.Second), Direction: "outgoing", NetworkID: net.ID, ClickID: clickID,
			Status: "CPA_ACCEPT", Result: "success", Message: "200 OK", AttemptCount: 1, ResponseStatusCode: 200, URL: "https://partner.example/pb?cid=" + clickID},
	}); err != nil {
		t.Fatalf("inserting postback attempts: %v", err)
	}
	rng := "?from=" + weekAgo + "&to=" + today
	var convs struct {
		Conversions []any `json:"conversions"`
	}
	do("listConversions", "/conversions"+rng+"&limit=20", nil, http.StatusOK, &convs)
	if len(convs.Conversions) != 1 {
		t.Errorf("conversions: want the 1 inserted, got %d", len(convs.Conversions))
	}
	do("getClickTimeline", "/conversions/"+clickID, nil, http.StatusOK, nil)
	var clicks struct {
		Logs []any `json:"logs"`
	}
	do("listClicks", "/click-log"+rng, nil, http.StatusOK, &clicks)
	if len(clicks.Logs) != 2 {
		t.Errorf("click log: want the 2 inserted clicks, got %d", len(clicks.Logs))
	}
	var pbs struct {
		Logs []any `json:"logs"`
	}
	do("listPostbackAttempts", "/postback-logs"+rng+"&clickId="+clickID, nil, http.StatusOK, &pbs)
	if len(pbs.Logs) != 2 {
		t.Errorf("postback log: want the 2 inserted attempts, got %d", len(pbs.Logs))
	}
	var rep struct {
		Rows []any `json:"rows"`
	}
	filter := []map[string]string{{"dimension": "campaign", "value": camp.ID}}
	do("report", "/analytics/report", map[string]any{
		"dims": []string{"country", "day"}, "filters": filter, "from": weekAgo, "to": today,
		"sort": map[string]string{"key": "revenue", "dir": "desc"}, "timezone": "Europe/Berlin",
	}, http.StatusOK, &rep)
	if len(rep.Rows) == 0 {
		t.Error("report: no rows for the inserted events")
	}
	do("report", "/analytics/report", map[string]any{"from": weekAgo, "to": today, "dateBasis": "click"}, http.StatusOK, nil)
	do("timeseries", "/analytics/timeseries", map[string]any{"filters": filter, "from": weekAgo, "to": today}, http.StatusOK, nil)
	do("breakdown", "/analytics/breakdown", map[string]any{"dim": "offer", "metric": "clicks", "filters": filter, "from": weekAgo, "to": today, "limit": 3}, http.StatusOK, nil)

	do("deleteCostEntry", costs+"/"+entry.ID, nil, http.StatusNoContent, nil)

	// Billing.
	do("getBilling", "/billing", nil, http.StatusOK, nil)
	do("listPlans", "/billing/plans", nil, http.StatusOK, nil)

	// Every operation ran; a read key is refused every write.
	var notCalled []string
	for id, op := range ops {
		c, ok := called[id]
		if !ok {
			notCalled = append(notCalled, id)
			continue
		}
		if op.Scope != "write" {
			continue
		}
		status, raw := reader.call(c.method, c.path, map[string]any{})
		if status != http.StatusForbidden {
			t.Errorf("%s with a read key = %d, want 403: %s", id, status, raw)
			continue
		}
		for _, e := range v.Validate(errorSchema, decode(raw)) {
			t.Errorf("%s 403: %s", id, e)
		}
	}
	sort.Strings(notCalled)
	if len(notCalled) > 0 {
		t.Errorf("operations in openapi.yaml this test never called: %v", notCalled)
	}

	// Errors keep the documented envelope.
	for _, c := range []struct {
		client keyClient
		path   string
		want   int
	}{
		{reader, "/campaigns/" + idgen.New(), http.StatusNotFound},
		{keyClient{t: t, h: env.srv.Handler(), key: "flx_live_not-a-key", ip: randomTestIP()}, "/campaigns", http.StatusUnauthorized},
		{reader, "/conversions?from=yesterday", http.StatusUnprocessableEntity},
	} {
		status, raw := c.client.call(http.MethodGet, c.path, nil)
		if status != c.want {
			t.Errorf("GET %s = %d, want %d: %s", c.path, status, c.want, raw)
			continue
		}
		for _, e := range v.Validate(errorSchema, decode(raw)) {
			t.Errorf("GET %s %d: %s", c.path, status, e)
		}
	}
}
