package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/classifier"
	"github.com/ismagilovnail/flox/apps/internal/clientip"
	"github.com/ismagilovnail/flox/apps/internal/eventbuf"
	"github.com/ismagilovnail/flox/apps/internal/routing"
	"github.com/ismagilovnail/flox/apps/internal/routingcache"
	"github.com/ismagilovnail/flox/apps/internal/routingstore"
)

// outageStore is routingstore with a switch that makes every read fail like
// an unreachable Postgres.
type outageStore struct {
	*routingstore.Store
	down atomic.Bool
}

var errPostgresDown = errors.New("dial tcp: connection refused")

func (s *outageStore) ResolveTrackingLink(ctx context.Context, host, slug string) (routingstore.TrackingLink, error) {
	if s.down.Load() {
		return routingstore.TrackingLink{}, errPostgresDown
	}
	return s.Store.ResolveTrackingLink(ctx, host, slug)
}

func (s *outageStore) LoadRoutingConfig(ctx context.Context, orgID, campaignID string) (routingstore.CampaignRouting, error) {
	if s.down.Load() {
		return routingstore.CampaignRouting{}, errPostgresDown
	}
	return s.Store.LoadRoutingConfig(ctx, orgID, campaignID)
}

func cachedHandler(t *testing.T, store routingcache.Store, cfg routingcache.Config) (http.Handler, *routingcache.Cache) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cache := routingcache.New(store, cfg, logger)
	events := eventbuf.New(eventbuf.DiscardSink{}, logger, eventbuf.Config{})
	t.Cleanup(events.Close)
	resolver, err := clientip.New(nil, logger)
	if err != nil {
		t.Fatalf("clientip: %v", err)
	}
	h := &Handler{
		clientIP: resolver, store: cache, classifier: classifier.New(nil, nil, nil),
		engine: &routing.Engine{}, events: events, logger: logger,
	}
	r := chi.NewRouter()
	h.Register(r)
	return r, cache
}

// Audit 2026-10-06, RL1/A1: with Postgres unreachable, a link the tracker
// has served keeps redirecting from memory; one it never saw cannot be
// answered.
func TestClicksSurvivePostgresOutage(t *testing.T) {
	pool := testPool(t)
	seen := seedFlowFixture(t, pool, nil, nil)
	unseen := seedFlowFixture(t, pool, nil, nil)
	store := &outageStore{Store: routingstore.New(pool)}
	// A tiny TTL: every click during the outage finds its entry stale.
	h, _ := cachedHandler(t, store, routingcache.Config{TTL: time.Millisecond})

	assertRedirect(t, get(h, seen.host, "/t/go"), landingTestOffer)

	store.down.Store(true)
	time.Sleep(5 * time.Millisecond)
	for i := 0; i < 20; i++ {
		assertRedirect(t, get(h, seen.host, "/t/go"), landingTestOffer)
	}
	if rec := get(h, unseen.host, "/t/go"); rec.Code != http.StatusBadGateway {
		t.Fatalf("never-seen link during the outage: %d, want 502", rec.Code)
	}
}

// An edit reaches the tracker through LISTEN/NOTIFY (migration 00050), not
// through the TTL: with a one-hour TTL only the notification can explain
// the new destination.
func TestEditInvalidatesTheCacheImmediately(t *testing.T) {
	pool := testPool(t)
	fx := seedFlowFixture(t, pool, nil, nil)
	h, cache := cachedHandler(t, routingstore.New(pool), routingcache.Config{TTL: time.Hour, NotFoundTTL: time.Hour})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cache.Listen(ctx, os.Getenv("DATABASE_URL"))
	// The listener flushes once when it connects; wait for that, so the
	// flush below can only come from the edit's NOTIFY.
	waitUntil(t, func() bool { return cache.Invalidations() >= 1 }, "the listener to connect")

	assertRedirect(t, get(h, fx.host, "/t/go"), landingTestOffer)
	before := cache.Invalidations()

	const edited = "https://edited.example/go"
	if _, err := pool.Exec(context.Background(),
		`UPDATE flows SET destination_url = $1 WHERE organization_id = $2`, edited, fx.orgID); err != nil {
		t.Fatalf("editing the flow: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		rec := get(h, fx.host, "/t/go")
		if rec.Header().Get("Location") == edited {
			if cache.Invalidations() == before {
				t.Fatal("new destination served without an invalidation")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("still redirecting to %q 3s after the edit, want %q", rec.Header().Get("Location"), edited)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitUntil(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
