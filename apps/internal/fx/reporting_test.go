package fx_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/fx"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

func on(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }

func TestNewConversionFillsDays(t *testing.T) {
	// Friday 2 Oct and Monday 5 Oct published; nothing after.
	rates := []fx.Rate{{Date: on(10, 2), RateToUSD: 1.25}, {Date: on(10, 5), RateToUSD: 0.5}}
	c := fx.NewConversion("EUR", rates, on(10, 1), on(10, 13))

	for _, tc := range []struct {
		at   time.Time
		want float64
		ok   bool
	}{
		{on(10, 1), 0, false},                      // before the first rate
		{on(10, 2).Add(23 * time.Hour), 0.8, true}, // 1 / 1.25
		{on(10, 4), 0.8, true},                     // weekend: Friday's rate
		{on(10, 5), 2, true},
		{on(10, 11), 2, true},  // 6 days old: still in force
		{on(10, 12), 0, false}, // 7 days old: stale (fx.MaxStaleDays)
	} {
		f, ok := c.Factor(tc.at)
		if ok != tc.ok || (ok && f != tc.want) {
			t.Errorf("Factor(%s) = %v, %v; want %v, %v", tc.at.Format(time.DateOnly), f, ok, tc.want, tc.ok)
		}
	}
	days, factors := c.Days()
	if len(days) != 10 || len(factors) != 10 || !days[0].Equal(on(10, 2)) {
		t.Fatalf("Days = %v / %v, want 2..11 Oct", days, factors)
	}
	if c.Identity() || c.Code() != "EUR" {
		t.Fatalf("Identity/Code = %v/%s", c.Identity(), c.Code())
	}
	if !fx.USD.Identity() || fx.USD.Code() != "USD" {
		t.Fatal("fx.USD is not the identity")
	}
	if f, ok := fx.USD.Factor(on(1, 1)); !ok || f != 1 {
		t.Fatalf("USD factor = %v, %v", f, ok)
	}
}

func TestReportingConversion(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	store := fx.NewStore(pool)
	const cur = "XXX"

	orgID := idgen.New()
	mustExec(t, pool, `INSERT INTO organizations (id, name, currency) VALUES ($1, 'fx reporting test', $2)`, orgID, cur)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM fx_rates WHERE currency = $1`, cur)
	})
	// The rate in force on the range's first day was published before it.
	if err := store.Upsert(ctx, []fx.Rate{{Currency: cur, Date: on(9, 28), RateToUSD: 4}, {Currency: cur, Date: on(10, 2), RateToUSD: 2}}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	c, err := store.ReportingConversion(ctx, orgID, on(10, 1), on(10, 3))
	if err != nil {
		t.Fatalf("ReportingConversion: %v", err)
	}
	if c.Code() != cur {
		t.Fatalf("Code = %s", c.Code())
	}
	for at, want := range map[time.Time]float64{on(10, 1): 0.25, on(10, 3): 0.5} {
		if f, ok := c.Factor(at); !ok || f != want {
			t.Errorf("Factor(%s) = %v, %v; want %v", at.Format(time.DateOnly), f, ok, want)
		}
	}

	usdOrg := idgen.New()
	mustExec(t, pool, `INSERT INTO organizations (id, name) VALUES ($1, 'fx reporting usd')`, usdOrg)
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, usdOrg) })
	if c, err := store.ReportingConversion(ctx, usdOrg, on(10, 1), on(10, 3)); err != nil || !c.Identity() {
		t.Fatalf("USD org = %+v, %v, want the identity", c, err)
	}
}
