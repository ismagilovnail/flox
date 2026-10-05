package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/campaign"
	"github.com/ismagilovnail/flox/apps/internal/classifier"
	"github.com/ismagilovnail/flox/apps/internal/clientip"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/eventbuf"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/landing"
	"github.com/ismagilovnail/flox/apps/internal/landinglink"
	"github.com/ismagilovnail/flox/apps/internal/routing"
	"github.com/ismagilovnail/flox/apps/internal/routingstore"
	"github.com/ismagilovnail/flox/apps/internal/storage"
	"github.com/ismagilovnail/flox/apps/internal/streamset"
	"github.com/ismagilovnail/flox/apps/internal/trafficsource"
)

const (
	landingTestOffer = "https://offer.example/go?x=1"
	landingTestPage  = `<!DOCTYPE html><html><head><title>Lander</title></head><body><a href="#">Go</a><a href="{{ trackUrl }}">Macro</a></body></html>`
)

var testLinkKey = []byte("0123456789abcdef0123456789abcdef")

// captureSink keeps every event the Writer flushes, so a test can assert
// on exactly what the hot path recorded.
type captureSink struct {
	mu     sync.Mutex
	events []event.Event
}

func (s *captureSink) Write(_ context.Context, batch []event.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, batch...)
	return nil
}

type landingFixture struct {
	host, slug, orgID, landingID string
	pool                         *pgxpool.Pool
}

// seedLandingFixture: one active campaign, one catch-all stream set, one
// flow whose landing stage points at a landing of the given type, and a
// redirect destination of landingTestOffer.
func seedLandingFixture(tb testing.TB, pool *pgxpool.Pool, lnd landing.Landing) landingFixture {
	tb.Helper()
	ctx := context.Background()

	orgID := idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, orgID, "Landing Org "+orgID); err != nil {
		tb.Fatalf("seeding organization: %v", err)
	}
	tb.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID)
	})

	lnd.ID = idgen.New()
	lnd.OrganizationID = orgID
	if lnd.Name == "" {
		lnd.Name = "Fixture Landing"
	}
	if _, err := landing.NewRepository(pool).Create(ctx, lnd); err != nil {
		tb.Fatalf("seeding landing: %v", err)
	}

	ts, err := trafficsource.NewService(trafficsource.NewRepository(pool)).Create(ctx, orgID, trafficsource.CreateInput{
		Name: "Landing Source", Type: "Facebook", CostIntegration: trafficsource.CostIntegrationNone,
		TrackingTemplate: "https://track.example.com/click?clickid={click_id}",
	})
	if err != nil {
		tb.Fatalf("seeding traffic source: %v", err)
	}
	campSvc := campaign.NewService(campaign.NewRepository(pool))
	camp, err := campSvc.Create(ctx, orgID, campaign.CreateInput{TrafficSourceID: ts.ID, Name: "Landing Campaign", FallbackURL: "https://fallback.example"})
	if err != nil {
		tb.Fatalf("seeding campaign: %v", err)
	}
	if _, err := campSvc.Activate(ctx, orgID, camp.ID); err != nil {
		tb.Fatalf("activating campaign: %v", err)
	}
	if _, err := streamset.NewService(streamset.NewRepository(pool)).Create(ctx, orgID, camp.ID, streamset.CreateInput{
		Name: "All Traffic", Priority: 1,
		RootFilter: streamset.FilterNode{Kind: streamset.NodeGroup, Joiner: routing.JoinAND},
		Flows: []streamset.FlowInput{{
			Name: "With Landing", Active: true, Weight: 100,
			Landing:     streamset.FlowLanding{Enabled: true, LandingID: lnd.ID},
			Destination: streamset.Destination{Kind: routing.DestinationRedirect, URL: landingTestOffer},
		}},
	}); err != nil {
		tb.Fatalf("seeding stream set: %v", err)
	}

	host := "lnd-" + strings.ToLower(idgen.New()) + ".example"
	domainID := idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO domains (id, organization_id, domain, status, purpose) VALUES ($1, $2, $3, 'active', '{tracking}')`,
		domainID, orgID, host); err != nil {
		tb.Fatalf("seeding domain: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tracking_links (id, organization_id, campaign_id, domain_id, slug) VALUES ($1, $2, $3, $4, 'go')`,
		idgen.New(), orgID, camp.ID, domainID); err != nil {
		tb.Fatalf("seeding tracking link: %v", err)
	}
	return landingFixture{host: host, slug: "go", orgID: orgID, landingID: lnd.ID, pool: pool}
}

