package conversion_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
)

func TestParseEventTime(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	at := time.Date(2026, 10, 8, 23, 30, 15, 0, time.UTC)
	for _, c := range []struct {
		raw  string
		want time.Time
	}{
		{"1791502215", at},                // unix seconds
		{"1791502215000", at},             // unix milliseconds
		{"2026-10-08T23:30:15Z", at},      // RFC 3339
		{"2026-10-09T04:30:15+05:00", at}, // with an offset
		{"2026-10-08T23:30:15.000Z", at},  // with fractions
		{"2026-10-08 23:30:15", at},       // a common network format, UTC
		{" 1791502215 ", at},              // padded
		{"2026-10-09T12:04:00Z", now},     // a clock 4 minutes ahead counts as now
		{"2026-07-12T12:00:00Z", now.Add(-89 * 24 * time.Hour)},
	} {
		got, err := conversion.ParseEventTime(c.raw, now)
		if err != nil || !got.Equal(c.want) || got.Location() != time.UTC {
			t.Errorf("ParseEventTime(%q) = %v, %v; want %v", c.raw, got, err, c.want)
		}
	}
	for _, raw := range []string{"", "yesterday", "{event_time}", "2026-10-09T12:06:00Z", "2026-07-01T00:00:00Z", "-5", "2026-13-01 00:00:00"} {
		if _, err := conversion.ParseEventTime(raw, now); !errors.Is(err, conversion.ErrInvalidEventTime) {
			t.Errorf("ParseEventTime(%q) err = %v, want ErrInvalidEventTime", raw, err)
		}
	}
}

// datedFX records the date it was asked for.
type datedFX struct{ at time.Time }

func (f *datedFX) ToUSD(_ context.Context, _ string, amount float64, at time.Time) (float64, bool, error) {
	f.at = at
	return amount * 1.1, true, nil
}

// The network's event time dates the conversion and its FX rate (tech
// audit T3): a nightly replay lands on the day it happened.
func TestEventTimeDatesTheConversionAndItsRate(t *testing.T) {
	mapper := newFakeMapper()
	mapper.set("net-1", "sale", event.CpaAccept)
	events := &fakeEvents{}
	store := newFakeStore(events, nil)
	fx := &datedFX{}
	clickAt := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	attr := &fakeAttribution{outcome: attribution.OutcomeAttributed, click: attribution.Click{ClickID: "click-1", OrganizationID: orgA, OccurredAt: clickAt}}
	svc := conversion.NewService(mapper, store, fx, attr, &fakeAttempts{})

	p := postback("net-1", "click-1", "sale")
	p.OccurredAt = time.Date(2026, 10, 3, 22, 0, 0, 0, time.UTC)
	rev := 100.0
	p.Revenue, p.Currency = &rev, "EUR"
	if _, err := svc.Record(ctx(), p); err != nil {
		t.Fatal(err)
	}
	if !fx.at.Equal(p.OccurredAt) {
		t.Errorf("FX date = %v, want the event's %v", fx.at, p.OccurredAt)
	}
	ev := events.last()
	if !ev.EventAt.Equal(p.OccurredAt) || ev.TimeToConversionMS != (61*time.Hour).Milliseconds() {
		t.Errorf("event at %v ttc %d, want %v and 61h", ev.EventAt, ev.TimeToConversionMS, p.OccurredAt)
	}
}

// An event time before the click is kept as sent: the negative time to
// conversion it gives is the clock-skew signal docs/attribution.md keeps
// on purpose, never floored at zero.
func TestEventTimeBeforeTheClickIsKept(t *testing.T) {
	mapper := newFakeMapper()
	mapper.set("net-1", "sale", event.CpaAccept)
	events := &fakeEvents{}
	store := newFakeStore(events, nil)
	clickAt := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	attr := &fakeAttribution{outcome: attribution.OutcomeAttributed, click: attribution.Click{ClickID: "click-1", OrganizationID: orgA, OccurredAt: clickAt}}
	svc := conversion.NewService(mapper, store, &fakeFX{}, attr, &fakeAttempts{})

	p := postback("net-1", "click-1", "sale")
	p.OccurredAt = clickAt.Add(-time.Minute)
	if _, err := svc.Record(ctx(), p); err != nil {
		t.Fatal(err)
	}
	ev := events.last()
	if !ev.EventAt.Equal(p.OccurredAt) || ev.TimeToConversionMS != -time.Minute.Milliseconds() {
		t.Errorf("event at %v ttc %d, want %v and -60000", ev.EventAt, ev.TimeToConversionMS, p.OccurredAt)
	}
}
