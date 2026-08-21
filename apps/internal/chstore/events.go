package chstore

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/ismagilovnail/flox/apps/internal/event"
)

// EventStore writes event.Event rows into ClickHouse's `events` table
// (schema/001_events.sql).
type EventStore struct {
	conn driver.Conn
}

func NewEventStore(conn driver.Conn) *EventStore { return &EventStore{conn: conn} }

// InsertBatch appends every event in one ClickHouse batch — batching is not
// an optimization here, it's how clickhouse-go's client-side batch API
// works: rows are buffered client-side and sent as one block on Send.
func (s *EventStore) InsertBatch(ctx context.Context, events []event.Event) error {
	if len(events) == 0 {
		return nil
	}

	batch, err := s.conn.PrepareBatch(ctx, "INSERT INTO events")
	if err != nil {
		return fmt.Errorf("chstore: preparing batch: %w", err)
	}

	for _, e := range events {
		if err := batch.Append(
			e.OrganizationID, e.EventAt, string(e.Type),
			e.CampaignID, e.ClickID, e.StreamSetID, e.FlowID, e.Destination, boolToUInt8(e.StickyApplied), e.ConfigVersion,
			e.Country, e.Region, e.City, e.Device, e.Platform, e.OS, e.OSVersion, e.Browser, e.BrowserVersion, e.Language,
			boolToUInt8(e.IsBot), boolToUInt8(e.IsProxy), e.ASN, e.ConnectionType, e.IP, e.UserAgent, e.Referrer,
			e.UTMSource, e.UTMMedium, e.UTMCampaign, e.UTMContent, e.UTMTerm,
			e.Subs[0], e.Subs[1], e.Subs[2], e.Subs[3], e.Subs[4], e.Subs[5], e.Subs[6], e.Subs[7], e.Subs[8], e.Subs[9],
			e.ExternalClickID, e.FBClickID, e.TTClickID, e.FilterReason,
			e.NetworkID, e.Revenue, e.Currency, e.USDValue, boolToUInt8(e.HasUSDValue), e.EventRef, e.NetworkTxnID,
			e.AttributionOutcome, e.AttributionMethod, e.TimeToConversionMS,
		); err != nil {
			return fmt.Errorf("chstore: appending event to batch: %w", err)
		}
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("chstore: sending batch: %w", err)
	}
	return nil
}

func boolToUInt8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}
