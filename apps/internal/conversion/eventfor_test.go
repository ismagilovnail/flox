package conversion_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// The reconciliation rebuilds a lost conversion with EventFor from its
// postbacks row and its click. That event must be the one the record path
// queued — same fields, same reversal, same id — or a repair would write
// different numbers than the live path.
func TestEventForRebuildsTheQueuedEvent(t *testing.T) {
	pool := mustPool(t)
	c := ctx()
	orgID := seedOrg(t, c, pool)
	networkID := seedNetwork(t, c, pool, orgID, false)

	clickID := idgen.New()
	click := attribution.Click{
		ClickID: clickID, OrganizationID: orgID, CampaignID: idgen.New(), StreamSetID: "set", FlowID: "flow",
		LandingID: "lnd", OfferID: "off", PwaID: "pwa", TrafficSourceID: "src", Country: "DE", Region: "BE", City: "Berlin",
		Device: "mobile", Platform: "android", OS: "Android", UTMSource: "fb", UTMCampaign: "c1",
		Subs: event.Subs{"a", "b"}, ExternalClickID: "fbclid-1", OccurredAt: time.Now().Add(-time.Hour).UTC(),
	}
	mapper := newFakeMapper()
	mapper.set(networkID, "ftd", event.CpaAccept)
	mapper.set(networkID, "chargeback", event.CpaDecline)
	probe := &eventProbe{}
	svc := conversion.NewService(mapper, conversion.NewPostgresStore(pool).WithDeliveries(probe),
		&fakeFX{rates: map[string]float64{"EUR": 1.1}},
		&fakeAttribution{outcome: attribution.OutcomeAttributed, click: click}, &fakeAttempts{})

	for _, raw := range []string{"ftd", "chargeback"} {
		amount := 25.0
		p := conversion.Postback{
			OrganizationID: orgID, NetworkID: networkID, ClickID: clickID, RawStatus: raw,
			Revenue: &amount, Currency: "EUR", NetworkTxnID: "txn-" + raw,
			PostbackURL: "https://partner.example/pb?c={click_id}",
			OccurredAt:  time.Now().UTC().Truncate(time.Microsecond),
		}
		if _, err := svc.Record(c, p); err != nil {
			t.Fatalf("Record(%s): %v", raw, err)
		}
		queued := probe.last

		status := event.CpaAccept
		if raw == "chargeback" {
			status = event.CpaDecline
		}
		rows, err := conversion.NewPostgresStore(pool).StoredSuccesses(c, orgID, status, p.OccurredAt.Add(-time.Second), p.OccurredAt.Add(time.Second))
		if err != nil || len(rows) != 1 {
			t.Fatalf("StoredSuccesses(%s): %d rows, %v", status, len(rows), err)
		}
		rebuilt := conversion.EventFor(rows[0], &click)

		// Postgres keeps microseconds and USD to 4 decimals; compare on that.
		queued.EventAt, rebuilt.EventAt = queued.EventAt.UTC(), rebuilt.EventAt.UTC()
		queued.USDValue = roundTo4(queued.USDValue)
		rebuilt.USDValue = roundTo4(rebuilt.USDValue)
		if !reflect.DeepEqual(queued, rebuilt) {
			t.Fatalf("%s: rebuilt event differs from the queued one\nqueued:  %+v\nrebuilt: %+v", status, queued, rebuilt)
		}
		if status == event.CpaDecline && rebuilt.Revenue != -25 {
			t.Fatalf("rebuilt decline revenue = %v, want the reversal -25", rebuilt.Revenue)
		}
	}
}

func roundTo4(v float64) float64 {
	return float64(int64(v*10000+0.5*sign(v))) / 10000
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}
