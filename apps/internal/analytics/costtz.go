package analytics

import (
	"context"
	"time"
)

// CostTimezones answers "is this report's spend cut in other days than its
// clicks?" (tech audit 2026-10-06, AN4). Ad platforms report spend per
// calendar day in the ad account's own zone, and cost_entries keep that
// day; a report cuts clicks in its own zone. When the two zones have
// different UTC offsets, a day's cost sits beside a day of clicks that is
// hours away — the daily totals cannot be re-cut without hourly spend.
//
// Returns the ad-account zones of the synced spend in [from, to] whose
// offset differs from the report zone's at the start or end of the range
// (so a DST change counts); empty when they all match, when there is only
// manual cost, or with no cost source.
func (s *Service) CostTimezones(ctx context.Context, organizationID string, from, to time.Time, timezone string) ([]string, error) {
	if err := s.validateReportInput(nil, nil, from, to); err != nil {
		return nil, err
	}
	sc, err := resolveScope(timezone)
	if err != nil {
		return nil, err
	}
	if s.cost == nil {
		return []string{}, nil
	}
	zones, err := s.cost.AccountTimezones(ctx, organizationID, from, to)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, name := range zones {
		loc, err := time.LoadLocation(name)
		if err != nil {
			continue // only valid zones are stored; a removed one says nothing
		}
		if offsetsDiffer(sc.loc, loc, from, to) {
			out = append(out, name)
		}
	}
	return out, nil
}

// offsetsDiffer compares the two zones' UTC offsets at noon (UTC) of the
// range's first and last day.
func offsetsDiffer(a, b *time.Location, from, to time.Time) bool {
	for _, d := range []time.Time{from, to} {
		at := time.Date(d.Year(), d.Month(), d.Day(), 12, 0, 0, 0, time.UTC)
		_, oa := at.In(a).Zone()
		_, ob := at.In(b).Zone()
		if oa != ob {
			return true
		}
	}
	return false
}
