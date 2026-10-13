// Package routingcache keeps apps/tracker's routing reads in memory, so a
// click does not need Postgres (tech audit RL1): uncached, every click
// runs six sequential queries (routingstore.ResolveTrackingLink +
// LoadRoutingConfig), a slow database makes every redirect slow, and a
// down one turns every click into a 502.
//
// For each read (tracking link, campaign routing config, internal landing
// page, PWA page):
//
//   - a fresh entry (younger than TTL) is served from memory;
//   - a stale entry is served at once while one background refresh runs;
//   - a miss loads synchronously, with LoadTimeout, and concurrent misses
//     for one key share a single query;
//   - when Postgres fails, the last known answer keeps being served — for
//     as long as the outage lasts; a database that is down cannot change
//     the configuration anyway;
//   - "not found" is cached for NotFoundTTL only, and capped in number, so
//     a flood of random slugs cannot grow the cache without bound.
//
// Configuration edits invalidate the whole cache at once: Postgres triggers
// (migration 00050) NOTIFY flox_routing_changed on every write to a routing
// table, and Listen flushes on each notification — and after reconnecting,
// for anything missed meanwhile. TTL bounds staleness if the listener is
// down. Flushing everything is deliberate: edits are rare next to clicks,
// and a flush costs one reload per active campaign.
package routingcache

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/routingstore"
)

// Store is what the cache reads through — routingstore.Store.
type Store interface {
	ResolveTrackingLink(ctx context.Context, host, slug string) (routingstore.TrackingLink, error)
	LoadRoutingConfig(ctx context.Context, orgID, campaignID string) (routingstore.CampaignRouting, error)
	LoadLandingPage(ctx context.Context, orgID, landingID string) (routingstore.LandingPage, error)
	LoadPwaPage(ctx context.Context, orgID, pwaID string) (routingstore.PwaPage, error)
	LoadPwaComments(ctx context.Context, orgID, pwaID string) ([]routingstore.PwaComment, error)
	LoadPostlanding(ctx context.Context, orgID, postlandingID string) (routingstore.FlowPostlanding, error)
}

var _ Store = (*routingstore.Store)(nil)

type Config struct {
	// TTL is how long an entry is served without a refresh.
	TTL time.Duration
	// NotFoundTTL is how long "not found" is remembered.
	NotFoundTTL time.Duration
	// LoadTimeout bounds one load from Postgres.
	LoadTimeout time.Duration
	// RetryAfterError is how long a stale entry waits before the next
	// refresh after a failed one, so an outage is not hammered per click.
	RetryAfterError time.Duration
	// MaxNotFound caps remembered "not found" answers per table.
	MaxNotFound int
}

func (c Config) withDefaults() Config {
	if c.TTL <= 0 {
		c.TTL = 30 * time.Second
	}
	if c.NotFoundTTL <= 0 {
		c.NotFoundTTL = 10 * time.Second
	}
	if c.LoadTimeout <= 0 {
		c.LoadTimeout = time.Second
	}
	if c.RetryAfterError <= 0 {
		c.RetryAfterError = 5 * time.Second
	}
	if c.MaxNotFound <= 0 {
		c.MaxNotFound = 10_000
	}
	return c
}

// Cache implements Store over another Store.
type Cache struct {
	store  Store
	logger *slog.Logger

	links    *table[string, routingstore.TrackingLink]
	routing  *table[string, routingstore.CampaignRouting]
	landings *table[string, routingstore.LandingPage]
	pwas     *table[string, routingstore.PwaPage]
	posts    *table[string, routingstore.FlowPostlanding]

	invalidations atomic.Uint64
}

var _ Store = (*Cache)(nil)

func New(store Store, cfg Config, logger *slog.Logger) *Cache {
	cfg = cfg.withDefaults()
	return &Cache{
		store:    store,
		logger:   logger,
		links:    newTable[string, routingstore.TrackingLink]("tracking_link", cfg, logger),
		routing:  newTable[string, routingstore.CampaignRouting]("routing_config", cfg, logger),
		landings: newTable[string, routingstore.LandingPage]("landing_page", cfg, logger),
		pwas:     newTable[string, routingstore.PwaPage]("pwa_page", cfg, logger),
		posts:    newTable[string, routingstore.FlowPostlanding]("postlanding", cfg, logger),
	}
}

