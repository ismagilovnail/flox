package push

import (
	"fmt"
	"slices"
	"time"
)

// clock parses Recurrence.Time.
func parseClock(s string) (hour, minute int, err error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, 0, fmt.Errorf("push: time %q: want HH:MM", s)
	}
	return t.Hour(), t.Minute(), nil
}

// isoWeekday is time.Weekday as ISO 8601 (Monday 1 … Sunday 7).
func isoWeekday(d time.Weekday) int {
	if d == time.Sunday {
		return 7
	}
	return int(d)
}

// NextOccurrence is rec's first slot strictly after after, and false when
// there is none on or before Until. A wall-clock time that does not exist
// that day (the spring DST gap) moves forward as time.Date normalizes it;
// one that happens twice (autumn) is its first.
func NextOccurrence(rec Recurrence, after time.Time) (time.Time, bool, error) {
	loc, err := time.LoadLocation(rec.Timezone)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("push: timezone %q: %w", rec.Timezone, err)
	}
	hour, minute, err := parseClock(rec.Time)
	if err != nil {
		return time.Time{}, false, err
	}
	var until time.Time
	if rec.Until != "" {
		if until, err = time.ParseInLocation(time.DateOnly, rec.Until, loc); err != nil {
			return time.Time{}, false, fmt.Errorf("push: until %q: want YYYY-MM-DD", rec.Until)
		}
	}
	local := after.In(loc)
	// Eight days covers every weekday set; the first day may already be past.
	for i := 0; i <= 7; i++ {
		day := time.Date(local.Year(), local.Month(), local.Day()+i, 0, 0, 0, 0, loc)
		if !slices.Contains(rec.Weekdays, isoWeekday(day.Weekday())) {
			continue
		}
		slot := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, loc)
		if !slot.After(after) {
			continue
		}
		if !until.IsZero() && day.After(until) {
			return time.Time{}, false, nil
		}
		return slot, true, nil
	}
	return time.Time{}, false, nil // no weekdays: validation keeps this out
}
