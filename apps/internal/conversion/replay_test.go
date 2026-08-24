package conversion_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/postbacklogs"
)

// fakeNetworkLookup is an in-memory conversion.NetworkLookup, standing in
// for PostgresNetworkLookup to test ReplayNetworkLookup's own adapting
// job without a database.
type fakeNetworkLookup struct {
	byID map[string]conversion.Network
}

func (f *fakeNetworkLookup) ByID(_ context.Context, networkID string) (conversion.Network, error) {
	n, ok := f.byID[networkID]
	if !ok {
		return conversion.Network{}, conversion.ErrNetworkNotFound
	}
	return n, nil
}

// TestReplayNetworkLookupMapsFieldsAndErrorThrough exercises
// ReplayNetworkLookup's one job: translate conversion.Network/
// ErrNetworkNotFound into postbacklogs.IncomingNetwork/
// ErrIncomingNetworkNotFound without postbacklogs ever importing
// conversion to recognize the sentinel.
func TestReplayNetworkLookupMapsFieldsAndErrorThrough(t *testing.T) {
	lookup := conversion.NewReplayNetworkLookup(&fakeNetworkLookup{byID: map[string]conversion.Network{
		"net-1": {ID: "net-1", OrganizationID: orgA, AcceptDuplicates: true, PostbackURL: "https://network.example/out"},
	}})

	n, err := lookup.ByID(ctx(), "net-1")
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	want := postbacklogs.IncomingNetwork{OrganizationID: orgA, AcceptDuplicates: true, PostbackURL: "https://network.example/out"}
	if n != want {
		t.Fatalf("ByID = %+v, want %+v", n, want)
	}

	if _, err := lookup.ByID(ctx(), "missing"); !errors.Is(err, postbacklogs.ErrIncomingNetworkNotFound) {
		t.Fatalf("ByID for an unknown network: err = %v, want postbacklogs.ErrIncomingNetworkNotFound", err)
	}
}

// TestReplayRecorderRunsThroughTheRealEngine exercises ReplayRecorder end
// to end against a real *conversion.Service (built the same way
// newHarness() builds one for every other test in this package) — a
// replay must hit the exact same mapping/dedup/attribution/FX path a live
// network hit does, not a shortcut around it.
func TestReplayRecorderRunsThroughTheRealEngine(t *testing.T) {
	svc, mapper, _, events, deliveries := newHarness()
	mapper.set("net-1", "ftd", event.CpaAccept)
	recorder := conversion.NewReplayRecorder(svc)

	revenue := 100.0
	outcome, err := recorder.Record(ctx(), postbacklogs.IncomingRecord{
		OrganizationID: orgA, NetworkID: "net-1", ClickID: "click-1", RawStatus: "ftd",
		Revenue: &revenue, Currency: "EUR", PostbackURL: "https://network.example/out",
		OccurredAt: time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if outcome.Result != string(conversion.ResultSuccess) || outcome.Status != string(event.CpaAccept) {
		t.Fatalf("outcome = %+v, want a success CPA_ACCEPT", outcome)
	}
	if outcome.ID == "" {
		t.Fatal("outcome.ID was left empty")
	}
	if events.count() != 1 {
		t.Fatalf("events emitted = %d, want 1 (a successful replay records exactly like a first attempt)", events.count())
	}
	if deliveries.count() != 1 {
		t.Fatalf("deliveries enqueued = %d, want 1 (PostbackURL was set)", deliveries.count())
	}

	// A second replay of the exact same attempt must come back duplicate,
	// not a second success — the whole point of running through the real
	// engine instead of a shortcut.
	outcome2, err := recorder.Record(ctx(), postbacklogs.IncomingRecord{
		OrganizationID: orgA, NetworkID: "net-1", ClickID: "click-1", RawStatus: "ftd",
		Revenue: &revenue, Currency: "EUR", PostbackURL: "https://network.example/out",
		OccurredAt: time.Date(2026, 8, 24, 12, 5, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Record (replay of a replay): %v", err)
	}
	if outcome2.Result != string(conversion.ResultDuplicate) {
		t.Fatalf("second identical replay Result = %q, want duplicate", outcome2.Result)
	}
	if events.count() != 1 {
		t.Fatal("a duplicate replay must not emit a second event")
	}
}
