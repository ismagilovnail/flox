package postback_test

import (
	"context"
	"net/http"
	"sort"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/postback"
)

// Audit 2026-10-06, Q1: when the worker stops mid-batch, the delivery
// already in flight finishes and is recorded, and the ones not yet started
// go back to the queue instead of staying 'processing'.
func TestDelivererReleasesUnattemptedClaimsOnShutdown(t *testing.T) {
	store := newFakeStore(
		postback.Delivery{ID: "d1", URL: "https://net.example/1"},
		postback.Delivery{ID: "d2", URL: "https://net.example/2"},
		postback.Delivery{ID: "d3", URL: "https://net.example/3"},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var sent int
	client := &fakeHTTPClient{respond: func(req *http.Request) (*http.Response, error) {
		sent++
		cancel() // SIGTERM arrives during the first request
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		return statusResponse(http.StatusOK)
	}}
	// One at a time, so "the first request" is the only one in flight.
	d := postback.NewDeliverer(store, client, &fakeAttemptLogger{}, quietLogger()).WithConcurrency(1)

	n, err := d.RunOnce(ctx, 10)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 3 {
		t.Fatalf("claimed = %d, want 3", n)
	}
	if sent != 1 {
		t.Fatalf("requests sent = %d, want 1 (no new attempts after shutdown)", sent)
	}
	if got := store.markCount("success"); got != 1 {
		t.Fatalf("success marks = %d, want 1 (the in-flight attempt must complete)", got)
	}
	if len(store.released) != 2 {
		t.Fatalf("released = %v, want the 2 unattempted deliveries", store.released)
	}
	released := append([]string(nil), store.released...)
	sort.Strings(released)
	for _, id := range released {
		if d := store.deliveries[id]; d == nil || d.AttemptCount != 0 {
			t.Fatalf("released delivery %s = %+v, want attempt count restored to 0", id, d)
		}
	}
}
