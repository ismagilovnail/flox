package eventbuf_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/eventbuf"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// collectingSink records everything it is handed, safely across the
// background flush goroutine.
type collectingSink struct {
	mu     sync.Mutex
	events []event.Event
}

func (s *collectingSink) Write(ctx context.Context, batch []event.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, batch...)
	return nil
}

func (s *collectingSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

// blockingSink stalls until released, simulating a queue/network outage.
type blockingSink struct {
	release chan struct{}
	once    sync.Once
}

func newBlockingSink() *blockingSink { return &blockingSink{release: make(chan struct{})} }

func (s *blockingSink) Write(ctx context.Context, batch []event.Event) error {
	<-s.release
	return nil
}

func (s *blockingSink) unblock() { s.once.Do(func() { close(s.release) }) }

func TestEnqueueNeverBlocksWhenSinkIsStalled(t *testing.T) {
	// The single most important property in this package (§41: "Do not
	// block the redirect on event persistence"). With a permanently
	// stalled sink and a tiny buffer, Enqueue must still return promptly
	// — dropping events rather than making a user wait on a redirect.
	sink := newBlockingSink()
	defer sink.unblock()

	w := eventbuf.New(sink, quietLogger(), eventbuf.Config{BufferSize: 4, BatchSize: 1, FlushInterval: time.Hour})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			w.Enqueue(event.Event{Type: event.SourceClick})
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Enqueue blocked while the sink was stalled — the redirect path would have hung")
	}

	stats := w.Stats()
	if stats.Dropped == 0 {
		t.Fatal("expected drops with a stalled sink and a 4-deep buffer, got none")
	}
	if stats.Enqueued+stats.Dropped != 1000 {
		t.Fatalf("accounting mismatch: enqueued=%d dropped=%d, want total 1000", stats.Enqueued, stats.Dropped)
	}
}

func TestEnqueueIsFastPerCall(t *testing.T) {
	sink := newBlockingSink()
	defer sink.unblock()
	w := eventbuf.New(sink, quietLogger(), eventbuf.Config{BufferSize: 2, BatchSize: 1, FlushInterval: time.Hour})

	// Even the calls that drop must be cheap — a drop is a non-blocking
	// channel send that fails, not a retry or a wait.
	start := time.Now()
	for i := 0; i < 10_000; i++ {
		w.Enqueue(event.Event{Type: event.SourceClick})
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("10k Enqueue calls took %v — too slow for the hot path", elapsed)
	}
}

func TestFlushesOnBatchSize(t *testing.T) {
	sink := &collectingSink{}
	w := eventbuf.New(sink, quietLogger(), eventbuf.Config{BufferSize: 100, BatchSize: 5, FlushInterval: time.Hour})
	defer w.Close()

	for i := 0; i < 5; i++ {
		w.Enqueue(event.Event{Type: event.SourceClick})
	}

	waitFor(t, func() bool { return sink.count() == 5 }, "batch of 5 to flush on BatchSize")
}

func TestFlushesOnInterval(t *testing.T) {
	sink := &collectingSink{}
	// Batch size far above what we enqueue, so only the interval can flush it.
	w := eventbuf.New(sink, quietLogger(), eventbuf.Config{BufferSize: 100, BatchSize: 1000, FlushInterval: 20 * time.Millisecond})
	defer w.Close()

	w.Enqueue(event.Event{Type: event.SourceClick})

	waitFor(t, func() bool { return sink.count() == 1 }, "partial batch to flush on FlushInterval")
}

func TestCloseDrainsBufferedEvents(t *testing.T) {
	sink := &collectingSink{}
	// Neither BatchSize nor FlushInterval will fire during the test — only
	// Close's drain can deliver these.
	w := eventbuf.New(sink, quietLogger(), eventbuf.Config{BufferSize: 100, BatchSize: 1000, FlushInterval: time.Hour})

	for i := 0; i < 7; i++ {
		w.Enqueue(event.Event{Type: event.SourceClick})
	}
	w.Close()

	if got := sink.count(); got != 7 {
		t.Fatalf("Close() delivered %d events, want 7 — accepted events must not be discarded on shutdown", got)
	}
}

func TestSinkFailureIsCountedNotFatal(t *testing.T) {
	w := eventbuf.New(failingSink{}, quietLogger(), eventbuf.Config{BufferSize: 10, BatchSize: 1, FlushInterval: time.Hour})
	defer w.Close()

	w.Enqueue(event.Event{Type: event.SourceClick})
	waitFor(t, func() bool { return w.Stats().Failed == 1 }, "sink failure to be counted")

	if w.Stats().Written != 0 {
		t.Fatal("a failed write must not count as written")
	}
}

type failingSink struct{}

func (failingSink) Write(ctx context.Context, batch []event.Event) error { return errBoom }

var errBoom = &boomError{}

type boomError struct{}

func (*boomError) Error() string { return "boom" }

func waitFor(t *testing.T, cond func() bool, what string) {
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
