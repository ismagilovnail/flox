package conversion_test

import (
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
)

// clickMadeAt is a click id the tracker would have made at t.
func clickMadeAt(t time.Time) string {
	return ulid.MustNew(ulid.Timestamp(t), ulid.DefaultEntropy()).String()
}

// Audit UNI-1 part 2: a page's script, with no secret, may report only a
// lead or a purchase, once each, said so in the log.
func TestBrowserReportsOnlyALeadOrAPurchase(t *testing.T) {
	svc, mapper, store, _, _ := newHarness()
	for raw, status := range map[string]event.Type{"lead": event.CpaHold, "purchase": event.CpaAccept, "repeat_purchase": event.CpaRedep, "refund": event.CpaDecline, "spam": event.CpaTrash} {
		mapper.set("net-1", raw, status)
	}
	browser := func(status, txn string) conversion.Postback {
		p := postback("net-1", "click-1", status)
		p.Browser, p.NetworkTxnID = true, txn
		return p
	}

	for _, raw := range []string{"repeat_purchase", "refund", "spam"} {
		res, err := svc.Record(ctx(), browser(raw, "A-"+raw))
		if err != nil {
			t.Fatal(err)
		}
		if res.Kind != conversion.ResultError || !strings.Contains(res.Message, "only a lead or a purchase") {
			t.Fatalf("%s from a page = %s %q, want refused", raw, res.Kind, res.Message)
		}
	}
	for _, raw := range []string{"lead", "purchase"} {
		res, err := svc.Record(ctx(), browser(raw, "A-1"))
		if err != nil {
			t.Fatal(err)
		}
		if res.Kind != conversion.ResultSuccess || !strings.Contains(res.Message, "reported by the page's script") {
			t.Fatalf("%s from a page = %s %q, want recorded and marked", raw, res.Kind, res.Message)
		}
	}
	// Once per click, whatever order number the page sends next.
	if res, _ := svc.Record(ctx(), browser("purchase", "A-2")); res.Kind == conversion.ResultSuccess {
		t.Fatalf("a second purchase from a page = %s, want not recorded", res.Kind)
	}
	if n := len(store.records); n == 0 {
		t.Fatal("nothing recorded")
	}
}

// A click the lookup does not know is taken from a page only while it is
// fresh enough to be on its way; an old or made-up one is refused.
func TestBrowserUnknownClickOnlyWhenFresh(t *testing.T) {
	mapper := newFakeMapper()
	mapper.set("net-1", "purchase", event.CpaAccept)
	svc := conversion.NewService(mapper, newFakeStore(&fakeEvents{}, nil), &fakeFX{}, &fakeAttribution{outcome: attribution.OutcomeUnknownClick}, &fakeAttempts{})
	now := time.Now().UTC()

	for name, tc := range map[string]struct {
		click string
		want  conversion.ResultKind
	}{
		"fresh":      {clickMadeAt(now.Add(-2 * time.Minute)), conversion.ResultSuccess},
		"old":        {clickMadeAt(now.Add(-time.Hour)), conversion.ResultError},
		"not a ULID": {"click-made-up", conversion.ResultError},
	} {
		p := postback("net-1", tc.click, "purchase")
		p.Browser, p.OccurredAt = true, now
		res, err := svc.Record(ctx(), p)
		if err != nil {
			t.Fatal(err)
		}
		if res.Kind != tc.want {
			t.Errorf("%s: %s %q, want %s", name, res.Kind, res.Message, tc.want)
		}
	}
	// The server's postback for an old unknown click is kept for retries,
	// as before.
	p := postback("net-1", clickMadeAt(now.Add(-time.Hour)), "purchase")
	p.OccurredAt = now
	if res, _ := svc.Record(ctx(), p); res.Kind != conversion.ResultSuccess {
		t.Errorf("server postback for an old unknown click = %s, want recorded", res.Kind)
	}
}
