package chstore

import (
	"context"
	"fmt"
	"time"
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
