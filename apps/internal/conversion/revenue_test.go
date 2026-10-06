package conversion_test

import (
	"errors"
	"math"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
)

func TestParseRevenue(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want float64
		ok   bool
	}{
		{"12.5", 12.5, true},
		{"0", 0, true},
		{" 3 ", 3, true},
		{"1e9", 1e9, true},
		{"NaN", 0, false},
		{"nan", 0, false},
		{"Inf", 0, false},
		{"+Inf", 0, false},
		{"-Infinity", 0, false},
		{"-1", 0, false},
		{"-0.01", 0, false},
		{"1e10", 0, false},
		{"1e400", 0, false}, // overflows to +Inf in ParseFloat
		{"abc", 0, false},
		{"", 0, false},
	} {
		got, err := conversion.ParseRevenue(tc.raw)
		if tc.ok {
			if err != nil || got != tc.want {
				t.Errorf("ParseRevenue(%q) = %v, %v; want %v, nil", tc.raw, got, err, tc.want)
			}
			continue
		}
		if !errors.Is(err, conversion.ErrInvalidRevenue) {
			t.Errorf("ParseRevenue(%q) = %v, %v; want ErrInvalidRevenue", tc.raw, got, err)
		}
	}
}

// Replay and any other caller bypass the HTTP handler's check; the service
// records the attempt as an error and never applies the amount.
func TestServiceRejectsNonFiniteRevenue(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1), -5, 2e10} {
		svc, mapper, store, events, _ := newHarness()
		mapper.set("net-1", "sale", event.CpaAccept)
		p := postback("net-1", "click-1", "sale")
		p.Revenue = &v

		result, err := svc.Record(ctx(), p)
		if err != nil {
			t.Fatalf("revenue %v: Record: %v", v, err)
		}
		if result.Kind != conversion.ResultError {
			t.Fatalf("revenue %v: kind = %q, want error", v, result.Kind)
		}
		if events.count() != 0 {
			t.Fatalf("revenue %v: a CPA event was emitted", v)
		}
		if _, ok, _ := store.LastStatus(ctx(), orgA, "click-1"); ok {
			t.Fatalf("revenue %v: a success was recorded", v)
		}
	}
}
