package chstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

func TestListAndCountClickLog(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()

	orgID := idgen.New()
	campaignID := idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)

	rows := []chstore.ClickLogRow{
		{
			EventAt: day.Add(1 * time.Hour), Type: "SOURCE_CLICK",
			CampaignID: campaignID, ClickID: idgen.New(),
			Country: "US", City: "New York", Device: "desktop", OS: "Windows", Browser: "Chrome",
			IP: "198.51.100.1", UserAgent: "Mozilla/5.0 test-agent",
		},
		{
			EventAt: day.Add(2 * time.Hour), Type: "SOURCE_FILTER",
			CampaignID: campaignID, ClickID: idgen.New(),
			Country: "DE", City: "Berlin", Device: "mobile", OS: "Android", Browser: "Chrome Mobile",
			IP: "198.51.100.2", IsBot: true, FilterReason: "geo not allowed",
		},
		{
			EventAt: day.Add(3 * time.Hour), Type: "SOURCE_CLICK",
			CampaignID: campaignID, ClickID: idgen.New(),
			Country: "US", City: "Chicago", Device: "desktop", OS: "macOS", Browser: "Safari",
			IP: "198.51.100.3",
		},
	}
	for i := range rows {
		rows[i].Subs[0] = "affid-1"
	}
	batch, err := conn.PrepareBatch(ctx, "INSERT INTO click_events")
	if err != nil {
		t.Fatalf("PrepareBatch: %v", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			orgID, r.EventAt, r.Type,
			r.CampaignID, r.ClickID, r.StreamSetID, r.FlowID, r.Destination, uint8(0), int64(0),
			r.Country, r.Region, r.City, r.Device, r.Platform, r.OS, r.OSVersion, r.Browser, r.BrowserVersion, r.Language,
			boolToUInt8ForTest(r.IsBot), boolToUInt8ForTest(r.IsProxy), r.ASN, r.ConnectionType, r.IP, r.UserAgent, r.Referrer,
			"", "", "", "", "",
			r.Subs[0], r.Subs[1], r.Subs[2], r.Subs[3], r.Subs[4], r.Subs[5], r.Subs[6], r.Subs[7], r.Subs[8], r.Subs[9],
			"", "", "", r.FilterReason,
			"", "", "", "", // landing_id, offer_id, pwa_id, traffic_source_id (schema/008-011)
			"", // event_id (schema/014)
			"", // postlanding_id (schema/016)
			"", // network_id (schema/017)
		); err != nil {
			t.Fatalf("batch.Append: %v", err)
		}
	}
	if err := batch.Send(); err != nil {
		t.Fatalf("batch.Send: %v", err)
	}

	from, to := day, day.Add(24*time.Hour)

	total, err := store.CountClickLog(ctx, orgID, from, to)
	if err != nil {
		t.Fatalf("CountClickLog: %v", err)
	}
	if total != 3 {
		t.Fatalf("CountClickLog = %d, want 3", total)
	}

	got, err := store.ListClickLog(ctx, orgID, from, to, 2, 0)
	if err != nil {
		t.Fatalf("ListClickLog: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListClickLog page 1 len = %d, want 2", len(got))
	}
	// Newest first.
	if got[0].ClickID != rows[2].ClickID {
		t.Fatalf("ListClickLog[0].ClickID = %q, want %q (newest first)", got[0].ClickID, rows[2].ClickID)
	}
	if got[0].IP != "198.51.100.3" || got[0].Country != "US" || got[0].City != "Chicago" ||
		got[0].Device != "desktop" || got[0].OS != "macOS" || got[0].Browser != "Safari" {
		t.Fatalf("ListClickLog[0] IP/geo/device round-trip = %+v, want the row inserted above", got[0])
	}
	if got[1].ClickID != rows[1].ClickID || got[1].Type != "SOURCE_FILTER" || !got[1].IsBot || got[1].FilterReason != "geo not allowed" {
		t.Fatalf("ListClickLog[1] = %+v, want the SOURCE_FILTER row with IsBot=true", got[1])
	}
	if got[0].Subs[0] != "affid-1" {
		t.Fatalf("ListClickLog[0].Subs[0] = %q, want affid-1", got[0].Subs[0])
	}

	rest, err := store.ListClickLog(ctx, orgID, from, to, 2, 2)
	if err != nil {
		t.Fatalf("ListClickLog page 2: %v", err)
	}
	if len(rest) != 1 || rest[0].ClickID != rows[0].ClickID {
		t.Fatalf("ListClickLog page 2 = %+v, want the oldest row alone", rest)
	}

	// Tenant isolation: a different org sees none of this.
	otherTotal, err := store.CountClickLog(ctx, idgen.New(), from, to)
	if err != nil {
		t.Fatalf("CountClickLog for another org: %v", err)
	}
	if otherTotal != 0 {
		t.Fatalf("CountClickLog for another org = %d, want 0", otherTotal)
	}
}

func boolToUInt8ForTest(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}
