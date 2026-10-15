package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/landing"
	"github.com/ismagilovnail/flox/apps/internal/streamset"
)

func asPwa(orgID string, f *streamset.FlowInput) { f.Landing.AsPwa = true }

// landingAppBase follows the click to the landing app's own path.
func landingAppBase(t *testing.T, h http.Handler, host, ua string) string {
	t.Helper()
	click := launch(h, host, "/t/go?sub1=a", ua)
	loc := click.Header().Get("Location")
	if click.Code != http.StatusFound || !strings.HasPrefix(loc, "http://"+host+"/lpwa/") || !strings.HasSuffix(loc, "/") {
		t.Fatalf("click = %d → %q, want 302 to /lpwa/<token>/", click.Code, loc)
	}
	return strings.TrimPrefix(loc, "http://"+host)
}

// The whole landing app: page (the landing, installable — manifest, icon,
// service worker, beacon, absolute against the landing's <base>), its
// manifest named after the landing, the install beacon, the landing's own
// click-through still a LAND_CLICK, and the installed app's launch → the
// offer. Events carry the landing, no PWA id.
func TestLandingAsPwa(t *testing.T) {
	pool := testPool(t)
	lnd := internalLanding(landing.StatusActive)
	lnd.Title = "Lucky Landing"
	fx := seedFlowFixture(t, pool, &lnd, asPwa)
	h, sink, closeEvents := newLandingHandler(t, pool, true)

	base := landingAppBase(t, h, fx.host, androidUA)
	abs := "http://" + fx.host + base
	page := launch(h, fx.host, base, androidUA)
	body := page.Body.String()
	for _, want := range []string{
		`<link rel="manifest" href="` + abs + `manifest.webmanifest">`,
		`<link rel="apple-touch-icon" href="https://example.com/l.png">`,
		`navigator.serviceWorker.register(base+"sw.js",{scope:base})`,
		`"` + abs + `"`, // base, absolute
		`<base href="https://cdn.example/`,
		`var trackUrl = "http://` + fx.host + `/lc/`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("landing app page lacks %q:\n%s", want, body)
		}
	}

	var manifest map[string]any
	m := launch(h, fx.host, base+"manifest.webmanifest", androidUA)
	if err := json.Unmarshal(m.Body.Bytes(), &manifest); err != nil || m.Header().Get("Content-Type") != "application/manifest+json" {
		t.Fatalf("manifest: %v / %q", err, m.Header().Get("Content-Type"))
	}
	if manifest["name"] != "Lucky Landing" || manifest["start_url"] != base+"open" || manifest["scope"] != base || manifest["display"] != "standalone" {
		t.Fatalf("manifest = %v", manifest)
	}
	if sw := launch(h, fx.host, base+"sw.js", androidUA); sw.Code != http.StatusOK || !strings.Contains(sw.Body.String(), `addEventListener("fetch"`) {
		t.Fatalf("service worker = %d", sw.Code)
	}
	if rec := post(h, fx.host, base+"installed"); rec.Code != http.StatusNoContent {
		t.Fatalf("installed = %d", rec.Code)
	}
	assertRedirect(t, launch(h, fx.host, base+"open", androidUA), landingTestOffer)

	// The landing's own button still goes through /lc/ to the offer.
	m2 := trackURLInPage.FindStringSubmatch(body)
	assertRedirect(t, get(h, fx.host, strings.TrimPrefix(m2[1], "http://"+fx.host)), landingTestOffer)

	if rec := launch(h, "other.example", base, androidUA); rec.Code != http.StatusNotFound {
		t.Fatalf("another host = %d, want 404", rec.Code)
	}

	closeEvents()
	counts := map[event.Type]int{}
	src := sink.events[0]
	for _, e := range sink.events {
		counts[e.Type]++
		if e.ClickID != src.ClickID || e.PwaID != "" || (e.Type != event.SourceClick && e.LandingID != fx.landingID) {
			t.Fatalf("%s = click %q landing %q pwa %q", e.Type, e.ClickID, e.LandingID, e.PwaID)
		}
	}
	want := map[event.Type]int{event.SourceClick: 1, event.LandView: 1, event.PwaView: 1, event.PwaInstall: 1, event.PwaOpen: 1, event.LandClick: 1}
	for typ, n := range want {
		if counts[typ] != n {
			t.Fatalf("events = %v, want %v", counts, want)
		}
	}
	if len(sink.events) != 6 {
		t.Fatalf("events = %v", counts)
	}
}

