package chstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// Audit 2026-10-15, DATA-1: revenue that got no USD value is missing from
// RevenueUSD; the report must say how many conversions that is, so the
// Overview can warn instead of showing a quietly lower figure.
func TestReportCountsUnpricedConversions(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	day := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)

	conv := func(typ event.Type, revenue, usd float64, priced bool) event.Event {
		return event.Event{Type: typ, EventAt: day, OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(),
			Revenue: revenue, Currency: "TRY", USDValue: usd, HasUSDValue: priced}
	}
	if err := store.InsertBatch(ctx, []event.Event{
		conv(event.CpaAccept, 25, 25, true),   // priced
		conv(event.CpaRedep, 1000, 0, false),  // unpriced: counts
		conv(event.CpaAccept, 50, 0, false),   // unpriced: counts
		conv(event.CpaAccept, 0, 0, false),    // no money: not unpriced
		conv(event.CpaHold, 300, 0, false),    // a HOLD is not revenue yet
		conv(event.CpaDecline, -10, 0, false), // a reversal, not a conversion
	}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	from, to := day.Truncate(24*time.Hour), day.Truncate(24*time.Hour).Add(24*time.Hour)
	for _, q := range []struct {
		name string
		dims []chstore.ReportDimension
	}{
		{"plain", nil},
		{"joined to clicks", []chstore.ReportDimension{chstore.DimOS}},
	} {
		aggs, err := store.ReportConversions(ctx, chstore.ReportQuery{OrganizationID: orgID, From: from, To: to, Dims: q.dims})
		if err != nil {
			t.Fatalf("%s: ReportConversions: %v", q.name, err)
		}
		var unpriced uint64
		var revenue float64
		for _, a := range aggs {
			unpriced += a.Unpriced
			revenue += a.RevenueUSD
		}
		if unpriced != 2 {
			t.Errorf("%s: unpriced = %d, want 2", q.name, unpriced)
		}
		if revenue != 25 {
			t.Errorf("%s: revenue = %v, want 25 (only the priced accept)", q.name, revenue)
		}
	}
}
