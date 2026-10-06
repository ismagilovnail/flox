package ecb_test

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/fx"
	"github.com/ismagilovnail/flox/apps/internal/fx/ecb"
)

// The real feed's shape, trimmed: namespaced envelope, one Cube per day.
const feed = `<?xml version="1.0" encoding="UTF-8"?>
<gesmes:Envelope xmlns:gesmes="http://www.gesmes.org/xml/2002-08-01" xmlns="http://www.ecb.int/vocabulary/2002-08-01/eurofxref">
<gesmes:subject>Reference rates</gesmes:subject>
<Cube>
  <Cube time="2026-10-06">
    <Cube currency="USD" rate="1.25"/>
    <Cube currency="GBP" rate="0.5"/>
    <Cube currency="JPY" rate="125"/>
  </Cube>
  <Cube time="2026-10-05">
    <Cube currency="GBP" rate="0.5"/>
  </Cube>
  <Cube time="2026-09-01">
    <Cube currency="USD" rate="1.1"/>
  </Cube>
</Cube>
</gesmes:Envelope>`

func day(s string) time.Time {
	d, _ := time.Parse(time.DateOnly, s)
	return d
}

func byKey(rates []fx.Rate) map[string]float64 {
	out := map[string]float64{}
	for _, r := range rates {
		out[r.Currency+"@"+r.Date.Format(time.DateOnly)] = r.RateToUSD
	}
	return out
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestParseConvertsEURQuotesToUSD(t *testing.T) {
	rates, err := ecb.Parse(strings.NewReader(feed), day("2026-09-15"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := byKey(rates)
	// 1 EUR = 1.25 USD = 0.5 GBP  =>  1 GBP = 2.5 USD; 1 JPY = 0.01 USD.
	for key, want := range map[string]float64{
		"EUR@2026-10-06": 1.25,
		"GBP@2026-10-06": 2.5,
		"JPY@2026-10-06": 0.01,
	} {
		if !near(got[key], want) {
			t.Errorf("%s = %v, want %v", key, got[key], want)
		}
	}
	if _, ok := got["USD@2026-10-06"]; ok {
		t.Error("USD itself was stored; it converts 1:1 without a row")
	}
	if len(got) != 3 {
		t.Errorf("rates = %v, want exactly the 3 for 2026-10-06 (10-05 has no USD quote, 09-01 is before since)", got)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	for _, doc := range []string{
		`not xml`,
		`<Envelope><Cube></Cube></Envelope>`,
		`<Envelope><Cube><Cube time="2026-10-06"><Cube currency="USD" rate="abc"/></Cube></Cube></Envelope>`,
		`<Envelope><Cube><Cube time="2026-10-06"><Cube currency="USD" rate="0"/></Cube></Cube></Envelope>`,
	} {
		if _, err := ecb.Parse(strings.NewReader(doc), time.Time{}); err == nil {
			t.Errorf("Parse(%q) succeeded, want an error", doc)
		}
	}
}

// A recent since is served from the small 90-day feed; reaching further
// back needs the full history.
func TestRatesPicksTheFeedBySince(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.Path)
		_, _ = w.Write([]byte(feed))
	}))
	defer srv.Close()
	a := ecb.New()
	a.Last90DaysURL, a.HistoryURL = srv.URL+"/90d", srv.URL+"/hist"

	if _, err := a.Rates(context.Background(), time.Now().AddDate(0, 0, -10)); err != nil {
		t.Fatalf("Rates (recent): %v", err)
	}
	if _, err := a.Rates(context.Background(), time.Now().AddDate(-1, 0, 0)); err != nil {
		t.Fatalf("Rates (a year back): %v", err)
	}
	if strings.Join(hits, ",") != "/90d,/hist" {
		t.Fatalf("feeds fetched = %v, want /90d then /hist", hits)
	}
}

func TestRatesReportsHTTPErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "maintenance", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	a := ecb.New()
	a.Last90DaysURL = srv.URL
	if _, err := a.Rates(context.Background(), time.Now()); err == nil {
		t.Fatal("a 503 from the provider was not an error")
	}
}
