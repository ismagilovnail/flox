package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// CHUsage counts clicks from click_events_daily_campaign (chstore schema
// 006): a per-day rollup the click_events insert fills synchronously, so
// a month is at most a few thousand summed rows, never a scan of events.
// Its day is toDate(event_at) of a UTC DateTime64 — a UTC day, the same
// boundary as Period.
type CHUsage struct{ conn driver.Conn }

func NewCHUsage(conn driver.Conn) *CHUsage { return &CHUsage{conn: conn} }

func (u *CHUsage) Clicks(ctx context.Context, orgID string, p Period) (int64, error) {
	var n uint64
	if err := u.conn.QueryRow(ctx, `
		SELECT sum(event_count) FROM click_events_daily_campaign
		WHERE organization_id = ? AND type IN ('SOURCE_CLICK', 'SOURCE_FILTER')
		  AND day >= toDate(?) AND day < toDate(?)`,
		orgID, p.From.Format(time.DateOnly), p.To.Format(time.DateOnly),
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("billing: counting clicks: %w", err)
	}
	return int64(n), nil
}

// PeriodAt is the UTC calendar month holding t.
func PeriodAt(t time.Time) Period {
	t = t.UTC()
	from := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	return Period{From: from, To: from.AddDate(0, 1, 0)}
}
