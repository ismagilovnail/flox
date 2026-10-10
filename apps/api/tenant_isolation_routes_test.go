package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/alert"
	"github.com/ismagilovnail/flox/apps/internal/apikey"
	"github.com/ismagilovnail/flox/apps/internal/auth"
	"github.com/ismagilovnail/flox/apps/internal/campaign"
	"github.com/ismagilovnail/flox/apps/internal/clientip"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/contentgallery"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/cost"
	"github.com/ismagilovnail/flox/apps/internal/custommetrics"
	"github.com/ismagilovnail/flox/apps/internal/domain"
	"github.com/ismagilovnail/flox/apps/internal/errtrack"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/eventmapping"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/landing"
	"github.com/ismagilovnail/flox/apps/internal/network"
	"github.com/ismagilovnail/flox/apps/internal/offer"
	"github.com/ismagilovnail/flox/apps/internal/pixel"
	"github.com/ismagilovnail/flox/apps/internal/postlanding"
	"github.com/ismagilovnail/flox/apps/internal/pwa"
	"github.com/ismagilovnail/flox/apps/internal/referral"
	"github.com/ismagilovnail/flox/apps/internal/reportpresets"
	"github.com/ismagilovnail/flox/apps/internal/routing"
	"github.com/ismagilovnail/flox/apps/internal/streamset"
	"github.com/ismagilovnail/flox/apps/internal/tags"
	"github.com/ismagilovnail/flox/apps/internal/trackinglink"
	"github.com/ismagilovnail/flox/apps/internal/trafficsource"
)

// segmentKinds names the resource a path parameter stands for, by the path
// segment right before it. A new parameterised route whose segment is not
// here (and not in notTenantResources) fails the test until it is added
// with a seeded resource.
var segmentKinds = map[string]string{
	"alert-rules":     "alertRule",
	"api-keys":        "apiKey",
	"campaigns":       "campaign",
	"cost-entries":    "costEntry",
	"stream-sets":     "streamSet",
	"tracking-links":  "trackingLink",
	"folders":         "folder",
	"custom-metrics":  "customMetric",
	"domains":         "domain",
	"event-mappings":  "eventMapping",
	"landings":        "landing",
	"networks":        "network",
	"offers":          "offer",
	"pixels":          "pixel",
	"postlandings":    "postlanding",
	"pwas":            "pwa",
	"payouts":         "payout",
	"report-presets":  "reportPreset",
	"tags":            "tag",
	"members":         "member",
	"traffic-sources": "trafficSource",
}

// notTenantResources are path parameters that do not name another
// organization's row.
var notTenantResources = map[string]string{
	"token":    "an invite token: public by design, it is the credential",
	"provider": "an integration provider name (facebook, namecheap…); the row is always the caller's own",
	"clickId":  "a click id is not a Postgres row; ClickHouse reads filter on the session's org (chstore TestCrossTenantIsolation)",
}

// missingIDs holds, for every kind, an id no row has.
var missingIDs = func() map[string]string {
	m := map[string]string{}
	for _, kind := range segmentKinds {
		m[kind] = idgen.New()
	}
	return m
}()

var paramRE = regexp.MustCompile(`\{([^}]+)\}`)

