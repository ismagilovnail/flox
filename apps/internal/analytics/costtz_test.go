package analytics_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/analytics"
)

// The note names only the ad-account zones whose offset differs from the
// report's over the range (tech audit AN4).
func TestCostTimezonesNamesOnlyZonesWithAnotherOffset(t *testing.T) {
	src := &fakeCost{zones: []string{"America/Los_Angeles", "Europe/Berlin", "Europe/Paris", "Mars/Base"}}
	svc := analytics.NewService(nil, src)
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

	for _, tc := range []struct {
		name, tz string
		from, to time.Time
		want     []string
	}{
		// Berlin and Paris share an offset all year: no note for either.
		{"report in Berlin", "Europe/Berlin", day(2026, 7, 1), day(2026, 7, 31), []string{"America/Los_Angeles"}},
		{"report in UTC", "", day(2026, 1, 1), day(2026, 1, 31), []string{"America/Los_Angeles", "Europe/Berlin", "Europe/Paris"}},
		// London is UTC+0 in winter: every other zone differs.
		{"report in London, winter", "Europe/London", day(2026, 1, 5), day(2026, 1, 6), []string{"America/Los_Angeles", "Europe/Berlin", "Europe/Paris"}},
	} {
		got, err := svc.CostTimezones(context.Background(), "org", tc.from, tc.to, tc.tz)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}

	same := &fakeCost{zones: []string{"Europe/London"}}
	got, err := analytics.NewService(nil, same).CostTimezones(context.Background(), "org", day(2026, 3, 20), day(2026, 4, 2), "UTC")
	if err != nil || len(got) != 1 {
		t.Fatalf("London vs UTC across the March DST change = %v, %v; want London named", got, err)
	}
	got, err = analytics.NewService(nil, same).CostTimezones(context.Background(), "org", day(2026, 1, 1), day(2026, 1, 31), "UTC")
	if err != nil || len(got) != 0 {
		t.Fatalf("London vs UTC in January = %v, %v; want nothing", got, err)
	}
	if got, err := analytics.NewService(nil, nil).CostTimezones(context.Background(), "org", day(2026, 1, 1), day(2026, 1, 2), ""); err != nil || len(got) != 0 {
		t.Fatalf("no cost source = %v, %v; want empty", got, err)
	}
	if _, err := svc.CostTimezones(context.Background(), "org", day(2026, 1, 1), day(2026, 1, 2), "Nowhere/City"); err == nil {
		t.Fatal("an unknown report zone is not rejected")
	}
}
