// Package e2e is Phase 32's full-funnel scenario (§57): one continuous
// walk through Create organization -> Create source -> Create network ->
// Create offer -> Create landing -> Create flow -> Create Stream Set ->
// Create filter -> Create campaign -> Enter cost -> Generate tracking URL
// -> Click -> Route -> Record event -> Receive conversion (HOLD -> ACCEPT
// -> REDEP) -> Attribute conversion -> Send postback -> Analytics + LTV,
// verifying every step against real running services (CLAUDE.md Phase 32
// scope decision: an automated Go test, not a manual walkthrough).
//
// It expects apps/api, apps/tracker, and apps/worker to already be
// running against real Postgres/ClickHouse/Redis (the documented local
// dev workflow — `go run ./api`, `./tracker`, `./worker` from apps/), the
// same way every other browser-driven validation pass in this project
// works. It does not spawn those processes itself: t.Skip fires with a
// clear message if they (or DATABASE_URL/CLICKHOUSE_URL) aren't
// reachable, so `go test ./...` stays green in an environment that never
// started them.
//
// One HTTP endpoint this scenario needs does not exist yet: there is no
// /domains or /tracking-links API anywhere in apps/api (grep confirms
// only apps/cmd/loadtestseed's raw SQL ever writes those tables). Rather
// than expand this phase into building that endpoint, the "Generate
// tracking URL" step below seeds the same two rows loadtestseed already
// does, directly over the test's own Postgres connection — see
// seedTrackingLink. The missing endpoint is a real product gap, tracked
// in CLAUDE.md's CURRENT STATE, not silently worked around.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

const dateLayout = "2006-01-02"

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// requireHealthy skips the whole test (not fails it) when a service isn't
// reachable — this scenario needs real running binaries, and a down
// dependency in an environment that never started them is not a bug in
// the code under test.
func requireHealthy(t *testing.T, base string) {
	t.Helper()
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(base + "/health")
	if err != nil {
		t.Skipf("Phase 32 E2E scenario needs %s running (GET /health failed: %v) — start apps/api, apps/tracker and apps/worker per the documented local dev workflow first", base, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("Phase 32 E2E scenario needs %s healthy (GET /health returned %d)", base, resp.StatusCode)
	}
}

// scenario carries every dependency and every id minted along the way —
// one struct instead of a chain of return values, since later steps
// (postback, analytics) need ids from almost every earlier one.
type scenario struct {
	t           *testing.T
	apiBase     string
	trackerBase string
	appURL      string
	http        *http.Client
	pg          *pgxpool.Pool
	ch          driver.Conn

	orgID           string
	trafficSourceID string
	networkID       string
	postbackSecret  string
	offerID         string
	landingID       string
	campaignID      string
	streamSetID     string
	flowID          string
	domain          string
	slug            string
	clickID         string
}

func TestFullFunnel(t *testing.T) {
	apiBase := envOr("FLOX_E2E_API_URL", "http://localhost:8080")
	trackerBase := envOr("FLOX_E2E_TRACKER_URL", "http://localhost:8081")
	appURL := envOr("FLOX_E2E_APP_URL", "http://localhost:3000")

	dsn := os.Getenv("DATABASE_URL")
	chURL := os.Getenv("CLICKHOUSE_URL")
	if dsn == "" || chURL == "" {
		t.Skip("Phase 32 E2E scenario needs DATABASE_URL and CLICKHOUSE_URL set (same values apps/api/apps/worker were started with) to verify durable state, not just HTTP responses")
	}

	requireHealthy(t, apiBase)
	requireHealthy(t, trackerBase)

	ctx := context.Background()

	pg, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting to postgres: %v", err)
	}
	t.Cleanup(pg.Close)

	ch, err := chconn.NewConn(ctx, config.ClickHouseConfig{
		URL:      chURL,
		Database: envOr("CLICKHOUSE_DATABASE", "flox"),
		User:     envOr("CLICKHOUSE_USER", "flox"),
		Password: os.Getenv("CLICKHOUSE_PASSWORD"),
	})
	if err != nil {
		t.Fatalf("connecting to clickhouse: %v", err)
	}
	t.Cleanup(func() { _ = ch.Close() })

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("creating cookie jar: %v", err)
	}

	s := &scenario{
		t:           t,
		apiBase:     apiBase,
		trackerBase: trackerBase,
		appURL:      appURL,
		http:        &http.Client{Jar: jar, Timeout: 10 * time.Second},
		pg:          pg,
		ch:          ch,
	}

	// Cleanup runs regardless of where the scenario fails, so a failed run
	// never leaves fixtures behind for the next one — same stance as the
	// Phase 31 load test / benchmark fixtures (organizations cascade in
	// Postgres; ClickHouse has no FK/cascade, cleaned explicitly).
	t.Cleanup(func() {
		if s.orgID == "" {
			return
		}
		cleanupCtx := context.Background()
		if _, err := pg.Exec(cleanupCtx, `DELETE FROM organizations WHERE id = $1`, s.orgID); err != nil {
			t.Logf("cleanup: deleting organization: %v", err)
		}
		for _, table := range []string{
			"click_events", "tracking_events", "conversion_events", "postback_events",
			"click_events_daily_campaign", "click_events_daily_geo", "conversion_events_daily_campaign",
		} {
			if err := ch.Exec(cleanupCtx, "ALTER TABLE "+table+" DELETE WHERE organization_id = ?", s.orgID); err != nil {
				t.Logf("cleanup: clearing %s: %v", table, err)
			}
		}
	})

	s.createOrganization()
	s.createTrafficSource()
	s.createNetwork()
	s.createOffer()
	s.createLanding()
	s.createCampaign()
	s.createStreamSetWithFlowAndFilter()
	s.enterCost()
	s.generateTrackingURL()
	s.registerEventMappings()
	s.clickAndRoute()
	s.recordEvent()
	s.receiveConversions()
	s.attributeConversion()
	s.sendPostback()
	s.analyticsAndLTV()
}

