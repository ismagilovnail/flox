package main

import (
	"context"
	"net/http"
	"testing"
)

// Only an active — ownership-verified — domain tracks (tech audit T6): the
// same link on a domain taken back to pending is a 404.
func TestTrackOnlyOnActiveDomains(t *testing.T) {
	pool := testPool(t)
	fx := seedFlowFixture(t, pool, nil, nil)
	h, _, closeEvents := newLandingHandler(t, pool, false)
	defer closeEvents()

	if rec := get(h, fx.host, "/t/go"); rec.Code != http.StatusFound {
		t.Fatalf("active domain: status %d, want 302", rec.Code)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE domains SET status = 'pending' WHERE organization_id = $1`, fx.orgID); err != nil {
		t.Fatal(err)
	}
	if rec := get(h, fx.host, "/t/go"); rec.Code != http.StatusNotFound {
		t.Fatalf("pending domain: status %d, want 404", rec.Code)
	}
}
