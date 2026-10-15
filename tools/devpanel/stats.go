package main

import (
	"sort"
	"time"
)

// Stats is what the dashboard draws. Everything is derived from the items
// and their change log, so it needs no table of its own and is always in
// step with the board.
type Stats struct {
	Totals     Totals          `json:"totals"`
	Series     []DayPoint      `json:"series"`
	Weekly     []WeekPoint     `json:"weekly"`
	ByPriority []PriorityCount `json:"byPriority"`
	ByKind     []KindCount     `json:"byKind"`
	NextUp     []Item          `json:"nextUp"`
	Recent     []Activity      `json:"recent"`
}

type Totals struct {
	All       int `json:"all"`
	Open      int `json:"open"` // todo + doing + blocked
	Doing     int `json:"doing"`
	Blocked   int `json:"blocked"`
	Done      int `json:"done"`
	Wontfix   int `json:"wontfix"`
	P0Open    int `json:"p0Open"`
	DoneLast7 int `json:"doneLast7"`
}

// DayPoint is the board at the end of one local day.
type DayPoint struct {
	Day    string `json:"day"` // YYYY-MM-DD
	Open   int    `json:"open"`
	P0Open int    `json:"p0Open"`
	Done   int    `json:"done"`
}

// WeekPoint counts items moved to done during one Monday-based week.
type WeekPoint struct {
	WeekStart string `json:"weekStart"`
	Done      int    `json:"done"`
}

type PriorityCount struct {
	Priority string `json:"priority"`
	Open     int    `json:"open"`
	Done     int    `json:"done"`
	Total    int    `json:"total"` // without wontfix
}

type KindCount struct {
	Kind string `json:"kind"`
	Open int    `json:"open"`
}

type Activity struct {
	ItemID string `json:"itemId"`
	Code   string `json:"code"`
	Title  string `json:"title"`
	Field  string `json:"field"`
	Old    string `json:"old"`
	New    string `json:"new"`
	At     string `json:"at"`
}

func isOpen(status string) bool { return status == "todo" || status == "doing" || status == "blocked" }

var (
	priorityRank = map[string]int{"P0": 0, "P1": 1, "P2": 2, "P3": 3}
	severityRank = map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
)

// kindOrder is the chart's fixed order of kinds (the board's own
// vocabulary), so a bar never moves because another one grew.
var kindOrder = []string{"security", "data", "ux", "bug", "gap", "ops", "risk", "business"}

// computeStats builds the dashboard for the `days` local days ending today.
// changes may be in any order; only status changes affect history.
func computeStats(items []Item, changes []Change, now time.Time, days int, loc *time.Location) Stats {
	if days < 1 {
		days = 1
	}
	now = now.In(loc)
	byItem := map[string][]statusAt{}
	for _, c := range changes {
		if c.Field != "status" {
			continue
		}
		at, err := time.Parse(time.RFC3339, c.At)
		if err != nil {
			continue
		}
		byItem[c.ItemID] = append(byItem[c.ItemID], statusAt{at: at, old: c.Old, new: c.New})
	}
	for id := range byItem {
		sort.SliceStable(byItem[id], func(i, j int) bool { return byItem[id][i].at.Before(byItem[id][j].at) })
	}

	var s Stats
	weekAgo := now.Add(-7 * 24 * time.Hour)

	// Totals, priorities and areas: the board as it is now.
	prio := map[string]*PriorityCount{}
	for _, p := range []string{"P0", "P1", "P2", "P3"} {
		prio[p] = &PriorityCount{Priority: p}
	}
	kinds := map[string]int{}
	var open []Item
	for _, it := range items {
		s.Totals.All++
		switch it.Status {
		case "doing":
			s.Totals.Doing++
		case "blocked":
			s.Totals.Blocked++
		case "done":
			s.Totals.Done++
		case "wontfix":
			s.Totals.Wontfix++
		}
		pc := prio[it.Priority]
		if pc == nil {
			pc = &PriorityCount{Priority: it.Priority}
			prio[it.Priority] = pc
		}
		if it.Status != "wontfix" {
			pc.Total++
		}
		if it.Status == "done" {
			pc.Done++
		}
		if isOpen(it.Status) {
			s.Totals.Open++
			pc.Open++
			if it.Priority == "P0" {
				s.Totals.P0Open++
			}
			kinds[it.Kind]++
			open = append(open, it)
		}
		// Closed this week: done now, and moved to done within 7 days.
		for _, ch := range byItem[it.ID] {
			if it.Status == "done" && ch.new == "done" && ch.at.After(weekAgo) {
				s.Totals.DoneLast7++
				break
			}
		}
	}
	for _, p := range []string{"P0", "P1", "P2", "P3"} {
		s.ByPriority = append(s.ByPriority, *prio[p])
	}

	for _, k := range kindOrder {
		s.ByKind = append(s.ByKind, KindCount{Kind: k, Open: kinds[k]})
		delete(kinds, k)
	}
	for k, n := range kinds { // a kind outside the vocabulary, if any
		s.ByKind = append(s.ByKind, KindCount{Kind: k, Open: n})
	}

	// Next up: what is being worked on, then the most urgent open items.
	sort.SliceStable(open, func(i, j int) bool {
		a, b := open[i], open[j]
		if (a.Status == "doing") != (b.Status == "doing") {
			return a.Status == "doing"
		}
		if (a.Status == "blocked") != (b.Status == "blocked") {
			return b.Status == "blocked"
		}
		if priorityRank[a.Priority] != priorityRank[b.Priority] {
			return priorityRank[a.Priority] < priorityRank[b.Priority]
		}
		if severityRank[a.Severity] != severityRank[b.Severity] {
			return severityRank[a.Severity] < severityRank[b.Severity]
		}
		return a.Order < b.Order
	})
	if len(open) > 6 {
		open = open[:6]
	}
	s.NextUp = append([]Item{}, open...)

	// History: the board at the end of each day.
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	for d := days - 1; d >= 0; d-- {
		day := today.AddDate(0, 0, -d)
		end := day.AddDate(0, 0, 1)
		p := DayPoint{Day: day.Format("2006-01-02")}
		for _, it := range items {
			created, err := time.Parse(time.RFC3339, it.CreatedAt)
			if err == nil && !created.Before(end) {
				continue
			}
			st := statusBefore(it, byItem[it.ID], end)
			switch {
			case isOpen(st):
				p.Open++
				if it.Priority == "P0" {
					p.P0Open++
				}
			case st == "done":
				p.Done++
			}
		}
		s.Series = append(s.Series, p)
	}

	// Done per week, the last 8 weeks, Monday first.
	monday := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
	for w := 7; w >= 0; w-- {
		start := monday.AddDate(0, 0, -7*w)
		end := start.AddDate(0, 0, 7)
		n := 0
		for _, chs := range byItem {
			for _, ch := range chs {
				if ch.new == "done" && !ch.at.Before(start) && ch.at.Before(end) {
					n++
				}
			}
		}
		s.Weekly = append(s.Weekly, WeekPoint{WeekStart: start.Format("2006-01-02"), Done: n})
	}
	return s
}

type statusAt struct {
	at       time.Time
	old, new string
}

// statusBefore is it's status just before t: the last change before t, or —
// with no change yet — what the first change moved it away from (the status
// it was created with). An item never changed has had its status all along.
func statusBefore(it Item, chs []statusAt, t time.Time) string {
	if len(chs) == 0 {
		return it.Status
	}
	st := chs[0].old
	for _, ch := range chs {
		if !ch.at.Before(t) {
			break
		}
		st = ch.new
	}
	return st
}