// --- HTTP helpers -----------------------------------------------------

// apiRequest builds a request against apps/api with the Origin header
// every mutating call needs (§54/Phase 30 CSRF, tenant.RequireSameOrigin)
// and the session cookie the jar already carries after signup.
func (s *scenario) apiRequest(method, path string, body any) *http.Request {
	s.t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			s.t.Fatalf("marshaling request body for %s %s: %v", method, path, err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, s.apiBase+path, r)
	if err != nil {
		s.t.Fatalf("building request %s %s: %v", method, path, err)
	}
	req.Header.Set("Origin", s.appURL)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

// doAPI runs req, decodes the JSON response into out (unless nil), and
// fails the test loudly — with the response body attached — if the
// status doesn't match wantStatus.
func (s *scenario) doAPI(req *http.Request, wantStatus int, out any) {
	s.t.Helper()
	resp, err := s.http.Do(req)
	if err != nil {
		s.t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		s.t.Fatalf("%s %s: want status %d, got %d: %s", req.Method, req.URL.Path, wantStatus, resp.StatusCode, string(body))
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			s.t.Fatalf("%s %s: decoding response %q: %v", req.Method, req.URL.Path, string(body), err)
		}
	}
}

func (s *scenario) post(path string, body, out any, wantStatus int) {
	s.t.Helper()
	s.doAPI(s.apiRequest(http.MethodPost, path, body), wantStatus, out)
}

func (s *scenario) get(path string, out any, wantStatus int) {
	s.t.Helper()
	s.doAPI(s.apiRequest(http.MethodGet, path, nil), wantStatus, out)
}

// --- steps --------------------------------------------------------------

