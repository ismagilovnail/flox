package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/landing"
	"github.com/ismagilovnail/flox/apps/internal/landinglink"
	"github.com/ismagilovnail/flox/apps/internal/pwa"
	"github.com/ismagilovnail/flox/apps/internal/routingstore"
	"github.com/ismagilovnail/flox/apps/internal/streamset"
)

func seedPwa(t *testing.T, pool *pgxpool.Pool, orgID string, bounce bool) string {
	t.Helper()
	p, err := pwa.NewService(pwa.NewRepository(pool)).Create(context.Background(), orgID, pwa.CreateInput{
		Name: "Lucky App", ShortName: "Lucky", ThemeColor: "#16a34a", BackgroundColor: "#ffffff",
		IconURL: "https://cdn.example/icon.png", StartURL: "/install", BounceInAppWebview: bounce,
		Developer: "Lucky Dev", Description: "Spin daily.", Screenshots: []string{"https://cdn.example/s1.png"},
		Rating: ptr(4.9), RatingCount: ptrInt(120000), DownloadsLabel: "1M+",
		RatingStars5: ptr(87.5), AgeRating: "12+ Teen", WhatsNew: "Bug fixes and a new wheel.", Version: "2.4.1",
		Category: "Casual", ContainsAds: true,
	})
	if err != nil {
		t.Fatalf("seeding pwa: %v", err)
	}
	return p.ID
}

func ptr(f float64) *float64 { return &f }
func ptrInt(i int) *int      { return &i }

func withPwa(t *testing.T, pool *pgxpool.Pool, pwaID *string, pwaType streamset.PwaType, bounce bool) func(string, *streamset.FlowInput) {
	return func(orgID string, f *streamset.FlowInput) {
		*pwaID = seedPwa(t, pool, orgID, bounce)
		f.Pwa = streamset.FlowPwa{Enabled: true, PwaID: *pwaID, PwaType: pwaType}
	}
}

func post(h http.Handler, host, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "http://"+host+path, nil)
	req.Host = host
	req.RemoteAddr = "198.51.100.7:12345"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestPwaStageDirect walks the whole PWA funnel with no landing: click →
