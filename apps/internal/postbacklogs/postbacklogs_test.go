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
	svc := postbacklogs.NewService(repo)
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
	svc := postbacklogs.NewService(repo)

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
