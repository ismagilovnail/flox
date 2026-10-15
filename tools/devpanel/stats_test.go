package main

import (
	"testing"
	"time"
)

var tz = time.FixedZone("UTC+5", 5*3600)

func at(day, hour int) string {
	return time.Date(2026, 10, day, hour, 0, 0, 0, tz).Format(time.RFC3339)
}

func TestComputeStatsHistory(t *testing.T) {
	// Three items created on the 10th. A: todo → doing (12th) → done (14th).
	// B: P0, todo all along. C: created as blocked, → wontfix on the 13th.
	items := []Item{
		{ID: "A", Code: "A-1", Priority: "P1", Severity: "high", Area: "web", Status: "done", CreatedAt: at(10, 9)},
		{ID: "B", Code: "B-1", Priority: "P0", Severity: "critical", Area: "web", Status: "todo", CreatedAt: at(10, 9)},
		{ID: "C", Code: "C-1", Priority: "P2", Severity: "low", Area: "", Status: "wontfix", CreatedAt: at(10, 9)},
		// D appears only on the 13th.
		{ID: "D", Code: "D-1", Priority: "P0", Severity: "high", Area: "tracker", Status: "doing", CreatedAt: at(13, 10)},
	}
	changes := []Change{
		{ItemID: "A", Field: "status", Old: "doing", New: "done", At: at(14, 15)}, // out of order on purpose
		{ItemID: "A", Field: "status", Old: "todo", New: "doing", At: at(12, 11)},
		{ItemID: "A", Field: "notes", Old: "", New: "x", At: at(12, 12)}, // not a status change
		{ItemID: "C", Field: "status", Old: "blocked", New: "wontfix", At: at(13, 8)},
		{ItemID: "D", Field: "status", Old: "todo", New: "doing", At: at(13, 11)},
	}
	now := time.Date(2026, 10, 15, 20, 0, 0, 0, tz)
	st := computeStats(items, changes, now, 7, tz)

	want := map[string][3]int{ // day → open, p0Open, done
		"2026-10-09": {0, 0, 0},
		"2026-10-10": {3, 1, 0}, // A todo, B, C blocked
		"2026-10-11": {3, 1, 0},
		"2026-10-12": {3, 1, 0}, // A doing
		"2026-10-13": {3, 2, 0}, // C gone (wontfix), D arrives
		"2026-10-14": {2, 2, 1}, // A done
		"2026-10-15": {2, 2, 1},
	}
	if len(st.Series) != 7 {
		t.Fatalf("series: %d days, want 7", len(st.Series))
	}
	for _, p := range st.Series {
		w, ok := want[p.Day]
		if !ok {
			t.Fatalf("unexpected day %s", p.Day)
		}
		if got := [3]int{p.Open, p.P0Open, p.Done}; got != w {
			t.Errorf("%s: open/p0/done = %v, want %v", p.Day, got, w)
		}
	}

	tot := st.Totals
	if tot.All != 4 || tot.Open != 2 || tot.Doing != 1 || tot.Done != 1 || tot.Wontfix != 1 || tot.P0Open != 2 || tot.DoneLast7 != 1 {
		t.Errorf("totals = %+v", tot)
	}
	if st.NextUp[0].ID != "D" {
		t.Errorf("next up starts with %s, want D (in progress first)", st.NextUp[0].ID)
	}
	p1 := st.ByPriority[1]
	if p1.Priority != "P1" || p1.Done != 1 || p1.Total != 1 || p1.Open != 0 {
		t.Errorf("P1 = %+v", p1)
	}
	if p2 := st.ByPriority[2]; p2.Total != 0 { // wontfix is out of the total
		t.Errorf("P2 total = %d, want 0", p2.Total)
	}
	var weekDone int
	for _, w := range st.Weekly {
		weekDone += w.Done
	}
	if len(st.Weekly) != 8 || weekDone != 1 || st.Weekly[7].WeekStart != "2026-10-12" {
		t.Errorf("weekly = %+v", st.Weekly)
	}
}

func TestComputeStatsDoneLast7NeedsDoneNow(t *testing.T) {
	// Closed, then reopened: not "closed this week".
	items := []Item{{ID: "A", Priority: "P1", Status: "todo", CreatedAt: at(1, 9)}}
	changes := []Change{
		{ItemID: "A", Field: "status", Old: "todo", New: "done", At: at(14, 9)},
		{ItemID: "A", Field: "status", Old: "done", New: "todo", At: at(14, 10)},
	}
	st := computeStats(items, changes, time.Date(2026, 10, 15, 12, 0, 0, 0, tz), 7, tz)
	if st.Totals.DoneLast7 != 0 {
		t.Errorf("doneLast7 = %d, want 0", st.Totals.DoneLast7)
	}
}

func TestComputeStatsByKindFixedOrder(t *testing.T) {
	items := []Item{
		{ID: "a", Priority: "P1", Status: "todo", Kind: "ops", CreatedAt: at(1, 9)},
		{ID: "b", Priority: "P1", Status: "doing", Kind: "ops", CreatedAt: at(1, 9)},
		{ID: "c", Priority: "P1", Status: "done", Kind: "security", CreatedAt: at(1, 9)},
	}
	st := computeStats(items, nil, time.Date(2026, 10, 15, 12, 0, 0, 0, tz), 7, tz)
	if len(st.ByKind) != len(kindOrder) {
		t.Fatalf("kinds = %d, want %d", len(st.ByKind), len(kindOrder))
	}
	for i, k := range kindOrder {
		if st.ByKind[i].Kind != k {
			t.Errorf("kind %d = %s, want %s", i, st.ByKind[i].Kind, k)
		}
	}
	if st.ByKind[0].Open != 0 || st.ByKind[5].Kind != "ops" || st.ByKind[5].Open != 2 {
		t.Errorf("byKind = %+v", st.ByKind)
	}
}
