package conversion_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/postbacklogs"
)

// Audit 2026-10-15, DATA-1: a postback with revenue but no currency used to
// be stored with no USD value, so a $50 FTD vanished from Revenue and ROI.

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func currencyHarness() (*conversion.Service, *fakeEvents, *fakeAttempts) {
	mapper := newFakeMapper()
	mapper.set("net-1", "reg", event.CpaHold)
	mapper.set("net-1", "ftd", event.CpaAccept)
	events := &fakeEvents{}
	attempts := &fakeAttempts{}
	attr := &fakeAttribution{outcome: attribution.OutcomeUnknownClick}
	svc := conversion.NewService(mapper, newFakeStore(events, nil), &fakeFX{rates: map[string]float64{"EUR": 1.1}}, attr, attempts)
	return svc, events, attempts
}

func TestRevenueWithoutCurrencyTakesTheNetworksCurrency(t *testing.T) {
	svc, events, attempts := currencyHarness()
	p := postback("net-1", "click-1", "ftd")
	revenue := 50.0
	p.Revenue = &revenue
	p.NetworkCurrency = "EUR"

	result, err := svc.Record(ctx(), p)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if result.Kind != conversion.ResultSuccess {
		t.Fatalf("Kind = %q, want success", result.Kind)
	}
	ev := events.last()
	if ev.Currency != "EUR" || !ev.HasUSDValue || !near(ev.USDValue, 55) {
		t.Fatalf("event currency/USD = %q %v %v, want EUR, priced at 55", ev.Currency, ev.HasUSDValue, ev.USDValue)
	}
	if !strings.Contains(result.Message, "currency not sent, the network's EUR assumed") {
		t.Errorf("result message %q does not say the currency was assumed", result.Message)
	}
	rec := attempts.recs[len(attempts.recs)-1]
	if rec.Currency != "EUR" || !strings.Contains(rec.Message, "EUR assumed") {
		t.Errorf("attempt log = %q %q, want EUR and the note", rec.Currency, rec.Message)
	}
}

func TestSentCurrencyWinsOverTheNetworks(t *testing.T) {
	svc, events, _ := currencyHarness()
	p := postback("net-1", "click-1", "ftd")
	revenue := 50.0
	p.Revenue = &revenue
	p.Currency = " usd " // lower case and padded: still USD
	p.NetworkCurrency = "EUR"

	result, err := svc.Record(ctx(), p)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	ev := events.last()
	if ev.Currency != "USD" || !near(ev.USDValue, 50) {
		t.Fatalf("event currency/USD = %q %v, want USD 50", ev.Currency, ev.USDValue)
	}
	if strings.Contains(result.Message, "assumed") {
		t.Errorf("message %q claims an assumed currency, but one was sent", result.Message)
	}
}

func TestNoRevenueAssumesNoCurrency(t *testing.T) {
	svc, events, _ := currencyHarness()
	p := postback("net-1", "click-1", "reg")
	p.NetworkCurrency = "EUR"

	result, err := svc.Record(ctx(), p)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if ev := events.last(); ev.Currency != "" || ev.HasUSDValue {
		t.Fatalf("a HOLD without revenue got currency %q / USD %v", ev.Currency, ev.HasUSDValue)
	}
	if strings.Contains(result.Message, "currency") || strings.Contains(result.Message, "revenue") {
		t.Errorf("message %q mentions money the postback never had", result.Message)
	}
}

func TestUnpricedRevenueSaysItIsLeftOutOfReports(t *testing.T) {
	svc, events, attempts := currencyHarness()
	p := postback("net-1", "click-1", "ftd")
	revenue := 1000.0
	p.Revenue = &revenue
	p.Currency = "TRY" // no rate on file
	p.OccurredAt = time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)

	result, err := svc.Record(ctx(), p)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if result.Kind != conversion.ResultSuccess || events.last().HasUSDValue {
		t.Fatalf("Kind %q, HasUSDValue %v: want a success without a USD value", result.Kind, events.last().HasUSDValue)
	}
	const want = "no TRY rate for 2026-10-15, revenue is left out of revenue reports"
	if !strings.Contains(result.Message, want) {
		t.Errorf("result message %q, want it to contain %q", result.Message, want)
	}
	if rec := attempts.recs[len(attempts.recs)-1]; !strings.Contains(rec.Message, want) {
		t.Errorf("attempt log message %q, want it to contain %q", rec.Message, want)
	}
}

func TestReplayTakesTheNetworksCurrencyToo(t *testing.T) {
	svc, events, _ := currencyHarness()
	revenue := 50.0
	outcome, err := conversion.NewReplayRecorder(svc).Record(ctx(), postbacklogs.IncomingRecord{
		OrganizationID: orgA, NetworkID: "net-1", ClickID: "click-1", RawStatus: "ftd",
		Revenue: &revenue, NetworkCurrency: "EUR",
		OccurredAt: time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if outcome.Result != string(conversion.ResultSuccess) || events.last().Currency != "EUR" || !near(events.last().USDValue, 55) {
		t.Fatalf("outcome %+v, event %q %v: want success priced in EUR", outcome, events.last().Currency, events.last().USDValue)
	}
}

func TestRefusedPostbackSaysNothingAboutCurrency(t *testing.T) {
	svc, _, _ := currencyHarness()
	p := postback("net-1", "", "ftd") // no click_id: refused
	revenue := 50.0
	p.Revenue = &revenue
	p.NetworkCurrency = "USD"

	result, err := svc.Record(ctx(), p)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if result.Kind != conversion.ResultError || result.Message != "missing click_id" {
		t.Fatalf("result = %+v, want the bare refusal", result)
	}
}
