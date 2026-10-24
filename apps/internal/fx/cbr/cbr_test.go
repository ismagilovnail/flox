package cbr_test

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"

	"github.com/ismagilovnail/flox/apps/internal/fx"
	"github.com/ismagilovnail/flox/apps/internal/fx/cbr"
)

func day(s string) time.Time {
	d, _ := time.Parse(time.DateOnly, s)
	return d
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-12 }

// cp1251 encodes a document the way the Bank serves it.
func cp1251(t *testing.T, s string) []byte {
	t.Helper()
	b, err := charmap.Windows1251.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The real service's shapes, trimmed: the daily list (ids, Russian names in
// windows-1251) and one dynamic series per id. USD and KZT have a gap over
// the New Year holidays; EUR and XDR are listed but not this provider's.
const dailyXML = `<?xml version="1.0" encoding="windows-1251"?><ValCurs Date="14.01.2026" name="Foreign Currency Market">` +
	`<Valute ID="R01235"><NumCode>840</NumCode><CharCode>USD</CharCode><Nominal>1</Nominal><Name>Доллар США</Name><Value>80,0000</Value></Valute>` +
	`<Valute ID="R01239"><NumCode>978</NumCode><CharCode>EUR</CharCode><Nominal>1</Nominal><Name>Евро</Name><Value>90,0000</Value></Valute>` +
	`<Valute ID="R01335"><NumCode>398</NumCode><CharCode>KZT</CharCode><Nominal>100</Nominal><Name>Казахстанских тенге</Name><Value>16,0000</Value></Valute>` +
	`<Valute ID="R01589"><NumCode>960</NumCode><CharCode>XDR</CharCode><Nominal>1</Nominal><Name>СДР</Name><Value>105,0000</Value></Valute>` +
	`</ValCurs>`

var series = map[string]string{
	"R01235": `<Record Date="30.12.2025" Id="R01235"><Nominal>1</Nominal><Value>78,0000</Value></Record>` +
		`<Record Date="31.12.2025" Id="R01235"><Nominal>1</Nominal><Value>79,0000</Value></Record>` +
		`<Record Date="13.01.2026" Id="R01235"><Nominal>1</Nominal><Value>80,0000</Value></Record>`,
	"R01335": `<Record Date="31.12.2025" Id="R01335"><Nominal>100</Nominal><Value>15,8000</Value></Record>` +
		`<Record Date="13.01.2026" Id="R01335"><Nominal>100</Nominal><Value>16,0000</Value></Record>`,
}

type fake struct {
	t        *testing.T
	requests []string
	fail     string // a path prefix answered 503
	flaky    int    // this many first requests answered 503, then 429
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.requests = append(f.requests, r.URL.RequestURI())
	if (f.fail != "" && strings.HasPrefix(r.URL.Path, f.fail)) || len(f.requests) < f.flaky {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if len(f.requests) == f.flaky {
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	switch r.URL.Path {
	case "/XML_daily.asp":
		_, _ = w.Write(cp1251(f.t, dailyXML))
	case "/XML_dynamic.asp":
		id := r.URL.Query().Get("VAL_NM_RQ")
		doc := `<?xml version="1.0" encoding="windows-1251"?><ValCurs ID="` + id + `" name="Foreign Currency Market Dynamic">` + series[id] + `</ValCurs>`
		_, _ = w.Write(cp1251(f.t, doc))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func adapter(srv *httptest.Server, today string) *cbr.Adapter {
	return &cbr.Adapter{BaseURL: srv.URL, Client: srv.Client(), Now: func() time.Time { return day(today).Add(10 * time.Hour) }}
}

func byKey(rates []fx.Rate) map[string]float64 {
	out := map[string]float64{}
	for _, r := range rates {
		out[r.Currency+"@"+r.Date.Format(time.DateOnly)] = r.RateToUSD
	}
	return out
}

// Rates in USD through the dollar's rouble rate, the nominal applied; the
// rate in force carried over the New Year gap; only this provider's
// currencies; the series read from three weeks before since.
func TestRatesCrossThroughTheDollarAndCarryOverGaps(t *testing.T) {
	f := &fake{t: t}
	srv := httptest.NewServer(f)
	defer srv.Close()

	rates, err := adapter(srv, "2026-01-14").Rates(context.Background(), day("2025-12-31"))
	if err != nil {
		t.Fatalf("Rates: %v", err)
	}
	got := byKey(rates)
	for key, want := range map[string]float64{
		"RUB@2025-12-31": 1.0 / 79,
		"RUB@2026-01-08": 1.0 / 79, // the holidays: 31.12's rate is in force
		"RUB@2026-01-13": 1.0 / 80,
		"RUB@2026-01-14": 1.0 / 80, // today, carried from 13.01
		"KZT@2025-12-31": 0.158 / 79,
		"KZT@2026-01-12": 0.158 / 79,
		"KZT@2026-01-13": 0.16 / 80,
	} {
		if v, ok := got[key]; !ok || !near(v, want) {
			t.Errorf("%s = %v (present %v), want %v", key, v, ok, want)
		}
	}
	for key := range got {
		if strings.HasPrefix(key, "EUR@") || strings.HasPrefix(key, "XDR@") || strings.HasPrefix(key, "USD@") {
			t.Errorf("%s is not this provider's currency", key)
		}
		if key == "RUB@2025-12-30" {
			t.Errorf("%s is before since", key)
		}
	}
	if len(got) != 2*15 {
		t.Errorf("got %d rates, want RUB and KZT for each of the 15 days 31.12–14.01", len(got))
	}
	for _, r := range f.requests {
		if strings.HasPrefix(r, "/XML_dynamic.asp") && !strings.Contains(r, "date_req1=10/12/2025") {
			t.Errorf("series request %s, want it to start 21 days before since", r)
		}
	}
}

// A 5xx or a 429 is tried again (up to 3 times); a bad document is not.
func TestRatesRetriesTheBanksHiccups(t *testing.T) {
	f := &fake{t: t, flaky: 2}
	srv := httptest.NewServer(f)
	defer srv.Close()
	rates, err := adapter(srv, "2026-01-14").Rates(context.Background(), day("2026-01-13"))
	if err != nil || len(rates) == 0 {
		t.Fatalf("two 503s then answers: rates = %d, err = %v", len(rates), err)
	}
	if !strings.HasPrefix(f.requests[2], "/XML_daily.asp") {
		t.Fatalf("third request = %s, want the daily list again", f.requests[2])
	}
}

func TestRatesReportsFailures(t *testing.T) {
	for _, fail := range []string{"/XML_daily.asp", "/XML_dynamic.asp"} {
		srv := httptest.NewServer(&fake{t: t, fail: fail})
		if _, err := adapter(srv, "2026-01-14").Rates(context.Background(), day("2026-01-01")); err == nil || !strings.Contains(err.Error(), "503") {
			t.Errorf("%s down: err = %v, want the 503", fail, err)
		}
		srv.Close()
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/XML_daily.asp" {
			_, _ = w.Write(cp1251(t, dailyXML))
			return
		}
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="windows-1251"?><ValCurs><Record Date="13.01.2026"><Nominal>1</Nominal><Value>abc</Value></Record></ValCurs>`))
	}))
	defer bad.Close()
	if _, err := adapter(bad, "2026-01-14").Rates(context.Background(), day("2026-01-01")); err == nil || !strings.Contains(err.Error(), "bad rate") {
		t.Errorf("garbage value: err = %v, want bad rate", err)
	}
}

// The real service, when asked for (CBR_LIVE=1): the rouble and the tenge
// for the last week, at sane magnitudes.
func TestLiveRates(t *testing.T) {
	if os.Getenv("CBR_LIVE") == "" {
		t.Skip("CBR_LIVE not set")
	}
	rates, err := cbr.New().Rates(context.Background(), time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -7))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, r := range rates {
		got[r.Currency] = r.RateToUSD
	}
	if rub := got["RUB"]; rub < 1.0/300 || rub > 1.0/30 {
		t.Errorf("RUB = %v USD", rub)
	}
	if kzt := got["KZT"]; kzt < 1.0/2000 || kzt > 1.0/100 {
		t.Errorf("KZT = %v USD", kzt)
	}
	t.Logf("%d rates; 1 RUB = %.6f USD, 1 KZT = %.6f USD, 1 UAH = %.6f USD", len(rates), got["RUB"], got["KZT"], got["UAH"])
}