// install page (manifest, service worker, no store imitation) → installed
// beacon → app launch → destination, every event on one click_id with
// pwa_id set.
func TestPwaStageDirect(t *testing.T) {
	pool := testPool(t)
	var pwaID string
	fx := seedFlowFixture(t, pool, nil, withPwa(t, pool, &pwaID, streamset.PwaTypeInternal, false))
	h, sink, closeEvents := newLandingHandler(t, pool, true)

	click := get(h, fx.host, "/t/go?sub1=a")
	loc := click.Header().Get("Location")
	if click.Code != http.StatusFound || !strings.HasPrefix(loc, "http://"+fx.host+"/pwa/") || !strings.HasSuffix(loc, "/") {
		t.Fatalf("click = %d → %q, want 302 to the PWA page", click.Code, loc)
	}
	base := strings.TrimPrefix(loc, "http://"+fx.host)

	page := get(h, fx.host, base)
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, `<link rel="manifest" href="manifest.webmanifest">`) || !strings.Contains(body, "Lucky App") || !strings.Contains(body, "Lucky Dev") {
		t.Fatalf("install page = %d:\n%s", page.Code, body)
	}
	// The visitor-facing page never imitates a store or shows its listing stats.
	for _, banned := range []string{"Google Play", "4.9", "120000", "1M+", "87.5", "12+ Teen"} {
		if strings.Contains(body, banned) {
			t.Fatalf("install page contains %q — store imitation / listing stats must never reach visitors", banned)
		}
	}

	// What the editor preview shows besides the numbers does reach the visitor.
	for _, want := range []string{"Bug fixes and a new wheel.", "2.4.1", "Casual", "Contains ads", "install", `id="sheet"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("install page lacks %q", want)
		}
	}

	var manifest map[string]any
	m := get(h, fx.host, base+"manifest.webmanifest")
	if err := json.Unmarshal(m.Body.Bytes(), &manifest); err != nil || m.Header().Get("Content-Type") != "application/manifest+json" {
		t.Fatalf("manifest: %v / %q", err, m.Header().Get("Content-Type"))
	}
	if manifest["start_url"] != base+"open" || manifest["scope"] != base || manifest["display"] != "standalone" {
		t.Fatalf("manifest = %v", manifest)
	}
	if sw := get(h, fx.host, base+"sw.js"); sw.Code != http.StatusOK || !strings.Contains(sw.Body.String(), `addEventListener("fetch"`) {
		t.Fatalf("service worker = %d", sw.Code)
	}

	if rec := post(h, fx.host, base+"installed"); rec.Code != http.StatusNoContent {
		t.Fatalf("installed beacon = %d", rec.Code)
	}
	assertRedirect(t, get(h, fx.host, base+"open"), landingTestOffer)
	assertRedirect(t, get(h, fx.host, base+"continue"), landingTestOffer)

	closeEvents()
	got := map[event.Type]event.Event{}
	for _, e := range sink.events {
		got[e.Type] = e
	}
	src := got[event.SourceClick]
	for _, typ := range []event.Type{event.SourceClick, event.PwaView, event.PwaInstall, event.PwaOpen} {
		e, ok := got[typ]
		if !ok {
			t.Fatalf("missing %s in %+v", typ, sink.events)
		}
		if e.ClickID != src.ClickID || e.PwaID != pwaID || e.OrganizationID != fx.orgID {
			t.Fatalf("%s = click %q pwa %q, want click %q pwa %q", typ, e.ClickID, e.PwaID, src.ClickID, pwaID)
		}
	}
	if len(sink.events) != 4 {
		t.Fatalf("want exactly SOURCE_CLICK, PWA_VIEW, PWA_INSTALL, PWA_OPEN (continue records nothing), got %d", len(sink.events))
	}
}

// TestPwaAfterLanding: landing click-through leads to the PWA page, not
// the destination.
func TestPwaAfterLanding(t *testing.T) {
	pool := testPool(t)
	var pwaID string
	lnd := internalLanding(landing.StatusActive)
	fx := seedFlowFixture(t, pool, &lnd, withPwa(t, pool, &pwaID, streamset.PwaTypeInternal, false))
	h, sink, closeEvents := newLandingHandler(t, pool, true)

	rec := get(h, fx.host, "/t/go")
	m := trackURLInPage.FindStringSubmatch(rec.Body.String())
	if rec.Code != http.StatusOK || m == nil {
		t.Fatalf("landing not served: %d", rec.Code)
	}
	lc := get(h, fx.host, strings.TrimPrefix(m[1], "http://"+fx.host))
	if lc.Code != http.StatusFound || !strings.Contains(lc.Header().Get("Location"), "/pwa/") {
		t.Fatalf("landing click-through = %d → %q, want the PWA page", lc.Code, lc.Header().Get("Location"))
	}
	closeEvents()
	for _, e := range sink.events {
		if e.PwaID != pwaID {
			t.Fatalf("%s PwaID = %q, want %q", e.Type, e.PwaID, pwaID)
		}
	}
}

func TestPwaStageSkipped(t *testing.T) {
	pool := testPool(t)

	t.Run("non-internal type", func(t *testing.T) {
		var pwaID string
		fx := seedFlowFixture(t, pool, nil, withPwa(t, pool, &pwaID, streamset.PwaTypeExternal, false))
		h, sink, closeEvents := newLandingHandler(t, pool, true)
		assertRedirect(t, get(h, fx.host, "/t/go"), landingTestOffer)
		closeEvents()
		if sink.events[0].PwaID != "" {
			t.Fatal("skipped PWA stage was attributed")
		}
	})

	t.Run("paused pwa", func(t *testing.T) {
		var pwaID string
		fx := seedFlowFixture(t, pool, nil, withPwa(t, pool, &pwaID, streamset.PwaTypeInternal, false))
		if _, err := pool.Exec(context.Background(), `UPDATE pwas SET status = 'paused' WHERE id = $1`, pwaID); err != nil {
			t.Fatal(err)
		}
		h, _, closeEvents := newLandingHandler(t, pool, true)
		defer closeEvents()
		assertRedirect(t, get(h, fx.host, "/t/go"), landingTestOffer)
	})
}

// TestPwaTestLinkRecordsNothing: the constructor's test link serves the
// real page but never writes analytics.
func TestPwaTestLinkRecordsNothing(t *testing.T) {
	pool := testPool(t)
	var pwaID string
	fx := seedFlowFixture(t, pool, nil, withPwa(t, pool, &pwaID, streamset.PwaTypeInternal, false))
	h, sink, closeEvents := newLandingHandler(t, pool, true)

	tok := landinglink.NewSigner(testLinkKey).Sign(landinglink.Claims{
		OrganizationID: fx.orgID, ClickID: "test-1", PwaID: pwaID, Destination: "http://app.example/pwa/" + pwaID,
		IssuedAt: time.Now().Unix(), Test: true,
	})
	base := "/pwa/" + tok + "/"
	page := get(h, "localhost", base)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Test mode") {
		t.Fatalf("test page = %d", page.Code)
	}
	post(h, "localhost", base+"installed")
	assertRedirect(t, get(h, "localhost", base+"open"), "http://app.example/pwa/"+pwaID)
	closeEvents()
	if len(sink.events) != 0 {
		t.Fatalf("test link recorded events: %+v", sink.events)
	}
}

func TestPwaInAppWebviewBounce(t *testing.T) {
	pool := testPool(t)
	var pwaID string
	fx := seedFlowFixture(t, pool, nil, withPwa(t, pool, &pwaID, streamset.PwaTypeInternal, true))
	h, sink, closeEvents := newLandingHandler(t, pool, true)

	loc := get(h, fx.host, "/t/go").Header().Get("Location")
	base := strings.TrimPrefix(loc, "http://"+fx.host)

	req := httptest.NewRequest(http.MethodGet, "http://"+fx.host+base, nil)
	req.Host = fx.host
	req.RemoteAddr = "198.51.100.7:12345"
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 14) Chrome/126.0 Mobile [FB_IAB/FB4A;FBAV/450.0]")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "intent://"+fx.host+base) {
		t.Fatalf("in-app WebView not bounced to the external browser:\n%s", rec.Body.String())
	}
	closeEvents()
	for _, e := range sink.events {
		if e.Type == event.PwaView {
			t.Fatal("a bounce is not a view — PWA_VIEW must wait for the real browser")
		}
	}
}

func TestBounceTarget(t *testing.T) {
	self := "https://trk.example/pwa/tok/"
	if got := bounceTarget("Mozilla/5.0 (Linux; Android 14) Chrome/126 Mobile", self); got != "" {
		t.Fatalf("plain Chrome bounced: %q", got)
	}
	if got := bounceTarget("Mozilla/5.0 (iPhone; CPU iPhone OS 17_0) Instagram 300.0", self); got != "x-safari-https://trk.example/pwa/tok/" {
		t.Fatalf("iOS Instagram bounce = %q", got)
	}
}

// TestRenderPwaPageNoListingStats needs no database: the editor-style page
// carries the listing's plain facts and never its operator-typed numbers.
func TestRenderPwaPageNoListingStats(t *testing.T) {
	p := routingstore.PwaPage{
		Name: "Lucky App", ThemeColor: "#16a34a", BackgroundColor: "#111111", IconURL: "https://cdn.example/i.png",
		Developer: "Lucky Dev", Description: "Spin daily.", WhatsNew: "New wheel", Version: "2.4.1",
		ContainsAds: true, InAppPurchases: true, UpdatedDate: "2026-08-21",
	}
	for _, lang := range []string{"ru", "en"} {
		out, err := renderPwaPage(p, false, lang)
		if err != nil {
			t.Fatalf("%s render: %v", lang, err)
		}
		body := string(out)
		for _, want := range []string{"Lucky App", "Lucky Dev", "Spin daily.", "New wheel", "2.4.1", "2026-08-21", "--bg:#18181b", "--accent:#16a34a"} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s page lacks %q", lang, want)
			}
		}
		for _, banned := range []string{"Google Play", "Gift cards", "Подарочные карты", "verified", "Downloads", "Скачивания"} {
			if strings.Contains(body, banned) {
				t.Fatalf("%s page contains %q", lang, banned)
			}
		}
	}
	// No optional fields: no empty sections or sheet.
	bareOut, err := renderPwaPage(routingstore.PwaPage{Name: "X"}, false, "en")
	if err != nil {
		t.Fatalf("bare render: %v", err)
	}
	bare := string(bareOut)
	for _, banned := range []string{`id="sheet"`, "What's new", "Updated on", `class="shots"`} {
		if strings.Contains(bare, banned) {
			t.Fatalf("bare page contains %q", banned)
		}
	}
}

// TestRenderPwaStoreTemplateOnlyForTest: the store-style template (rating,
// reviews, downloads, store chrome) renders fully for the operator's test
// link and never for a visitor render of the same PWA.
func TestRenderPwaStoreTemplateOnlyForTest(t *testing.T) {
	p := routingstore.PwaPage{
		Name: "Lucky App", ThemeColor: "#16a34a", BackgroundColor: "#ffffff", IconURL: "https://cdn.example/i.png",
		Developer: "Lucky Dev", Description: "Spin daily.", WhatsNew: "New wheel", Version: "2.4.1",
		Rating: 4.5, RatingCount: 1200, DownloadsLabel: "1M+", SizeMB: 38, AgeRating: "12+", RatingStars5: 80,
		Screenshots: []string{"https://cdn.example/s1.png"},
	}
	for _, lang := range []string{"ru", "en"} {
		test, err := renderPwaPage(p, true, lang)
		if err != nil {
			t.Fatalf("%s test render: %v", lang, err)
		}
		tb := string(test)
		for _, want := range []string{`class="topbar"`, "4.5", "1200", "1M&#43;", "width:80%", `class="bottom-nav"`, "</html>"} {
			if !strings.Contains(tb, want) {
				t.Fatalf("%s test page lacks %q", lang, want)
			}
		}
		visitor, err := renderPwaPage(p, false, lang)
		if err != nil {
			t.Fatalf("%s visitor render: %v", lang, err)
		}
		for _, banned := range []string{`class="topbar"`, "1200", "1M+", "bottom-nav"} {
			if strings.Contains(string(visitor), banned) {
				t.Fatalf("%s visitor page contains %q", lang, banned)
			}
		}
	}
}
