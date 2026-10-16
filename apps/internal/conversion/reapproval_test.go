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

// Audit 2026-10-15, DATA-3 (§45 amended): a conversion accepted, declined
// and approved again after a dispute counts again — when the re-approval
// carries a txn_id no earlier accept of the click carried. Replays of the
// history (the original accept, the first decline) stay duplicates, so a
// chargeback is never undone by a nightly replay; a reversal of the
// re-approval is keyed by its txn_id the same way. On the real store: the
// key and the reversal are decided under the click's lock.
func TestReapprovalAfterDeclineNeedsANewTxnID(t *testing.T) {
	pool := mustPool(t)
	c := ctx()
	orgID := seedOrg(t, c, pool)
	networkID := seedNetwork(t, c, pool, orgID, false)
	mapper := newFakeMapper()
	mapper.set(networkID, "approved", event.CpaAccept)
	mapper.set(networkID, "chargeback", event.CpaDecline)
	mapper.set(networkID, "trash", event.CpaTrash)
	svc := conversion.NewService(mapper, conversion.NewPostgresStore(pool), &fakeFX{},
		&fakeAttribution{outcome: attribution.OutcomeUnknownClick}, &fakeAttempts{})

	clickID := idgen.New()
	step := func(raw, txn string, want conversion.ResultKind, wantInMessage string) {
		t.Helper()
		amount := 100.0
		res, err := svc.Record(c, conversion.Postback{
			OrganizationID: orgID, NetworkID: networkID, ClickID: clickID, RawStatus: raw,
			Revenue: &amount, Currency: "USD", NetworkTxnID: txn, OccurredAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("%s txn %q: %v", raw, txn, err)
		}
		if res.Kind != want || !strings.Contains(res.Message, wantInMessage) {
			t.Fatalf("%s txn %q = %s %q; want %s mentioning %q", raw, txn, res.Kind, res.Message, want, wantInMessage)
		}
	}

	step("approved", "T1", conversion.ResultSuccess, "")
	step("approved", "T1b", conversion.ResultDuplicate, "") // not declined: a fresh txn id is a retry, not a re-approval
	step("chargeback", "", conversion.ResultSuccess, "reverses the CPA_ACCEPT payout")
	step("approved", "T1", conversion.ResultDuplicate, "re-approval counts only with a txn_id") // the replay
	step("approved", "T1b", conversion.ResultDuplicate, "re-approval counts only")             // also seen on an accept row
	step("approved", "", conversion.ResultDuplicate, "re-approval counts only")
	step("approved", "T2", conversion.ResultSuccess, "re-approval after CPA_DECLINE")
	step("approved", "T2", conversion.ResultDuplicate, "")   // replay of the re-approval
	step("chargeback", "", conversion.ResultDuplicate, "")   // replay of the first decline
	step("chargeback", "T1", conversion.ResultDuplicate, "") // the same, with the first accept's id
	step("chargeback", "T2", conversion.ResultSuccess, "reverses the re-approval")
	step("chargeback", "T2", conversion.ResultDuplicate, "")
	step("approved", "T2", conversion.ResultDuplicate, "re-approval counts only")
	step("approved", "T3", conversion.ResultSuccess, "re-approval after CPA_DECLINE")
	step("trash", "T3", conversion.ResultSuccess, "reverses the re-approval")
	step("approved", "T4", conversion.ResultSuccess, "re-approval after CPA_TRASH")

	// Three accepts stand on the ledger, two reversed: +100 net.
	var accepts, reversals int
	var net float64
	if err := pool.QueryRow(c, `
		SELECT count(*) FILTER (WHERE status = 'CPA_ACCEPT'),
		       count(*) FILTER (WHERE reversal_usd_value IS NOT NULL),
		       coalesce(sum(usd_value) FILTER (WHERE status = 'CPA_ACCEPT'), 0) - coalesce(sum(reversal_usd_value), 0)
		FROM postbacks WHERE organization_id = $1 AND click_id = $2 AND result = 'success'`,
		orgID, clickID).Scan(&accepts, &reversals, &net); err != nil {
		t.Fatal(err)
	}
	if accepts != 4 || reversals != 3 || net != 100 {
		t.Fatalf("accepts %d, reversals %d, net %v; want 4, 3, 100", accepts, reversals, net)
	}
}
