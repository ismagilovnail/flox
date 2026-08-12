package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/routing"
)

func requestWithQuery(query string) *http.Request {
	return httptest.NewRequest(http.MethodGet, "/t/abc?"+query, nil)
}

func TestParseParams_FullyPopulated(t *testing.T) {
	r := requestWithQuery("utm_source=fb&utm_medium=cpc&utm_campaign=summer&utm_content=ad1&utm_term=shoes" +
		"&sub1=a&sub2=b&sub3=c&sub4=d&sub5=e&sub6=f&sub7=g&sub8=h&sub9=i&sub10=j" +
		"&external_click_id=ext-1&fbclid=fb-1&ttclid=tt-1")

	p := parseParams(r)

	if p.utmSource != "fb" || p.utmMedium != "cpc" || p.utmCampaign != "summer" || p.utmContent != "ad1" || p.utmTerm != "shoes" {
		t.Fatalf("utm params not preserved: %+v", p)
	}
	want := [10]string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	if p.subs != want {
		t.Fatalf("subs = %v, want %v", p.subs, want)
	}
	if p.subs.SubCount() != 10 {
		t.Fatalf("SubCount() = %d, want 10", p.subs.SubCount())
	}
	if p.fbClickID != "fb-1" || p.ttClickID != "tt-1" {
		t.Fatalf("network click ids not preserved: %+v", p)
	}
	// Explicit external_click_id wins over the network-specific ones.
	if p.externalClickID != "ext-1" {
		t.Fatalf("externalClickID = %q, want %q", p.externalClickID, "ext-1")
	}
}

// TestParseParams_EmptyFacebookSubs is §42's "unfilled FB subs" rule: a
// portion of Facebook/Instagram clicks arrive with no subs at all. They
// must be persisted as empty — never a placeholder, never "unknown
// campaign" — and the count must be observable so buyers can see the
// share of subs-less traffic instead of it silently miscounting
// attribution.
func TestParseParams_EmptyFacebookSubs(t *testing.T) {
	r := requestWithQuery("fbclid=IwAR_abc123")

	p := parseParams(r)

	if p.subs != [10]string{} {
		t.Fatalf("expected all subs empty, got %v", p.subs)
	}
	if got := p.subs.SubCount(); got != 0 {
		t.Fatalf("SubCount() = %d, want 0 — the diagnostic must report zero, not hide it", got)
	}
	for i, s := range p.subs {
		if s != "" {
			t.Fatalf("sub%d = %q — missing subs must stay empty, not be filled in", i+1, s)
		}
	}
	// With no external_click_id, fbclid becomes the attributable external
	// id rather than the click being left unattributable.
	if p.externalClickID != "IwAR_abc123" {
		t.Fatalf("externalClickID = %q, want fbclid to be used as a fallback", p.externalClickID)
	}
}

func TestParseParams_PartialSubs(t *testing.T) {
	r := requestWithQuery("sub1=a&sub5=e")
	p := parseParams(r)

	if p.subs[0] != "a" || p.subs[4] != "e" {
		t.Fatalf("present subs not preserved: %v", p.subs)
	}
	if got := p.subs.SubCount(); got != 2 {
		t.Fatalf("SubCount() = %d, want 2", got)
	}
}

func TestParseParams_TTClidFallback(t *testing.T) {
	p := parseParams(requestWithQuery("ttclid=tt-only"))
	if p.externalClickID != "tt-only" {
		t.Fatalf("externalClickID = %q, want ttclid fallback", p.externalClickID)
	}
}

func TestApplyTo_MakesParamsFilterable(t *testing.T) {
	// Stream-set filters can target utm_*/sub1-10/referrer, so the
	// pass-through params must land in the same attribute map the
	// classifier populates, keyed by the same routing.FilterField values.
	p := parseParams(requestWithQuery("utm_source=fb&sub1=creative-7"))
	attrs := routing.Attributes{}
	p.applyTo(attrs)

	if attrs[routing.FieldUTMSource] != "fb" {
		t.Fatalf("utm_source not filterable: %q", attrs[routing.FieldUTMSource])
	}
	if attrs[routing.FieldSub1] != "creative-7" {
		t.Fatalf("sub1 not filterable: %q", attrs[routing.FieldSub1])
	}
	if _, ok := attrs[routing.FieldSub10]; !ok {
		t.Fatal("sub10 should be present (as empty), so NOT_EXISTS filters behave correctly")
	}
}