func internalLanding(status landing.Status) landing.Landing {
	return landing.Landing{
		Type: landing.TypeInternal, Title: "T", LogoURL: "https://example.com/l.png",
		Content: landingTestPage, EntryFile: "index.html", BundlePrefix: "landings/o/l/v1", Status: status,
	}
}

func newLandingHandler(tb testing.TB, pool *pgxpool.Pool, withLinks bool) (http.Handler, *captureSink, func()) {
	tb.Helper()
	sink := &captureSink{}
	events := eventbuf.New(sink, slog.New(slog.NewTextHandler(io.Discard, nil)), eventbuf.Config{})
	assets, err := storage.NewClient(config.S3Config{Endpoint: "http://minio:9000", PublicEndpoint: "https://cdn.example", Bucket: "flox"})
	if err != nil {
		tb.Fatalf("storage client: %v", err)
	}
	resolver, err := clientip.New(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		tb.Fatalf("clientip: %v", err)
	}
	h := &Handler{
		clientIP:   resolver,
		store:      routingstore.New(pool),
		classifier: classifier.New(nil, nil, nil),
		engine:     &routing.Engine{},
		events:     events,
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		assets:     assets,
	}
	if withLinks {
		h.links = landinglink.NewSigner(testLinkKey)
	}
	r := chi.NewRouter()
	h.Register(r)
	return r, sink, events.Close
}

func get(h http.Handler, host, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "http://"+host+path, nil)
	req.Host = host
	req.RemoteAddr = "198.51.100.7:12345"
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 14) Chrome/126.0 Mobile")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

var trackURLInPage = regexp.MustCompile(`var trackUrl = "([^"]+)";`)