// Tech audit 2026-10-06, §16: organization A, signed in as its owner,
// calls every route of the real router with organization B's ids. No call
// may succeed, show B's data, or change any of B's rows; list routes must
// not show B's ids either. Organization A owns the same kinds of resources,
// so nested routes are also tried with A's parent and B's child.
func TestEveryRouteRefusesAnotherOrganizationsIDs(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	for _, k := range []string{"TOTP_ENCRYPTION_KEY", "INTEGRATION_ENCRYPTION_KEY", "ADACCOUNT_ENCRYPTION_KEY"} {
		if os.Getenv(k) == "" {
			key := make([]byte, 32)
			_, _ = rand.Read(key)
			t.Setenv(k, base64.StdEncoding.EncodeToString(key))
		}
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	reporter, err := errtrack.New(errtrack.Config{})
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := clientip.New(nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	srv, closeAll, err := buildServer(ctx, cfg, logger, reporter, resolver)
	if err != nil {
		t.Fatalf("building the API: %v", err)
	}
	t.Cleanup(closeAll)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	c := &isoClient{t: t, h: srv.Handler(), origin: cfg.AppURL, ip: randomTestIP()}
	orgA, userA := c.signUp(pool)
	orgB, userB := c.signUp(pool)
	idsA := seedEveryKind(t, ctx, pool, orgA, userA, "a")
	idsB := seedEveryKind(t, ctx, pool, orgB, userB, "b")
	c.signIn(orgA)

	before := snapshotOrg(t, ctx, pool, orgB)
	leakIDs := append([]string{orgB, userB}, values(idsB)...)

	type route struct{ method, path string }
	var routes []route
	if err := chi.Walk(srv.Mux(), func(method, path string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes = append(routes, route{method, strings.TrimSuffix(path, "/")})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Reads first, then writes, deletes last: a delete that wrongly
	// succeeded must not hide what the others would have shown.
	order := map[string]int{http.MethodGet: 0, http.MethodPost: 1, http.MethodPut: 1, http.MethodPatch: 1, http.MethodDelete: 2}
	sort.SliceStable(routes, func(i, j int) bool { return order[routes[i].method] < order[routes[j].method] })

	tried := 0
	for _, rt := range routes {
		params := paramRE.FindAllStringSubmatchIndex(rt.path, -1)
		if len(params) == 0 {
			if rt.method == http.MethodGet {
				status, body := c.do(rt.method, rt.path)
				if leaked := firstContained(body, leakIDs); leaked != "" {
					t.Errorf("GET %s (%d) shows organization B's id %s", rt.path, status, leaked)
				}
			}
			continue
		}
		kinds, skip := paramKinds(t, rt.path)
		if skip {
			continue
		}
		// All of B's ids, then for nested routes A's parent with B's child.
		variants := [][]map[string]string{repeat(idsB, len(kinds))}
		if len(kinds) > 1 {
			v := repeat(idsB, len(kinds))
			v[0] = idsA
			variants = append(variants, v)
		}
		for _, owners := range variants {
			// The same call with ids that exist nowhere: B's ids must be
			// answered exactly like those, or A learns B's rows exist.
			none := make([]map[string]string, len(owners))
			for i, m := range owners {
				none[i] = m
				if m["domain"] == idsB["domain"] {
					none[i] = missingIDs
				}
			}
			unknownPath := fillPath(t, rt.path, kinds, none)
			wantStatus, wantBody := c.do(rt.method, unknownPath)
			path := fillPath(t, rt.path, kinds, owners)
			status, body := c.do(rt.method, path)
			tried++
			if status >= 500 {
				t.Errorf("%s %s = %d with organization B's id, want a 4xx\n%s", rt.method, path, status, trim(body))
			}
			if status != wantStatus || body != wantBody {
				t.Errorf("%s %s answers B's id differently from an unknown id:\n  B's id:     %d %s\n  unknown id: %d %s",
					rt.method, path, status, trim(body), wantStatus, trim(wantBody))
			}
			if leaked := firstContained(body, leakIDs); leaked != "" {
				t.Errorf("%s %s (%d) shows organization B's id %s", rt.method, path, status, leaked)
			}
		}
	}
	if tried < 100 {
		t.Fatalf("only %d calls with B's ids — the route walk no longer finds the routes", tried)
	}

	after := snapshotOrg(t, ctx, pool, orgB)
	for table, sum := range before {
		// Tests of other packages, run in parallel, create and drop
		// scratch tables of their own; only tables in both count.
		if got, ok := after[table]; ok && got != sum {
			t.Errorf("organization B's rows in %s changed while A called the API", table)
		}
	}
}

func paramKinds(t *testing.T, path string) (kinds []string, skip bool) {
	t.Helper()
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, seg := range segments {
		m := paramRE.FindStringSubmatch(seg)
		if m == nil {
			continue
		}
		if _, ok := notTenantResources[m[1]]; ok {
			return nil, true
		}
		if i == 0 {
			t.Fatalf("%s: a parameter with no segment before it", path)
		}
		kind, ok := segmentKinds[segments[i-1]]
		if !ok {
			t.Fatalf("%s: no resource kind for {%s} after %q — add it to segmentKinds and seedEveryKind, or to notTenantResources with a reason", path, m[1], segments[i-1])
		}
		kinds = append(kinds, kind)
	}
	return kinds, false
}

func fillPath(t *testing.T, path string, kinds []string, owners []map[string]string) string {
	t.Helper()
	i := 0
	return paramRE.ReplaceAllStringFunc(path, func(string) string {
		id, ok := owners[i][kinds[i]]
		if !ok {
			t.Fatalf("%s: nothing seeded for kind %q", path, kinds[i])
		}
		i++
		return id
	})
}

func repeat(m map[string]string, n int) []map[string]string {
	out := make([]map[string]string, n)
	for i := range out {
		out[i] = m
	}
	return out
}

func values(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func firstContained(body string, ids []string) string {
	for _, id := range ids {
		if strings.Contains(body, id) {
			return id
		}
	}
	return ""
}

func trim(s string) string {
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}

// isoClient calls the in-process router as one browser: same IP, same
// cookie jar of one.
type isoClient struct {
	t       *testing.T
	h       http.Handler
	origin  string
	ip      string
	cookies []*http.Cookie
	email   map[string]string // org id -> owner email
}

const isoPassword = "correct horse battery staple 1"

func (c *isoClient) request(method, path string, body any) (int, string, []*http.Cookie) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		r = bytes.NewReader(b)
	} else if method != http.MethodGet && method != http.MethodDelete {
		r = strings.NewReader("{}")
	}
	req := httptest.NewRequest(method, "/v1"+path, r)
	req.RemoteAddr = "[" + c.ip + "]:1234"
	req.Header.Set("Origin", c.origin)
	if r != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, ck := range c.cookies {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String(), rec.Result().Cookies()
}

func (c *isoClient) do(method, path string) (int, string) {
	status, body, _ := c.request(method, path, nil)
	return status, body
}

func (c *isoClient) signUp(pool *pgxpool.Pool) (orgID, userID string) {
	c.t.Helper()
	email := "iso-" + strings.ToLower(idgen.New()) + "@example.test"
	status, body, _ := c.request(http.MethodPost, "/auth/signup", map[string]string{
		"organizationName": "Isolation " + idgen.New(), "name": "Isolation Owner", "email": email, "password": isoPassword,
	})
	if status != http.StatusCreated {
		c.t.Fatalf("signup = %d: %s", status, body)
	}
	var resp struct {
		Organization struct{ ID string } `json:"organization"`
		User         struct{ ID string } `json:"user"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil || resp.Organization.ID == "" {
		c.t.Fatalf("signup response %s: %v", body, err)
	}
	// Verification is not what this test is about (auth's own tests are).
	if err := pool.QueryRow(context.Background(),
		`UPDATE users SET email_verified_at = now() WHERE lower(email) = lower($1) RETURNING id`, email).Scan(&userID); err != nil {
		c.t.Fatalf("verifying %s: %v", email, err)
	}
	if c.email == nil {
		c.email = map[string]string{}
	}
	c.email[resp.Organization.ID] = email
	return resp.Organization.ID, userID
}

func (c *isoClient) signIn(orgID string) {
	c.t.Helper()
	status, body, cookies := c.request(http.MethodPost, "/auth/login", map[string]string{"email": c.email[orgID], "password": isoPassword})
	if status != http.StatusOK || len(cookies) == 0 {
		c.t.Fatalf("login = %d: %s", status, body)
	}
	c.cookies = cookies
}

// snapshotOrg fingerprints every row of every table that has an
// organization_id, for one organization.
func snapshotOrg(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID string) map[string]string {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT c.table_name FROM information_schema.columns c
		JOIN information_schema.tables tb ON tb.table_schema = c.table_schema AND tb.table_name = c.table_name
		WHERE c.table_schema = 'public' AND c.column_name = 'organization_id' AND tb.table_type = 'BASE TABLE'
		ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	rows.Close()
	if len(tables) < 20 {
		t.Fatalf("found only %d tables with organization_id", len(tables))
	}
	out := map[string]string{}
	for _, table := range tables {
		var sum string
		q := fmt.Sprintf(`SELECT count(*)::text || ':' || coalesce(md5(string_agg(t::text, '|' ORDER BY t::text)), '') FROM %q t WHERE organization_id = $1`, table)
		if err := pool.QueryRow(ctx, q, orgID).Scan(&sum); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
				continue // a scratch table dropped since the listing
			}
			t.Fatalf("snapshot of %s: %v", table, err)
		}
		out[table] = sum
	}
	return out
}

// randomTestIP is a documentation-range address (2001:db8::/32) of its
// own per run, so the per-IP sign-up and API limits never carry over.
func randomTestIP() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("2001:db8:%x:%x:%x:%x::1", b[0:2], b[2:4], b[4:6], b[6:8])
}

type noMail struct{}

func (noMail) Send(context.Context, string, string, string) error { return nil }

// seedEveryKind creates one resource of every kind segmentKinds names, in
// orgID, and returns their ids by kind.
func seedEveryKind(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID, userID, tag string) map[string]string {
	t.Helper()
	ids := map[string]string{}
	must := func(what string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("seeding %s for %s: %v", what, tag, err)
		}
	}
	name := func(s string) string { return s + " " + tag + " " + idgen.New()[18:] }

	dom, err := domain.NewService(domain.NewRepository(pool), nil, nil, nil, "", nil).Create(ctx, orgID, domain.CreateInput{
		Domain: strings.ToLower(tag + "-" + idgen.New()[14:] + ".example.test"), Purpose: []string{"tracking"},
	})
	must("domain", err)
	ids["domain"] = dom.ID
	// The worker's SSL check claims never-checked domains of every
	// organization once a minute; a write of its own mid-test would read as
	// A changing B's row.
	_, err = pool.Exec(ctx, `UPDATE domains SET ssl_checked_at = now() WHERE id = $1`, dom.ID)
	must("domain ssl stamp", err)

	src, err := trafficsource.NewService(trafficsource.NewRepository(pool)).Create(ctx, orgID, trafficsource.CreateInput{
		Name: name("Source"), Type: "Facebook", TrackingTemplate: "https://track.example.com/click?clickid={click_id}",
		CostIntegration: trafficsource.CostIntegrationManual,
	})
	must("traffic source", err)
	ids["trafficSource"] = src.ID

	net, err := network.NewService(network.NewRepository(pool)).Create(ctx, orgID, network.CreateInput{
		Name: name("Network"), PostbackURL: "https://partner.example/postback?click_id={click_id}",
	})
	must("network", err)
	ids["network"] = net.Network.ID

	em, err := eventmapping.NewService(eventmapping.NewRepository(pool)).Create(ctx, orgID, eventmapping.CreateInput{
		NetworkID: net.Network.ID, NetworkStatus: "ftd", FloxStatus: event.CpaAccept,
	})
	must("event mapping", err)
	ids["eventMapping"] = em.ID

	off, err := offer.NewService(offer.NewRepository(pool)).Create(ctx, orgID, offer.CreateInput{
		NetworkID: net.Network.ID, Name: name("Offer"), Countries: []string{"US"}, Payout: 10, Currency: "USD",
		Links: []offer.LinkInput{{Label: "Main", URL: "https://offer.example.com/?click_id={click_id}"}},
	})
	must("offer", err)
	ids["offer"] = off.ID

	camp, err := campaign.NewService(campaign.NewRepository(pool)).Create(ctx, orgID, campaign.CreateInput{
		TrafficSourceID: src.ID, Name: name("Campaign"), FallbackURL: "https://example.com/fallback",
	})
	must("campaign", err)
	ids["campaign"] = camp.ID

	ss, err := streamset.NewService(streamset.NewRepository(pool)).Create(ctx, orgID, camp.ID, streamset.CreateInput{
		Name: name("Set"), Priority: 1, FallbackURL: "https://example.com/fallback",
		RootFilter: streamset.FilterNode{Kind: streamset.NodeGroup, Joiner: routing.JoinAND},
		Flows: []streamset.FlowInput{{
			Name: "Flow", Active: true, Weight: 100,
			Destination: streamset.Destination{Kind: routing.DestinationOffer, NetworkID: net.Network.ID, OfferID: off.ID},
		}},
	})
	must("stream set", err)
	ids["streamSet"] = ss.ID

	link, err := trackinglink.NewService(trackinglink.NewRepository(pool)).Create(ctx, orgID, camp.ID, trackinglink.CreateInput{
		DomainID: dom.ID, Slug: strings.ToLower("s" + idgen.New()[16:]),
	})
	must("tracking link", err)
	ids["trackingLink"] = link.ID

	entry, err := cost.NewService(cost.NewRepository(pool), conversion.NewPostgresFX(pool)).Upsert(ctx, orgID, camp.ID, cost.UpsertInput{
		EntryDate: time.Now().UTC().Truncate(24 * time.Hour), Amount: 100, Currency: "USD",
	})
	must("cost entry", err)
	ids["costEntry"] = entry.ID

	lnd, err := landing.NewService(landing.NewRepository(pool), nil, slog.New(slog.NewTextHandler(io.Discard, nil))).Create(ctx, orgID, landing.CreateInput{
		Name: name("Lander"), Title: "Promo", Type: landing.TypeExternal, URL: "https://example.com/lnd",
	})
	must("landing", err)
	ids["landing"] = lnd.ID

	app, err := pwa.NewService(pwa.NewRepository(pool)).Create(ctx, orgID, pwa.CreateInput{
		Name: name("App"), ShortName: "App", ThemeColor: "#000000", BackgroundColor: "#ffffff",
		IconURL: "https://cdn.example/i.png", StartURL: "/install",
	})
	must("pwa", err)
	ids["pwa"] = app.ID

	psl, err := postlanding.NewService(postlanding.NewRepository(pool)).Create(ctx, orgID, postlanding.CreateInput{
		Name: name("Postlanding"), URL: "https://example.com/thanks", Events: []string{"NOTIFICATION_REQUEST"},
	})
	must("postlanding", err)
	ids["postlanding"] = psl.ID

	px, err := pixel.NewService(pixel.NewRepository(pool)).Create(ctx, orgID, pixel.CreateInput{
		Name: name("Pixel"), Provider: pixel.ProviderFacebook, PixelID: "1029384756102938", Events: []string{"CPA_ACCEPT"},
	})
	must("pixel", err)
	ids["pixel"] = px.ID

	folder, err := contentgallery.NewService(contentgallery.NewRepository(pool), nil).CreateFolder(ctx, orgID, name("Folder"))
	must("content gallery folder", err)
	ids["folder"] = folder.ID

	cm, err := custommetrics.NewService(custommetrics.NewRepository(pool)).Create(ctx, orgID, userID, custommetrics.CreateInput{
		Name: name("Metric"), GroupName: "Profitability", Formula: "DIV({revenue}, {cost})",
		Format: custommetrics.FormatCurrency, Status: custommetrics.StatusDraft,
	})
	must("custom metric", err)
	ids["customMetric"] = cm.ID

	days := 7
	rp, err := reportpresets.NewService(reportpresets.NewRepository(pool)).Create(ctx, orgID, userID, reportpresets.CreateInput{
		Name: name("Preset"), Dimensions: []string{"campaign"}, Metrics: []string{"clicks"}, GroupBy: "campaign",
		Period: reportpresets.Period{Type: reportpresets.PeriodRelative, Days: &days}, Timezone: "UTC",
	})
	must("report preset", err)
	ids["reportPreset"] = rp.ID

	tg, err := tags.NewService(tags.NewRepository(pool)).Create(ctx, orgID, tags.CreateTagInput{Name: name("Tag"), Color: "green"})
	must("tag", err)
	ids["tag"] = tg.ID

	threshold := 10.0
	rule, err := alert.NewService(alert.NewRepository(pool)).CreateRule(ctx, orgID, alert.RuleInput{
		// Disabled: the worker's evaluation would otherwise fire it on the
		// seeded spend and add a notification to B mid-test.
		Kind: alert.KindSpendOver, Threshold: &threshold, Enabled: false,
	})
	must("alert rule", err)
	ids["alertRule"] = rule.ID

	key, err := apikey.NewService(apikey.NewRepository(pool), slog.New(slog.NewTextHandler(io.Discard, nil))).Create(ctx, orgID, userID, apikey.CreateInput{
		Name: name("Key"), Scope: apikey.ScopeRead,
	})
	must("api key", err)
	ids["apiKey"] = key.ApiKey.ID

	authSvc := auth.NewService(auth.NewRepository(pool), "http://localhost:3000", noMail{})
	_, err = authSvc.Invite(ctx, orgID, userID, auth.InviteInput{
		Email: "member-" + strings.ToLower(idgen.New()) + "@example.test", Name: "Member", RoleKey: "Viewer",
	})
	must("member", err)
	var memberID string
	must("member id", pool.QueryRow(ctx,
		`SELECT id FROM memberships WHERE organization_id = $1 AND user_id <> $2 LIMIT 1`, orgID, userID).Scan(&memberID))
	ids["member"] = memberID

	refSvc := referral.NewService(referral.NewRepository(pool))
	_, err = refSvc.AddTransaction(ctx, orgID, userID, referral.AddTransactionInput{Type: referral.TxnAccrual, Amount: 100, Description: "Seed"})
	must("referral accrual", err)
	payout, err := refSvc.RequestPayout(ctx, orgID, userID, 50)
	must("payout", err)
	ids["payout"] = payout.ID

	for _, kind := range segmentKinds {
		if ids[kind] == "" {
			t.Fatalf("seedEveryKind does not seed %q", kind)
		}
	}
	return ids
}
