package main

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/landing"
	"github.com/ismagilovnail/flox/apps/internal/landinglink"
	"github.com/ismagilovnail/flox/apps/internal/streamset"
)

const externalPwaURL = "https://pwa.example/app?cid={click_id}&s1={sub1}&flox={pwa_link}"

func withExternalPwa(orgID string, f *streamset.FlowInput) {
	f.Pwa = streamset.FlowPwa{Enabled: true, PwaType: streamset.PwaTypeExternal, URL: externalPwaURL}
}

// pwaLinkFrom is the {pwa_link} an external PWA got, as a tracker path.
func pwaLinkFrom(t *testing.T, location, host string) string {
	t.Helper()
	u, err := url.Parse(location)
	if err != nil {
		t.Fatal(err)
	}
	link := u.Query().Get("flox")
	if !strings.HasPrefix(link, "http://"+host+"/pwx/") || !strings.HasSuffix(link, "/") {
		t.Fatalf("{pwa_link} = %q", link)
	}
	return strings.TrimPrefix(link, "http://"+host)
}

// An external PWA: the click goes to its URL with the macros filled and a
// {pwa_link}; PWA_VIEW is recorded; the PWA reports its install and opens
// through {pwa_link}, which continues to the offer — all on one click_id,
// with no PWA id (FLOX does not host it).
func TestExternalPwa(t *testing.T) {
	pool := testPool(t)
	fx := seedFlowFixture(t, pool, nil, withExternalPwa)
	h, sink, closeEvents := newLandingHandler(t, pool, true)

	click := get(h, fx.host, "/t/go?sub1=adset7")
	loc := click.Header().Get("Location")
	if click.Code != http.StatusFound || !strings.HasPrefix(loc, "https://pwa.example/app?cid=") || !strings.Contains(loc, "s1=adset7") {
		t.Fatalf("click = %d → %q, want the external PWA with macros", click.Code, loc)
	}
	link := pwaLinkFrom(t, loc, fx.host)

	if rec := get(h, fx.host, link+"installed"); rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("installed (GET beacon) = %d, CORS %q", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec := post(h, fx.host, link+"installed"); rec.Code != http.StatusNoContent {
		t.Fatalf("installed (POST) = %d", rec.Code)
	}
	assertRedirect(t, get(h, fx.host, link+"open"), landingTestOffer)

	// Not on another host, not a forged or an internal-PWA token.
	if rec := get(h, "other.example", link+"installed"); rec.Code != http.StatusNotFound {
		t.Fatalf("another host = %d, want 404", rec.Code)
	}
	if rec := get(h, fx.host, "/pwx/garbage/installed"); rec.Code != http.StatusNotFound {
		t.Fatalf("forged token = %d, want 404", rec.Code)
	}
	internal := landinglink.NewSigner(testLinkKey).Sign(landinglink.Claims{
		OrganizationID: fx.orgID, ClickID: "c", PwaID: "p", Destination: landingTestOffer, Host: landinglink.NormalizeHost(fx.host), IssuedAt: time.Now().Unix(),
	})
	if rec := get(h, fx.host, "/pwx/"+internal+"/installed"); rec.Code != http.StatusNotFound {
		t.Fatalf("an internal PWA's token = %d, want 404", rec.Code)
	}

	closeEvents()
	counts := map[event.Type]int{}
	src := sink.events[0]
	for _, e := range sink.events {
		counts[e.Type]++
		if e.ClickID != src.ClickID || e.PwaID != "" || e.OrganizationID != fx.orgID {
			t.Fatalf("%s = click %q pwa %q, want the click and no PWA id", e.Type, e.ClickID, e.PwaID)
		}
	}
	if counts[event.SourceClick] != 1 || counts[event.PwaView] != 1 || counts[event.PwaInstall] != 2 || counts[event.PwaOpen] != 1 || len(sink.events) != 5 {
		t.Fatalf("events = %v", counts)
	}
}

// After an internal landing the click-through goes to the external PWA.
func TestExternalPwaAfterLanding(t *testing.T) {
	pool := testPool(t)
	lnd := internalLanding(landing.StatusActive)
	fx := seedFlowFixture(t, pool, &lnd, withExternalPwa)
	h, sink, closeEvents := newLandingHandler(t, pool, true)

	rec := get(h, fx.host, "/t/go?sub1=z")
	m := trackURLInPage.FindStringSubmatch(rec.Body.String())
	if rec.Code != http.StatusOK || m == nil {
		t.Fatalf("landing not served: %d", rec.Code)
	}
	lc := get(h, fx.host, strings.TrimPrefix(m[1], "http://"+fx.host))
	loc := lc.Header().Get("Location")
	if lc.Code != http.StatusFound || !strings.HasPrefix(loc, "https://pwa.example/app?") || !strings.Contains(loc, "s1=z") {
		t.Fatalf("click-through = %d → %q, want the external PWA", lc.Code, loc)
	}
	pwaLinkFrom(t, loc, fx.host)
	closeEvents()
	types := []event.Type{}
	for _, e := range sink.events {
		types = append(types, e.Type)
	}
	if strings.Join(eventTypes(types), ",") != "SOURCE_CLICK,LAND_VIEW,LAND_CLICK,PWA_VIEW" {
		t.Fatalf("events = %v", types)
	}
}

func eventTypes(ts []event.Type) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = string(t)
	}
	return out
}

