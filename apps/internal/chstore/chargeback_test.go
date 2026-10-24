package chstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// Audit 2026-10-06, T4/AN3 — §45: a chargeback nets the accept's revenue
// out of the report, on the decline's own day, without changing the
// conversion counts.
func TestReportRevenueNetsChargebacks(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewRawEventStore(conn)
	ctx := context.Background()
	orgID, campaignID, clickID := idgen.New(), idgen.New(), idgen.New()
	ftdDay := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)
	cbDay := ftdDay.AddDate(0, 0, 3)

	cpa := func(typ event.Type, at time.Time, usd float64) event.Event {
		return event.Event{Type: typ, EventAt: at, OrganizationID: orgID, CampaignID: campaignID, ClickID: clickID,
			Revenue: usd, Currency: "USD", USDValue: usd, HasUSDValue: true}
	}
	if err := store.InsertBatch(ctx, []event.Event{
		cpa(event.CpaAccept, ftdDay, 25),
		cpa(event.CpaRedep, ftdDay.Add(time.Hour), 10),
		cpa(event.CpaDecline, cbDay, -25), // as conversion.PostgresStore writes a chargeback
	}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	report := func(from, to time.Time) chstore.ConversionAgg {
		t.Helper()
		aggs, err := store.ReportConversions(ctx, chstore.ReportQuery{OrganizationID: orgID, From: from, To: to})
		if err != nil || len(aggs) != 1 {
			t.Fatalf("ReportConversions: %v, %d rows", err, len(aggs))
		}
		return aggs[0]
	}

	all := report(ftdDay.Truncate(24*time.Hour), cbDay.Add(24*time.Hour))
	if all.RevenueUSD != 10 || all.Conversions != 2 || all.Purchases != 1 {
		t.Fatalf("whole period: revenue=%v conversions=%d purchases=%d, want 10 (the redeposit stays), 2, 1",
			all.RevenueUSD, all.Conversions, all.Purchases)
	}
	// The FTD's own day keeps what it showed; the decline's day shows the reversal.
	if day := report(ftdDay.Truncate(24*time.Hour), ftdDay.Truncate(24*time.Hour).Add(24*time.Hour)); day.RevenueUSD != 35 {
		t.Fatalf("FTD day revenue = %v, want 35 (closed periods are not rewritten)", day.RevenueUSD)
	}
	if day := report(cbDay.Truncate(24*time.Hour), cbDay.Truncate(24*time.Hour).Add(24*time.Hour)); day.RevenueUSD != -25 || day.Conversions != 0 {
		t.Fatalf("decline day: revenue=%v conversions=%d, want -25 and 0", day.RevenueUSD, day.Conversions)
	}

	var ltvRows uint64
	if err := conn.QueryRow(ctx, `SELECT count() FROM ltv_events WHERE organization_id = ? AND type = 'CPA_DECLINE'`, orgID).Scan(&ltvRows); err != nil {
		t.Fatalf("counting ltv_events: %v", err)
	}
	if ltvRows != 1 {
		t.Fatalf("ltv_events declines = %d, want 1 (the view must carry chargebacks)", ltvRows)
	}
}
