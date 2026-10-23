package billing

import (
	"testing"
	"time"
)

// What the billing page says about raw clicks (audit DATA-6): the months
// kept now, and a shorter period the plan brings with the date it applies.
func TestRawRetention(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	wait := 30 * 24 * time.Hour
	free := Subscription{Plan: Plan{RawRetentionMonths: 3}}
	business := Subscription{Plan: Plan{RawRetentionMonths: 24}}
	since := now.AddDate(0, 0, -10)
	for _, c := range []struct {
		name       string
		sub        Subscription
		months     int
		shortensTo int
		at         time.Time
	}{
		{"not applied yet, Business", business, 24, 0, time.Time{}},
		{"not applied yet, Free: 24 until the wait from now", free, 24, 3, now.Add(wait)},
		{"Free, waiting since 10 days", Subscription{Plan: free.Plan, Retention: &RetentionState{Months: 24, Target: 3, TargetSince: since}}, 24, 3, since.Add(wait)},
		{"Free, applied", Subscription{Plan: free.Plan, Retention: &RetentionState{Months: 3, Target: 3}}, 3, 0, time.Time{}},
		{"upgraded, applying within the hour", Subscription{Plan: business.Plan, Retention: &RetentionState{Months: 3, Target: 24}}, 3, 0, time.Time{}},
	} {
		months, change := rawRetention(c.sub, now, wait)
		if months != c.months {
			t.Errorf("%s: months %d, want %d", c.name, months, c.months)
		}
		switch {
		case c.shortensTo == 0 && change != nil:
			t.Errorf("%s: shortens to %+v, want nothing", c.name, *change)
		case c.shortensTo != 0 && (change == nil || change.Months != c.shortensTo || !change.At.Equal(c.at)):
			t.Errorf("%s: shortens to %+v, want %d at %v", c.name, change, c.shortensTo, c.at)
		}
	}
}
