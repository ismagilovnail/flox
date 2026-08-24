package postback_test

import (
	"context"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/postback"
	"github.com/ismagilovnail/flox/apps/internal/postbacklogs"
)

// TestReplayEnqueuerMapsFieldsThrough exercises ReplayEnqueuer's one job:
// translate postbacklogs.ReplayInput into postback.EnqueueInput without
// dropping or misassigning a field, using the same fakeStore
// deliverer_test.go uses to avoid a database.
func TestReplayEnqueuerMapsFieldsThrough(t *testing.T) {
	store := newFakeStore()
	enq := postback.NewReplayEnqueuer(store)

	id, err := enq.Enqueue(context.Background(), postbacklogs.ReplayInput{
		OrganizationID: "org1", NetworkID: "net1", SourcePostbackID: "postback-row-1",
		ClickID: "click1", Status: "CPA_ACCEPT", URL: "https://network.example/pb",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	got, ok := store.deliveries[id]
	if !ok {
		t.Fatalf("no delivery stored under id %q", id)
	}
	want := &postback.Delivery{
		ID: id, OrganizationID: "org1", NetworkID: "net1", SourcePostbackID: "postback-row-1",
		ClickID: "click1", Status: event.Type("CPA_ACCEPT"), URL: "https://network.example/pb",
	}
	if *got != *want {
		t.Fatalf("stored delivery = %+v, want %+v", *got, *want)
	}
}
