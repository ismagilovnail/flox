package e2e

// TestFilterMatrix drives every stream-set filter field and operator through
// the real stack: a stream set is created over the API with ONE condition (or
// a small tree), a request is sent to the real tracker with controlled headers
// and query parameters, and the 302 Location says which way it was routed —
// the stream set's flow ("matched") or the campaign fallback ("not matched").
// It checks the integration the unit tests can't: classifier (User-Agent,
// Accept-Language, GeoIP, ASN), the tracker's parameter plumbing, the API's
// stream-set validation and the routing engine's operators, together.
//
// Like TestFullFunnel it needs the dev stack running and DATABASE_URL set, and
// skips otherwise. It uses its own throwaway organization.

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/config"
)

const (
	matchedURL  = "https://matched.example.test/flow"
	fallbackURL = "https://fallback.example.test"

	uaChromeWin = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// visit is one request to the tracker. The zero value is filled with the
// defaults of baseVisit.
type visit struct {
	ip       string
	ua       string
	lang     string
	referrer string
	query    url.Values
}

func baseVisit() visit {
	q := url.Values{}
	q.Set("utm_source", "facebook")
	q.Set("utm_medium", "cpc")
	q.Set("utm_campaign", "spring_sale")
	q.Set("utm_content", "creative7")
	q.Set("utm_term", "running shoes")
	q.Set("external_click_id", "ext-12345")
	for i, v := range []string{"", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india", "juliet"} {
		if v != "" {
			q.Set("sub"+strconv.Itoa(i+1), v)
		}
	}
	q.Set("sub1", "50") // numeric on purpose: the comparison operators need a number
	return visit{
		ip:       "8.8.8.8", // US, AS15169
		ua:       uaChromeWin,
		lang:     "ru-RU,ru;q=0.9,en;q=0.8",
		referrer: "https://news.example.com/article?id=1",
		query:    q,
	}
}

// with returns a copy of the base visit changed by f.
func with(f func(v *visit)) visit {
	v := baseVisit()
	f(&v)
	return v
}

type cond struct{ field, op, value, valueTo string }

func (c cond) node() map[string]any {
	return map[string]any{"type": "condition", "field": c.field, "operator": c.op, "value": c.value, "valueTo": c.valueTo}
}

func group(joiner string, children ...map[string]any) map[string]any {
	return map[string]any{"type": "group", "joiner": joiner, "children": children}
}

type filterCase struct {
	name string
	tree map[string]any
	v    visit
	want bool // true: routed through the stream set's flow
}

// one is a single-condition case.
func one(name string, c cond, v visit, want bool) filterCase {
	return filterCase{name: name, tree: group("AND", c.node()), v: v, want: want}
}

// filterEnv is a throwaway organization with one active campaign and a tracking
// link, plus a click helper that never follows the 302.
type filterEnv struct {
	s     *scenario
	click func(v visit) string
}

func newFilterEnv(t *testing.T) *filterEnv {
	t.Helper()
	apiBase := envOr("FLOX_E2E_API_URL", "http://localhost:8080")
	trackerBase := envOr("FLOX_E2E_TRACKER_URL", "http://localhost:8081")
	appURL := envOr("FLOX_E2E_APP_URL", "http://localhost:3000")
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("filter tests need DATABASE_URL (same value apps/api was started with)")
	}
	requireHealthy(t, apiBase)
	requireHealthy(t, trackerBase)

	ctx := context.Background()
	pg, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting to postgres: %v", err)
	}
	t.Cleanup(pg.Close)
	jar, _ := cookiejar.New(nil)
	s := &scenario{t: t, apiBase: apiBase, trackerBase: trackerBase, appURL: appURL,
		http: &http.Client{Jar: jar, Timeout: 10 * time.Second}, pg: pg}
	t.Cleanup(func() {
		if s.orgID != "" {
			_, _ = pg.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, s.orgID)
			if chURL := os.Getenv("CLICKHOUSE_URL"); chURL != "" {
				if ch, err := chconn.NewConn(context.Background(), config.ClickHouseConfig{
					URL: chURL, Database: envOr("CLICKHOUSE_DATABASE", "flox"), User: envOr("CLICKHOUSE_USER", "flox"),
					Password: os.Getenv("CLICKHOUSE_PASSWORD"),
				}); err == nil {
					for _, table := range []string{"click_events", "tracking_events", "click_events_daily_campaign", "click_events_daily_geo"} {
						_ = ch.Exec(context.Background(), "ALTER TABLE "+table+" DELETE WHERE organization_id = ?", s.orgID)
					}
					_ = ch.Close()
				}
			}
		}
	})

	s.createOrganization()
	s.createTrafficSource()
	var camp struct {
		ID string `json:"id"`
	}
	s.post("/campaigns", map[string]any{
		"trafficSourceId": s.trafficSourceID, "name": "Filter matrix", "fallbackUrl": fallbackURL,
		"notes": "", "externalCampaignId": "",
	}, &camp, http.StatusCreated)
	s.campaignID = camp.ID
	s.post("/campaigns/"+s.campaignID+"/activate", nil, nil, http.StatusOK)
	s.generateTrackingURL()

	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	click := func(v visit) string {
		t.Helper()
		u := trackerBase + "/t/" + s.slug + "?" + v.query.Encode()
		req, _ := http.NewRequest(http.MethodGet, u, nil)
		req.Host = s.domain
		req.Header.Set("User-Agent", v.ua)
		req.Header.Set("Accept-Language", v.lang)
		req.Header.Set("X-Forwarded-For", v.ip)
		if v.referrer != "" {
			req.Header.Set("Referer", v.referrer)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("click: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusFound {
			t.Fatalf("click: want 302, got %d", resp.StatusCode)
		}
		return resp.Header.Get("Location")
	}
	return &filterEnv{s: s, click: click}
}

