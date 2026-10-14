package push_test

import (
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/push"
)

func TestNextOccurrence(t *testing.T) {
	at := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	daily := []int{1, 2, 3, 4, 5, 6, 7}
	for name, tc := range map[string]struct {
		rec   push.Recurrence
		after string
		want  string // "" = none
	}{
		// 2026-10-14 is a Wednesday; Moscow is UTC+3 all year.
		"later today":         {push.Recurrence{Weekdays: daily, Time: "19:00", Timezone: "Europe/Moscow"}, "2026-10-14T10:00:00Z", "2026-10-14T16:00:00Z"},
		"at the slot → next":  {push.Recurrence{Weekdays: daily, Time: "19:00", Timezone: "Europe/Moscow"}, "2026-10-14T16:00:00Z", "2026-10-15T16:00:00Z"},
		"next Monday":         {push.Recurrence{Weekdays: []int{1}, Time: "09:30", Timezone: "Europe/Moscow"}, "2026-10-14T10:00:00Z", "2026-10-19T06:30:00Z"},
		"Sunday is 7":         {push.Recurrence{Weekdays: []int{7}, Time: "12:00", Timezone: "UTC"}, "2026-10-14T10:00:00Z", "2026-10-18T12:00:00Z"},
		"same weekday, later": {push.Recurrence{Weekdays: []int{3}, Time: "08:00", Timezone: "UTC"}, "2026-10-14T10:00:00Z", "2026-10-21T08:00:00Z"},
		"until is inclusive":  {push.Recurrence{Weekdays: daily, Time: "19:00", Timezone: "Europe/Moscow", Until: "2026-10-14"}, "2026-10-14T10:00:00Z", "2026-10-14T16:00:00Z"},
		"past until":          {push.Recurrence{Weekdays: daily, Time: "19:00", Timezone: "Europe/Moscow", Until: "2026-10-14"}, "2026-10-14T16:00:00Z", ""},
		// Berlin leaves summer time on 2026-10-25: 09:00 is 07:00Z the day
		// before, 08:00Z that day.
		"autumn DST": {push.Recurrence{Weekdays: daily, Time: "09:00", Timezone: "Europe/Berlin"}, "2026-10-24T07:00:00Z", "2026-10-25T08:00:00Z"},
		// 02:30 does not exist on 2027-03-28 in Berlin: it moves forward.
		"spring gap": {push.Recurrence{Weekdays: daily, Time: "02:30", Timezone: "Europe/Berlin"}, "2027-03-27T12:00:00Z", "2027-03-28T01:30:00Z"},
	} {
		got, ok, err := push.NextOccurrence(tc.rec, at(tc.after))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if tc.want == "" {
			if ok {
				t.Errorf("%s: got %s, want none", name, got.UTC().Format(time.RFC3339))
			}
			continue
		}
		if !ok || !got.Equal(at(tc.want)) {
			t.Errorf("%s: got %s (%v), want %s", name, got.UTC().Format(time.RFC3339), ok, tc.want)
		}
	}
	if _, _, err := push.NextOccurrence(push.Recurrence{Weekdays: daily, Time: "25:00", Timezone: "UTC"}, time.Now()); err == nil {
		t.Error("bad time accepted")
	}
}
