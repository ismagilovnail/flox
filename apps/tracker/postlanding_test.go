package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/landing"
	"github.com/ismagilovnail/flox/apps/internal/landinglink"
	"github.com/ismagilovnail/flox/apps/internal/postlanding"
	"github.com/ismagilovnail/flox/apps/internal/routingstore"
	"github.com/ismagilovnail/flox/apps/internal/streamset"
)

const postlandingTestURL = "https://post.example/lp?go={next_url}&cid={click_id}&s1={sub1}"

func seedPostlanding(t *testing.T, pool *pgxpool.Pool, orgID, rawURL string) string {
	t.Helper()
	p, err := postlanding.NewService(postlanding.NewRepository(pool)).Create(context.Background(), orgID, postlanding.CreateInput{
		Name: "Thank you", URL: rawURL, Events: []string{"TG_JOIN"},
	})
	if err != nil {
		t.Fatalf("seeding postlanding: %v", err)
	}
	return p.ID
}

func withPostlanding(t *testing.T, pool *pgxpool.Pool, id *string, rawURL string) func(string, *streamset.FlowInput) {
	return func(orgID string, f *streamset.FlowInput) {
		*id = seedPostlanding(t, pool, orgID, rawURL)
		f.Postlanding = streamset.FlowPostlanding{Enabled: true, PostlandingID: *id}
	}
}

// both applies several flow modifiers.
func both(mods ...func(string, *streamset.FlowInput)) func(string, *streamset.FlowInput) {
	return func(orgID string, f *streamset.FlowInput) {
		for _, m := range mods {
			m(orgID, f)
		}
	}
}

// nextURLOf parses a postlanding redirect: the page's URL with the click's
// macros resolved, and the {next_url} it carries as a tracker path.
func nextURLOf(t *testing.T, host, location string) (url.Values, string) {
	t.Helper()
	u, err := url.Parse(location)
	if err != nil || u.Host != "post.example" || u.Path != "/lp" {
		t.Fatalf("redirect %q is not the postlanding", location)
	}
	q := u.Query()
	next := q.Get("go")
	if !strings.HasPrefix(next, "http://"+host+postClickPath) {
		t.Fatalf("next_url = %q, want a %s link on %s", next, postClickPath, host)
	}
	return q, strings.TrimPrefix(next, "http://"+host)
}

func eventsByType(sink *captureSink) map[event.Type][]event.Event {
	got := map[event.Type][]event.Event{}
	for _, e := range sink.events {
		got[e.Type] = append(got[e.Type], e)
	}
	return got
}

// TestPostlandingStageDirect: click → postlanding (POSTLANDING_VIEW, the
// click's macros and a signed {next_url}) → /pc/ (POSTLANDING_CLICK) →
// destination, every event on one click_id with postlanding_id set.
func TestPostlandingStageDirect(t *testing.T) {
	pool := testPool(t)
	var plID string
	fx := seedFlowFixture(t, pool, nil, withPostlanding(t, pool, &plID, postlandingTestURL))
	h, sink, closeEvents := newLandingHandler(t, pool, true)

	click := get(h, fx.host, "/t/go?sub1=a%26b")
	if click.Code != http.StatusFound {
		t.Fatalf("click = %d, want 302 to the postlanding", click.Code)
	}
	q, next := nextURLOf(t, fx.host, click.Header().Get("Location"))
	if q.Get("cid") == "" || q.Get("s1") != "a&b" {
		t.Fatalf("postlanding macros = %v, want the click id and sub1 a&b (escaped, one value)", q)
	}

	assertRedirect(t, get(h, fx.host, next), landingTestOffer)
	// Every click through is recorded, as on a landing.
	assertRedirect(t, get(h, fx.host, next), landingTestOffer)

	closeEvents()
	got := eventsByType(sink)
	src := got[event.SourceClick]
	if len(src) != 1 || len(got[event.PostlandingView]) != 1 || len(got[event.PostlandingClick]) != 2 || len(sink.events) != 4 {
		t.Fatalf("want SOURCE_CLICK, POSTLANDING_VIEW, 2×POSTLANDING_CLICK, got %+v", sink.events)
	}
	if src[0].ClickID != q.Get("cid") {
		t.Fatalf("{click_id} = %q, want %q", q.Get("cid"), src[0].ClickID)
	}
	for _, e := range sink.events {
		if e.ClickID != src[0].ClickID || e.PostlandingID != plID || e.OrganizationID != fx.orgID {
			t.Fatalf("%s = click %q postlanding %q, want click %q postlanding %q", e.Type, e.ClickID, e.PostlandingID, src[0].ClickID, plID)
		}
	}
}

