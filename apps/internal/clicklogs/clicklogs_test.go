package clicklogs_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/clicklogs"
)

// fakeRepo is an in-memory clicklogs.Repository, avoiding the need for a
// real ClickHouse connection to test Service's own logic.
type fakeRepo struct {
	rowsByOrg map[string][]chstore.ClickLogRow
}

func (f *fakeRepo) ListClickLog(_ context.Context, orgID string, from, to time.Time, clickID string, limit, offset int) ([]chstore.ClickLogRow, error) {
	rows := f.rowsByOrg[orgID]
	sorted := make([]chstore.ClickLogRow, len(rows))
	copy(sorted, rows)
	for i := range sorted {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].EventAt.After(sorted[i].EventAt) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	if offset >= len(sorted) {
		return nil, nil
	}
	end := offset + limit
	if end > len(sorted) {
		end = len(sorted)
	}
	return sorted[offset:end], nil
}

func (f *fakeRepo) CountClickLog(_ context.Context, orgID string, from, to time.Time, clickID string) (int, error) {
	return len(f.rowsByOrg[orgID]), nil
}

func TestListValidatesDateRangeAndClampsLimit(t *testing.T) {
	repo := &fakeRepo{rowsByOrg: map[string][]chstore.ClickLogRow{
		"org1": {{EventAt: time.Now(), Type: "SOURCE_CLICK", ClickID: "c1", Country: "US"}},
	}}
	svc := clicklogs.NewService(repo)
	ctx := context.Background()
	now := time.Now()

	if _, err := svc.List(ctx, "", now.AddDate(0, 0, -1), now, "", 0, 0); err == nil {
		t.Fatal("List with empty org id: want an error")
	}
	if _, err := svc.List(ctx, "org1", now, now.AddDate(0, 0, -1), "", 0, 0); err == nil {
		t.Fatal("List with to before from: want an error")
	}
	if _, err := svc.List(ctx, "org1", now.AddDate(0, 0, -200), now, "", 0, 0); err == nil {
		t.Fatal("List with a 200-day range: want a validation error (cap is 90 days)")
	}

	result, err := svc.List(ctx, "org1", now.AddDate(0, 0, -1), now, "", 0, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.Total != 1 || len(result.Logs) != 1 {
		t.Fatalf("List result = %+v, want 1 log", result)
	}
	if result.Logs[0].Country != "US" {
		t.Fatalf("List result country = %q, want US", result.Logs[0].Country)
	}
}

func TestListPaginatesNewestFirst(t *testing.T) {
	base := time.Now()
	repo := &fakeRepo{rowsByOrg: map[string][]chstore.ClickLogRow{
		"org1": {
			{EventAt: base, Type: "SOURCE_CLICK", ClickID: "c1"},
			{EventAt: base.Add(time.Minute), Type: "SOURCE_FILTER", ClickID: "c2"},
			{EventAt: base.Add(2 * time.Minute), Type: "SOURCE_CLICK", ClickID: "c3"},
		},
	}}
	svc := clicklogs.NewService(repo)

	result, err := svc.List(context.Background(), "org1", base.Add(-time.Hour), base.Add(time.Hour), "", 2, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.Total != 3 || len(result.Logs) != 2 {
		t.Fatalf("List page 1 = %+v, want total=3 len=2", result)
	}
	if result.Logs[0].ClickID != "c3" {
		t.Fatalf("List page 1[0].ClickID = %q, want c3 (newest first)", result.Logs[0].ClickID)
	}
}

// The subs array and the IP/geo/device fields must survive the
// chstore.ClickLogRow -> clicklogs.ClickLog mapping unmangled — this is
// the whole point of the page (§ current session: "does FLOX capture
// IP/geo per click").
func TestListMapsIPGeoDeviceAndSubsThrough(t *testing.T) {
	base := time.Now()
	row := chstore.ClickLogRow{
		EventAt: base, Type: "SOURCE_CLICK", CampaignID: "camp1", ClickID: "c1",
		Country: "DE", City: "Berlin", Device: "mobile", OS: "iOS", Browser: "Safari",
		IP: "203.0.113.7", UserAgent: "ua-string", IsBot: true, IsProxy: false,
	}
	row.Subs[0] = "affid-1"
	row.Subs[9] = "affid-10"
	repo := &fakeRepo{rowsByOrg: map[string][]chstore.ClickLogRow{"org1": {row}}}
	svc := clicklogs.NewService(repo)

	result, err := svc.List(context.Background(), "org1", base.Add(-time.Hour), base.Add(time.Hour), "", 0, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := result.Logs[0]
	if got.IP != "203.0.113.7" || got.Country != "DE" || got.City != "Berlin" ||
		got.Device != "mobile" || got.OS != "iOS" || got.Browser != "Safari" ||
		got.UserAgent != "ua-string" || !got.IsBot || got.IsProxy {
		t.Fatalf("List result = %+v, want IP/geo/device/UA/bot mapped through unchanged", got)
	}
	if got.Sub1 != "affid-1" || got.Sub10 != "affid-10" {
		t.Fatalf("List result subs = sub1=%q sub10=%q, want affid-1/affid-10", got.Sub1, got.Sub10)
	}
}
