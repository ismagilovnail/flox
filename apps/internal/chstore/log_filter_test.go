package chstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// The raw-event logs find one click by its exact id: list and count agree,
// an unknown id finds nothing, and another organization's click id finds
// nothing either (the organization still filters first).
func TestLogsFilterByClickID(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewRawEventStore(conn)
	ctx := context.Background()
	org, other := idgen.New(), idgen.New()
	day := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	from, to := day, day.Add(24*time.Hour)
	wanted, rest, theirs := idgen.New(), idgen.New(), idgen.New()

	clicks := []event.Event{
		{Type: event.SourceClick, EventAt: day.Add(time.Hour), OrganizationID: org, CampaignID: "c", ClickID: wanted},
		{Type: event.SourceClick, EventAt: day.Add(2 * time.Hour), OrganizationID: org, CampaignID: "c", ClickID: rest},
		{Type: event.SourceClick, EventAt: day.Add(2 * time.Hour), OrganizationID: other, CampaignID: "c", ClickID: theirs},
		{Type: event.CpaHold, EventAt: day.Add(3 * time.Hour), OrganizationID: org, CampaignID: "c", ClickID: wanted},
		{Type: event.CpaAccept, EventAt: day.Add(4 * time.Hour), OrganizationID: org, CampaignID: "c", ClickID: wanted},
		{Type: event.CpaHold, EventAt: day.Add(3 * time.Hour), OrganizationID: org, CampaignID: "c", ClickID: rest},
		{Type: event.CpaHold, EventAt: day.Add(3 * time.Hour), OrganizationID: other, CampaignID: "c", ClickID: theirs},
	}
	if err := store.InsertBatch(ctx, clicks); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}
	attempts := []chstore.PostbackAttempt{
		{OrganizationID: org, EventAt: day.Add(3 * time.Hour), Direction: "incoming", ClickID: wanted, Result: "success"},
		{OrganizationID: org, EventAt: day.Add(4 * time.Hour), Direction: "incoming", ClickID: rest, Result: "success"},
		{OrganizationID: other, EventAt: day.Add(4 * time.Hour), Direction: "incoming", ClickID: theirs, Result: "success"},
	}
	if err := store.InsertPostbackAttempts(ctx, attempts); err != nil {
		t.Fatalf("InsertPostbackAttempts: %v", err)
	}

	for clickID, want := range map[string]int{wanted: 1, idgen.New(): 0, theirs: 0} {
		rows, err := store.ListClickLog(ctx, org, from, to, clickID, 50, 0, nil)
		n, cerr := store.CountClickLog(ctx, org, from, to, clickID, nil)
		if err != nil || cerr != nil || len(rows) != want || n != want || (want == 1 && rows[0].ClickID != wanted) {
			t.Fatalf("click log for %s = %d rows, count %d (%v, %v); want %d", clickID, len(rows), n, err, cerr, want)
		}
	}
	for clickID, want := range map[string]int{wanted: 2, idgen.New(): 0, theirs: 0} {
		rows, err := store.ListConversions(ctx, org, from, to, clickID, 50, 0, nil)
		n, cerr := store.CountConversions(ctx, org, from, to, clickID, nil)
		if err != nil || cerr != nil || len(rows) != want || n != want {
			t.Fatalf("conversions for %s = %d rows, count %d (%v, %v); want %d", clickID, len(rows), n, err, cerr, want)
		}
	}
	for clickID, want := range map[string]int{wanted: 1, idgen.New(): 0, theirs: 0} {
		rows, err := store.ListPostbackAttempts(ctx, org, from, to, clickID, 50, 0, nil)
		n, cerr := store.CountPostbackAttempts(ctx, org, from, to, clickID, nil)
		if err != nil || cerr != nil || len(rows) != want || n != want {
			t.Fatalf("postback attempts for %s = %d rows, count %d (%v, %v); want %d", clickID, len(rows), n, err, cerr, want)
		}
	}
	// No click id: the whole window, as before.
	if n, err := store.CountClickLog(ctx, org, from, to, "", nil); err != nil || n != 2 {
		t.Fatalf("click log without a click id = %d, %v; want 2", n, err)
	}
}
