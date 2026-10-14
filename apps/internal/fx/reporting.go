package fx

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Reporting currency (§50-FX "Base reporting currency configurable per
// organization"). Money is stored in USD at the event's own rate; a report
// in another currency converts each day's USD at that day's USD → base
// rate, so a past day reads the same however wide the report is.

// Conversion turns USD into one organization's reporting currency, per UTC
// day. The zero value and nil are the identity (a USD organization).
type Conversion struct {
	Currency string
	// factors: UTC day "2006-01-02" → base units per 1 USD. A day missing
	// here has no rate within MaxStaleDays: its money is unknown, not 0.
	factors map[string]float64
	from    time.Time
	to      time.Time
}

// USD is the identity conversion.
var USD = &Conversion{Currency: "USD"}

// Identity reports whether amounts pass through unchanged.
func (c *Conversion) Identity() bool { return c == nil || c.factors == nil }

// Code is the currency amounts come out in.
func (c *Conversion) Code() string {
	if c == nil || c.Currency == "" {
		return "USD"
	}
	return c.Currency
}

// Factor is the multiplier for USD amounts of t's UTC day; false when that
// day has no rate.
func (c *Conversion) Factor(t time.Time) (float64, bool) {
	if c.Identity() {
		return 1, true
	}
	f, ok := c.factors[t.UTC().Format(time.DateOnly)]
	return f, ok
}

// Days lists every day of the loaded range that has a factor, in order —
// what a ClickHouse query inlines to convert per row.
func (c *Conversion) Days() ([]time.Time, []float64) {
	if c.Identity() {
		return nil, nil
	}
	var days []time.Time
	var factors []float64
	for d := c.from; !d.After(c.to); d = d.AddDate(0, 0, 1) {
		if f, ok := c.factors[d.Format(time.DateOnly)]; ok {
			days = append(days, d)
			factors = append(factors, f)
		}
	}
	return days, factors
}

// NewConversion builds a conversion from base-currency rates (1 unit of
// currency = RateToUSD USD) for every UTC day in [from, to], each day taking
// the latest rate on or before it, at most MaxStaleDays old — the rule
// conversion.PostgresFX uses for the USD side. Exported for tests.
func NewConversion(currency string, rates []Rate, from, to time.Time) *Conversion {
	c := &Conversion{Currency: currency, factors: map[string]float64{}, from: utcDay(from), to: utcDay(to)}
	i := 0
	var last *Rate
	for d := c.from; !d.After(c.to); d = d.AddDate(0, 0, 1) {
		for i < len(rates) && !utcDay(rates[i].Date).After(d) {
			last = &rates[i]
			i++
		}
		if last != nil && last.RateToUSD > 0 && d.Sub(utcDay(last.Date)) < MaxStaleDays*24*time.Hour {
			c.factors[d.Format(time.DateOnly)] = 1 / last.RateToUSD
		}
	}
	return c
}

func utcDay(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// ReportingConversion loads the organization's reporting currency and,
// unless it is USD, its rates for [from, to] (plus the rate in force on
// from). The organization row is read by its own id only.
func (s *Store) ReportingConversion(ctx context.Context, orgID string, from, to time.Time) (*Conversion, error) {
	var currency string
	if err := s.db.QueryRow(ctx, `SELECT currency FROM organizations WHERE id = $1`, orgID).Scan(&currency); err != nil {
		return nil, fmt.Errorf("fx: reading the organization's currency: %w", err)
	}
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" || currency == "USD" {
		return USD, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT rate_date, rate_to_usd::float8 FROM fx_rates
		WHERE currency = $1 AND rate_date > $2::date - $4::int AND rate_date <= $3::date
		ORDER BY rate_date`,
		currency, utcDay(from).Format(time.DateOnly), utcDay(to).Format(time.DateOnly), MaxStaleDays)
	if err != nil {
		return nil, fmt.Errorf("fx: reading %s rates: %w", currency, err)
	}
	defer rows.Close()
	var rates []Rate
	for rows.Next() {
		r := Rate{Currency: currency}
		if err := rows.Scan(&r.Date, &r.RateToUSD); err != nil {
			return nil, fmt.Errorf("fx: scanning %s rate: %w", currency, err)
		}
		rates = append(rates, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("fx: reading %s rates: %w", currency, err)
	}
	return NewConversion(currency, rates, from, to), nil
}
