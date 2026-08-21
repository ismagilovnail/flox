package chstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

func TestListAndCountPostbackAttempts(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()

	orgID := idgen.New()
	networkID := idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)

	attempts := []chstore.PostbackAttempt{
		{
			OrganizationID: orgID, EventAt: day.Add(1 * time.Hour), Direction: "incoming",
			NetworkID: networkID, ClickID: idgen.New(), Status: "CPA_HOLD", RawStatus: "lead",
			Result: "success", Message: "conversion recorded",
		},
		{
			OrganizationID: orgID, EventAt: day.Add(2 * time.Hour), Direction: "outgoing",
			NetworkID: networkID, ClickID: idgen.New(), Status: "CPA_ACCEPT",
			Result: "success", Message: "delivered", AttemptCount: 1, ResponseStatusCode: 200,
			URL: "https://network.example/postback?status=ftd",
		},
		{
			OrganizationID: orgID, EventAt: day.Add(3 * time.Hour), Direction: "incoming",
			NetworkID: networkID, ClickID: idgen.New(), RawStatus: "bogus",
			Result: "error", Message: "no event mapping configured for status \"bogus\"",
		},
	}
	if err := store.InsertPostbackAttempts(ctx, attempts); err != nil {
		t.Fatalf("InsertPostbackAttempts: %v", err)
	}

	total, err := store.CountPostbackAttempts(ctx, orgID, day, day.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("CountPostbackAttempts: %v", err)
	}
	if total != 3 {
		t.Fatalf("CountPostbackAttempts = %d, want 3", total)
	}

	rows, err := store.ListPostbackAttempts(ctx, orgID, day, day.Add(24*time.Hour), 2, 0)
	if err != nil {
		t.Fatalf("ListPostbackAttempts: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("ListPostbackAttempts with limit 2 = %d rows, want 2", len(rows))
	}
	if !rows[0].EventAt.After(rows[1].EventAt) {
		t.Fatalf("ListPostbackAttempts not newest-first: %+v", rows)
	}
	if rows[0].Result != "error" {
		t.Fatalf("ListPostbackAttempts[0].Result = %q, want error (the newest attempt)", rows[0].Result)
	}

	page2, err := store.ListPostbackAttempts(ctx, orgID, day, day.Add(24*time.Hour), 2, 2)
	if err != nil {
		t.Fatalf("ListPostbackAttempts page 2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("ListPostbackAttempts page 2 (offset 2, limit 2) = %d rows, want 1", len(page2))
	}
	if page2[0].Direction != "incoming" || page2[0].RawStatus != "lead" {
		t.Fatalf("ListPostbackAttempts page 2[0] = %+v, want the oldest (incoming, lead) attempt", page2[0])
	}
}
