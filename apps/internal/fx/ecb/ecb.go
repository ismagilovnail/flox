// Package ecb is the European Central Bank reference-rate fx.Provider —
// free, keyless, published once per TARGET working day around 16:00 CET.
//
// ECB quotes every currency against EUR ("1 EUR = r_C units of C"), so a
// rate in USD is r_USD / r_C, and EUR itself is r_USD. The set is about 30
// currencies; anything else (RUB has been suspended since 2022, UAH and KZT
// were never published) gets no rate here and stays without a USD value
// until another provider covers it.
package ecb

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/fx"
)

const (
	// Last90DaysURL is the 90-day feed (~100 KB), used whenever it reaches
	// back far enough.
	Last90DaysURL = "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-hist-90d.xml"
	// HistoryURL is every rate since 1999 (~7.5 MB), only for a first sync
	// that has to repair older events.
	HistoryURL = "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-hist.xml"
)

// the 90-day feed is trusted to cover this many days, leaving a margin.
const last90DaysSpan = 85

// maxBody bounds a response read; the full history is about 7.5 MB.
const maxBody = 32 << 20

type Adapter struct {
	Last90DaysURL string
	HistoryURL    string
	HTTPClient    *http.Client
}

func New() *Adapter {
	return &Adapter{
		Last90DaysURL: Last90DaysURL,
		HistoryURL:    HistoryURL,
		HTTPClient:    &http.Client{Timeout: 60 * time.Second},
	}
}

var _ fx.Provider = (*Adapter)(nil)

type envelope struct {
	Days []struct {
		Time  string `xml:"time,attr"`
		Rates []struct {
			Currency string `xml:"currency,attr"`
			Rate     string `xml:"rate,attr"`
		} `xml:"Cube"`
	} `xml:"Cube>Cube"`
}

func (a *Adapter) Rates(ctx context.Context, since time.Time) ([]fx.Rate, error) {
	url := a.Last90DaysURL
	if since.Before(time.Now().UTC().AddDate(0, 0, -last90DaysSpan)) {
		url = a.HistoryURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ecb: fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ecb: %s responded %d", url, resp.StatusCode)
	}
	return Parse(io.LimitReader(resp.Body, maxBody), since)
}

// Parse converts an ECB eurofxref document into USD rates dated on or after
// since. A day without a USD quote cannot be converted and is skipped.
func Parse(r io.Reader, since time.Time) ([]fx.Rate, error) {
	var doc envelope
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("ecb: decoding rates: %w", err)
	}
	if len(doc.Days) == 0 {
		return nil, fmt.Errorf("ecb: document has no rates")
	}

	var out []fx.Rate
	for _, day := range doc.Days {
		date, err := time.Parse(time.DateOnly, day.Time)
		if err != nil {
			return nil, fmt.Errorf("ecb: bad date %q: %w", day.Time, err)
		}
		if date.Before(since) {
			continue
		}
		perEUR := make(map[string]float64, len(day.Rates))
		for _, q := range day.Rates {
			v, err := strconv.ParseFloat(strings.TrimSpace(q.Rate), 64)
			if err != nil || v <= 0 {
				return nil, fmt.Errorf("ecb: bad rate %q for %s on %s", q.Rate, q.Currency, day.Time)
			}
			perEUR[strings.ToUpper(strings.TrimSpace(q.Currency))] = v
		}
		usdPerEUR, ok := perEUR["USD"]
		if !ok {
			continue
		}
		out = append(out, fx.Rate{Currency: "EUR", Date: date, RateToUSD: usdPerEUR})
		for cur, v := range perEUR {
			if cur == "USD" {
				continue
			}
			out = append(out, fx.Rate{Currency: cur, Date: date, RateToUSD: usdPerEUR / v})
		}
	}
	return out, nil
}