// TestPostlandingAfterLandingAndPwa: the landing's click-through leads to
// the PWA, and the PWA's launch and "continue" lead to the postlanding —
// the full §25 order, Landing → PWA → Postlanding → Offer.
func TestPostlandingAfterLandingAndPwa(t *testing.T) {
	pool := testPool(t)
	var pwaID, plID string
	lnd := internalLanding(landing.StatusActive)
	fx := seedFlowFixture(t, pool, &lnd, both(
		withPwa(t, pool, &pwaID, streamset.PwaTypeInternal, false),
		withPostlanding(t, pool, &plID, postlandingTestURL),
	))
	h, sink, closeEvents := newLandingHandler(t, pool, true)

	rec := get(h, fx.host, "/t/go")
	m := trackURLInPage.FindStringSubmatch(rec.Body.String())
	if rec.Code != http.StatusOK || m == nil {
		t.Fatalf("landing not served: %d", rec.Code)
	}
	lc := get(h, fx.host, strings.TrimPrefix(m[1], "http://"+fx.host))
	pwaLoc := lc.Header().Get("Location")
	if lc.Code != http.StatusFound || !strings.Contains(pwaLoc, "/pwa/") {
		t.Fatalf("landing click-through = %d → %q, want the PWA page", lc.Code, pwaLoc)
	}
	base := strings.TrimPrefix(pwaLoc, "http://"+fx.host)

	for _, step := range []string{"open", "continue"} {
		r := get(h, fx.host, base+step)
		if r.Code != http.StatusFound {
			t.Fatalf("pwa %s = %d", step, r.Code)
		}
		_, next := nextURLOf(t, fx.host, r.Header().Get("Location"))
		assertRedirect(t, get(h, fx.host, next), landingTestOffer)
	}

	closeEvents()
	got := eventsByType(sink)
	for typ, n := range map[event.Type]int{
		event.SourceClick: 1, event.LandView: 1, event.LandClick: 1,
		event.PwaOpen: 1, event.PostlandingView: 2, event.PostlandingClick: 2,
	} {
		if len(got[typ]) != n {
			t.Fatalf("%s × %d, want %d (all: %+v)", typ, len(got[typ]), n, sink.events)
		}
	}
	for _, e := range sink.events {
		if e.PostlandingID != plID || e.PwaID != pwaID || e.LandingID != fx.landingID {
			t.Fatalf("%s carries landing %q pwa %q postlanding %q", e.Type, e.LandingID, e.PwaID, e.PostlandingID)
		}
	}
}

