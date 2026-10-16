package conversion_test

import (
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	pbstore "github.com/ismagilovnail/flox/apps/internal/postback"
)

// Audit 2026-10-15, INT-1: a conversion attributed to a click from a traffic
// source with a postback queues that postback — for the source's statuses
// only, with the source's own click id the click carried — in the same
// queue as a network's outgoing postback. Another organization's source is
// never called.
func TestConversionQueuesThePostbackToItsTrafficSource(t *testing.T) {
	pool := mustPool(t)
	c := ctx()
	orgID := seedOrg(t, c, pool)
	networkID := seedNetwork(t, c, pool, orgID, false)
	sourceID, otherSource := idgen.New(), idgen.New()
	otherOrg := seedOrg(t, c, pool)
	if _, err := pool.Exec(c, `INSERT INTO traffic_sources (id, organization_id, name, type, postback_url, postback_statuses)
		VALUES ($1, $2, 'Propeller', 'Push', 'https://ad.propellerads.com/conversion.php?aid=1&tid=2&visitor_id={external_click_id}&payout={revenue}', '{CPA_ACCEPT}'),
		       ($3, $4, 'Other org source', 'Push', 'https://other.example/pb?c={external_click_id}', '{CPA_HOLD,CPA_ACCEPT}')`,
		sourceID, orgID, otherSource, otherOrg); err != nil {
		t.Fatal(err)
	}

	mapper := newFakeMapper()
	mapper.set(networkID, "reg", event.CpaHold)
	mapper.set(networkID, "ftd", event.CpaAccept)
	clickID := idgen.New()
	attr := &fakeAttribution{outcome: attribution.OutcomeAttributed, click: attribution.Click{
		ClickID: clickID, OrganizationID: orgID, CampaignID: "c", TrafficSourceID: sourceID,
		ExternalClickID: "prop-visitor-77", OccurredAt: time.Now().Add(-time.Hour).UTC(),
	}}
	svc := conversion.NewService(mapper, conversion.NewPostgresStore(pool).WithDeliveries(pbstore.NewPostgresStore(pool)),
		&fakeFX{}, attr, &fakeAttempts{}).WithSourcePostbacks(conversion.NewPostgresSourcePostbacks(pool))

	deliveries := func() []string {
		t.Helper()
		rows, err := pool.Query(c, `SELECT status || ' ' || url FROM postback_deliveries
			WHERE organization_id = $1 AND traffic_source_id = $2 ORDER BY created_at`, orgID, sourceID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out = append(out, s)
		}
		return out
	}
	amount := 40.0
	for _, raw := range []string{"reg", "ftd"} {
		if res, err := svc.Record(c, conversion.Postback{OrganizationID: orgID, NetworkID: networkID, ClickID: clickID, RawStatus: raw,
			Revenue: &amount, Currency: "USD", OccurredAt: time.Now().UTC()}); err != nil || res.Kind != conversion.ResultSuccess {
			t.Fatalf("Record(%s) = %+v, %v", raw, res, err)
		}
	}
	got := deliveries()
	want := "CPA_ACCEPT https://ad.propellerads.com/conversion.php?aid=1&tid=2&visitor_id=prop-visitor-77&payout=40"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("source deliveries = %q; want only the accept: %q", got, want)
	}

	// The click names another organization's source: nothing is sent.
	attr.click.TrafficSourceID = otherSource
	attr.click.ClickID = idgen.New()
	if _, err := svc.Record(c, conversion.Postback{OrganizationID: orgID, NetworkID: networkID, ClickID: attr.click.ClickID, RawStatus: "ftd",
		OccurredAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	var leaked int
	if err := pool.QueryRow(c, `SELECT count(*) FROM postback_deliveries WHERE traffic_source_id = $1`, otherSource).Scan(&leaked); err != nil || leaked != 0 {
		t.Fatalf("another organization's source got %d deliveries (%v)", leaked, err)
	}
}
