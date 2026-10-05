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
	// Everything the editor preview shows, numbers included, reaches the visitor.
	for _, want := range []string{"4.9", "120000", "87.5", "Bug fixes and a new wheel.", "2.4.1", "Casual", "Contains ads", "install", `id="sheet"`} {
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

// TestRenderPwaPage needs no database: the editor-style page carries the
// listing's facts and renders optional sections only when they are set.
func TestRenderPwaPage(t *testing.T) {
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

// TestRenderPwaPageAllFormFields: every constructor form value reaches the
// page, for a visitor render and for the constructor's test link alike.
func TestRenderPwaPageAllFormFields(t *testing.T) {
	p := routingstore.PwaPage{
		Name: "NAMEX", ShortName: "SHORTX", ThemeColor: "#16a34a", BackgroundColor: "#ffffff", IconURL: "https://cdn.example/i.png",
		Developer: "DEVX", Description: "DESCX", Screenshots: []string{"https://cdn.example/s1.png"},
		WhatsNew: "WNX", Version: "VERX", Category: "CATX", InteractiveElements: "INTX", ContainsAds: true, InAppPurchases: true,
		UpdatedDate: "2026-08-21", Rating: 4.3, RatingCount: 777, DownloadsLabel: "DLX", SizeMB: 38, AgeRating: "AGEX",
		RatingStars5: 61, RatingStars4: 22, RatingStars3: 7, RatingStars2: 5, RatingStars1: 3, ShowHeader: true,
	}
	for _, lang := range []string{"ru", "en"} {
		for _, test := range []bool{false, true} {
			out, err := renderPwaPage(p, test, lang)
			if err != nil {
				t.Fatalf("%s test=%v render: %v", lang, test, err)
			}
			body := string(out)
			for _, want := range []string{"NAMEX", "SHORTX", "DEVX", "DESCX", "s1.png", "WNX", "VERX", "CATX", "INTX", "2026-08-21",
				"4.3", "777", "DLX", "38", "AGEX", "width:61%", "width:22%", "width:7%", "width:5%", "width:3%",
				`class="topbar"`, `class="bottom-nav"`, "</html>"} {
				if !strings.Contains(body, want) {
					t.Errorf("%s test=%v page lacks %q", lang, test, want)
				}
			}
			if got := strings.Contains(body, `class="test"`); got != test {
				t.Errorf("%s test=%v: test banner present = %v", lang, test, got)
			}
		}
	}
}

// TestRenderPwaPageThemeModes: fixed light/dark come from the background
// color; auto starts light and flips on the visitor's prefers-color-scheme.
func TestRenderPwaPageThemeModes(t *testing.T) {
	base := routingstore.PwaPage{Name: "X", ThemeColor: "#16a34a"}
	render := func(bg string, auto bool) string {
		p := base
		p.BackgroundColor, p.ThemeAuto = bg, auto
		out, err := renderPwaPage(p, false, "en")
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	if b := render("#ffffff", false); !strings.Contains(b, "--bg:#ffffff") || strings.Contains(b, "prefers-color-scheme") {
		t.Fatal("fixed light page must be light with no media query")
	}
	if b := render("#0a0a0a", false); !strings.Contains(b, "--bg:#18181b") || strings.Contains(b, "prefers-color-scheme") {
		t.Fatal("fixed dark page must be dark with no media query")
	}
	// Auto ignores the background color for the base palette: light first,
	// dark only inside the media query.
	for _, bg := range []string{"#ffffff", "#0a0a0a"} {
		b := render(bg, true)
		i := strings.Index(b, "--bg:#ffffff")
		j := strings.Index(b, "prefers-color-scheme: dark")
		k := strings.LastIndex(b, "--bg:#18181b")
		if i < 0 || j < i || k < j || !strings.Contains(b, `content="light dark"`) {
			t.Fatalf("auto page (bg %s) lacks light base + dark media palette", bg)
		}
	}
}

// TestRenderPwaPageShowHeader: the top bar (brand mark, title, search) is
// rendered only while the PWA's show_header toggle is on.
func TestRenderPwaPageShowHeader(t *testing.T) {
	for _, show := range []bool{true, false} {
		out, err := renderPwaPage(routingstore.PwaPage{Name: "X", ShowHeader: show}, false, "en")
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(string(out), `class="topbar"`); got != show {
			t.Fatalf("show_header=%v: top bar present = %v", show, got)
		}
		if !strings.Contains(string(out), `id="head"`) {
			t.Fatalf("show_header=%v: app header must stay", show)
		}
	}
}

// TestRenderPwaPageHeaderBrand: the top bar shows the operator's own title
// and logo when set, and the default "Apps" + built-in mark otherwise.
func TestRenderPwaPageHeaderBrand(t *testing.T) {
	render := func(p routingstore.PwaPage) string {
		p.Name, p.ShowHeader = "X", true
		out, err := renderPwaPage(p, false, "en")
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	def := render(routingstore.PwaPage{})
	if !strings.Contains(def, `<span class="brand-title">Apps</span>`) || !strings.Contains(def, `fill="#EA4335"`) {
		t.Fatal("default header must show Apps and the built-in mark")
	}
	own := render(routingstore.PwaPage{HeaderTitle: "My <Store>", HeaderLogoURL: "https://cdn.example/logo.png"})
	for _, want := range []string{`<span class="brand-title">My &lt;Store&gt;</span>`, `<img src="https://cdn.example/logo.png" alt="">`} {
		if !strings.Contains(own, want) {
			t.Fatalf("branded header lacks %q", want)
		}
	}
	if strings.Contains(own, `fill="#EA4335"`) || strings.Contains(own, `brand-title">Apps<`) {
		t.Fatal("branded header must not keep the default mark or title")
	}
}
