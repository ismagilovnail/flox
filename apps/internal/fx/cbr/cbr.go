// Package cbr is the Bank of Russia (ЦБ РФ) official-rate fx.Provider —
// free, keyless, set every working day for the next one (audit DATA-8).
//
// It covers what the ECB does not publish: the rouble and the currencies
// common in CIS traffic (Currencies). The ECB stays the source of every
// currency it publishes, so one currency never mixes two sources' rates.
//
// The Bank quotes every currency in roubles ("Nominal units of C = Value
// RUB"), so a rate in USD is (Value_C/Nominal_C) / (Value_USD/Nominal_USD),
// and the rouble itself is 1 / (Value_USD/Nominal_USD). A rate is in force
// until the next one is set — over a weekend, and over the New Year
// holidays, which run longer than fx.MaxStaleDays — so each rate is
// written for every day up to the next one.
package cbr

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"

	"github.com/ismagilovnail/flox/apps/internal/fx"
)

// BaseURL is the Bank's XML service.
const BaseURL = "https://www.cbr.ru/scripts"

// Currencies are the ones this provider stores: what the Bank publishes and
// the ECB does not, plus the rouble. XDR (an IMF unit) is left out.
var Currencies = []string{
	"RUB", "AED", "AMD", "AZN", "BDT", "BHD", "BOB", "BYN", "CUP", "DZD", "EGP", "ETB", "GEL", "IRR", "KGS",
	"KZT", "MDL", "MMK", "MNT", "NGN", "OMR", "QAR", "RSD", "SAR", "TJS", "TMT", "UAH", "UZS", "VND",
}

// usdID is the Bank's id for the US dollar.
const usdID = "R01235"

// lookback is how far before since a series is read, so the rate in force
// on since is known (the longest gap, the New Year holidays, is ~2 weeks).
const lookback = 21

// maxBody bounds one response; a year of one currency is ~30 KB.
const maxBody = 8 << 20

// attempts is how many times a request is tried: the Bank's site, behind
// DDoS protection, now and then stalls a TLS handshake, answers 5xx, or
// 429 to a burst, and a sync that fails waits for the next one (hours).
const attempts = 3

// Adapter fetches from BaseURL (tests point it at a fake).
type Adapter struct {
	BaseURL string
	Client  *http.Client
	// Now is the clock (tests): rates are written up to its UTC day.
	Now func() time.Time
	// Backoff is the pause before the second try, doubled for the third.
	Backoff time.Duration
	// Pace is the pause between requests: a sync is ~30 of them (one per
	// currency), and the site answers a burst with 429.
	Pace time.Duration
}

func New() *Adapter {
	return &Adapter{BaseURL: BaseURL, Client: &http.Client{Timeout: 30 * time.Second}, Now: time.Now,
		Backoff: 5 * time.Second, Pace: 300 * time.Millisecond}
}

// Rates returns the rates in force on every day from since to today (and a
// rate already set for tomorrow) for Currencies.
func (a *Adapter) Rates(ctx context.Context, since time.Time) ([]fx.Rate, error) {
	today := a.Now().UTC().Truncate(24 * time.Hour)
	ids, err := a.ids(ctx, today)
	if err != nil {
		return nil, err
	}
	from := since.UTC().Truncate(24*time.Hour).AddDate(0, 0, -lookback)
	usd, err := a.series(ctx, usdID, from, today)
	if err != nil {
		return nil, err
	}
	if len(usd) == 0 {
		return nil, fmt.Errorf("cbr: no USD rate since %s", from.Format(time.DateOnly))
	}
	usdDays := fill(usd, since, today)

	var out []fx.Rate
	for _, d := range sortedDays(usdDays) {
		out = append(out, fx.Rate{Currency: "RUB", Date: d, RateToUSD: 1 / usdDays[d]})
	}
	for _, cur := range Currencies {
		id, ok := ids[cur]
		if cur == "RUB" || !ok {
			continue // the rouble is done; a currency the Bank no longer lists has no rate
		}
		s, err := a.series(ctx, id, from, today)
		if err != nil {
			return nil, err
		}
		days := fill(s, since, today)
		for _, d := range sortedDays(days) {
			if u, ok := usdDays[d]; ok {
				out = append(out, fx.Rate{Currency: cur, Date: d, RateToUSD: days[d] / u})
			}
		}
	}
	return out, nil
}

