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

// A sink that never recovers is retried until Close, then given up on within
// ShutdownRetryBudget — counted as failed, never written, never hanging.
func TestDeadSinkIsGivenUpOnAtShutdown(t *testing.T) {
	w := eventbuf.New(failingSink{}, quietLogger(), eventbuf.Config{
		BufferSize: 10, BatchSize: 1, FlushInterval: time.Hour,
		RetryMin: time.Millisecond, RetryMax: 5 * time.Millisecond, ShutdownRetryBudget: 50 * time.Millisecond,
	})
	w.Enqueue(event.Event{Type: event.SourceClick})
	waitFor(t, func() bool { return w.Stats().Retried >= 3 }, "the failing batch to be retried")
	if w.Stats().Failed != 0 {
		t.Fatal("a batch still being retried was already counted as failed")
	}

	start := time.Now()
	w.Close()
	if took := time.Since(start); took > time.Second {
		t.Fatalf("Close took %s with a dead sink, want about the 50ms retry budget", took)
	}
	if s := w.Stats(); s.Failed != 1 || s.Written != 0 {
		t.Fatalf("stats = %+v, want 1 failed, 0 written", s)
	}
}

// Audit 2026-10-06, RL2: a sink that fails for a while (a Postgres blip)
// loses nothing — the batch is retried until it goes through.
func TestTransientSinkFailureIsRetried(t *testing.T) {
	sink := &flakySink{failures: 3}
	w := eventbuf.New(sink, quietLogger(), eventbuf.Config{
		BatchSize: 2, FlushInterval: time.Hour, RetryMin: time.Millisecond, RetryMax: 5 * time.Millisecond,
	})
	w.Enqueue(event.Event{Type: event.SourceClick, ClickID: "a"})
	w.Enqueue(event.Event{Type: event.SourceClick, ClickID: "b"})
	waitFor(t, func() bool { return w.Stats().Written == 2 }, "the batch to be written after the sink recovers")
	w.Close()

	if s := w.Stats(); s.Failed != 0 || s.Retried != 3 {
		t.Fatalf("stats = %+v, want 0 failed and 3 retries", s)
	}
	if got := sink.written(); len(got) != 2 {
		t.Fatalf("sink received %v, want both events exactly once", got)
	}
}

// A SinkError says what to do with each event: written ones are not sent
// again, refused ones are counted as failed, only Retry is retried.
func TestSinkErrorRetriesOnlyWhatItSays(t *testing.T) {
	sink := &partialSink{}
	w := eventbuf.New(sink, quietLogger(), eventbuf.Config{
		BatchSize: 3, FlushInterval: time.Hour, RetryMin: time.Millisecond,
	})
	for _, id := range []string{"written", "refused", "retried"} {
		w.Enqueue(event.Event{Type: event.SourceClick, ClickID: id})
	}
	waitFor(t, func() bool { return w.Stats().Written == 2 }, "the retried event to be written")
	w.Close()

	if s := w.Stats(); s.Failed != 1 || s.Written != 2 {
		t.Fatalf("stats = %+v, want 2 written (one on retry) and 1 failed", s)
	}
	if got := sink.calls(); len(got) != 2 || len(got[1]) != 1 || got[1][0] != "retried" {
		t.Fatalf("sink calls = %v, want the full batch, then only \"retried\"", got)
	}
}

type flakySink struct {
	mu       sync.Mutex
	failures int
	got      []string
}

func (s *flakySink) Write(_ context.Context, batch []event.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failures > 0 {
		s.failures--
		return errBoom
	}
	for _, e := range batch {
		s.got = append(s.got, e.ClickID)
	}
	return nil
}

func (s *flakySink) written() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.got...)
}

// partialSink writes "written", refuses "refused" for good and asks for
// "retried" again on its first call; the second call succeeds.
type partialSink struct {
	mu  sync.Mutex
	log [][]string
}

func (s *partialSink) Write(_ context.Context, batch []event.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []string
	for _, e := range batch {
		ids = append(ids, e.ClickID)
	}
	s.log = append(s.log, ids)
	if len(s.log) > 1 {
		return nil
	}
	var retry []event.Event
	for _, e := range batch {
		if e.ClickID == "retried" {
			retry = append(retry, e)
		}
	}
	return &eventbuf.SinkError{Retry: retry, Dropped: 1, Err: errBoom}
}

func (s *partialSink) calls() [][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][]string(nil), s.log...)
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

// Audit 2026-10-06, RL3: apps/tracker closes the writer explicitly and again
// in a defer; the second close used to panic every clean shutdown.
func TestCloseIsIdempotent(t *testing.T) {
	sink := &collectingSink{}
	w := eventbuf.New(sink, quietLogger(), eventbuf.Config{})
	w.Enqueue(event.Event{Type: event.SourceClick})

	w.Close()
	w.Close() // must not panic

	if sink.count() != 1 {
		t.Fatalf("flushed = %d, want 1 (the first Close drains the buffer)", sink.count())
	}
}

// An event offered after Close has nobody left to flush it; it is counted as
// dropped instead of reported enqueued and silently lost.
func TestEnqueueAfterCloseIsDropped(t *testing.T) {
	w := eventbuf.New(&collectingSink{}, quietLogger(), eventbuf.Config{})
	w.Close()

	if w.Enqueue(event.Event{Type: event.SourceClick}) {
		t.Fatal("Enqueue after Close reported success")
	}
	if s := w.Stats(); s.Dropped != 1 || s.Enqueued != 0 {
		t.Fatalf("stats = %+v, want 1 dropped, 0 enqueued", s)
	}
}

// Every event leaves the buffer with an id (audit 2026-10-06): the queue
// row id that makes a re-sent write a no-op. One already set is kept.
func TestEnqueueAssignsEventIDs(t *testing.T) {
	sink := &collectingSink{}
	w := eventbuf.New(sink, quietLogger(), eventbuf.Config{})
	w.Enqueue(event.Event{Type: event.SourceClick})
	w.Enqueue(event.Event{Type: event.SourceClick, ID: "kept"})
	w.Close()

	if len(sink.events) != 2 || sink.events[0].ID == "" || sink.events[1].ID != "kept" {
		t.Fatalf("ids = %q, %q; want a generated one and \"kept\"", sink.events[0].ID, sink.events[1].ID)
	}
}