// The installed landing continues to the flow's PWA stage when there is
// one; an iPhone's first launch is IOS_INSTALL.
func TestLandingAsPwaOpensNextStage(t *testing.T) {
	pool := testPool(t)
	lnd := internalLanding(landing.StatusActive)
	fx := seedFlowFixture(t, pool, &lnd, func(orgID string, f *streamset.FlowInput) {
		f.Landing.AsPwa = true
		withExternalPwa(orgID, f)
	})
	h, sink, closeEvents := newLandingHandler(t, pool, true)

	base := landingAppBase(t, h, fx.host, iPhoneUA)
	open := launch(h, fx.host, base+"open", iPhoneUA)
	if loc := open.Header().Get("Location"); open.Code != http.StatusFound || !strings.HasPrefix(loc, "https://pwa.example/app?") {
		t.Fatalf("open = %d → %q, want the flow's external PWA", open.Code, loc)
	}
	closeEvents()
	counts := map[event.Type]int{}
	for _, e := range sink.events {
		counts[e.Type]++
	}
	if counts[event.IosInstall] != 1 || counts[event.PwaOpen] != 1 || counts[event.PwaView] != 1 {
		t.Fatalf("events = %v, want IOS_INSTALL, PWA_OPEN and the external PWA's PWA_VIEW", counts)
	}
}

// In an in-app WebView the app page bounces to the external browser
// (nothing recorded); asPwa off serves the landing on the click as before.
func TestLandingAsPwaBounceAndOff(t *testing.T) {
	pool := testPool(t)
	lnd := internalLanding(landing.StatusActive)
	fx := seedFlowFixture(t, pool, &lnd, asPwa)
	h, sink, closeEvents := newLandingHandler(t, pool, true)

	base := landingAppBase(t, h, fx.host, androidUA)
	inApp := launch(h, fx.host, base, "Mozilla/5.0 (Linux; Android 14) [FBAN/FB4A;FBAV/450.0]")
	if !strings.Contains(inApp.Body.String(), "intent://"+fx.host+base) {
		t.Fatalf("in-app WebView = %d:\n%s", inApp.Code, inApp.Body.String())
	}
	closeEvents()
	if len(sink.events) != 1 {
		t.Fatalf("events = %d, want only the click (the bounce records nothing)", len(sink.events))
	}

	lnd2 := internalLanding(landing.StatusActive)
	fx2 := seedFlowFixture(t, pool, &lnd2, nil)
	h2, _, close2 := newLandingHandler(t, pool, true)
	defer close2()
	if rec := get(h2, fx2.host, "/t/go"); rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `rel="manifest"`) {
		t.Fatalf("asPwa off = %d, want the plain landing on the click", rec.Code)
	}
}

// With an internal PWA stage after it: the landing app's PWA_* events do
// not take that PWA's id (LAND_VIEW does, as any landing's), and its
// launch opens the PWA's page.
func TestLandingAsPwaThenInternalPwa(t *testing.T) {
	pool := testPool(t)
	lnd := internalLanding(landing.StatusActive)
	var pwaID string
	fx := seedFlowFixture(t, pool, &lnd, func(orgID string, f *streamset.FlowInput) {
		f.Landing.AsPwa = true
		pwaID = seedPwa(t, pool, orgID, false)
		f.Pwa = streamset.FlowPwa{Enabled: true, PwaType: streamset.PwaTypeInternal, PwaID: pwaID}
	})
	h, sink, closeEvents := newLandingHandler(t, pool, true)

	base := landingAppBase(t, h, fx.host, androidUA)
	launch(h, fx.host, base, androidUA)
	post(h, fx.host, base+"installed")
	open := launch(h, fx.host, base+"open", androidUA)
	if loc := open.Header().Get("Location"); !strings.Contains(loc, "/pwa/") {
		t.Fatalf("open → %q, want the flow's PWA page", loc)
	}
	closeEvents()
	seen := 0
	for _, e := range sink.events {
		switch e.Type {
		case event.PwaView, event.PwaInstall, event.PwaOpen:
			seen++
			if e.PwaID != "" || e.LandingID != fx.landingID {
				t.Fatalf("landing app %s = pwa %q landing %q, want no pwa and the landing", e.Type, e.PwaID, e.LandingID)
			}
		case event.LandView:
			if e.PwaID != pwaID {
				t.Fatalf("LAND_VIEW pwa = %q, want the click's %q like any landing's", e.PwaID, pwaID)
			}
		}
	}
	if seen != 3 {
		t.Fatalf("landing app PWA events = %d, want 3", seen)
	}
}