func (s *scenario) createOrganization() {
	s.t.Helper()
	var resp struct {
		Organization struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"organization"`
	}
	s.post("/auth/signup", map[string]string{
		"organizationName": "E2E Org " + idgen.New(),
		"name":             "E2E Tester",
		"email":            "e2e-" + strings.ToLower(idgen.New()) + "@example.test",
		"password":         "correct horse battery staple 1",
	}, &resp, http.StatusCreated)
	if resp.Organization.ID == "" {
		s.t.Fatal("signup: response carried no organization id")
	}
	s.orgID = resp.Organization.ID
	s.t.Logf("created organization %s", s.orgID)
}

func (s *scenario) createTrafficSource() {
	s.t.Helper()
	var resp struct {
		ID string `json:"id"`
	}
	s.post("/traffic-sources", map[string]any{
		"name":             "E2E Traffic Source",
		"type":             "Facebook",
		"trackingTemplate": "https://track.example.test/click?clickid={click_id}",
		"costIntegration":  "manual",
	}, &resp, http.StatusCreated)
	if resp.ID == "" {
		s.t.Fatal("create traffic source: response carried no id")
	}
	s.trafficSourceID = resp.ID
	s.t.Logf("created traffic source %s", s.trafficSourceID)
}

func (s *scenario) createNetwork() {
	s.t.Helper()
	var resp struct {
		ID             string `json:"id"`
		PostbackSecret string `json:"postbackSecret"`
	}
	s.post("/networks", map[string]any{
		"name": "E2E Network",
		// The macros this scenario's "Send postback" step verifies FLOX
		// resolves and delivers back out (internal/conversion.go's
		// deliveryMacroValues / internal/macro).
		"postbackUrl":      "https://cb.example.test/postback?click_id={click_id}&status={status}",
		"acceptDuplicates": false,
	}, &resp, http.StatusCreated)
	if resp.ID == "" || resp.PostbackSecret == "" {
		s.t.Fatal("create network: response carried no id or postbackSecret")
	}
	s.networkID = resp.ID
	s.postbackSecret = resp.PostbackSecret
	s.t.Logf("created network %s", s.networkID)
}

func (s *scenario) createOffer() {
	s.t.Helper()
	var resp struct {
		ID string `json:"id"`
	}
	s.post("/offers", map[string]any{
		"networkId": s.networkID,
		"name":      "E2E Offer",
		"countries": []string{"US"},
		"payout":    10.5,
		"currency":  "USD",
		"cap":       nil,
		"links": []map[string]string{
			{"label": "Main", "url": "https://offer.example.test/go"},
		},
	}, &resp, http.StatusCreated)
	if resp.ID == "" {
		s.t.Fatal("create offer: response carried no id")
	}
	s.offerID = resp.ID
	s.t.Logf("created offer %s", s.offerID)
}

func (s *scenario) createLanding() {
	s.t.Helper()
	var resp struct {
		ID string `json:"id"`
	}
	s.post("/landings", map[string]any{
		"name":    "E2E Landing",
		"type":    "external",
		"url":     "https://landing.example.test",
		"content": "",
	}, &resp, http.StatusCreated)
	if resp.ID == "" {
		s.t.Fatal("create landing: response carried no id")
	}
	s.landingID = resp.ID
	s.t.Logf("created landing %s", s.landingID)
}

func (s *scenario) createCampaign() {
	s.t.Helper()
	var created struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	s.post("/campaigns", map[string]any{
		"trafficSourceId":    s.trafficSourceID,
		"name":               "E2E Campaign",
		"fallbackUrl":        "https://fallback.example.test",
		"notes":              "",
		"externalCampaignId": "",
	}, &created, http.StatusCreated)
	if created.ID == "" {
		s.t.Fatal("create campaign: response carried no id")
	}
	if created.Status != "draft" {
		s.t.Fatalf("create campaign: want status draft, got %q", created.Status)
	}
	s.campaignID = created.ID
	s.t.Logf("created campaign %s (draft)", s.campaignID)

	var activated struct {
		Status string `json:"status"`
	}
	s.post(fmt.Sprintf("/campaigns/%s/activate", s.campaignID), nil, &activated, http.StatusOK)
	if activated.Status != "active" {
		s.t.Fatalf("activate campaign: want status active, got %q", activated.Status)
	}
	s.t.Logf("activated campaign %s", s.campaignID)
}

// createStreamSetWithFlowAndFilter covers three of §57's diagram steps in
// one call ("Create flow", "Create Stream Set", "Create filter") because
// that's how the real API models them: a stream set's flows and its
// AND/OR filter tree are nested fields on one create body, not separate
// endpoints (apps/internal/streamset/handler.go). The filter is `bot IS
// "0"` — deterministic under classifier.New(nil, nil, nil)'s
// HeuristicBotDetector regardless of geo/ASN vendor wiring (unlike a
// country filter, which Phase 31's own load test found doesn't match in
// this dev environment).
func (s *scenario) createStreamSetWithFlowAndFilter() {
	s.t.Helper()
	var resp struct {
		ID    string `json:"id"`
		Flows []struct {
			ID string `json:"id"`
		} `json:"flows"`
	}
	s.post(fmt.Sprintf("/campaigns/%s/stream-sets", s.campaignID), map[string]any{
		"name":        "E2E Stream Set",
		"fallbackUrl": "https://set-fallback.example.test",
		"rootFilter": map[string]any{
			"type":   "group",
			"joiner": "AND",
			"children": []map[string]any{
				{"type": "condition", "field": "bot", "operator": "IS", "value": "0", "valueTo": ""},
			},
		},
		"flows": []map[string]any{
			{
				"name":   "E2E Flow",
				"active": true,
				"weight": 100,
				"landing": map[string]any{
					"enabled":   true,
					"landingId": s.landingID,
					"asPwa":     false,
				},
				"pwa": map[string]any{
					"enabled": false,
					"pwaId":   "",
					"pwaType": "",
				},
				"postlanding": map[string]any{
					"enabled":       false,
					"postlandingId": "",
				},
				"destination": map[string]any{
					"kind": "redirect",
					"url":  destinationURL,
				},
			},
		},
		"pixelIds": []string{},
	}, &resp, http.StatusCreated)
	if resp.ID == "" || len(resp.Flows) != 1 || resp.Flows[0].ID == "" {
		s.t.Fatalf("create stream set: unexpected response shape: %+v", resp)
	}
	s.streamSetID = resp.ID
	s.flowID = resp.Flows[0].ID
	s.t.Logf("created stream set %s with flow %s (filter: bot IS 0)", s.streamSetID, s.flowID)
}

// destinationURL is the flow's redirect target. It never needs to be a
// live server: the test client that hits the tracker below never follows
// the 302, it only inspects the Location header.
const destinationURL = "https://converted.example.test/dest"

func (s *scenario) enterCost() {
	s.t.Helper()
	var resp struct {
		AmountUSD *float64 `json:"amountUsd"`
	}
	s.post(fmt.Sprintf("/campaigns/%s/cost-entries", s.campaignID), map[string]any{
		"trafficSourceId": s.trafficSourceID,
		"entryDate":       time.Now().UTC().Format(dateLayout),
		"amount":          50.0,
		"currency":        "USD",
	}, &resp, http.StatusOK)
	if resp.AmountUSD == nil || *resp.AmountUSD <= 0 {
		s.t.Fatalf("enter cost: expected a positive USD amount, got %+v", resp.AmountUSD)
	}
	s.t.Logf("entered cost: $%.2f USD", *resp.AmountUSD)
}

// generateTrackingURL seeds a (domain, slug) -> campaign mapping directly
// in Postgres. See this file's top-of-file doc comment: no HTTP endpoint
// for this exists yet anywhere in apps/api — this mirrors exactly what
// apps/cmd/loadtestseed/main.go already does for the same reason.
func (s *scenario) generateTrackingURL() {
	s.t.Helper()
	ctx := context.Background()

	s.domain = "e2e-" + strings.ToLower(idgen.New()) + ".test"
	s.slug = "e2e"

	domainID := idgen.New()
	if _, err := s.pg.Exec(ctx,
		`INSERT INTO domains (id, organization_id, domain, status, purpose) VALUES ($1, $2, $3, 'active', '{tracking}')`,
		domainID, s.orgID, s.domain,
	); err != nil {
		s.t.Fatalf("seeding domain: %v", err)
	}

	trackingLinkID := idgen.New()
	if _, err := s.pg.Exec(ctx,
		`INSERT INTO tracking_links (id, organization_id, campaign_id, domain_id, slug) VALUES ($1, $2, $3, $4, $5)`,
		trackingLinkID, s.orgID, s.campaignID, domainID, s.slug,
	); err != nil {
		s.t.Fatalf("seeding tracking link: %v", err)
	}
	s.t.Logf("generated tracking URL: http://%s/t/%s", s.domain, s.slug)
}

// registerEventMappings maps the three raw network status strings this
// scenario's postbacks use onto FLOX's CPA_* vocabulary
// (apps/internal/eventmapping) — a prerequisite conversion.Service.Record
// enforces (an unmapped status is ErrUnmapped, not a real conversion) that
// §57's diagram doesn't call out as its own box but the funnel can't work
// without.
func (s *scenario) registerEventMappings() {
	s.t.Helper()
	for _, m := range []struct{ networkStatus, floxStatus string }{
		{"reg", "CPA_HOLD"},
		{"ftd", "CPA_ACCEPT"},
		{"redep", "CPA_REDEP"},
	} {
		var resp struct {
			ID string `json:"id"`
		}
		s.post("/event-mappings", map[string]any{
			"networkId":     s.networkID,
			"networkStatus": m.networkStatus,
			"floxStatus":    m.floxStatus,
		}, &resp, http.StatusCreated)
		if resp.ID == "" {
			s.t.Fatalf("register event mapping %s->%s: response carried no id", m.networkStatus, m.floxStatus)
		}
	}
	s.t.Logf("registered 3 event mappings (reg/ftd/redep -> CPA_HOLD/CPA_ACCEPT/CPA_REDEP)")
}

// clickAndRoute hits the tracker's real hot path (GET /t/{slug}, §41) and
// verifies it actually routed through the configured flow rather than
// falling back — the Location header alone proves this, since the flow's
// destination URL is unique to this scenario's fixtures.
func (s *scenario) clickAndRoute() {
	s.t.Helper()

	client := &http.Client{
		Timeout: 5 * time.Second,
		// Never follow the redirect — this test only needs to observe it.
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
	}

	u := s.trackerBase + "/t/" + s.slug + "?utm_source=e2e&sub1=phase32"
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		s.t.Fatalf("building click request: %v", err)
	}
	// The tracker resolves (host, slug) -> campaign by Host header, not by
	// the request's actual network destination (apps/tracker/handler.go's
	// track: `hostWithoutPort(r.Host)`) — Go's http.Client won't let a URL
	// override Host, so it's set directly on the request here.
	req.Host = s.domain
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) FloxE2E/1.0")

	resp, err := client.Do(req)
	if err != nil {
		s.t.Fatalf("clicking tracking URL: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		s.t.Fatalf("click: want 302, got %d: %s", resp.StatusCode, string(body))
	}
	loc := resp.Header.Get("Location")
	if loc != destinationURL {
		s.t.Fatalf("click: routed to %q, want the configured flow's destination %q — did the bot=0 filter or the flow fail to match?", loc, destinationURL)
	}
	s.t.Logf("click routed correctly to %s", loc)
}

// recordEvent confirms the click was actually recorded, by polling
// ClickHouse's click_events once apps/worker's flush poll loop picks it up
// (batch 500/idle 2s — up to a few seconds of real lag). This deliberately
// does NOT look at Postgres's event_queue first: that table is a
// claim-and-delete work queue, not a durable log (apps/internal/eventqueue's
// ClaimDue marks a row 'processing' and the flusher removes it once
// ClickHouse accepts it) — polling it raced the worker's own poll loop in
// practice (a row present at one instant can be gone, already flushed, by
// the next), so ClickHouse is the only state this test treats as settled.
// It's also the only state that matters downstream: apps/api's attribution
// resolver reads clicks from ClickHouse only (apps/api/main.go's
// attribution.NewService(chstore.NewClickResolver(chConn))), so the
// postback step below would misattribute if it ran before this settles.
func (s *scenario) recordEvent() {
	s.t.Helper()
	ctx := context.Background()

	var clickID, streamSetID, flowID string
	ok := pollUntil(s.t, 10*time.Second, 500*time.Millisecond, func() bool {
		row := s.ch.QueryRow(ctx,
			`SELECT click_id, stream_set_id, flow_id FROM click_events
			  WHERE organization_id = ? AND type = 'SOURCE_CLICK'
			  ORDER BY event_at DESC LIMIT 1`,
			s.orgID,
		)
		return row.Scan(&clickID, &streamSetID, &flowID) == nil && clickID != ""
	})
	if !ok {
		s.t.Fatal("record event: click never appeared in ClickHouse click_events after worker flush (waited 10s)")
	}
	if streamSetID != s.streamSetID || flowID != s.flowID {
		s.t.Fatalf("record event: click routed through stream set %q / flow %q, want %q / %q", streamSetID, flowID, s.streamSetID, s.flowID)
	}
	s.clickID = clickID
	s.t.Logf("confirmed click %s durably flushed to ClickHouse (stream set %s, flow %s)", s.clickID, streamSetID, flowID)
}

// sendConversionPostback hits the tracker's inbound conversion endpoint
// (GET/POST /postback/{networkId}, §45/§46) the way a real network would,
// including the postback secret (§54/Phase 30).
func (s *scenario) sendConversionPostback(rawStatus string, revenue *float64, txnID string) (result, status string) {
	s.t.Helper()
	q := url.Values{}
	q.Set("secret", s.postbackSecret)
	q.Set("click_id", s.clickID)
	q.Set("status", rawStatus)
	if revenue != nil {
		q.Set("revenue", fmt.Sprintf("%.2f", *revenue))
		q.Set("currency", "USD")
	}
	if txnID != "" {
		q.Set("txn_id", txnID)
	}

	resp, err := s.http.Get(fmt.Sprintf("%s/postback/%s?%s", s.trackerBase, s.networkID, q.Encode()))
	if err != nil {
		s.t.Fatalf("sending postback (status=%s): %v", rawStatus, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		s.t.Fatalf("postback (status=%s): want 200, got %d: %s", rawStatus, resp.StatusCode, string(body))
	}

	var out struct {
		Result  string `json:"result"`
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		s.t.Fatalf("postback (status=%s): decoding response %q: %v", rawStatus, string(body), err)
	}
	return out.Result, out.Status
}

// receiveConversions drives the CLAUDE.md canonical CPA_HOLD -> CPA_ACCEPT
// -> CPA_REDEP progression (§57's "Receive conversion" box) as three
// separate incoming postbacks, exactly as a real network would send them
// over time.
func (s *scenario) receiveConversions() {
	s.t.Helper()

	revenue := func(v float64) *float64 { return &v }

	if result, status := s.sendConversionPostback("reg", nil, ""); result != "success" || status != "CPA_HOLD" {
		s.t.Fatalf("HOLD postback: want result=success status=CPA_HOLD, got result=%s status=%s", result, status)
	}
	s.t.Log("received conversion: CPA_HOLD")

	if result, status := s.sendConversionPostback("ftd", revenue(25.50), "txn-accept-1"); result != "success" || status != "CPA_ACCEPT" {
		s.t.Fatalf("ACCEPT postback: want result=success status=CPA_ACCEPT, got result=%s status=%s", result, status)
	}
	s.t.Log("received conversion: CPA_ACCEPT ($25.50)")

	if result, status := s.sendConversionPostback("redep", revenue(10.00), "txn-redep-1"); result != "success" || status != "CPA_REDEP" {
		s.t.Fatalf("REDEP postback: want result=success status=CPA_REDEP, got result=%s status=%s", result, status)
	}
	s.t.Log("received conversion: CPA_REDEP ($10.00)")
}

// attributeConversion polls GET /conversions/{clickId} (apps/internal/
// conversions) — the merged funnel+conversion timeline, ClickHouse-backed
// like the click check above, so the same async-flush wait applies —
// until all three CPA events are visible, then checks attribution
// actually tied them back to this click's own campaign/network.
func (s *scenario) attributeConversion() {
	s.t.Helper()

	type timelineEvent struct {
		Type         string  `json:"type"`
		IsConversion bool    `json:"isConversion"`
		USDValue     float64 `json:"usdValue"`
		HasUSDValue  bool    `json:"hasUsdValue"`
	}
	var timeline struct {
		ClickID    string          `json:"clickId"`
		CampaignID string          `json:"campaignId"`
		NetworkID  string          `json:"networkId"`
		Events     []timelineEvent `json:"events"`
	}

	seen := map[string]timelineEvent{}
	ok := pollUntil(s.t, 10*time.Second, 500*time.Millisecond, func() bool {
		s.get("/conversions/"+s.clickID, &timeline, http.StatusOK)
		seen = map[string]timelineEvent{}
		for _, e := range timeline.Events {
			seen[e.Type] = e
		}
		_, hasHold := seen["CPA_HOLD"]
		_, hasAccept := seen["CPA_ACCEPT"]
		_, hasRedep := seen["CPA_REDEP"]
		return hasHold && hasAccept && hasRedep
	})
	if !ok {
		s.t.Fatalf("attribute conversion: not all of CPA_HOLD/CPA_ACCEPT/CPA_REDEP appeared in /conversions/%s within 10s (saw: %v)", s.clickID, seen)
	}

	if timeline.CampaignID != s.campaignID {
		s.t.Fatalf("attribute conversion: timeline campaignId=%q, want %q — conversion attributed to the wrong campaign", timeline.CampaignID, s.campaignID)
	}
	if timeline.NetworkID != s.networkID {
		s.t.Fatalf("attribute conversion: timeline networkId=%q, want %q", timeline.NetworkID, s.networkID)
	}
	accept := seen["CPA_ACCEPT"]
	if !accept.HasUSDValue || accept.USDValue != 25.50 {
		s.t.Fatalf("attribute conversion: CPA_ACCEPT usdValue=%v hasUsdValue=%v, want 25.50/true", accept.USDValue, accept.HasUSDValue)
	}
	redep := seen["CPA_REDEP"]
	if !redep.HasUSDValue || redep.USDValue != 10.00 {
		s.t.Fatalf("attribute conversion: CPA_REDEP usdValue=%v hasUsdValue=%v, want 10.00/true", redep.USDValue, redep.HasUSDValue)
	}
	s.t.Logf("attribution confirmed: click %s -> campaign %s, network %s, full HOLD/ACCEPT/REDEP timeline", s.clickID, timeline.CampaignID, timeline.NetworkID)
}

// sendPostback verifies FLOX's outbound side (§46): a successful incoming
// conversion enqueues a macro-resolved delivery back to the network's own
// postback_url (internal/conversion.go's Record, `s.deliveries.Enqueue`).
// That enqueue is a synchronous Postgres insert (internal/postback's
// Enqueuer), so the row is queryable immediately — no poll needed, unlike
// the ClickHouse-backed steps above. This checks FLOX correctly queued and
// macro-resolved the delivery; it does not require cb.example.test to be
// a real reachable server (apps/worker's Deliverer will attempt and fail
// that part harmlessly, same as any offline network endpoint would).
func (s *scenario) sendPostback() {
	s.t.Helper()
	ctx := context.Background()

	rows, err := s.pg.Query(ctx,
		`SELECT status, url FROM postback_deliveries WHERE organization_id = $1 AND click_id = $2 ORDER BY created_at`,
		s.orgID, s.clickID,
	)
	if err != nil {
		s.t.Fatalf("querying postback_deliveries: %v", err)
	}
	defer rows.Close()

	found := map[string]bool{}
	for rows.Next() {
		var status, deliveryURL string
		if err := rows.Scan(&status, &deliveryURL); err != nil {
			s.t.Fatalf("scanning postback_deliveries row: %v", err)
		}
		if !strings.Contains(deliveryURL, s.clickID) {
			s.t.Fatalf("postback delivery for status %s has url %q — click_id macro was not resolved", status, deliveryURL)
		}
		found[status] = true
	}
	if err := rows.Err(); err != nil {
		s.t.Fatalf("reading postback_deliveries: %v", err)
	}

	for _, want := range []string{"CPA_HOLD", "CPA_ACCEPT", "CPA_REDEP"} {
		if !found[want] {
			s.t.Fatalf("send postback: no outbound delivery was enqueued for %s", want)
		}
	}
	s.t.Log("outbound postback deliveries enqueued and macro-resolved for CPA_HOLD/CPA_ACCEPT/CPA_REDEP")
}

// analyticsAndLTV closes the funnel: the click and the revenue it drove
// must show up in the two aggregate views a real operator would actually
// look at (§57's final "Analytics + LTV" box). Both are ClickHouse-backed
// off the same ..._daily materialized views the worker's flush populates,
// so they share the poll-with-timeout treatment used above.
func (s *scenario) analyticsAndLTV() {
	s.t.Helper()
	today := time.Now().UTC().Format(dateLayout)

	var daily struct {
		Counts []struct {
			Type       string `json:"type"`
			EventCount uint64 `json:"eventCount"`
		} `json:"counts"`
	}
	ok := pollUntil(s.t, 10*time.Second, 500*time.Millisecond, func() bool {
		s.get(fmt.Sprintf("/analytics/campaigns/%s/daily?from=%s&to=%s", s.campaignID, today, today), &daily, http.StatusOK)
		for _, c := range daily.Counts {
			if c.Type == "SOURCE_CLICK" && c.EventCount >= 1 {
				return true
			}
		}
		return false
	})
	if !ok {
		s.t.Fatalf("analytics: SOURCE_CLICK never appeared in /analytics/campaigns/%s/daily (got %+v)", s.campaignID, daily.Counts)
	}
	s.t.Log("analytics: click volume confirmed in daily aggregate")

	var revenue struct {
		Revenue []struct {
			Type       string  `json:"type"`
			EventCount uint64  `json:"eventCount"`
			RevenueUSD float64 `json:"revenueUsd"`
		} `json:"revenue"`
	}
	ok = pollUntil(s.t, 10*time.Second, 500*time.Millisecond, func() bool {
		s.get(fmt.Sprintf("/analytics/campaigns/%s/daily-revenue?from=%s&to=%s", s.campaignID, today, today), &revenue, http.StatusOK)
		for _, r := range revenue.Revenue {
			if r.Type == "CPA_ACCEPT" && r.RevenueUSD > 0 {
				return true
			}
		}
		return false
	})
	if !ok {
		s.t.Fatalf("analytics: CPA_ACCEPT revenue never appeared in /analytics/campaigns/%s/daily-revenue (got %+v)", s.campaignID, revenue.Revenue)
	}
	s.t.Log("analytics: CPA_ACCEPT revenue confirmed in daily-revenue aggregate")

	// ?from=today&to=today deliberately exercises the same-day filter a
	// real report load would use. internal/chstore's LTVFilter query is a
	// half-open `event_at >= from AND event_at < to` range
	// (apps/internal/chstore/ltv.go); this scenario is also this
	// endpoint's regression test for the bug that filter shape caused
	// until apps/internal/ltv/handler.go's parseParams end-of-day fix
	// (this phase): a bare date-only `to` used to parse as that day's
	// midnight, silently excluding every same-day anchor event, including
	// the one this scenario just created. parseParams now adds
	// 23:59:59.999 to a same-day `to`, matching internal/conversions'
	// handler, which already did this for the identical reason.
	var ftd struct {
		Cohorts []struct {
			CampaignID  string `json:"campaignId"`
			AnchorCount int    `json:"anchorCount"`
		} `json:"cohorts"`
	}
	s.get(fmt.Sprintf("/analytics/ltv/ftd-cohorts?from=%s&to=%s&campaignId=%s", today, today, s.campaignID), &ftd, http.StatusOK)
	if !hasAnchor(ftd.Cohorts, s.campaignID) {
		s.t.Fatalf("ltv: no FTD cohort (anchored on CPA_ACCEPT) found for campaign %s: %+v", s.campaignID, ftd.Cohorts)
	}
	s.t.Log("ltv: FTD cohort confirmed (anchored on CPA_ACCEPT)")

	var reg struct {
		Cohorts []struct {
			CampaignID  string `json:"campaignId"`
			AnchorCount int    `json:"anchorCount"`
		} `json:"cohorts"`
	}
	s.get(fmt.Sprintf("/analytics/ltv/reg-cohorts?from=%s&to=%s&campaignId=%s", today, today, s.campaignID), &reg, http.StatusOK)
	if !hasAnchor(reg.Cohorts, s.campaignID) {
		s.t.Fatalf("ltv: no registration cohort (anchored on CPA_HOLD) found for campaign %s: %+v", s.campaignID, reg.Cohorts)
	}
	s.t.Log("ltv: registration cohort confirmed (anchored on CPA_HOLD)")
}

func hasAnchor(cohorts []struct {
	CampaignID  string `json:"campaignId"`
	AnchorCount int    `json:"anchorCount"`
}, campaignID string) bool {
	for _, c := range cohorts {
		if c.CampaignID == campaignID && c.AnchorCount >= 1 {
			return true
		}
	}
	return false
}

// pollUntil retries fn every interval until it returns true or timeout
// elapses. Every ClickHouse-backed read in this scenario needs this: the
// worker's flush poll loop (apps/worker/main.go, batch 500/idle 2s) means
// there is real, expected lag between "the tracker/postback handler
// returned 200" and "the row is visible to a query" — asserting
// synchronously would make this test flaky by construction, not
// discovering a real bug.
func pollUntil(t *testing.T, timeout, interval time.Duration, fn func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if fn() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(interval)
	}
}