func TestFilterMatrix(t *testing.T) {
	env := newFilterEnv(t)
	s, click := env.s, env.click

	// One stream set for the whole matrix: each case only swaps its filter tree
	// (PATCH), which keeps the run well under the API's 300 requests/minute.
	var set struct {
		ID string `json:"id"`
	}
	s.post("/campaigns/"+s.campaignID+"/stream-sets", map[string]any{
		"name": "case", "fallbackUrl": "", "rootFilter": group("AND"),
		"flows": []map[string]any{{
			"name": "f", "active": true, "weight": 100,
			"landing":     map[string]any{"enabled": false, "landingId": "", "asPwa": false},
			"pwa":         map[string]any{"enabled": false, "pwaId": "", "pwaType": ""},
			"postlanding": map[string]any{"enabled": false, "postlandingId": ""},
			"destination": map[string]any{"kind": "redirect", "url": matchedURL},
		}},
		"pixelIds": []string{},
	}, &set, http.StatusCreated)

	run := func(c filterCase) {
		t.Helper()
		flushes := s.trackerRoutingFlushes()
		s.doAPI(s.apiRequest(http.MethodPatch, "/campaigns/"+s.campaignID+"/stream-sets/"+set.ID, map[string]any{"rootFilter": c.tree}), http.StatusOK, nil)
		s.waitForTrackerRoutingFlush(flushes)
		got := click(c.v)
		wantURL := fallbackURL
		if c.want {
			wantURL = matchedURL
		}
		if got != wantURL {
			t.Errorf("%s: routed to %q, want %q (matched=%v)", c.name, got, wantURL, c.want)
		}
	}

	iphone := "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1"
	ipad := "Mozilla/5.0 (iPad; CPU OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1"
	androidChrome := "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Mobile Safari/537.36"
	macSafari := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15"
	linuxFirefox := "Mozilla/5.0 (X11; Linux x86_64; rv:125.0) Gecko/20100101 Firefox/125.0"
	yandex := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 YaBrowser/24.4.0.0 Safari/537.36"
	opera := uaChromeWin + " OPR/110.0.0.0"
	googlebot := "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"

	setUA := func(ua string) visit { return with(func(v *visit) { v.ua = ua }) }
	setIP := func(ip string) visit { return with(func(v *visit) { v.ip = ip }) }
	noQ := func(k string) visit { return with(func(v *visit) { v.query.Del(k) }) }

	cases := []filterCase{
		// --- geo / network (GeoIP + ASN databases) ---
		one("country IS US / US ip", cond{"country", "IS", "US", ""}, baseVisit(), true),
		one("country IS US / RU ip", cond{"country", "IS", "US", ""}, setIP("77.88.8.8"), false),
		one("country IS lowercase value", cond{"country", "IS", "us", ""}, baseVisit(), true),
		one("country IS_NOT US / RU ip", cond{"country", "IS_NOT", "US", ""}, setIP("77.88.8.8"), true),
		one("country IN RU, UA, KZ / RU ip", cond{"country", "IN", "RU,UA,KZ", ""}, setIP("77.88.8.8"), true),
		one("country IN with spaces after commas", cond{"country", "IN", "RU, UA, KZ", ""}, setIP("77.88.8.8"), true),
		one("country IN RU, UA / US ip", cond{"country", "IN", "RU,UA", ""}, baseVisit(), false),
		one("country NOT_IN RU, UA / US ip", cond{"country", "NOT_IN", "RU,UA", ""}, baseVisit(), true),
		one("country NOT_IN RU, UA / RU ip", cond{"country", "NOT_IN", "RU,UA", ""}, setIP("77.88.8.8"), false),
		one("asn IS AS15169", cond{"asn", "IS", "AS15169", ""}, baseVisit(), true),
		one("asn IS 15169 (without the AS prefix)", cond{"asn", "IS", "15169", ""}, baseVisit(), true),
		one("asn IS AS13238 / US ip", cond{"asn", "IS", "AS13238", ""}, baseVisit(), false),
		one("city IS Mountain View (still supported server side)", cond{"city", "IS", "Mountain View", ""}, baseVisit(), true),
		one("region IS California (still supported server side)", cond{"region", "IS", "California", ""}, baseVisit(), true),

		// --- device / platform / os ---
		one("device IS desktop", cond{"device", "IS", "desktop", ""}, baseVisit(), true),
		one("device IS mobile / iPhone", cond{"device", "IS", "mobile", ""}, setUA(iphone), true),
		one("device IS mobile / desktop", cond{"device", "IS", "mobile", ""}, baseVisit(), false),
		one("device IS tablet / iPad", cond{"device", "IS", "tablet", ""}, setUA(ipad), true),
		one("device IN mobile,tablet / iPad", cond{"device", "IN", "mobile,tablet", ""}, setUA(ipad), true),
		one("platform IS ios / iPhone", cond{"platform", "IS", "ios", ""}, setUA(iphone), true),
		one("platform IS android / Pixel", cond{"platform", "IS", "android", ""}, setUA(androidChrome), true),
		one("platform IS windows", cond{"platform", "IS", "windows", ""}, baseVisit(), true),
		one("platform IS macos / Mac Safari", cond{"platform", "IS", "macos", ""}, setUA(macSafari), true),
		one("platform IS linux / Firefox", cond{"platform", "IS", "linux", ""}, setUA(linuxFirefox), true),
		one("os IS ios / iPhone", cond{"os", "IS", "ios", ""}, setUA(iphone), true),
		one("os IS android / desktop", cond{"os", "IS", "android", ""}, baseVisit(), false),
		one("os_version IS 17.4 / iPhone", cond{"os_version", "IS", "17.4", ""}, setUA(iphone), true),
		one("os_version GT 9 (numeric 10.0 > 9) / Windows 10", cond{"os_version", "GT", "9", ""}, baseVisit(), true),
		one("os_version LT 9 / Windows 10", cond{"os_version", "LT", "9", ""}, baseVisit(), false),
		one("os_version BETWEEN 10 and 12 / iOS 17.4", cond{"os_version", "BETWEEN", "10", "12"}, setUA(iphone), false),
		one("os_version BETWEEN 14 and 18 / iOS 17.4", cond{"os_version", "BETWEEN", "14", "18"}, setUA(iphone), true),

		// --- browser ---
		one("browser IS chrome", cond{"browser", "IS", "chrome", ""}, baseVisit(), true),
		one("browser IS safari / iPhone", cond{"browser", "IS", "safari", ""}, setUA(iphone), true),
		one("browser IS firefox", cond{"browser", "IS", "firefox", ""}, setUA(linuxFirefox), true),
		one("browser IS yandex", cond{"browser", "IS", "yandex", ""}, setUA(yandex), true),
		one("browser IS chrome / Yandex (must not fall into chrome)", cond{"browser", "IS", "chrome", ""}, setUA(yandex), false),
		one("browser IS opera", cond{"browser", "IS", "opera", ""}, setUA(opera), true),
		one("browser IN chrome,edge", cond{"browser", "IN", "chrome,edge", ""}, baseVisit(), true),
		one("browser_version IS 124.0.0.0", cond{"browser_version", "IS", "124.0.0.0", ""}, baseVisit(), true),
		one("browser_version STARTS_WITH 124", cond{"browser_version", "STARTS_WITH", "124", ""}, baseVisit(), true),
		one("browser_version GTE 100 (numeric)", cond{"browser_version", "GTE", "100", ""}, baseVisit(), true),

		// --- language (Accept-Language -> primary subtag) ---
		one("language IS ru / ru-RU", cond{"language", "IS", "ru", ""}, baseVisit(), true),
		one("language IS en / ru-RU", cond{"language", "IS", "en", ""}, baseVisit(), false),
		one("language IS en / en-US", cond{"language", "IS", "en", ""}, with(func(v *visit) { v.lang = "en-US,en;q=0.9" }), true),
		one("language IS uk / uk-UA", cond{"language", "IS", "uk", ""}, with(func(v *visit) { v.lang = "uk-UA" }), true),
		one("language IN ru,uk,be", cond{"language", "IN", "ru,uk,be", ""}, baseVisit(), true),
		one("language NOT_EXISTS / no header", cond{"language", "NOT_EXISTS", "", ""}, with(func(v *visit) { v.lang = "" }), true),

		// --- bot / proxy / connection ---
		one("bot IS 0 / normal browser", cond{"bot", "IS", "0", ""}, baseVisit(), true),
		one("bot IS 1 / Googlebot", cond{"bot", "IS", "1", ""}, setUA(googlebot), true),
		one("bot IS 0 / Googlebot", cond{"bot", "IS", "0", ""}, setUA(googlebot), false),
		one("proxy IS 0", cond{"proxy", "IS", "0", ""}, baseVisit(), true),
		one("connection_type IS unknown", cond{"connection_type", "IS", "unknown", ""}, baseVisit(), true),

		// --- link parameters (utm_*, sub1..10, external id, referrer) ---
		one("utm_source IS facebook", cond{"utm_source", "IS", "facebook", ""}, baseVisit(), true),
		one("utm_source IS case-insensitive FACEBOOK", cond{"utm_source", "IS", "FACEBOOK", ""}, baseVisit(), true),
		one("utm_source IS tiktok", cond{"utm_source", "IS", "tiktok", ""}, baseVisit(), false),
		one("utm_medium IS cpc", cond{"utm_medium", "IS", "cpc", ""}, baseVisit(), true),
		one("utm_campaign CONTAINS sale", cond{"utm_campaign", "CONTAINS", "sale", ""}, baseVisit(), true),
		one("utm_content ENDS_WITH 7", cond{"utm_content", "ENDS_WITH", "7", ""}, baseVisit(), true),
		one("utm_term IS (with a space)", cond{"utm_term", "IS", "running shoes", ""}, baseVisit(), true),
		one("external_click_id STARTS_WITH ext-", cond{"external_click_id", "STARTS_WITH", "ext-", ""}, baseVisit(), true),
		one("external_click_id EXISTS", cond{"external_click_id", "EXISTS", "", ""}, baseVisit(), true),
		one("external_click_id NOT_EXISTS / missing", cond{"external_click_id", "NOT_EXISTS", "", ""}, noQ("external_click_id"), true),
		one("external_click_id EXISTS / missing", cond{"external_click_id", "EXISTS", "", ""}, noQ("external_click_id"), false),
		one("referrer CONTAINS news.example.com", cond{"referrer", "CONTAINS", "news.example.com", ""}, baseVisit(), true),
		one("referrer EXISTS / none", cond{"referrer", "EXISTS", "", ""}, with(func(v *visit) { v.referrer = "" }), false),
		one("referrer NOT_EXISTS / none", cond{"referrer", "NOT_EXISTS", "", ""}, with(func(v *visit) { v.referrer = "" }), true),

		// --- operators on one string field (utm_source = "facebook") ---
		one("op IS", cond{"utm_source", "IS", "facebook", ""}, baseVisit(), true),
		one("op IS_NOT match", cond{"utm_source", "IS_NOT", "tiktok", ""}, baseVisit(), true),
		one("op IS_NOT miss", cond{"utm_source", "IS_NOT", "facebook", ""}, baseVisit(), false),
		one("op IN", cond{"utm_source", "IN", "google,facebook", ""}, baseVisit(), true),
		one("op NOT_IN", cond{"utm_source", "NOT_IN", "google,tiktok", ""}, baseVisit(), true),
		one("op CONTAINS", cond{"utm_source", "CONTAINS", "ceboo", ""}, baseVisit(), true),
		one("op NOT_CONTAINS", cond{"utm_source", "NOT_CONTAINS", "google", ""}, baseVisit(), true),
		one("op NOT_CONTAINS miss", cond{"utm_source", "NOT_CONTAINS", "face", ""}, baseVisit(), false),
		one("op STARTS_WITH", cond{"utm_source", "STARTS_WITH", "face", ""}, baseVisit(), true),
		one("op ENDS_WITH", cond{"utm_source", "ENDS_WITH", "book", ""}, baseVisit(), true),
		one("op MATCHES", cond{"utm_source", "MATCHES", "^(face|insta)book$", ""}, baseVisit(), true),
		one("op MATCHES miss", cond{"utm_source", "MATCHES", "^tik", ""}, baseVisit(), false),
		one("op EXISTS", cond{"utm_source", "EXISTS", "", ""}, baseVisit(), true),
		one("op NOT_EXISTS miss", cond{"utm_source", "NOT_EXISTS", "", ""}, baseVisit(), false),

		// --- numeric operators on sub1 = 50 ---
		one("sub1 GT 40", cond{"sub1", "GT", "40", ""}, baseVisit(), true),
		one("sub1 GT 50", cond{"sub1", "GT", "50", ""}, baseVisit(), false),
		one("sub1 GTE 50", cond{"sub1", "GTE", "50", ""}, baseVisit(), true),
		one("sub1 LT 60", cond{"sub1", "LT", "60", ""}, baseVisit(), true),
		one("sub1 LT 50", cond{"sub1", "LT", "50", ""}, baseVisit(), false),
		one("sub1 LTE 50", cond{"sub1", "LTE", "50", ""}, baseVisit(), true),
		one("sub1 BETWEEN 40 and 60", cond{"sub1", "BETWEEN", "40", "60"}, baseVisit(), true),
		one("sub1 BETWEEN 60 and 80", cond{"sub1", "BETWEEN", "60", "80"}, baseVisit(), false),
		one("sub1 GT 9 (numeric, not 'text 50' < '9')", cond{"sub1", "GT", "9", ""}, baseVisit(), true),
		one("sub1 BETWEEN inclusive upper bound", cond{"sub1", "BETWEEN", "10", "50"}, baseVisit(), true),

		// --- groups: AND / OR / nested ---
		{"AND both true", group("AND", cond{"country", "IS", "US", ""}.node(), cond{"browser", "IS", "chrome", ""}.node()), baseVisit(), true},
		{"AND one false", group("AND", cond{"country", "IS", "US", ""}.node(), cond{"browser", "IS", "firefox", ""}.node()), baseVisit(), false},
		{"OR one true", group("OR", cond{"country", "IS", "RU", ""}.node(), cond{"browser", "IS", "chrome", ""}.node()), baseVisit(), true},
		{"OR both false", group("OR", cond{"country", "IS", "RU", ""}.node(), cond{"browser", "IS", "firefox", ""}.node()), baseVisit(), false},
		{"nested AND(country, OR(browser,device))",
			group("AND", cond{"country", "IS", "US", ""}.node(),
				group("OR", cond{"browser", "IS", "firefox", ""}.node(), cond{"device", "IS", "desktop", ""}.node())),
			baseVisit(), true},
		{"nested AND(country, OR(false,false))",
			group("AND", cond{"country", "IS", "US", ""}.node(),
				group("OR", cond{"browser", "IS", "firefox", ""}.node(), cond{"device", "IS", "mobile", ""}.node())),
			baseVisit(), false},
		{"nested OR(country RU, AND(true,true))",
			group("OR", cond{"country", "IS", "RU", ""}.node(),
				group("AND", cond{"platform", "IS", "windows", ""}.node(), cond{"utm_source", "IS", "facebook", ""}.node())),
			baseVisit(), true},
		{"empty group matches everything", group("AND"), baseVisit(), true},
	}

	// sub2..sub10: each one is its own parameter, IS must read the right one.
	subValues := []string{"bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india", "juliet"}
	for i, val := range subValues {
		field := "sub" + strconv.Itoa(i+2)
		cases = append(cases,
			one(field+" IS "+val, cond{field, "IS", val, ""}, baseVisit(), true),
			one(field+" IS other", cond{field, "IS", "zzz", ""}, baseVisit(), false),
		)
	}

	for _, c := range cases {
		run(c)
	}
}

