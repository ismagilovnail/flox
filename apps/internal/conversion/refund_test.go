package conversion_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// Audit UNI-4 (§45 amended): a refund carrying a repeat purchase's txn_id
// reverses that repeat purchase, not the first one; refund_amount reverses
// part of a purchase, never more than is left; replays stay duplicates; a
// refund of one order does not make the click a declined one. On the real
// store, under the click's lock.
func TestRefundsByOrder(t *testing.T) {
	pool := mustPool(t)
	c := ctx()
	orgID := seedOrg(t, c, pool)
	networkID := seedNetwork(t, c, pool, orgID, false)
	mapper := newFakeMapper()
	mapper.set(networkID, "purchase", event.CpaAccept)
	mapper.set(networkID, "repeat_purchase", event.CpaRedep)
	mapper.set(networkID, "refund", event.CpaDecline)
	svc := conversion.NewService(mapper, conversion.NewPostgresStore(pool), &fakeFX{},
		&fakeAttribution{outcome: attribution.OutcomeUnknownClick}, &fakeAttempts{})

	clickID := idgen.New()
	type opt struct {
		revenue, refund *float64
		currency        string
		refundID        string
	}
	f := func(v float64) *float64 { return &v }
	step := func(raw, txn string, o opt, want conversion.ResultKind, wantInMessage string) {
		t.Helper()
		currency := o.currency
		if currency == "" && o.revenue != nil {
			currency = "USD"
		}
		res, err := svc.Record(c, conversion.Postback{
			OrganizationID: orgID, NetworkID: networkID, ClickID: clickID, RawStatus: raw,
			Revenue: o.revenue, Currency: currency, NetworkTxnID: txn, RefundAmount: o.refund, RefundID: o.refundID,
			OccurredAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("%s txn %q: %v", raw, txn, err)
		}
		if res.Kind != want || !strings.Contains(res.Message, wantInMessage) {
			t.Fatalf("%s txn %q %+v = %s %q; want %s mentioning %q", raw, txn, o, res.Kind, res.Message, want, wantInMessage)
		}
	}

	step("purchase", "A-1", opt{revenue: f(1000)}, conversion.ResultSuccess, "")
	step("repeat_purchase", "A-2", opt{revenue: f(400)}, conversion.ResultSuccess, "")
	step("repeat_purchase", "A-3", opt{revenue: f(300)}, conversion.ResultSuccess, "")

	// A repeat purchase's refund takes its own payout back, once.
	step("refund", "A-2", opt{}, conversion.ResultSuccess, "reverses the CPA_REDEP payout 400 USD (txn_id A-2)")
	step("refund", "A-2", opt{}, conversion.ResultDuplicate, "")
	// Part of another: 100, a replay of it, then a second refund (its own
	// id) capped at what is left, then nothing left.
	step("refund", "A-3", opt{refund: f(100)}, conversion.ResultSuccess, "reverses 100 USD of the CPA_REDEP payout 300 USD")
	step("refund", "A-3", opt{refund: f(100)}, conversion.ResultDuplicate, "")
	step("refund", "A-3", opt{refund: f(500), refundID: "R2"}, conversion.ResultSuccess, "reverses 200 USD of the CPA_REDEP payout 300 USD")
	step("refund", "A-3", opt{refund: f(10), refundID: "R3"}, conversion.ResultSuccess, "already refunded in full")

	// The click still stands accepted: a replay of the purchase is a
	// duplicate, not a re-approval; the first purchase can be refunded
	// in part and then in full — the full refund takes what is left.
	step("purchase", "A-1", opt{revenue: f(1000)}, conversion.ResultDuplicate, "")
	step("refund", "", opt{refund: f(250)}, conversion.ResultSuccess, "reverses 250 USD of the CPA_ACCEPT payout 1000 USD")
	// An amount in another currency than the purchase's is refused.
	step("refund", "A-1", opt{refund: f(1), currency: "EUR", refundID: "R9"}, conversion.ResultError, "in the purchase's currency, USD")
	step("refund", "A-1", opt{}, conversion.ResultSuccess, "reverses 750 USD of the CPA_ACCEPT payout 1000 USD")
	step("refund", "", opt{}, conversion.ResultDuplicate, "")

	// An amount on a purchase is refused.
	step("purchase", "A-9", opt{revenue: f(1), refund: f(1)}, conversion.ResultError, "refund_amount is for a refund")

	// Every purchase nets to zero; each reversal points at its purchase.
	var gross, reversed float64
	var unlinked int
	if err := pool.QueryRow(c, `
		SELECT coalesce(sum(usd_value) FILTER (WHERE status IN ('CPA_ACCEPT', 'CPA_REDEP')), 0),
		       coalesce(sum(reversal_usd_value), 0),
		       count(*) FILTER (WHERE reversal_usd_value IS NOT NULL AND reverses_postback_id IS NULL)
		FROM postbacks WHERE organization_id = $1 AND click_id = $2 AND result = 'success'`,
		orgID, clickID).Scan(&gross, &reversed, &unlinked); err != nil {
		t.Fatal(err)
	}
	if gross != 1700 || reversed != 1700 || unlinked != 0 {
		t.Fatalf("gross %v, reversed %v, unlinked reversals %d; want 1700, 1700, 0", gross, reversed, unlinked)
	}
	var decisions int
	if err := pool.QueryRow(c, `
		SELECT count(*) FROM postbacks
		WHERE organization_id = $1 AND click_id = $2 AND result = 'success' AND status = 'CPA_DECLINE' AND refund_kind = ''`,
		orgID, clickID).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if decisions != 1 {
		t.Fatalf("declines that decide the click = %d, want only the full refund of the first purchase", decisions)
	}
}
