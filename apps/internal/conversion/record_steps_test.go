package conversion_test

import (
	"math"
	"strings"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
)

// Every network-supplied string is trimmed before any step uses it: the
// ledger row, the dedup key, the FX lookup, the CPA event and the outgoing
// postback all see the same values.
func TestNetworkFieldsAreTrimmedEverywhere(t *testing.T) {
	svc, mapper, store, events, deliveries := newHarness()
	mapper.set("net-1", "dep", event.CpaRedep)

	p := postback("net-1", "  click-1 ", " dep ")
	revenue := 10.0
	p.Revenue = &revenue
	p.Currency = " EUR "
	p.NetworkTxnID = " txn-1 "
	p.ExternalClickID = " ext-1 "
	p.PostbackURL = "  https://net.example/pb?c={click_id}&s={status}&cur={currency} "
	if _, err := svc.Record(ctx(), p); err != nil {
		t.Fatalf("Record: %v", err)
	}

	row := store.records[0]
	if row.ClickID != "click-1" || row.RawStatus != "dep" || row.Currency != "EUR" || row.NetworkTxnID != "txn-1" || row.EventRef != "txn-1" {
		t.Fatalf("ledger row not trimmed: click=%q raw=%q currency=%q txn=%q ref=%q", row.ClickID, row.RawStatus, row.Currency, row.NetworkTxnID, row.EventRef)
	}
	if row.USDValue == nil || math.Abs(*row.USDValue-11) > 1e-9 {
		t.Fatalf("USDValue = %v, want 11 (the EUR rate, found through the trimmed currency)", row.USDValue)
	}
	ev := events.last()
	if ev.ClickID != "click-1" || ev.Currency != "EUR" || ev.NetworkTxnID != "txn-1" || ev.EventRef != "txn-1" {
		t.Fatalf("event not trimmed: %+v", ev)
	}
	if got, want := deliveries.last().URL, "https://net.example/pb?c=click-1&s=CPA_REDEP&cur=EUR"; got != want {
		t.Fatalf("delivery URL = %q, want %q", got, want)
	}
}

// What a postback refused before attribution leaves in the ledger.
func TestRefusalLedgerRows(t *testing.T) {
	nan := math.NaN()
	for _, tc := range []struct {
		name       string
		raw        string
		revenue    *float64
		wantKind   conversion.ResultKind
		wantClick  string
		wantStatus event.Type
		wantMsg    string
	}{
		{"missing status: logged without its click", "", nil, conversion.ResultError, "", "", "missing status"},
		{"invalid revenue keeps the click", "sale", &nan, conversion.ResultError, "click-1", "", "conversion: invalid revenue"},
		{"unmapped status keeps the click", "nope", nil, conversion.ResultError, "click-1", "", "no event mapping configured"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, mapper, store, events, _ := newHarness()
			mapper.set("net-1", "sale", event.CpaAccept)
			p := postback("net-1", "click-1", tc.raw)
			p.Revenue = tc.revenue

			result, err := svc.Record(ctx(), p)
			if err != nil {
				t.Fatalf("Record: %v", err)
			}
			if len(store.records) != 1 || events.count() != 0 {
				t.Fatalf("rows = %d, events = %d; want one ledger row and no event", len(store.records), events.count())
			}
			row := store.records[0]
			if result.Kind != tc.wantKind || row.Kind != tc.wantKind || row.ClickID != tc.wantClick || row.Status != tc.wantStatus || row.EventRef != "" ||
				!strings.HasPrefix(row.Message, tc.wantMsg) || result.Message != row.Message {
				t.Fatalf("result %+v, row %+v; want kind %q, click %q, status %q, message %q…", result, row, tc.wantKind, tc.wantClick, tc.wantStatus, tc.wantMsg)
			}
		})
	}

	// A progression refusal knows its status and event_ref.
	svc, mapper, store, _, _ := newHarness()
	mapper.set("net-1", "sale", event.CpaAccept)
	mapper.set("net-1", "reg", event.CpaHold)
	if _, err := svc.Record(ctx(), postback("net-1", "click-1", "sale")); err != nil {
		t.Fatal(err)
	}
	result, err := svc.Record(ctx(), postback("net-1", "click-1", "reg"))
	if err != nil {
		t.Fatal(err)
	}
	row := store.records[len(store.records)-1]
	if result.Kind != conversion.ResultIgnored || row.Kind != conversion.ResultIgnored || row.ClickID != "click-1" || row.Status != event.CpaHold ||
		!strings.Contains(row.Message, "back to CPA_HOLD") {
		t.Fatalf("progression refusal: result %+v, row %+v", result, row)
	}
}
