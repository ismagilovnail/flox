package main

import (
	"net/url"
	"strings"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/landing"
	"github.com/ismagilovnail/flox/apps/internal/routing"
	"github.com/ismagilovnail/flox/apps/internal/streamset"
)

// Audit 2026-10-06, T7: the URL the visitor is sent to carries the click's
// own id, so the partner can echo it back in its postback.
const macroOffer = "https://offer.example/go?cid={click_id}&camp={campaign_id}&s1={sub1}&p={payout}"

func withDestination(dest string) func(string, *streamset.FlowInput) {
	return func(_ string, f *streamset.FlowInput) {
		f.Destination = streamset.Destination{Kind: routing.DestinationRedirect, URL: dest}
	}
}

func sourceClick(t *testing.T, sink *captureSink) event.Event {
	t.Helper()
	for _, e := range sink.events {
		if e.Type == event.SourceClick {
			return e
		}
	}
	t.Fatalf("no SOURCE_CLICK recorded: %+v", sink.events)
	return event.Event{}
}

func locationQuery(t *testing.T, location string) url.Values {
	t.Helper()
	u, err := url.Parse(location)
	if err != nil {
		t.Fatalf("Location %q: %v", location, err)
	}
	return u.Query()
}

func TestRedirectResolvesDestinationMacros(t *testing.T) {
	pool := testPool(t)
	fx := seedFlowFixture(t, pool, nil, withDestination(macroOffer))
	h, sink, closeEvents := newLandingHandler(t, pool, true)

	// sub1 tries to smuggle a second click_id into the offer URL.
	rec := get(h, fx.host, "/t/go?sub1="+url.QueryEscape("adset&cid=evil"))
	closeEvents()
	if rec.Code != 302 {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	click := sourceClick(t, sink)
	loc := rec.Header().Get("Location")
	q := locationQuery(t, loc)
	if got := q["cid"]; len(got) != 1 || got[0] != click.ClickID {
		t.Fatalf("cid = %v in %q, want exactly the recorded click id %q", got, loc, click.ClickID)
	}
	if q.Get("camp") != click.CampaignID || q.Get("s1") != "adset&cid=evil" {
		t.Fatalf("camp=%q s1=%q in %q, want the campaign id and the escaped sub1", q.Get("camp"), q.Get("s1"), loc)
	}
	if !strings.Contains(loc, "p={payout}") {
		t.Fatalf("an unknown token was not left literal: %q", loc)
	}
	if click.Destination != loc {
		t.Fatalf("recorded destination %q, want the resolved URL %q", click.Destination, loc)
	}
}

func TestExternalLandingResolvesMacros(t *testing.T) {
	pool := testPool(t)
	fx := seedLandingFixture(t, pool, landing.Landing{
		Type: landing.TypeExternal, URL: "https://advertiser.example/lp?cid={click_id}", Status: landing.StatusActive,
	})
	h, sink, closeEvents := newLandingHandler(t, pool, true)
	rec := get(h, fx.host, "/t/go")
	closeEvents()
	if want := "https://advertiser.example/lp?cid=" + sourceClick(t, sink).ClickID; rec.Header().Get("Location") != want {
		t.Fatalf("Location = %q, want %q", rec.Header().Get("Location"), want)
	}
}

// The internal landing's click link carries the destination in its signed
// claims; the click-through must land on the resolved URL with the same id.
func TestInternalLandingClickThroughResolvesMacros(t *testing.T) {
	pool := testPool(t)
	lnd := internalLanding(landing.StatusActive)
	fx := seedFlowFixture(t, pool, &lnd, withDestination(macroOffer))
	h, sink, closeEvents := newLandingHandler(t, pool, true)

	page := get(h, fx.host, "/t/go?sub1=s")
	m := trackURLInPage.FindStringSubmatch(page.Body.String())
	if m == nil {
		t.Fatalf("no trackUrl in served landing:\n%s", page.Body)
	}
	click := get(h, fx.host, strings.TrimPrefix(m[1], "http://"+fx.host))
	closeEvents()
	q := locationQuery(t, click.Header().Get("Location"))
	if q.Get("cid") != sourceClick(t, sink).ClickID || q.Get("s1") != "s" {
		t.Fatalf("click-through Location = %q, want the source click's id and sub1", click.Header().Get("Location"))
	}
}