// TestAnalyticsFilterMatrix sends four distinguishable clicks through the
// tracker and then asks the report endpoint for the clicks under each filter
// dimension (and a few AND combinations): every count must be exactly the
// number of those clicks that really carried that value.
func TestAnalyticsFilterMatrix(t *testing.T) {
	env := newFilterEnv(t)
	s := env.s

	var set struct {
		ID    string `json:"id"`
		Flows []struct {
			ID string `json:"id"`
		} `json:"flows"`
	}
	s.post("/campaigns/"+s.campaignID+"/stream-sets", map[string]any{
		"name": "all traffic", "fallbackUrl": "", "rootFilter": group("AND"),
		"flows": []map[string]any{{
			"name": "f", "active": true, "weight": 100,
			"landing":     map[string]any{"enabled": false, "landingId": "", "asPwa": false},
			"pwa":         map[string]any{"enabled": false, "pwaId": "", "pwaType": ""},
			"postlanding": map[string]any{"enabled": false, "postlandingId": ""},
			"destination": map[string]any{"kind": "redirect", "url": matchedURL},
		}},
		"pixelIds": []string{},
	}, &set, http.StatusCreated)

	iphone := "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1"
	android := "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Mobile Safari/537.36"
	firefox := "Mozilla/5.0 (X11; Linux x86_64; rv:125.0) Gecko/20100101 Firefox/125.0"
	visits := []visit{
		{ip: "8.8.8.8", ua: uaChromeWin, lang: "ru-RU,ru;q=0.9", query: url.Values{}}, // US desktop windows chrome ru
		{ip: "77.88.8.8", ua: iphone, lang: "en-US,en;q=0.9", query: url.Values{}},    // RU mobile ios safari en
		{ip: "77.88.8.8", ua: android, lang: "uk-UA,uk;q=0.9", query: url.Values{}},   // RU mobile android chrome uk
		{ip: "5.9.0.1", ua: firefox, lang: "de-DE,de;q=0.9", query: url.Values{}},     // DE desktop linux firefox de
	}
	for _, v := range visits {
		if got := env.click(v); got != matchedURL {
			t.Fatalf("setup click routed to %q, want the all-traffic flow", got)
		}
	}

	type flt = map[string]string
	report := func(filters ...flt) (clicks float64, ok bool) {
		t.Helper()
		today := time.Now().UTC()
		var resp struct {
			Rows []struct {
				Metrics struct {
					Clicks float64 `json:"clicks"`
				} `json:"metrics"`
			} `json:"rows"`
		}
		fs := make([]map[string]string, 0, len(filters))
		fs = append(fs, filters...)
		s.post("/analytics/report", map[string]any{
			"dims": []string{}, "filters": fs,
			"from": today.AddDate(0, 0, -1).Format(dateLayout), "to": today.AddDate(0, 0, 1).Format(dateLayout),
			"sort": map[string]string{"key": "clicks", "dir": "desc"},
		}, &resp, http.StatusOK)
		if len(resp.Rows) == 0 {
			return 0, true
		}
		return resp.Rows[0].Metrics.Clicks, true
	}

	// The worker flushes events to ClickHouse every couple of seconds.
	if !pollUntil(t, 30*time.Second, 500*time.Millisecond, func() bool { c, _ := report(); return c == 4 }) {
		c, _ := report()
		t.Fatalf("expected 4 clicks in the report after the worker flush, got %v", c)
	}

	f := func(dim, value string) flt { return flt{"dimension": dim, "value": value} }
	cases := []struct {
		name    string
		filters []flt
		want    float64
	}{
		{"country US", []flt{f("country", "US")}, 1},
		{"country RU", []flt{f("country", "RU")}, 2},
		{"country DE", []flt{f("country", "DE")}, 1},
		{"country FR (none)", []flt{f("country", "FR")}, 0},
		{"region California", []flt{f("region", "California")}, 1},
		{"city Mountain View", []flt{f("city", "Mountain View")}, 1},
		{"device desktop", []flt{f("device", "desktop")}, 2},
		{"device mobile", []flt{f("device", "mobile")}, 2},
		{"platform ios", []flt{f("platform", "ios")}, 1},
		{"os windows", []flt{f("os", "windows")}, 1},
		{"os android", []flt{f("os", "android")}, 1},
		{"os linux", []flt{f("os", "linux")}, 1},
		{"browser chrome", []flt{f("browser", "chrome")}, 2},
		{"browser safari", []flt{f("browser", "safari")}, 1},
		{"browser firefox", []flt{f("browser", "firefox")}, 1},
		{"language ru", []flt{f("language", "ru")}, 1},
		{"language en", []flt{f("language", "en")}, 1},
		{"language uk", []flt{f("language", "uk")}, 1},
		{"language de", []flt{f("language", "de")}, 1},
		{"campaign", []flt{f("campaign", s.campaignID)}, 4},
		{"campaign (other id)", []flt{f("campaign", "no-such-campaign")}, 0},
		{"source (traffic source)", []flt{f("source", s.trafficSourceID)}, 4},
		{"stream_set", []flt{f("stream_set", set.ID)}, 4},
		{"flow", []flt{f("flow", set.Flows[0].ID)}, 4},
		{"landing empty bucket", []flt{f("landing", "")}, 4},
		{"offer empty bucket", []flt{f("offer", "")}, 4},
		{"pwa empty bucket", []flt{f("pwa", "")}, 4},
		{"AND country RU + device mobile", []flt{f("country", "RU"), f("device", "mobile")}, 2},
		{"AND country RU + os ios", []flt{f("country", "RU"), f("os", "ios")}, 1},
		{"AND country US + device mobile (none)", []flt{f("country", "US"), f("device", "mobile")}, 0},
		{"AND browser chrome + country RU + language uk", []flt{f("browser", "chrome"), f("country", "RU"), f("language", "uk")}, 1},
	}
	for _, c := range cases {
		if got, _ := report(c.filters...); got != c.want {
			t.Errorf("%s: clicks = %v, want %v", c.name, got, c.want)
		}
	}
}
