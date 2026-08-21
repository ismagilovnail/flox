package chstore

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// PostbackAttempt is one row of postback_events (schema/005_postback_events.sql)
// — an incoming postback's outcome (internal/conversion) or an outgoing
// delivery's attempt (internal/postback), discriminated by Direction.
type PostbackAttempt struct {
	OrganizationID string
	EventAt        time.Time
	Direction      string // "incoming" | "outgoing"

	NetworkID string
	ClickID   string
	Status    string
	EventRef  string
	RawStatus string // incoming only

	Result             string
	Message            string
	AttemptCount       int64  // outgoing only
	ResponseStatusCode int64  // outgoing only
	URL                string // outgoing only

	Revenue  float64 // incoming only
	Currency string
}

const postbackAttemptColumns = `organization_id, event_at, direction, network_id, click_id, status, event_ref, raw_status, result, message, attempt_count, response_status_code, url, revenue, currency`

func scanPostbackAttempt(r driver.Rows) (PostbackAttempt, error) {
	var a PostbackAttempt
	err := r.Scan(
		&a.OrganizationID, &a.EventAt, &a.Direction, &a.NetworkID, &a.ClickID, &a.Status, &a.EventRef, &a.RawStatus,
		&a.Result, &a.Message, &a.AttemptCount, &a.ResponseStatusCode, &a.URL, &a.Revenue, &a.Currency,
	)
	return a, err
}

// ListPostbackAttempts reads postback_events for one organization over
// [from, to], newest first, both directions mixed — the Postback Logs
// page's single table shows incoming and outgoing attempts together,
// discriminated by the Direction column, not as separate queries.
func (s *EventStore) ListPostbackAttempts(ctx context.Context, organizationID string, from, to time.Time, limit, offset int) ([]PostbackAttempt, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT `+postbackAttemptColumns+`
		FROM postback_events
		WHERE organization_id = ? AND event_at >= ? AND event_at <= ?
		ORDER BY event_at DESC
		LIMIT ? OFFSET ?`,
		organizationID, from, to, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying postback attempts: %w", err)
	}
	defer rows.Close()

	var out []PostbackAttempt
	for rows.Next() {
		a, err := scanPostbackAttempt(rows)
		if err != nil {
			return nil, fmt.Errorf("chstore: scanning postback attempt: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chstore: reading postback attempts: %w", err)
	}
	return out, nil
}

// CountPostbackAttempts is ListPostbackAttempts' companion for the list
// page's total count (pagination), over the same [from, to] window.
func (s *EventStore) CountPostbackAttempts(ctx context.Context, organizationID string, from, to time.Time) (int, error) {
	row := s.conn.QueryRow(ctx, `
		SELECT count()
		FROM postback_events
		WHERE organization_id = ? AND event_at >= ? AND event_at <= ?`,
		organizationID, from, to,
	)
	var total uint64
	if err := row.Scan(&total); err != nil {
		return 0, fmt.Errorf("chstore: counting postback attempts: %w", err)
	}
	return int(total), nil
}

// InsertPostbackAttempts batch-writes attempts into postback_events.
func (s *EventStore) InsertPostbackAttempts(ctx context.Context, attempts []PostbackAttempt) error {
	if len(attempts) == 0 {
		return nil
	}

	batch, err := s.conn.PrepareBatch(ctx, "INSERT INTO postback_events")
	if err != nil {
		return fmt.Errorf("chstore: preparing postback_events batch: %w", err)
	}
	for _, a := range attempts {
		if err := batch.Append(
			a.OrganizationID, a.EventAt, a.Direction,
			a.NetworkID, a.ClickID, a.Status, a.EventRef, a.RawStatus,
			a.Result, a.Message, a.AttemptCount, a.ResponseStatusCode, a.URL,
			a.Revenue, a.Currency,
		); err != nil {
			return fmt.Errorf("chstore: appending to postback_events batch: %w", err)
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("chstore: sending postback_events batch: %w", err)
	}
	return nil
}
