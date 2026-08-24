package postbacklogs_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/postbacklogs"
)

// fakeRepo is an in-memory postbacklogs.Repository, avoiding the need for
// a real ClickHouse connection to test Service's own logic.
type fakeRepo struct {
	attemptsByOrg map[string][]chstore.PostbackAttempt
}

func (f *fakeRepo) ListPostbackAttempts(_ context.Context, orgID string, from, to time.Time, limit, offset int) ([]chstore.PostbackAttempt, error) {
	rows := f.attemptsByOrg[orgID]
	sorted := make([]chstore.PostbackAttempt, len(rows))
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

func (f *fakeRepo) CountPostbackAttempts(_ context.Context, orgID string, from, to time.Time) (int, error) {
	return len(f.attemptsByOrg[orgID]), nil
}

func TestListValidatesDateRangeAndClampsLimit(t *testing.T) {
	repo := &fakeRepo{attemptsByOrg: map[string][]chstore.PostbackAttempt{
		"org1": {{EventAt: time.Now(), Direction: "incoming", ClickID: "c1", Result: "success"}},
	}}
	svc := postbacklogs.NewService(repo, nil, nil)
	ctx := context.Background()
	now := time.Now()

	if _, err := svc.List(ctx, "", now.AddDate(0, 0, -1), now, 0, 0); err == nil {
		t.Fatal("List with empty org id: want an error")
	}
	if _, err := svc.List(ctx, "org1", now, now.AddDate(0, 0, -1), 0, 0); err == nil {
		t.Fatal("List with to before from: want an error")
	}
	if _, err := svc.List(ctx, "org1", now.AddDate(0, 0, -200), now, 0, 0); err == nil {
		t.Fatal("List with a 200-day range: want a validation error (cap is 90 days)")
	}

	result, err := svc.List(ctx, "org1", now.AddDate(0, 0, -1), now, 0, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.Total != 1 || len(result.Logs) != 1 {
		t.Fatalf("List result = %+v, want 1 log", result)
	}
	if result.Logs[0].Direction != "incoming" {
		t.Fatalf("List result direction = %q, want incoming", result.Logs[0].Direction)
	}
}

func TestListPaginates(t *testing.T) {
	base := time.Now()
	repo := &fakeRepo{attemptsByOrg: map[string][]chstore.PostbackAttempt{
		"org1": {
			{EventAt: base, Direction: "incoming", ClickID: "c1", Result: "success"},
			{EventAt: base.Add(time.Minute), Direction: "outgoing", ClickID: "c2", Result: "success"},
			{EventAt: base.Add(2 * time.Minute), Direction: "incoming", ClickID: "c3", Result: "error"},
		},
	}}
	svc := postbacklogs.NewService(repo, nil, nil)

	result, err := svc.List(context.Background(), "org1", base.Add(-time.Hour), base.Add(time.Hour), 2, 0)
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

// fakeSourceLookup is an in-memory postbacklogs.SourcePostbackLookup,
// keyed exactly like the real dedup key it resolves.
type fakeSourceLookup struct {
	ids map[[4]string]string // [networkID, clickID, status, eventRef] -> postbacks.id
}

func (f *fakeSourceLookup) FindSuccessID(_ context.Context, _, networkID, clickID, status, eventRef string) (string, bool, error) {
	id, ok := f.ids[[4]string{networkID, clickID, status, eventRef}]
	return id, ok, nil
}

// fakeEnqueuer is an in-memory postbacklogs.OutgoingEnqueuer, recording
// every call so a test can assert exactly what got queued.
type fakeEnqueuer struct {
	calls  []postbacklogs.ReplayInput
	nextID string
}

func (f *fakeEnqueuer) Enqueue(_ context.Context, in postbacklogs.ReplayInput) (string, error) {
	f.calls = append(f.calls, in)
	return f.nextID, nil
}

func TestReplayOutgoingEnqueuesAgainstTheResolvedSourceID(t *testing.T) {
	lookup := &fakeSourceLookup{ids: map[[4]string]string{
		{"net1", "click1", "CPA_ACCEPT", ""}: "postback-row-1",
	}}
	enq := &fakeEnqueuer{nextID: "delivery-1"}
	svc := postbacklogs.NewService(nil, lookup, enq)

	result, err := svc.ReplayOutgoing(context.Background(), "org1", postbacklogs.ReplayOutgoingInput{
		NetworkID: "net1", ClickID: "click1", Status: "CPA_ACCEPT", URL: "https://network.example/pb",
	})
	if err != nil {
		t.Fatalf("ReplayOutgoing: %v", err)
	}
	if result.DeliveryID != "delivery-1" {
		t.Fatalf("DeliveryID = %q, want delivery-1", result.DeliveryID)
	}
	if len(enq.calls) != 1 {
		t.Fatalf("Enqueue calls = %d, want 1", len(enq.calls))
	}
	got := enq.calls[0]
	want := postbacklogs.ReplayInput{
		OrganizationID: "org1", NetworkID: "net1", SourcePostbackID: "postback-row-1",
		ClickID: "click1", Status: "CPA_ACCEPT", URL: "https://network.example/pb",
	}
	if got != want {
		t.Fatalf("Enqueue input = %+v, want %+v", got, want)
	}
}

// A CPA_REDEP click can have more than one successful row, one per
// redeposit — event_ref is what tells them apart, so a replay must use
// the exact one the browser's row carries, not just the first match for
// (network, click, status).
func TestReplayOutgoingUsesEventRefToDisambiguateRedeposits(t *testing.T) {
	lookup := &fakeSourceLookup{ids: map[[4]string]string{
		{"net1", "click1", "CPA_REDEP", "txn-1"}: "postback-row-1",
		{"net1", "click1", "CPA_REDEP", "txn-2"}: "postback-row-2",
	}}
	enq := &fakeEnqueuer{nextID: "delivery-2"}
	svc := postbacklogs.NewService(nil, lookup, enq)

	_, err := svc.ReplayOutgoing(context.Background(), "org1", postbacklogs.ReplayOutgoingInput{
		NetworkID: "net1", ClickID: "click1", Status: "CPA_REDEP", EventRef: "txn-2", URL: "https://network.example/pb",
	})
	if err != nil {
		t.Fatalf("ReplayOutgoing: %v", err)
	}
	if enq.calls[0].SourcePostbackID != "postback-row-2" {
		t.Fatalf("SourcePostbackID = %q, want postback-row-2 (the txn-2 redeposit)", enq.calls[0].SourcePostbackID)
	}
}

func TestReplayOutgoingNotFoundWhenNoMatchingSourceRow(t *testing.T) {
	svc := postbacklogs.NewService(nil, &fakeSourceLookup{ids: map[[4]string]string{}}, &fakeEnqueuer{})

	_, err := svc.ReplayOutgoing(context.Background(), "org1", postbacklogs.ReplayOutgoingInput{
		NetworkID: "net1", ClickID: "click1", Status: "CPA_ACCEPT", URL: "https://network.example/pb",
	})
	if err == nil {
		t.Fatal("ReplayOutgoing with no matching source row: want an error")
	}
}

func TestReplayOutgoingValidatesRequiredFields(t *testing.T) {
	svc := postbacklogs.NewService(nil, &fakeSourceLookup{}, &fakeEnqueuer{})

	if _, err := svc.ReplayOutgoing(context.Background(), "", postbacklogs.ReplayOutgoingInput{
		NetworkID: "net1", ClickID: "click1", Status: "CPA_ACCEPT", URL: "https://network.example/pb",
	}); err == nil {
		t.Fatal("ReplayOutgoing with empty organization id: want an error")
	}
	if _, err := svc.ReplayOutgoing(context.Background(), "org1", postbacklogs.ReplayOutgoingInput{
		ClickID: "click1", Status: "CPA_ACCEPT", URL: "https://network.example/pb",
	}); err == nil {
		t.Fatal("ReplayOutgoing with no networkId: want an error")
	}
	if _, err := svc.ReplayOutgoing(context.Background(), "org1", postbacklogs.ReplayOutgoingInput{
		NetworkID: "net1", ClickID: "click1", Status: "CPA_ACCEPT",
	}); err == nil {
		t.Fatal("ReplayOutgoing with no url: want an error")
	}
}