func TestTrackServesInternalLandingThenClickThrough(t *testing.T) {
	pool := testPool(t)
	fx := seedLandingFixture(t, pool, internalLanding(landing.StatusActive))
	h, sink, closeEvents := newLandingHandler(t, pool, true)

	rec := get(h, fx.host, "/t/go?utm_source=fb&sub1=adset7&sub3=x")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (landing served):\n%s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q", ct)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("served landing must not be cacheable")
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<base href="https://cdn.example/flox/landings/o/l/v1/">`) {
		t.Fatalf("no <base href> to the bundle:\n%s", body)
	}
	m := trackURLInPage.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no trackUrl in page:\n%s", body)
	}
	trackURL := m[1]
	prefix := "http://" + fx.host + "/lc/"
	if !strings.HasPrefix(trackURL, prefix) {
		t.Fatalf("trackUrl = %q, want an absolute link on the tracking host", trackURL)
	}
	if !strings.Contains(body, `<a href="`+trackURL+`">Macro</a>`) {
		t.Fatal("{{ trackUrl }} macro not substituted with the click link")
	}

	click := get(h, fx.host, strings.TrimPrefix(trackURL, "http://"+fx.host))
	if click.Code != http.StatusFound || click.Header().Get("Location") != landingTestOffer {
		t.Fatalf("click-through = %d → %q, want 302 → %s", click.Code, click.Header().Get("Location"), landingTestOffer)
	}

	closeEvents()
	types := map[event.Type]event.Event{}
	for _, e := range sink.events {
		types[e.Type] = e
	}
	src, view, lc := types[event.SourceClick], types[event.LandView], types[event.LandClick]
	if src.ClickID == "" || view.ClickID != src.ClickID || lc.ClickID != src.ClickID {
		t.Fatalf("want SOURCE_CLICK, LAND_VIEW, LAND_CLICK on one click_id, got %+v", sink.events)
	}
	if lc.OrganizationID != fx.orgID || lc.FlowID != src.FlowID || lc.Destination != landingTestOffer {
		t.Fatalf("LAND_CLICK lost routing context: %+v", lc)
	}
	if lc.UTMSource != "fb" || lc.Subs[0] != "adset7" || lc.Subs[2] != "x" {
		t.Fatalf("LAND_CLICK lost attribution params: utm=%q subs=%v", lc.UTMSource, lc.Subs)
	}
}

func TestTrackLandingStageFallbacks(t *testing.T) {
	pool := testPool(t)

	t.Run("paused landing is skipped", func(t *testing.T) {
		fx := seedLandingFixture(t, pool, internalLanding(landing.StatusPaused))
		h, sink, closeEvents := newLandingHandler(t, pool, true)
		assertRedirect(t, get(h, fx.host, "/t/go"), landingTestOffer)
		closeEvents()
		assertNoLandView(t, sink)
	})

	t.Run("archived landing is still served", func(t *testing.T) {
		fx := seedLandingFixture(t, pool, internalLanding(landing.StatusArchived))
		h, _, closeEvents := newLandingHandler(t, pool, true)
		defer closeEvents()
		if rec := get(h, fx.host, "/t/go"); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("no signing key skips internal landings", func(t *testing.T) {
		fx := seedLandingFixture(t, pool, internalLanding(landing.StatusActive))
		h, sink, closeEvents := newLandingHandler(t, pool, false)
		assertRedirect(t, get(h, fx.host, "/t/go"), landingTestOffer)
		closeEvents()
		assertNoLandView(t, sink)
	})

	t.Run("external landing redirects to its URL", func(t *testing.T) {
		fx := seedLandingFixture(t, pool, landing.Landing{Type: landing.TypeExternal, URL: "https://advertiser.example/lp", Status: landing.StatusActive})
		h, _, closeEvents := newLandingHandler(t, pool, true)
		defer closeEvents()
		assertRedirect(t, get(h, fx.host, "/t/go"), "https://advertiser.example/lp")
	})
}

func TestLandClickRejectsBadTokens(t *testing.T) {
	pool := testPool(t)
	h, sink, closeEvents := newLandingHandler(t, pool, true)

	forged := landinglink.NewSigner([]byte("ffffffffffffffffffffffffffffffff")).Sign(landinglink.Claims{
		OrganizationID: "o", ClickID: "k", Destination: "https://evil.example", IssuedAt: 1 << 31,
	})
	for _, path := range []string{"/lc/garbage", "/lc/a.b", "/lc/" + forged} {
		if rec := get(h, "any.example", path); rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want 404 (no open redirect)", path, rec.Code)
		}
	}
	closeEvents()
	if len(sink.events) != 0 {
		t.Fatalf("bad tokens recorded events: %+v", sink.events)
	}
}

// TestLoadLandingPageIsOrgScoped: the tracker never reads another
// organization's landing, even given its id.
func TestLoadLandingPageIsOrgScoped(t *testing.T) {
	pool := testPool(t)
	fx := seedLandingFixture(t, pool, internalLanding(landing.StatusActive))
	other := seedLandingFixture(t, pool, internalLanding(landing.StatusActive))

	store := routingstore.New(pool)
	if _, err := store.LoadLandingPage(context.Background(), other.orgID, fx.landingID); err == nil {
		t.Fatal("org B loaded org A's landing page")
	}
	if p, err := store.LoadLandingPage(context.Background(), fx.orgID, fx.landingID); err != nil || p.Content != landingTestPage {
		t.Fatalf("own landing: %+v, %v", p, err)
	}
}

func assertRedirect(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != want {
		t.Fatalf("got %d → %q, want 302 → %s", rec.Code, rec.Header().Get("Location"), want)
	}
}

func assertNoLandView(t *testing.T, sink *captureSink) {
	t.Helper()
	for _, e := range sink.events {
		if e.Type == event.LandView {
			t.Fatal("LAND_VIEW recorded though no landing was served")
		}
	}
}

func testPool(t *testing.T) *pgxpool.Pool {
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
