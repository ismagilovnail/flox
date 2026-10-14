package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/landinglink"
	"github.com/ismagilovnail/flox/apps/internal/streamset"
)

const (
	iPhoneUA  = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1"
	androidUA = "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Mobile Safari/537.36"
)

func launch(h http.Handler, host, path, ua string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "http://"+host+path, nil)
	req.Host = host
	req.RemoteAddr = "198.51.100.7:12345"
	req.Header.Set("User-Agent", ua)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func installedCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == iosInstalledCookie {
			return c
		}
	}
	return nil
}

// TestIosInstallOnFirstLaunch: iOS never fires appinstalled, so an iOS
// app's first launch from the Home Screen (start_url without the
// launched cookie, which a Home Screen app's own cookie store starts
// without) records IOS_INSTALL, before that launch's PWA_OPEN. Later
// launches, other platforms and test links record none.
func TestIosInstallOnFirstLaunch(t *testing.T) {
	pool := testPool(t)
	var pwaID string
	fx := seedFlowFixture(t, pool, nil, withPwa(t, pool, &pwaID, streamset.PwaTypeInternal, false))
	h, sink, closeEvents := newLandingHandler(t, pool, true)

	loc := get(h, fx.host, "/t/go").Header().Get("Location")
	base := strings.TrimPrefix(loc, "http://"+fx.host)

	first := launch(h, fx.host, base+"open", iPhoneUA)
	assertRedirect(t, first, landingTestOffer)
	c := installedCookie(first)
	if c == nil || c.Path != base || !c.HttpOnly || c.MaxAge < 365*24*60*60 || c.Secure {
		t.Fatalf("launched cookie = %+v, want HttpOnly, long-lived, scoped to %s, not Secure on http", c, base)
	}
	if again := launch(h, fx.host, base+"open", iPhoneUA, c); installedCookie(again) != nil {
		t.Fatal("a later launch set the cookie again")
	}
	if android := launch(h, fx.host, base+"open", androidUA); installedCookie(android) != nil {
		t.Fatal("an Android launch was taken for an iOS install")
	}
	// Another iPhone (or the app installed again): another installation.
	launch(h, fx.host, base+"open", iPhoneUA)

	// A constructor test link records nothing, iOS or not.
	tok := landinglink.NewSigner(testLinkKey).Sign(landinglink.Claims{
		OrganizationID: fx.orgID, ClickID: "test-ios", PwaID: pwaID, Destination: "http://app.example/x",
		IssuedAt: time.Now().Unix(), Test: true, Host: "localhost",
	})
	launch(h, "localhost", "/pwa/"+tok+"/open", iPhoneUA)

	closeEvents()
	var types []event.Type
	src := sink.events[0]
	for _, e := range sink.events {
		if e.Type == event.SourceClick {
			continue
		}
		types = append(types, e.Type)
		if e.ClickID != src.ClickID || e.PwaID != pwaID {
			t.Fatalf("%s on click %q pwa %q, want %q %q", e.Type, e.ClickID, e.PwaID, src.ClickID, pwaID)
		}
		if e.Type == event.IosInstall && e.OS != "ios" {
			t.Fatalf("IOS_INSTALL os = %q", e.OS)
		}
	}
	want := []event.Type{event.IosInstall, event.PwaOpen, event.PwaOpen, event.PwaOpen, event.IosInstall, event.PwaOpen}
	if len(types) != len(want) {
		t.Fatalf("events = %v, want %v", types, want)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("events = %v, want %v", types, want)
		}
	}
}
