package routingcache

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/routingstore"
)

// fakeStore answers ResolveTrackingLink from a map; everything else is
// unused here. calls counts reads that reached it.
type fakeStore struct {
	mu    sync.Mutex
	links map[string]string // slug -> campaign id
	err   error
	block chan struct{} // when set, reads wait on it
	calls atomic.Int64
}

func (s *fakeStore) ResolveTrackingLink(ctx context.Context, _, slug string) (routingstore.TrackingLink, error) {
	s.calls.Add(1)
	s.mu.Lock()
	block, err := s.block, s.err
	campaign, ok := s.links[slug]
	s.mu.Unlock()
	if block != nil {
		<-block
	}
	if err != nil {
		return routingstore.TrackingLink{}, err
	}
	if !ok {
		return routingstore.TrackingLink{}, routingstore.ErrNotFound
	}
	return routingstore.TrackingLink{CampaignID: campaign}, nil
}

func (s *fakeStore) set(slug, campaign string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.links[slug] = campaign
}

func (s *fakeStore) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

func (s *fakeStore) LoadRoutingConfig(context.Context, string, string) (routingstore.CampaignRouting, error) {
	return routingstore.CampaignRouting{}, errors.New("unused")
}
func (s *fakeStore) LoadLandingPage(context.Context, string, string) (routingstore.LandingPage, error) {
	return routingstore.LandingPage{}, errors.New("unused")
}
func (s *fakeStore) LoadPwaPage(context.Context, string, string) (routingstore.PwaPage, error) {
	return routingstore.PwaPage{}, errors.New("unused")
}
func (s *fakeStore) LoadPwaComments(context.Context, string, string) ([]routingstore.PwaComment, error) {
	return nil, errors.New("unused")
}

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

var testCfg = Config{TTL: 30 * time.Second, NotFoundTTL: 10 * time.Second, LoadTimeout: time.Second, RetryAfterError: 5 * time.Second, MaxNotFound: 3}

func newTestCache(store *fakeStore) (*Cache, *clock) {
	clk := &clock{now: time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)}
	c := New(store, testCfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	c.links.now = clk.Now
	return c, clk
}

func resolve(t *testing.T, c *Cache, slug string) (string, error) {
	t.Helper()
	l, err := c.ResolveTrackingLink(context.Background(), "Track.Example", slug)
	return l.CampaignID, err
}

