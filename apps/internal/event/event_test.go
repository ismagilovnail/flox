package event_test

import (
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/event"
)

// TestModelIsComplete guards CLAUDE.md non-negotiable #2 ("full event
// model, never truncated") against a future edit that quietly drops a
// type. The expected list is transcribed straight from §43 — if this test
// and event.All disagree, one of them changed and it needs to be a
// deliberate, reviewed decision, not an accident.
func TestModelIsComplete(t *testing.T) {
	want := []event.Type{
		"SOURCE_CLICK", "SOURCE_FILTER",
		"LAND_VIEW", "LAND_CLICK", "POSTLANDING_VIEW", "POSTLANDING_CLICK",
		"PWA_VIEW", "PWA_OPEN", "PWA_INSTALL", "IOS_INSTALL",
		"NOTIFICATION_REQUEST", "NOTIFICATION_SUBSCRIBE", "NOTIFICATION_DECLINE",
		"NOTIFICATION_UNSUBSCRIBE", "NOTIFICATION_CLICK",
		"TG_JOIN", "TG_START",
		"CPA_HOLD", "CPA_ACCEPT", "CPA_REDEP", "CPA_DECLINE", "CPA_TRASH",
	}

	if len(event.All) != len(want) {
		t.Fatalf("event.All has %d types, §43 defines %d", len(event.All), len(want))
	}
	got := map[event.Type]bool{}
	for _, e := range event.All {
		got[e] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("§43 event type %q missing from event.All", w)
		}
	}
}

func TestCPAStatusesAreDistinct(t *testing.T) {
	// The five CPA statuses must each be their own type — never collapsed
	// into one "conversion" (CLAUDE.md non-negotiable #2).
	cpa := []event.Type{event.CpaHold, event.CpaAccept, event.CpaRedep, event.CpaDecline, event.CpaTrash}
	seen := map[event.Type]bool{}
	for _, c := range cpa {
		if seen[c] {
			t.Fatalf("duplicate CPA status %q", c)
		}
		seen[c] = true
		if !c.IsCPA() {
			t.Errorf("%q.IsCPA() = false, want true", c)
		}
	}
	if event.SourceClick.IsCPA() {
		t.Error("SOURCE_CLICK.IsCPA() = true, want false")
	}
	if event.Type("CONVERSION").Valid() {
		t.Error(`a generic "CONVERSION" type must not exist in the model`)
	}
}

// TestEventClassificationIsExhaustiveAndDisjoint guards §48's three-way
// split (click_events/tracking_events/conversion_events): every type in
// the model must land in exactly one of IsClick()/IsCPA()/neither, so the
// ClickHouse ingestion router (internal/chstore) never silently drops a
// type or double-counts it into two tables.
func TestEventClassificationIsExhaustiveAndDisjoint(t *testing.T) {
	for _, ty := range event.All {
		isClick := ty.IsClick()
		isCPA := ty.IsCPA()
		if isClick && isCPA {
			t.Fatalf("%q is both IsClick() and IsCPA() — must be at most one", ty)
		}
	}
	if !event.SourceClick.IsClick() || !event.SourceFilter.IsClick() {
		t.Error("SOURCE_CLICK and SOURCE_FILTER must both be IsClick()")
	}
	if event.LandView.IsClick() || event.LandView.IsCPA() {
		t.Error("LAND_VIEW must be neither IsClick() nor IsCPA() (it's a tracking_events row)")
	}
}

func TestSubCount(t *testing.T) {
	cases := []struct {
		name string
		subs event.Subs
		want int
	}{
		{"all empty (the Facebook in-app WebView case)", event.Subs{}, 0},
		{"partially filled", event.Subs{"a", "", "c"}, 2},
		{"all filled", event.Subs{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.subs.SubCount(); got != tc.want {
				t.Fatalf("SubCount() = %d, want %d", got, tc.want)
			}
		})
	}
}
