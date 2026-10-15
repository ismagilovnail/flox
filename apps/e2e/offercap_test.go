package e2e

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/testpg"
)

// TestOfferDailyCap drives the whole path of the daily cap through the real
// stack: two offers split one stream set 50/50, offer A has a cap of 1; once
// A has one conversion today the tracker (via internal/offercap's background
// snapshot) must send every visitor to B.
func TestOfferDailyCap(t *testing.T) {
	apiBase := envOr("FLOX_E2E_API_URL", "http://localhost:8080")
	trackerBase := envOr("FLOX_E2E_TRACKER_URL", "http://localhost:8081")
	dsn, chURL := os.Getenv("DATABASE_URL"), os.Getenv("CLICKHOUSE_URL")
	if dsn == "" || chURL == "" {
		t.Skip("needs DATABASE_URL and CLICKHOUSE_URL")
	}
	requireHealthy(t, apiBase)
	requireHealthy(t, trackerBase)

	ctx := context.Background()
	pg, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pg.Close)
	ch, err := chconn.NewConn(ctx, config.ClickHouseConfig{
		URL: chURL, Database: envOr("CLICKHOUSE_DATABASE", "flox"),
		User: envOr("CLICKHOUSE_USER", "flox"), Password: os.Getenv("CLICKHOUSE_PASSWORD"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ch.Close() })
	jar, _ := cookiejar.New(nil)

	s := &scenario{
		t: t, apiBase: apiBase, trackerBase: trackerBase,
		appURL: envOr("FLOX_E2E_APP_URL", "http://localhost:3000"),
		http:   &http.Client{Jar: jar, Timeout: 10 * time.Second}, pg: pg, ch: ch,
	}
	t.Cleanup(func() {
		if s.orgID == "" {
			return
		}
		_ = testpg.DeleteOrganization(context.Background(), pg, s.orgID)
		for _, table := range []string{
			"click_events", "tracking_events", "conversion_events", "postback_events",
			"click_events_daily_campaign", "click_events_daily_geo", "conversion_events_daily_campaign",
		} {
			_ = ch.Exec(context.Background(), "ALTER TABLE "+table+" DELETE WHERE organization_id = ?", s.orgID)
		}
	})

	s.createOrganization()
	s.createTrafficSource()
	s.createNetwork()
	s.createCampaign()
	s.registerEventMappings()

	offer := func(name, link string, cap any) string {
		var resp struct {
			ID string `json:"id"`
		}
		s.post("/offers", map[string]any{
			"networkId": s.networkID, "name": name, "countries": []string{"US"},
			"payout": 10, "currency": "USD", "cap": cap,
			"links": []map[string]string{{"label": "Main", "url": link}},
		}, &resp, http.StatusCreated)
		return resp.ID
	}
	const urlA, urlB = "https://offer-a.example.test/go", "https://offer-b.example.test/go"
	offerA := offer("Capped A", urlA, 1)
	offerB := offer("Open B", urlB, nil)

	flow := func(name, offerID string) map[string]any {
		return map[string]any{
			"name": name, "active": true, "weight": 50,
			"landing":     map[string]any{"enabled": false, "landingId": "", "asPwa": false},
			"pwa":         map[string]any{"enabled": false, "pwaId": "", "pwaType": ""},
			"postlanding": map[string]any{"enabled": false, "postlandingId": ""},
			"destination": map[string]any{"kind": "offer", "offerId": offerID},
		}
	}
	var set struct {
		ID string `json:"id"`
	}
	s.post(fmt.Sprintf("/campaigns/%s/stream-sets", s.campaignID), map[string]any{
		"name": "Cap Set", "fallbackUrl": "https://set-fallback.example.test",
		"rootFilter": map[string]any{"type": "group", "joiner": "AND", "children": []map[string]any{
			{"type": "condition", "field": "bot", "operator": "IS", "value": "0", "valueTo": ""},
		}},
		"flows":    []map[string]any{flow("A", offerA), flow("B", offerB)},
		"pixelIds": []string{},
	}, &set, http.StatusCreated)
	s.generateTrackingURL()

	click := func(i int) string {
		req, _ := http.NewRequest(http.MethodGet, s.trackerBase+"/t/"+s.slug, nil)
		req.Host = s.domain
		req.Header.Set("User-Agent", fmt.Sprintf("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) FloxCap/%d", i))
		c := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.Header.Get("Location")
	}

	// Before any conversion both offers take traffic. Clicks on A are
	// remembered to convert one of them.
	seen := map[string]bool{}
	for i := 0; i < 60 && len(seen) < 2; i++ {
		seen[click(i)] = true
	}
	if !seen[urlA] || !seen[urlB] {
		t.Fatalf("before the cap both offers should receive traffic, saw %v", seen)
	}

	// Convert one click that went to A.
	var clickID string
	if !pollUntil(t, pollBudget, 500*time.Millisecond, func() bool {
		return ch.QueryRow(ctx, `SELECT click_id FROM click_events WHERE organization_id = ? AND type = 'SOURCE_CLICK' AND offer_id = ? LIMIT 1`, s.orgID, offerA).Scan(&clickID) == nil
	}) {
		t.Fatal("no click on offer A reached ClickHouse")
	}
	s.clickID = clickID
	if res, _ := s.sendConversionPostback("reg", nil, "", ""); res == "" {
		t.Fatal("postback was not recorded")
	}

	// Within a snapshot refresh (30s) + worker flush, nobody is sent to A.
	allToB := func() bool {
		for i := 100; i < 130; i++ {
			if click(i) != urlB {
				return false
			}
		}
		return true
	}
	if !pollUntil(t, 120*time.Second, 3*time.Second, allToB) {
		t.Fatal("offer A reached its cap of 1 but still receives traffic")
	}
}