// fill spreads a series of (date → RUB per unit) over every day from since
// to the later of until and its last date: each day gets the rate set on
// it or the latest one before it.
func fill(s map[time.Time]float64, since, until time.Time) map[time.Time]float64 {
	dates := sortedDays(s)
	out := map[time.Time]float64{}
	if len(dates) == 0 {
		return out
	}
	if last := dates[len(dates)-1]; last.After(until) {
		until = last
	}
	i, current, have := 0, 0.0, false
	for d := dates[0]; !d.After(until); d = d.AddDate(0, 0, 1) {
		for i < len(dates) && !dates[i].After(d) {
			current, have = s[dates[i]], true
			i++
		}
		if have && !d.Before(since) {
			out[d] = current
		}
	}
	return out
}

func sortedDays[V any](m map[time.Time]V) []time.Time {
	out := make([]time.Time, 0, len(m))
	for d := range m {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

type daily struct {
	Valutes []struct {
		ID       string `xml:"ID,attr"`
		CharCode string `xml:"CharCode"`
	} `xml:"Valute"`
}

// ids maps the Bank's currency codes to its ids, from day's list.
func (a *Adapter) ids(ctx context.Context, day time.Time) (map[string]string, error) {
	var doc daily
	if err := a.get(ctx, "/XML_daily.asp?date_req="+day.Format("02/01/2006"), &doc); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(doc.Valutes))
	for _, v := range doc.Valutes {
		out[strings.ToUpper(strings.TrimSpace(v.CharCode))] = strings.TrimSpace(v.ID)
	}
	if out["USD"] == "" {
		return nil, fmt.Errorf("cbr: the daily list has no USD")
	}
	return out, nil
}

type dynamic struct {
	Records []struct {
		Date    string `xml:"Date,attr"`
		Nominal string `xml:"Nominal"`
		Value   string `xml:"Value"`
	} `xml:"Record"`
}

// series is one currency's RUB per unit on each date the Bank set a rate.
func (a *Adapter) series(ctx context.Context, id string, from, to time.Time) (map[time.Time]float64, error) {
	var doc dynamic
	q := "/XML_dynamic.asp?date_req1=" + from.Format("02/01/2006") + "&date_req2=" + to.Format("02/01/2006") + "&VAL_NM_RQ=" + id
	if err := a.get(ctx, q, &doc); err != nil {
		return nil, err
	}
	out := make(map[time.Time]float64, len(doc.Records))
	for _, r := range doc.Records {
		d, err := time.Parse("02.01.2006", r.Date)
		if err != nil {
			return nil, fmt.Errorf("cbr: bad date %q for %s", r.Date, id)
		}
		nominal, err1 := strconv.ParseFloat(strings.TrimSpace(r.Nominal), 64)
		value, err2 := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(r.Value), ",", "."), 64)
		if err1 != nil || err2 != nil || nominal <= 0 || value <= 0 {
			return nil, fmt.Errorf("cbr: bad rate %q/%q for %s on %s", r.Value, r.Nominal, id, r.Date)
		}
		out[d] = value / nominal
	}
	return out, nil
}

// get fetches and decodes path, trying again after a network error, a 5xx
// or a 429 (not after a bad document: the same request gets the same
// answer).
func (a *Adapter) get(ctx context.Context, path string, into any) error {
	if a.Pace > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(a.Pace):
		}
	}
	wait := a.Backoff
	for try := 1; ; try++ {
		retry, err := a.getOnce(ctx, path, into)
		if err == nil || !retry || try == attempts {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		wait *= 2
	}
}

func (a *Adapter) getOnce(ctx context.Context, path string, into any) (retry bool, err error) {
	url := a.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("cbr: building request: %w", err)
	}
	req.Header.Set("User-Agent", "FLOX fx sync")
	resp, err := a.Client.Do(req)
	if err != nil {
		return ctx.Err() == nil, fmt.Errorf("cbr: fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		retry := resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		return retry, fmt.Errorf("cbr: %s responded %d", url, resp.StatusCode)
	}
	dec := xml.NewDecoder(io.LimitReader(resp.Body, maxBody))
	dec.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		if strings.EqualFold(charset, "windows-1251") {
			return charmap.Windows1251.NewDecoder().Reader(input), nil
		}
		return nil, fmt.Errorf("cbr: unexpected charset %q", charset)
	}
	if err := dec.Decode(into); err != nil {
		return false, fmt.Errorf("cbr: decoding %s: %w", url, err)
	}
	return false, nil
}
