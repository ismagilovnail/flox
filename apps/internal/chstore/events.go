package chstore

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/ismagilovnail/flox/apps/internal/event"
)

// EventStore writes event.Event rows into §48's three event-derived tables
// — click_events, tracking_events, conversion_events (schema/001-003.sql).
type EventStore struct {
	conn driver.Conn
}

func NewEventStore(conn driver.Conn) *EventStore { return &EventStore{conn: conn} }

// InsertBatch routes each event to its table by type — event.Type.IsClick()
// for click_events, IsCPA() for conversion_events, everything else to
// tracking_events (event.All's classification is exhaustive and disjoint,
// guarded by event.TestEventClassificationIsExhaustiveAndDisjoint) — and
// sends one ClickHouse batch per table. A caller passing a mixed batch
// (the normal case: apps/worker's flusher claims whatever's due, not
// grouped by type) still only pays for as many Send() calls as it has
// non-empty buckets, never one per row.
func (s *EventStore) InsertBatch(ctx context.Context, events []event.Event) error {
	if len(events) == 0 {
		return nil
	}

	var clicks, tracking, conversions []event.Event
	for _, e := range events {
		switch {
		case e.Type.IsClick():
			clicks = append(clicks, e)
		case e.Type.IsCPA():
			conversions = append(conversions, e)
		default:
			tracking = append(tracking, e)
		}
	}

	if err := s.insertClickEvents(ctx, clicks); err != nil {
		return err
	}
	if err := s.insertTrackingEvents(ctx, tracking); err != nil {
		return err
	}
	if err := s.insertConversionEvents(ctx, conversions); err != nil {
		return err
	}
	return nil
}

func (s *EventStore) insertClickEvents(ctx context.Context, events []event.Event) error {
	if len(events) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, "INSERT INTO click_events")
	if err != nil {
		return fmt.Errorf("chstore: preparing click_events batch: %w", err)
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
		); err != nil {
			return fmt.Errorf("chstore: appending to click_events batch: %w", err)
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("chstore: sending click_events batch: %w", err)
	}
	return nil
}

func (s *EventStore) insertTrackingEvents(ctx context.Context, events []event.Event) error {
	if len(events) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, "INSERT INTO tracking_events")
	if err != nil {
		return fmt.Errorf("chstore: preparing tracking_events batch: %w", err)
	}
	for _, e := range events {
		if err := batch.Append(
			e.OrganizationID, e.EventAt, string(e.Type),
			e.CampaignID, e.ClickID, e.StreamSetID, e.FlowID, e.Destination,
			e.Country, e.Region, e.City, e.Device, e.Platform, e.OS, e.OSVersion, e.Browser, e.BrowserVersion, e.Language,
			boolToUInt8(e.IsBot), boolToUInt8(e.IsProxy), e.IP, e.UserAgent, e.Referrer,
			e.UTMSource, e.UTMMedium, e.UTMCampaign, e.UTMContent, e.UTMTerm,
			e.Subs[0], e.Subs[1], e.Subs[2], e.Subs[3], e.Subs[4], e.Subs[5], e.Subs[6], e.Subs[7], e.Subs[8], e.Subs[9],
			e.ExternalClickID,
		); err != nil {
			return fmt.Errorf("chstore: appending to tracking_events batch: %w", err)
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("chstore: sending tracking_events batch: %w", err)
	}
	return nil
}

func (s *EventStore) insertConversionEvents(ctx context.Context, events []event.Event) error {
	if len(events) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, "INSERT INTO conversion_events")
	if err != nil {
		return fmt.Errorf("chstore: preparing conversion_events batch: %w", err)
	}
	for _, e := range events {
		if err := batch.Append(
			e.OrganizationID, e.EventAt, string(e.Type),
			e.CampaignID, e.ClickID, e.StreamSetID, e.FlowID,
			e.Country, e.Region, e.City, e.Device, e.Platform,
			e.UTMSource, e.UTMMedium, e.UTMCampaign, e.UTMContent, e.UTMTerm,
			e.Subs[0], e.Subs[1], e.Subs[2], e.Subs[3], e.Subs[4], e.Subs[5], e.Subs[6], e.Subs[7], e.Subs[8], e.Subs[9],
			e.ExternalClickID,
			e.NetworkID, e.Revenue, e.Currency, e.USDValue, boolToUInt8(e.HasUSDValue), e.EventRef, e.NetworkTxnID,
			e.AttributionOutcome, e.AttributionMethod, e.TimeToConversionMS,
		); err != nil {
			return fmt.Errorf("chstore: appending to conversion_events batch: %w", err)
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("chstore: sending conversion_events batch: %w", err)
	}
	return nil
}

func boolToUInt8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}