func TestPostlandingStageSkipped(t *testing.T) {
	pool := testPool(t)

	t.Run("paused postlanding", func(t *testing.T) {
		var plID string
		fx := seedFlowFixture(t, pool, nil, withPostlanding(t, pool, &plID, postlandingTestURL))
		if _, err := postlanding.NewService(postlanding.NewRepository(pool)).Pause(context.Background(), fx.orgID, plID); err != nil {
			t.Fatalf("pausing: %v", err)
		}
		h, sink, closeEvents := newLandingHandler(t, pool, true)
		assertRedirect(t, get(h, fx.host, "/t/go"), landingTestOffer)
		closeEvents()
		if len(sink.events) != 1 || sink.events[0].PostlandingID != "" {
			t.Fatalf("a paused postlanding is not attributed, got %+v", sink.events)
		}
	})

	t.Run("behind an external landing", func(t *testing.T) {
		var plID string
		ext := landing.Landing{Type: landing.TypeExternal, URL: "https://advertiser.example/lp", Status: landing.StatusActive}
		fx := seedFlowFixture(t, pool, &ext, withPostlanding(t, pool, &plID, postlandingTestURL))
		h, sink, closeEvents := newLandingHandler(t, pool, true)
		assertRedirect(t, get(h, fx.host, "/t/go"), "https://advertiser.example/lp")
		closeEvents()
		if len(sink.events) != 1 || sink.events[0].PostlandingID != "" {
			t.Fatalf("an unreachable postlanding is not attributed, got %+v", sink.events)
		}
	})

	t.Run("no link key", func(t *testing.T) {
		var plID string
		fx := seedFlowFixture(t, pool, nil, withPostlanding(t, pool, &plID, postlandingTestURL))
		h, sink, closeEvents := newLandingHandler(t, pool, false)
		assertRedirect(t, get(h, fx.host, "/t/go"), landingTestOffer)
		closeEvents()
		if len(sink.events) != 1 || sink.events[0].PostlandingID != "" {
			t.Fatalf("without signable links the stage is skipped, got %+v", sink.events)
		}
	})

	t.Run("paused after the link was issued", func(t *testing.T) {
		var plID string
		lnd := internalLanding(landing.StatusActive)
		fx := seedFlowFixture(t, pool, &lnd, withPostlanding(t, pool, &plID, postlandingTestURL))
		h, sink, closeEvents := newLandingHandler(t, pool, true)
		m := trackURLInPage.FindStringSubmatch(get(h, fx.host, "/t/go").Body.String())
		if m == nil {
			t.Fatal("landing not served")
		}
		if _, err := postlanding.NewService(postlanding.NewRepository(pool)).Pause(context.Background(), fx.orgID, plID); err != nil {
			t.Fatalf("pausing: %v", err)
		}
		assertRedirect(t, get(h, fx.host, strings.TrimPrefix(m[1], "http://"+fx.host)), landingTestOffer)
		closeEvents()
		if n := len(eventsByType(sink)[event.PostlandingView]); n != 0 {
			t.Fatalf("POSTLANDING_VIEW × %d for a postlanding paused before the visitor got there", n)
		}
	})
}

// TestPostClickRejectsBadTokens: /pc/ is an open redirect and an event
// injector unless the token is ours, for a postlanding, on this host.
func TestPostClickRejectsBadTokens(t *testing.T) {
	pool := testPool(t)
	h, sink, closeEvents := newLandingHandler(t, pool, true)
	signer := landinglink.NewSigner(testLinkKey)
	now := time.Now().Unix()

	forged := landinglink.NewSigner([]byte("ffffffffffffffffffffffffffffffff")).Sign(landinglink.Claims{
		OrganizationID: "o", ClickID: "k", PostlandingID: "p", Destination: "https://evil.example", IssuedAt: now, Host: "any.example",
	})
	noStage := signer.Sign(landinglink.Claims{
		OrganizationID: "o", ClickID: "k", Destination: "https://evil.example", IssuedAt: now, Host: "any.example",
	})
	otherHost := signer.Sign(landinglink.Claims{
		OrganizationID: "o", ClickID: "k", PostlandingID: "p", Destination: "https://evil.example", IssuedAt: now, Host: "other.example",
	})
	expired := signer.Sign(landinglink.Claims{
		OrganizationID: "o", ClickID: "k", PostlandingID: "p", Destination: "https://evil.example",
		IssuedAt: time.Now().Add(-landinglink.MaxAge - time.Hour).Unix(), Host: "any.example",
	})
	for _, path := range []string{"/pc/garbage", "/pc/a.b", "/pc/" + forged, "/pc/" + noStage, "/pc/" + otherHost, "/pc/" + expired} {
		if rec := get(h, "any.example", path); rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want 404", path, rec.Code)
		}
	}
	closeEvents()
	if len(sink.events) != 0 {
		t.Fatalf("rejected links recorded events: %+v", sink.events)
	}
}

// TestLoadPostlandingIsTenantScoped: the routes that continue a journey
// from a token read the postlanding by (organization, id) — another
// organization's postlanding id is not found, never served.
func TestLoadPostlandingIsTenantScoped(t *testing.T) {
	pool := testPool(t)
	var plID string
	fx := seedFlowFixture(t, pool, nil, withPostlanding(t, pool, &plID, postlandingTestURL))
	other := seedFlowFixture(t, pool, nil, nil)
	store := routingstore.New(pool)

	got, err := store.LoadPostlanding(context.Background(), fx.orgID, plID)
	if err != nil || got.URL != postlandingTestURL || got.Status != string(postlanding.StatusActive) {
		t.Fatalf("own postlanding = %+v, %v", got, err)
	}
	if _, err := store.LoadPostlanding(context.Background(), other.orgID, plID); !errors.Is(err, routingstore.ErrNotFound) {
		t.Fatalf("another organization's postlanding: err = %v, want ErrNotFound", err)
	}
}