func eventually(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestFreshEntryIsServedFromMemory(t *testing.T) {
	store := &fakeStore{links: map[string]string{"go": "c1"}}
	c, _ := newTestCache(store)
	for i := 0; i < 5; i++ {
		if got, err := resolve(t, c, "go"); err != nil || got != "c1" {
			t.Fatalf("resolve = %q, %v", got, err)
		}
	}
	if n := store.calls.Load(); n != 1 {
		t.Fatalf("store reads = %d, want 1", n)
	}
}

func TestStaleEntryIsServedWhileRefreshing(t *testing.T) {
	store := &fakeStore{links: map[string]string{"go": "c1"}}
	c, clk := newTestCache(store)
	resolve(t, c, "go")

	store.set("go", "c2")
	clk.advance(testCfg.TTL)
	if got, _ := resolve(t, c, "go"); got != "c1" {
		t.Fatalf("stale read = %q, want the cached c1 at once", got)
	}
	eventually(t, func() bool { got, _ := resolve(t, c, "go"); return got == "c2" }, "the background refresh")
}

// Audit 2026-10-06, RL1: Postgres failing must not fail clicks that were
// already served, and must not be retried on every click.
func TestLastKnownValueIsServedWhilePostgresFails(t *testing.T) {
	store := &fakeStore{links: map[string]string{"go": "c1"}}
	c, clk := newTestCache(store)
	resolve(t, c, "go")

	store.fail(errors.New("connection refused"))
	clk.advance(testCfg.TTL)
	for i := 0; i < 50; i++ {
		if got, err := resolve(t, c, "go"); err != nil || got != "c1" {
			t.Fatalf("during outage: resolve = %q, %v, want the last known c1", got, err)
		}
	}
	eventually(t, func() bool { return store.calls.Load() == 2 }, "one refresh attempt")
	time.Sleep(20 * time.Millisecond)
	if n := store.calls.Load(); n != 2 {
		t.Fatalf("store reads = %d during the outage, want 2 (no retry before RetryAfterError)", n)
	}

	clk.advance(testCfg.RetryAfterError)
	store.fail(nil)
	store.set("go", "c3")
	resolve(t, c, "go")
	eventually(t, func() bool { got, _ := resolve(t, c, "go"); return got == "c3" }, "recovery after RetryAfterError")
}

func TestMissWithPostgresDownIsAnError(t *testing.T) {
	store := &fakeStore{links: map[string]string{}}
	store.fail(errors.New("connection refused"))
	c, _ := newTestCache(store)
	if _, err := resolve(t, c, "go"); err == nil || errors.Is(err, routingstore.ErrNotFound) {
		t.Fatalf("err = %v, want the database error (nothing to serve)", err)
	}
}

func TestNotFoundIsCachedBrieflyAndBounded(t *testing.T) {
	store := &fakeStore{links: map[string]string{}}
	c, clk := newTestCache(store)

	for i := 0; i < 3; i++ {
		if _, err := resolve(t, c, "new"); !errors.Is(err, routingstore.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	}
	if n := store.calls.Load(); n != 1 {
		t.Fatalf("store reads = %d, want 1 (not found is cached)", n)
	}

	// A link created meanwhile appears once NotFoundTTL has passed.
	store.set("new", "c1")
	clk.advance(testCfg.NotFoundTTL)
	if got, err := resolve(t, c, "new"); err != nil || got != "c1" {
		t.Fatalf("after NotFoundTTL: %q, %v, want c1", got, err)
	}

	// Random slugs: only MaxNotFound are remembered.
	for _, slug := range []string{"r1", "r2", "r3", "r4", "r5"} {
		resolve(t, c, slug)
	}
	if got := c.links.notFound; got != testCfg.MaxNotFound {
		t.Fatalf("remembered not-found entries = %d, want the cap %d", got, testCfg.MaxNotFound)
	}
}

func TestConcurrentMissesShareOneLoad(t *testing.T) {
	store := &fakeStore{links: map[string]string{"go": "c1"}, block: make(chan struct{})}
	c, _ := newTestCache(store)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got, err := resolve(t, c, "go"); err != nil || got != "c1" {
				t.Errorf("resolve = %q, %v", got, err)
			}
		}()
	}
	eventually(t, func() bool { return store.calls.Load() == 1 }, "the shared load to start")
	time.Sleep(20 * time.Millisecond)
	close(store.block)
	wg.Wait()
	if n := store.calls.Load(); n != 1 {
		t.Fatalf("store reads = %d for 50 concurrent misses, want 1", n)
	}
}

// A load that started before an edit must not put its (pre-edit) answer
// back after the invalidation.
func TestInvalidateDuringLoadDiscardsTheResult(t *testing.T) {
	preEdit := make(chan struct{})
	store := &fakeStore{links: map[string]string{"go": "old"}, block: preEdit}
	c, _ := newTestCache(store)

	done := make(chan string)
	go func() { got, _ := resolve(t, c, "go"); done <- got }()
	eventually(t, func() bool { return store.calls.Load() == 1 }, "the pre-edit load to start")

	// The edit commits and is announced while that load is still running.
	store.mu.Lock()
	store.links["go"], store.block = "new", nil
	store.mu.Unlock()
	c.Invalidate()

	// A read after the invalidation must not join the pre-edit load (which
	// is still blocked) — joining it would hang here, then return "old".
	fresh := make(chan string, 1)
	go func() { got, _ := resolve(t, c, "go"); fresh <- got }()
	select {
	case got := <-fresh:
		if got != "new" {
			t.Fatalf("after invalidation: %q, want new", got)
		}
	case <-time.After(time.Second):
		close(preEdit)
		t.Fatal("a read after the invalidation joined the pre-edit load")
	}
	// ...and the pre-edit load, finishing late, must not overwrite it.
	close(preEdit)
	if got := <-done; got != "old" {
		t.Fatalf("pre-edit load returned %q, want old (it read before the edit)", got)
	}
	if got, _ := resolve(t, c, "go"); got != "new" {
		t.Fatalf("cache holds %q after the late pre-edit load, want new", got)
	}
}
