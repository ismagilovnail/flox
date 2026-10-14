package analytics

import (
	"context"
	"math"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/cost"
	"github.com/ismagilovnail/flox/apps/internal/fx"
)

// Reporting currency (§50-FX). Revenue is converted in ClickHouse per
// conversion day (chstore.ReportQuery.Money); spend here, per entry day. A
// day without a rate makes its amount NaN, and sanitizeMoney turns a NaN
// total into null — unknown, never 0 (CLAUDE.md #6/#7). The accumulator's
// revenueUSD/costUSD then hold the reporting currency.

// convertedCost is a CostSource whose amounts come out in conv's currency.
// The period sums are rebuilt from the per-day ones, so each day takes its
// own rate.
type convertedCost struct {
	inner CostSource
	conv  *fx.Conversion
}

func (c convertedCost) at(day time.Time, usd float64) float64 {
	f, ok := c.conv.Factor(day)
	if !ok {
		return math.NaN()
	}
	return usd * f
}

func (c convertedCost) SpendByCampaign(ctx context.Context, orgID string, from, to time.Time) ([]cost.CampaignSpend, error) {
	days, err := c.CampaignSpendByDay(ctx, orgID, from, to)
	if err != nil {
		return nil, err
	}
	var out []cost.CampaignSpend
	idx := map[string]int{}
	for _, d := range days {
		i, ok := idx[d.CampaignID]
		if !ok {
			i = len(out)
			idx[d.CampaignID] = i
			out = append(out, cost.CampaignSpend{CampaignID: d.CampaignID, AllConverted: true})
		}
		out[i].AmountUSD += d.AmountUSD
	}
	return out, nil
}

func (c convertedCost) SpendBySource(ctx context.Context, orgID string, from, to time.Time) ([]cost.SourceSpend, error) {
	days, err := c.SourceSpendByDay(ctx, orgID, from, to)
	if err != nil {
		return nil, err
	}
	var out []cost.SourceSpend
	idx := map[string]int{}
	for _, d := range days {
		i, ok := idx[d.TrafficSourceID]
		if !ok {
			i = len(out)
			idx[d.TrafficSourceID] = i
			out = append(out, cost.SourceSpend{TrafficSourceID: d.TrafficSourceID, AllConverted: true})
		}
		out[i].AmountUSD += d.AmountUSD
	}
	return out, nil
}

func (c convertedCost) CampaignSpendByDay(ctx context.Context, orgID string, from, to time.Time) ([]cost.CampaignDaySpend, error) {
	rows, err := c.inner.CampaignSpendByDay(ctx, orgID, from, to)
	for i := range rows {
		rows[i].AmountUSD = c.at(rows[i].Day, rows[i].AmountUSD)
	}
	return rows, err
}

func (c convertedCost) SourceSpendByDay(ctx context.Context, orgID string, from, to time.Time) ([]cost.SourceDaySpend, error) {
	rows, err := c.inner.SourceSpendByDay(ctx, orgID, from, to)
	for i := range rows {
		rows[i].AmountUSD = c.at(rows[i].Day, rows[i].AmountUSD)
	}
	return rows, err
}

func (c convertedCost) DailySpendForOrg(ctx context.Context, orgID string, from, to time.Time) ([]cost.DailySpend, error) {
	rows, err := c.inner.DailySpendForOrg(ctx, orgID, from, to)
	for i := range rows {
		rows[i].AmountUSD = c.at(rows[i].Day, rows[i].AmountUSD)
	}
	return rows, err
}

func (c convertedCost) SpendByCountry(ctx context.Context, orgID string, from, to time.Time, scope cost.CountryScope) ([]cost.CountryDaySpend, error) {
	rows, err := c.inner.SpendByCountry(ctx, orgID, from, to, scope)
	for i := range rows {
		rows[i].AmountUSD = c.at(rows[i].Day, rows[i].AmountUSD)
	}
	return rows, err
}

func (c convertedCost) AccountTimezones(ctx context.Context, orgID string, from, to time.Time) ([]string, error) {
	return c.inner.AccountTimezones(ctx, orgID, from, to)
}

// sanitizeMoney turns an amount that met a day without a rate into null.
func sanitizeMoney(acc *rowAccumulator) {
	if acc.revenueUSD != nil && math.IsNaN(*acc.revenueUSD) {
		acc.revenueUSD = nil
		acc.revenueUnknown = true
	}
	if acc.costUSD != nil && math.IsNaN(*acc.costUSD) {
		acc.costUSD = nil
	}
}