func (c *Cache) ResolveTrackingLink(ctx context.Context, host, slug string) (routingstore.TrackingLink, error) {
	// ResolveTrackingLink matches the host case-insensitively; so does the key.
	return c.links.get(ctx, strings.ToLower(host)+"/"+slug, func(ctx context.Context) (routingstore.TrackingLink, error) {
		return c.store.ResolveTrackingLink(ctx, host, slug)
	})
}

func (c *Cache) LoadRoutingConfig(ctx context.Context, orgID, campaignID string) (routingstore.CampaignRouting, error) {
	return c.routing.get(ctx, orgID+"/"+campaignID, func(ctx context.Context) (routingstore.CampaignRouting, error) {
		return c.store.LoadRoutingConfig(ctx, orgID, campaignID)
	})
}

func (c *Cache) LoadLandingPage(ctx context.Context, orgID, landingID string) (routingstore.LandingPage, error) {
	return c.landings.get(ctx, orgID+"/"+landingID, func(ctx context.Context) (routingstore.LandingPage, error) {
		return c.store.LoadLandingPage(ctx, orgID, landingID)
	})
}

func (c *Cache) LoadPwaPage(ctx context.Context, orgID, pwaID string) (routingstore.PwaPage, error) {
	return c.pwas.get(ctx, orgID+"/"+pwaID, func(ctx context.Context) (routingstore.PwaPage, error) {
		return c.store.LoadPwaPage(ctx, orgID, pwaID)
	})
}

func (c *Cache) LoadPostlanding(ctx context.Context, orgID, postlandingID string) (routingstore.FlowPostlanding, error) {
	return c.posts.get(ctx, orgID+"/"+postlandingID, func(ctx context.Context) (routingstore.FlowPostlanding, error) {
		return c.store.LoadPostlanding(ctx, orgID, postlandingID)
	})
}

// LoadPwaComments is not cached: only the PWA constructor's test view reads
// them, and an operator previewing an edit must see it.
func (c *Cache) LoadPwaComments(ctx context.Context, orgID, pwaID string) ([]routingstore.PwaComment, error) {
	return c.store.LoadPwaComments(ctx, orgID, pwaID)
}

// Invalidate drops every entry. Loads already in flight finish but do not
// store their (possibly pre-edit) result.
func (c *Cache) Invalidate() {
	c.links.invalidate()
	c.routing.invalidate()
	c.landings.invalidate()
	c.pwas.invalidate()
	c.posts.invalidate()
	c.invalidations.Add(1)
	metrics.RoutingCacheInvalidationsTotal.Inc()
}

// Invalidations counts flushes so far.
func (c *Cache) Invalidations() uint64 { return c.invalidations.Load() }

// Warm reports whether at least one campaign's routing is cached — the
// tracker can serve those clicks even with Postgres down (readiness).
func (c *Cache) Warm() bool { return c.routing.size() > 0 }

// table is one cached read.
type table[K comparable, V any] struct {
	name   string
	cfg    Config
	logger *slog.Logger
	now    func() time.Time

	mu       sync.Mutex
	entries  map[K]*entry[V]
	notFound int
	gen      uint64
	flight   singleflight.Group
}

type entry[V any] struct {
	val        V
	notFound   bool
	loadedAt   time.Time
	refreshing bool
	retryAt    time.Time
}

func newTable[K comparable, V any](name string, cfg Config, logger *slog.Logger) *table[K, V] {
	return &table[K, V]{name: name, cfg: cfg, logger: logger, now: time.Now, entries: map[K]*entry[V]{}}
}

func (t *table[K, V]) count(result string) {
	metrics.RoutingCacheRequestsTotal.WithLabelValues(t.name, result).Inc()
}