// ios_app: iPhone visitors go to the app link (no event: an App Store
// install is not seen here); others get the stage's internal PWA, or skip
// the stage when it has none.
func TestIOSAppPwa(t *testing.T) {
	pool := testPool(t)
	const appURL = "https://apps.apple.com/app/id123?ct={click_id}"

	t.Run("with a PWA for other devices", func(t *testing.T) {
		var pwaID string
		fx := seedFlowFixture(t, pool, nil, func(orgID string, f *streamset.FlowInput) {
			pwaID = seedPwa(t, pool, orgID, false)
			f.Pwa = streamset.FlowPwa{Enabled: true, PwaType: streamset.PwaTypeIOSApp, PwaID: pwaID, URL: appURL}
		})
		h, sink, closeEvents := newLandingHandler(t, pool, true)

		ios := launch(h, fx.host, "/t/go", iPhoneUA)
		if loc := ios.Header().Get("Location"); ios.Code != http.StatusFound || !strings.HasPrefix(loc, "https://apps.apple.com/app/id123?ct=") || strings.HasSuffix(loc, "ct=") {
			t.Fatalf("iPhone = %d → %q, want the app link with the click id", ios.Code, loc)
		}
		android := launch(h, fx.host, "/t/go", androidUA)
		if loc := android.Header().Get("Location"); !strings.Contains(loc, "/pwa/") {
			t.Fatalf("Android → %q, want the internal PWA page", loc)
		}

		// The PWA paused: iPhone still gets the app, Android skips the stage.
		if _, err := pool.Exec(context.Background(), `UPDATE pwas SET status = 'paused' WHERE id = $1`, pwaID); err != nil {
			t.Fatal(err)
		}
		h2, _, close2 := newLandingHandler(t, pool, true)
		defer close2()
		if loc := launch(h2, fx.host, "/t/go", iPhoneUA).Header().Get("Location"); !strings.HasPrefix(loc, "https://apps.apple.com/") {
			t.Fatalf("iPhone with the PWA paused → %q", loc)
		}
		assertRedirect(t, launch(h2, fx.host, "/t/go", androidUA), landingTestOffer)

		closeEvents()
		for _, e := range sink.events {
			if e.Type != event.SourceClick {
				t.Fatalf("event %s recorded, want only the clicks", e.Type)
			}
		}
		if sink.events[0].PwaID != "" || sink.events[1].PwaID != pwaID {
			t.Fatalf("pwa ids = %q, %q; want none for the app link and the PWA for Android", sink.events[0].PwaID, sink.events[1].PwaID)
		}
	})

	t.Run("app link only", func(t *testing.T) {
		fx := seedFlowFixture(t, pool, nil, func(orgID string, f *streamset.FlowInput) {
			f.Pwa = streamset.FlowPwa{Enabled: true, PwaType: streamset.PwaTypeIOSApp, URL: appURL}
		})
		h, _, closeEvents := newLandingHandler(t, pool, true)
		defer closeEvents()
		assertRedirect(t, launch(h, fx.host, "/t/go", androidUA), landingTestOffer)
		if loc := launch(h, fx.host, "/t/go", iPhoneUA).Header().Get("Location"); !strings.HasPrefix(loc, "https://apps.apple.com/") {
			t.Fatalf("iPhone → %q", loc)
		}
	})
}
