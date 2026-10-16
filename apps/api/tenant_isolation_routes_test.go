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
	"io/fs"
	"log/slog"
	mrand "math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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
	"github.com/ismagilovnail/flox/apps/internal/httpserver"
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
	"telegram-bots":   "telegramBot",
	"channels":        "telegramChannel", // /telegram-bots/{id}/channels/{channelId}
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
	"broadcasts":      "broadcast",
	"traffic-sources": "trafficSource",
	"exports":         "export",
}

// notTenantResources are path parameters that do not name another
// organization's row.
var notTenantResources = map[string]string{
	"token":    "an invite token: public by design, it is the credential",
	"provider": "an integration provider name (facebook, namecheap…); the row is always the caller's own",
	"clickId":  "a click id is not a Postgres row; ClickHouse reads filter on the session's org (chstore TestCrossTenantIsolation)",
	"itemId":   "a system gallery item: shipped with FLOX, the same for every organization; use copies it into the caller's own (contentgallery TestUseSystemItems)",
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
	env := newIsolationEnv(t)
	c, ctx, pool, srv := env.c, env.ctx, env.pool, env.srv
	orgB, userB, idsA, idsB := env.orgB, env.userB, env.idsA, env.idsB

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
		// A fresh address per route: the calls of every route together
		// outgrow the general per-IP limit (httpserver, 300/min), and a 429
		// would read as B's id being answered differently. Same session.
		c.ip = randomTestIP()
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
			// Two successes differ by what they created (names, order); that
			// nothing of B's was stored is checked below. A refusal must
			// read the same either way.
			ok2xx := status >= 200 && status < 300
			if status != wantStatus || !ok2xx && body != wantBody {
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

// isoReplayClick is the click of each organization's seeded conversion.
const isoReplayClick = "isolation-click"

// isolationEnv is the real API with two organizations, A and B, each
// owning one resource of every kind; the client is signed in as A's owner.
type isolationEnv struct {
	c            *isoClient
	ctx          context.Context
	pool         *pgxpool.Pool
	srv          *httpserver.Server
	orgA, orgB   string
	userA, userB string
	idsA, idsB   map[string]string
}

func newIsolationEnv(t *testing.T) *isolationEnv {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	for _, k := range []string{"TOTP_ENCRYPTION_KEY", "INTEGRATION_ENCRYPTION_KEY", "ADACCOUNT_ENCRYPTION_KEY", "PIXEL_ENCRYPTION_KEY"} {
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
	// Each organization referred a third one of its own: a referral signup
	// that names neither A nor B.
	for org, ids := range map[string]map[string]string{orgA: idsA, orgB: idsB} {
		referred := idgen.New()
		if _, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, 'Referred')`, referred); err != nil {
			t.Fatalf("seeding a referred organization: %v", err)
		}
		id := idgen.New()
		if _, err := pool.Exec(ctx, `INSERT INTO referral_signups (id, organization_id, referred_organization_id, name, email)
			VALUES ($1, $2, $3, 'Referred', 'referred@example.test')`, id, org, referred); err != nil {
			t.Fatalf("seeding a referral signup: %v", err)
		}
		ids["signup"] = id
		// A successful conversion, for the outgoing postback replay.
		if _, err := pool.Exec(ctx, `INSERT INTO postbacks (id, organization_id, network_id, click_id, status, direction, result)
			VALUES ($1, $2, $3, $4, 'CPA_ACCEPT', 'incoming', 'success')`, idgen.New(), org, ids["network"], isoReplayClick); err != nil {
			t.Fatalf("seeding a conversion: %v", err)
		}
	}
	c.signIn(orgA)
	return &isolationEnv{c: c, ctx: ctx, pool: pool, srv: srv, orgA: orgA, orgB: orgB, userA: userA, userB: userB, idsA: idsA, idsB: idsB}
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

	px, err := pixel.NewService(pixel.NewRepository(pool, make([]byte, 32))).Create(ctx, orgID, pixel.CreateInput{
		Name: name("Pixel"), Provider: pixel.ProviderFacebook, PixelID: "1029384756102938", Events: []string{"CPA_ACCEPT"},
	})
	must("pixel", err)
	ids["pixel"] = px.ID

	folder, err := contentgallery.NewService(contentgallery.NewRepository(pool), nil).CreateFolder(ctx, orgID, name("Folder"))
	must("content gallery folder", err)
	ids["folder"] = folder.ID

	fileID := idgen.New()
	file, err := contentgallery.NewRepository(pool).CreateFile(ctx, fileID, orgID, contentgallery.CreateFileInput{
		UploadedByUserID: userID, FileType: contentgallery.FileTypeImage, ObjectKey: "isolation/" + fileID,
		OriginalFilename: "a.png", MimeType: "image/png", SizeBytes: 1,
	})
	must("content gallery file", err)
	ids["file"] = file.ID
	ids["user"] = userID
	ids["org"] = orgID

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

	// A finished data export (internal/orgexport): another organization
	// downloading it must get what an unknown id gets.
	exportID := idgen.New()
	_, err = pool.Exec(ctx, `INSERT INTO organization_exports (id, organization_id, requested_by, status, from_date, to_date, object_key, size_bytes, finished_at, expires_at)
		VALUES ($1, $2, $3, 'done', current_date - 7, current_date, $4, 1, now(), now() + interval '1 day')`,
		exportID, orgID, userID, "exports/"+orgID+"/"+exportID+"-isolation.zip")
	must("export", err)
	ids["export"] = exportID

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

	// A Telegram bot, written directly: connecting one calls Telegram.
	ids["telegramBot"] = idgen.New()
	_, err = pool.Exec(ctx, `INSERT INTO telegram_bots (id, organization_id, tg_bot_id, username, token_encrypted, webhook_secret_hash)
		VALUES ($1, $2, $3, 'seed_bot', 'x', 'x')`, ids["telegramBot"], orgID, mrand.Int64N(1<<40))
	must("telegram bot", err)
	ids["telegramChannel"] = idgen.New()
	_, err = pool.Exec(ctx, `INSERT INTO telegram_channels (id, bot_id, organization_id, tg_chat_id, title, invite_link)
		VALUES ($1, $2, $3, $4, 'Seed', 'https://t.me/seedchan')`, ids["telegramChannel"], ids["telegramBot"], orgID, -mrand.Int64N(1<<40))
	must("telegram channel", err)

	// A push subscriber and a broadcast, written directly: a broadcast
	// sent through the service would queue deliveries the dev worker sends.
	ids["broadcast"] = idgen.New()
	_, err = pool.Exec(ctx, `INSERT INTO push_subscriptions (id, organization_id, pwa_id, campaign_id, endpoint, p256dh, auth, link_token, origin)
		VALUES ($1, $2, $3, $4, $5, 'k', 'a', 't', 'https://t.example')`,
		idgen.New(), orgID, ids["pwa"], ids["campaign"], "https://push.invalid/"+idgen.New())
	must("push subscription", err)
	_, err = pool.Exec(ctx, `INSERT INTO push_broadcasts (id, organization_id, name, title, target, total) VALUES ($1, $2, $3, 'Hi', 'destination', 1)`,
		ids["broadcast"], orgID, name("Broadcast"))
	must("push broadcast", err)

	for _, kind := range segmentKinds {
		if ids[kind] == "" {
			t.Fatalf("seedEveryKind does not seed %q", kind)
		}
	}
	return ids
}

// refCase is one request that names other resources in its body or query.
// Its path may hold {kind} placeholders, always filled with A's ids; each
// field in fields is a reference to a resource of the given kind.
type refCase struct {
	method, path string
	fields       map[string]string // field -> kind
	query        bool              // fields are query parameters, not body fields
	body         func(ref func(field string) string) any
}

// referenceCases covers every field that refers to another resource. They
// run in order: reorder before more stream sets exist, deletes last.
func referenceCases(env *isolationEnv) []refCase {
	uniq := func(s string) string { return s + " " + idgen.New()[16:] }
	today := time.Now().UTC().Format("2006-01-02")
	flows := func(ref func(string) string) []map[string]any {
		return []map[string]any{{
			"name": "Flow", "active": true, "weight": 100,
			"landing":     map[string]any{"enabled": true, "landingId": ref("landingId")},
			"pwa":         map[string]any{"enabled": true, "pwaId": ref("pwaId"), "pwaType": "internal"},
			"postlanding": map[string]any{"enabled": true, "postlandingId": ref("postlandingId")},
			"destination": map[string]any{"kind": "offer", "networkId": ref("networkId"), "offerId": ref("offerId")},
		}}
	}
	flowFields := map[string]string{
		"landingId": "landing", "pwaId": "pwa", "postlandingId": "postlanding",
		"networkId": "network", "offerId": "offer", "pixelIds": "pixel",
	}
	return []refCase{
		{http.MethodPost, "/campaigns/{campaign}/stream-sets/reorder", map[string]string{"orderedIds": "streamSet"}, false,
			func(ref func(string) string) any { return map[string]any{"orderedIds": []string{ref("orderedIds")}} }},
		{http.MethodPost, "/campaigns", map[string]string{"trafficSourceId": "trafficSource"}, false,
			func(ref func(string) string) any {
				return map[string]any{"trafficSourceId": ref("trafficSourceId"), "name": uniq("Campaign"), "fallbackUrl": "https://example.com/f"}
			}},
		{http.MethodPatch, "/campaigns/{campaign}", map[string]string{"trafficSourceId": "trafficSource"}, false,
			func(ref func(string) string) any { return map[string]any{"trafficSourceId": ref("trafficSourceId")} }},
		{http.MethodPost, "/campaigns/{campaign}/tracking-links", map[string]string{"domainId": "domain"}, false,
			func(ref func(string) string) any {
				return map[string]any{"domainId": ref("domainId"), "slug": strings.ToLower("r" + idgen.New()[16:])}
			}},
		{http.MethodPost, "/campaigns/{campaign}/cost-entries", map[string]string{"trafficSourceId": "trafficSource"}, false,
			func(ref func(string) string) any {
				return map[string]any{"trafficSourceId": ref("trafficSourceId"), "entryDate": today, "amount": 5, "currency": "USD"}
			}},
		{http.MethodPost, "/campaigns/{campaign}/cost-entries/import", map[string]string{"trafficSourceId": "trafficSource"}, false,
			func(ref func(string) string) any {
				return map[string]any{"csv": "date,amount,currency,traffic_source_id\n" + today + ",5,USD," + ref("trafficSourceId") + "\n"}
			}},
		{http.MethodPost, "/event-mappings", map[string]string{"networkId": "network"}, false,
			func(ref func(string) string) any {
				return map[string]any{"networkId": ref("networkId"), "networkStatus": strings.ToLower(idgen.New()[16:]), "floxStatus": "CPA_ACCEPT"}
			}},
		{http.MethodPost, "/offers", map[string]string{"networkId": "network"}, false,
			func(ref func(string) string) any {
				return map[string]any{"networkId": ref("networkId"), "name": uniq("Offer"), "countries": []string{"US"}, "payout": 5, "currency": "USD",
					"links": []map[string]string{{"label": "Main", "url": "https://offer.example.com/?c={click_id}"}}}
			}},
		{http.MethodPatch, "/offers/{offer}", map[string]string{"networkId": "network"}, false,
			func(ref func(string) string) any { return map[string]any{"networkId": ref("networkId")} }},
		{http.MethodPost, "/alert-rules", map[string]string{"campaignId": "campaign"}, false,
			func(ref func(string) string) any {
				return map[string]any{"kind": "spend_over", "campaignId": ref("campaignId"), "threshold": 10, "enabled": false}
			}},
		{http.MethodPut, "/alert-rules/{alertRule}", map[string]string{"campaignId": "campaign"}, false,
			func(ref func(string) string) any {
				return map[string]any{"kind": "spend_over", "campaignId": ref("campaignId"), "threshold": 10, "enabled": false}
			}},
		{http.MethodPost, "/campaigns/{campaign}/stream-sets", flowFields, false,
			func(ref func(string) string) any {
				return map[string]any{"name": uniq("Set"), "fallbackUrl": "https://example.com/f",
					"rootFilter": map[string]any{"type": "group", "joiner": "AND", "children": []any{}},
					"flows":      flows(ref), "pixelIds": []string{ref("pixelIds")}}
			}},
		{http.MethodPatch, "/campaigns/{campaign}/stream-sets/{streamSet}", flowFields, false,
			func(ref func(string) string) any {
				return map[string]any{"flows": flows(ref), "pixelIds": []string{ref("pixelIds")}}
			}},
		{http.MethodPost, "/tags/assignments/set", map[string]string{"entityId": "campaign", "tagIds": "tag"}, false,
			func(ref func(string) string) any {
				return map[string]any{"entityType": "campaign", "entityId": ref("entityId"), "tagIds": []string{ref("tagIds")}}
			}},
		{http.MethodPost, "/tags/assignments/bulk", map[string]string{"entityIds": "campaign", "toAdd": "tag", "toRemove": "tag"}, false,
			func(ref func(string) string) any {
				return map[string]any{"entityType": "campaign", "entityIds": []string{ref("entityIds")},
					"toAdd": []string{ref("toAdd")}, "toRemove": []string{ref("toRemove")}}
			}},
		{http.MethodPost, "/content-gallery/files/move", map[string]string{"ids": "file", "folderId": "folder"}, false,
			func(ref func(string) string) any {
				return map[string]any{"ids": []string{ref("ids")}, "folderId": ref("folderId")}
			}},
		{http.MethodPost, "/content-gallery/files/archive", map[string]string{"ids": "file"}, false,
			func(ref func(string) string) any { return map[string]any{"ids": []string{ref("ids")}} }},
		{http.MethodPost, "/content-gallery/files/restore", map[string]string{"ids": "file"}, false,
			func(ref func(string) string) any { return map[string]any{"ids": []string{ref("ids")}} }},
		{http.MethodGet, "/content-gallery/files", map[string]string{"folderId": "folder", "uploadedBy": "user"}, true, nil},
		{http.MethodGet, "/analytics/ltv/ftd-cohorts", map[string]string{"campaignId": "campaign", "sourceId": "trafficSource", "offerId": "offer", "networkId": "network"}, true, nil},
		{http.MethodGet, "/analytics/ltv/reg-cohorts", map[string]string{"campaignId": "campaign", "sourceId": "trafficSource", "offerId": "offer", "networkId": "network"}, true, nil},
		{http.MethodPost, "/postback-logs/replay-incoming", map[string]string{"networkId": "network"}, false,
			func(ref func(string) string) any {
				return map[string]any{"networkId": ref("networkId"), "clickId": idgen.New(), "rawStatus": "ftd"}
			}},
		{http.MethodPost, "/postback-logs/check", map[string]string{"networkId": "network"}, false,
			func(ref func(string) string) any {
				return map[string]any{"url": "https://t.example/postback/" + ref("networkId") + "?click_id=x&status=ftd"}
			}},
		{http.MethodPost, "/postback-logs/replay-outgoing", map[string]string{"networkId": "network"}, false,
			func(ref func(string) string) any {
				return map[string]any{"networkId": ref("networkId"), "clickId": isoReplayClick, "status": "CPA_ACCEPT",
					"url": "https://partner.example/postback?click_id=x"}
			}},
		{http.MethodPost, "/referral/transactions", map[string]string{"relatedSignupId": "signup"}, false,
			func(ref func(string) string) any {
				return map[string]any{"type": "accrual", "amount": 1, "description": "Isolation", "relatedSignupId": ref("relatedSignupId")}
			}},
		{http.MethodPost, "/auth/login", map[string]string{"organizationId": "org"}, false,
			func(ref func(string) string) any {
				return map[string]any{"email": env.c.email[env.orgA], "password": isoPassword, "organizationId": ref("organizationId")}
			}},
		{http.MethodPost, "/push/audience", map[string]string{"pwaIds": "pwa", "campaignIds": "campaign"}, false,
			func(ref func(string) string) any {
				return map[string]any{"pwaIds": []string{ref("pwaIds")}, "campaignIds": []string{ref("campaignIds")}}
			}},
		{http.MethodPost, "/content-gallery/files/delete", map[string]string{"ids": "file"}, false,
			func(ref func(string) string) any { return map[string]any{"ids": []string{ref("ids")}} }},
	}
}

// notReferences are request or response JSON fields named like ids that do
// not refer to another organization's row. A new id-named field fails
// TestEveryReferenceRefusesAnotherOrganizationsIDs until it is covered by
// a referenceCases entry or listed here with a reason.
var notReferences = map[string]string{
	"adAccountId":                  "the ad platform's own account id",
	"tgChatId":                     "response only: Telegram's id of a channel",
	"telegramChannelId":            "event payload only: set by the tracker's webhook from the bot's own channel, never read from a request",
	"clientId":                     "the ad platform's OAuth client id",
	"pixelId":                      "the ad platform's pixel id",
	"sourceEventId":                "pixel.Event, a queue payload FLOX writes itself — never read from a request",
	"externalCampaignId":           "the traffic source's own campaign id",
	"externalClickId":              "the traffic source's click id, a string, not a row",
	"fbClickId":                    "Facebook's click id",
	"ttClickId":                    "TikTok's click id",
	"networkTxnId":                 "the network's transaction id",
	"clickId":                      "a click id: ClickHouse reads filter on the session's org (chstore TestCrossTenantIsolation)",
	"clickIds":                     "click ids to erase: every delete filters on the session's org (visitordata TestFindAndEraseAVisitor)",
	"tgUserId":                     "response only: Telegram's id of a user",
	"unmatchedExternalCampaignIds": "response only: platform campaign ids a sync could not match",
	"createdByUserId":              "response only",
	"uploadedByUserId":             "response only; the list filter is uploadedBy, covered",
	"requestedByUserId":            "response only",
	"resolvedByUserId":             "response only",
	"referredOrganizationId":       "response only",
	"relatedPayoutId":              "response only",
	"replacedById":                 "response only: the server sets it when rotating a key (TestRotateRefusesRevokedAndOtherOrganizationsKeys)",
	"deliveryId":                   "response only",
	"parentId":                     "response only: the worker sets it on a recurring broadcast's sends (TestRecurringBroadcast)",
	"ruleId":                       "response only",
	"tagId":                        "response only; assignments take tagIds, covered",
	"userId":                       "response only (team members, auth/me)",
	"streamSetId":                  "response only (routing trace, events)",
	"flowId":                       "response only (routing trace, events)",
	"flowIds":                      "flow-summary filter: ClickHouse reads filter on the session's org, another org's flow is zeros like a made-up id (chstore TestFlowStageCountsPerFlowAndTenant)",
}

var idFieldRE = regexp.MustCompile(`json:"([a-zA-Z]*(?:Id|Ids|IDs))(?:,[a-z]+)?"`)

// Tech audit 2026-10-06, §16, the part the path walk cannot see: ids of
// organization B inside a request body or query string. For every field
// that refers to another resource, the request first succeeds with A's own
// id (so the body is valid and the check is not vacuous), then is sent with
// B's id and with an id that exists nowhere; the two answers must be the
// same. Afterwards none of A's rows may refer to any of B's, and none of
// B's rows may have changed.
func TestEveryReferenceRefusesAnotherOrganizationsIDs(t *testing.T) {
	cases := referenceCases(nil)
	covered := map[string]bool{}
	for _, rc := range cases {
		for f := range rc.fields {
			covered[f] = true
		}
	}
	assertEveryIDFieldCovered(t, covered)

	env := newIsolationEnv(t)
	c, ctx, pool := env.c, env.ctx, env.pool
	before := snapshotOrg(t, ctx, pool, env.orgB)
	cases = referenceCases(env)

	placeholder := regexp.MustCompile(`\{([a-zA-Z]+)\}`)
	bIDs := map[string]bool{env.orgB: true, env.userB: true}
	for _, id := range env.idsB {
		bIDs[id] = true
	}
	known := map[string]bool{}
	for _, id := range env.idsA {
		known[id] = true
	}
	known[env.orgA], known[env.userA] = true, true
	for _, rc := range cases {
		path := placeholder.ReplaceAllStringFunc(rc.path, func(m string) string {
			return env.idsA[strings.Trim(m, "{}")]
		})
		send := func(field, id string) (int, string) {
			ref := func(f string) string {
				if f == field {
					return id
				}
				return env.idsA[rc.fields[f]]
			}
			if rc.query {
				q := url.Values{}
				for f := range rc.fields {
					q.Set(f, ref(f))
				}
				status, body, _ := c.request(rc.method, path+"?"+q.Encode(), nil)
				return status, body
			}
			status, body, _ := c.request(rc.method, path, rc.body(ref))
			return status, body
		}
		name := rc.method + " " + rc.path
		if status, body := send("", ""); status < 200 || status >= 300 {
			t.Errorf("%s with A's own ids = %d, want 2xx — the case no longer builds a valid request\n%s", name, status, trim(body))
			continue
		}
		fields := make([]string, 0, len(rc.fields))
		for f := range rc.fields {
			fields = append(fields, f)
		}
		sort.Strings(fields)
		for _, field := range fields {
			bID := env.idsB[rc.fields[field]]
			unknown := idgen.New()
			wantStatus, wantBody := send(field, unknown)
			refsBefore := rowsReferring(t, ctx, pool, env.orgA, bIDs)
			status, body := send(field, bID)
			// The id sent may be echoed back (a validation message names
			// it); any other id of B's in the answer is a leak.
			echoed := strings.ReplaceAll(body, bID, "<id>")
			wantBody = normalize(strings.ReplaceAll(wantBody, unknown, "<id>"), known)
			body = normalize(echoed, known)
			if status >= 500 {
				t.Errorf("%s with B's %s in %s = %d, want a 4xx\n%s", name, rc.fields[field], field, status, trim(body))
			}
			// Two successes differ by what they created (names, order); that
			// nothing of B's was stored is checked below. A refusal must
			// read the same either way.
			ok2xx := status >= 200 && status < 300
			if status != wantStatus || !ok2xx && body != wantBody {
				t.Errorf("%s answers B's %s in %s differently from an unknown id:\n  B's id:     %d %s\n  unknown id: %d %s",
					name, rc.fields[field], field, status, trim(body), wantStatus, trim(wantBody))
			}
			if leaked := firstContained(echoed, append([]string{env.orgB, env.userB}, values(env.idsB)...)); leaked != "" {
				t.Errorf("%s with B's %s in %s shows organization B's id %s", name, rc.fields[field], field, leaked)
			}
			// Checked after every call: a later request may overwrite
			// what this one stored.
			for row := range rowsReferring(t, ctx, pool, env.orgA, bIDs) {
				if !refsBefore[row] {
					t.Errorf("%s with B's %s in %s stored a reference to B in A's data: %s", name, rc.fields[field], field, trim(row))
				}
			}
		}
	}

	after := snapshotOrg(t, ctx, pool, env.orgB)
	for table, sum := range before {
		if got, ok := after[table]; ok && got != sum {
			t.Errorf("organization B's rows in %s changed while A called the API", table)
		}
	}
	for row := range rowsReferring(t, ctx, pool, env.orgA, bIDs) {
		t.Errorf("organization A's data refers to B: %s", trim(row))
	}
}

var (
	ulidRE      = regexp.MustCompile(`\b[0-9A-HJKMNP-TV-Z]{26}\b`)
	timestampRE = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})`)
)

// normalize hides what differs between two otherwise equal answers: ids
// minted by the request itself and timestamps. Seeded ids stay visible.
func normalize(body string, known map[string]bool) string {
	body = ulidRE.ReplaceAllStringFunc(body, func(id string) string {
		if known[id] {
			return id
		}
		return "<new>"
	})
	return timestampRE.ReplaceAllString(body, "<time>")
}

// rowsReferring is every row of orgID, as "table: row", that contains any
// of ids.
func rowsReferring(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID string, ids map[string]bool) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for table, rows := range orgRows(t, ctx, pool, orgID) {
		for _, row := range rows {
			for id := range ids {
				if strings.Contains(row, id) {
					out[table+": "+row] = true
					break
				}
			}
		}
	}
	return out
}

// assertEveryIDFieldCovered reads every JSON tag named like an id in the
// non-test sources under internal/.
func assertEveryIDFieldCovered(t *testing.T, covered map[string]bool) {
	t.Helper()
	var missing []string
	seen := map[string]bool{}
	err := filepath.WalkDir("../internal", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		if strings.Contains(path, string(filepath.Separator)+"chstore"+string(filepath.Separator)) {
			return nil // ClickHouse rows, read by org (chstore TestCrossTenantIsolation)
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range idFieldRE.FindAllStringSubmatch(string(src), -1) {
			name := m[1]
			if seen[name] {
				continue
			}
			seen[name] = true
			if !covered[name] && notReferences[name] == "" {
				missing = append(missing, name+" ("+path+")")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) < 20 {
		t.Fatalf("found only %d id-named JSON fields — the scan no longer matches", len(seen))
	}
	if len(missing) > 0 {
		t.Fatalf("id-named JSON fields with no isolation case: %v\nadd a referenceCases entry, or list the field in notReferences with a reason", missing)
	}
}

// orgRows is every row, as text, of every table with an organization_id,
// for one organization.
func orgRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID string) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for table := range snapshotOrg(t, ctx, pool, orgID) {
		rows, err := pool.Query(ctx, fmt.Sprintf(`SELECT t::text FROM %q t WHERE organization_id = $1`, table), orgID)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
				continue
			}
			t.Fatalf("reading %s: %v", table, err)
		}
		for rows.Next() {
			var row string
			if err := rows.Scan(&row); err != nil {
				t.Fatal(err)
			}
			out[table] = append(out[table], row)
		}
		rows.Close()
	}
	return out
}