func (t *table[K, V]) get(ctx context.Context, key K, load func(context.Context) (V, error)) (V, error) {
	now := t.now()
	t.mu.Lock()
	e, ok := t.entries[key]
	var cached entry[V]
	if ok {
		cached = *e
	}
	gen := t.gen
	refresh := false
	if ok && !cached.notFound && now.Sub(cached.loadedAt) >= t.cfg.TTL && !cached.refreshing && !now.Before(cached.retryAt) {
		e.refreshing = true
		refresh = true
	}
	t.mu.Unlock()

	if ok {
		switch {
		case cached.notFound && now.Sub(cached.loadedAt) < t.cfg.NotFoundTTL:
			t.count("hit")
			var zero V
			return zero, routingstore.ErrNotFound
		case !cached.notFound && now.Sub(cached.loadedAt) < t.cfg.TTL:
			t.count("hit")
			return cached.val, nil
		case !cached.notFound:
			// Stale: answer now, refresh off the request path.
			if refresh {
				go t.refresh(key, gen, load)
			}
			t.count("stale")
			return cached.val, nil
		}
		// A stale "not found" is re-checked synchronously: a link that was
		// just created must start working at once.
	}

	v, err := t.load(ctx, key, gen, load)
	switch {
	case err == nil:
		t.count("miss")
		return v, nil
	case errors.Is(err, routingstore.ErrNotFound):
		t.count("miss")
		return v, err
	case ok && cached.notFound:
		// Postgres is failing; the last answer was "not found" — keep it.
		t.count("stale_on_error")
		return v, routingstore.ErrNotFound
	default:
		t.count("error")
		return v, err
	}
}

// load reads key once for every concurrent caller and stores the answer,
// unless the cache was invalidated meanwhile.
func (t *table[K, V]) load(ctx context.Context, key K, gen uint64, load func(context.Context) (V, error)) (V, error) {
	// The generation is part of the flight key: a caller arriving after an
	// invalidation must not join a load that started before the edit.
	res, err, _ := t.flight.Do(fmt.Sprint(gen, "/", key), func() (any, error) {
		// Detached from the caller: one visitor hanging up must not fail
		// the load for everyone waiting on it.
		lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), t.cfg.LoadTimeout)
		defer cancel()
		v, err := load(lctx)
		switch {
		case err == nil:
			t.store(key, gen, entry[V]{val: v, loadedAt: t.now()})
		case errors.Is(err, routingstore.ErrNotFound):
			t.store(key, gen, entry[V]{notFound: true, loadedAt: t.now()})
		}
		return v, err
	})
	v, _ := res.(V)
	return v, err
}

func (t *table[K, V]) refresh(key K, gen uint64, load func(context.Context) (V, error)) {
	_, err := t.load(context.Background(), key, gen, load)
	if err == nil || errors.Is(err, routingstore.ErrNotFound) {
		return
	}
	t.count("refresh_error")
	t.logger.Warn("routing cache refresh failed, serving the last known value", "table", t.name, "error", err)
	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.entries[key]; ok && t.gen == gen {
		e.refreshing = false
		e.retryAt = t.now().Add(t.cfg.RetryAfterError)
	}
}

func (t *table[K, V]) store(key K, gen uint64, e entry[V]) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.gen != gen {
		return // invalidated while loading: the result may predate the edit
	}
	old, existed := t.entries[key]
	if e.notFound && (!existed || !old.notFound) {
		if t.notFound >= t.cfg.MaxNotFound {
			// Bounded: under a flood of unknown slugs, stop remembering
			// them (each still gets a correct, uncached 404).
			if existed {
				delete(t.entries, key)
			}
			return
		}
		t.notFound++
	}
	if !e.notFound && existed && old.notFound {
		t.notFound--
	}
	t.entries[key] = &e
}

func (t *table[K, V]) invalidate() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries = map[K]*entry[V]{}
	t.notFound = 0
	t.gen++
}

func (t *table[K, V]) size() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.entries) - t.notFound
}
