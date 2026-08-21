package eventqueue_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/eventqueue"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeConsumer is an in-memory Consumer, enough to exercise Flusher's
// claim/insert/delete-or-requeue decision without a database.
type fakeConsumer struct {
	mu       sync.Mutex
	pending  []eventqueue.QueuedEvent
	claimed  map[string]eventqueue.QueuedEvent
	deleted  []string
	requeued []string
}

func newFakeConsumer(events ...eventqueue.QueuedEvent) *fakeConsumer {
	return &fakeConsumer{pending: events, claimed: map[string]eventqueue.QueuedEvent{}}
}

func (c *fakeConsumer) ClaimDue(_ context.Context, limit int) ([]eventqueue.QueuedEvent, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := limit
	if n > len(c.pending) {
		n = len(c.pending)
	}
	claimed := c.pending[:n]
	c.pending = c.pending[n:]
	for _, e := range claimed {
		c.claimed[e.ID] = e
	}
	return claimed, nil
}

func (c *fakeConsumer) Delete(_ context.Context, ids []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deleted = append(c.deleted, ids...)
	for _, id := range ids {
		delete(c.claimed, id)
	}
	return nil
}

func (c *fakeConsumer) Requeue(_ context.Context, ids []string, _ time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requeued = append(c.requeued, ids...)
	for _, id := range ids {
		if e, ok := c.claimed[id]; ok {
			c.pending = append(c.pending, e)
			delete(c.claimed, id)
		}
	}
	return nil
}

type fakeCH struct {
	fail    bool
	batches [][]event.Event
	mu      sync.Mutex
}

func (f *fakeCH) InsertBatch(_ context.Context, events []event.Event) error {
	if f.fail {
		return errors.New("clickhouse unavailable")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.batches = append(f.batches, events)
	return nil
}

func TestFlusherSuccessDeletesClaimedEvents(t *testing.T) {
	consumer := newFakeConsumer(
		eventqueue.QueuedEvent{ID: "e1", Event: event.Event{Type: event.SourceClick}},
		eventqueue.QueuedEvent{ID: "e2", Event: event.Event{Type: event.LandView}},
	)
	ch := &fakeCH{}
	f := eventqueue.NewFlusher(consumer, ch, quietLogger())

	n, err := f.RunOnce(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 2 {
		t.Fatalf("claimed = %d, want 2", n)
	}
	if len(ch.batches) != 1 || len(ch.batches[0]) != 2 {
		t.Fatalf("clickhouse batches = %+v, want one batch of 2", ch.batches)
	}
	if len(consumer.deleted) != 2 {
		t.Fatalf("deleted = %d, want 2", len(consumer.deleted))
	}
	if len(consumer.requeued) != 0 {
		t.Fatal("nothing should be requeued on success")
	}
}

func TestFlusherFailureRequeuesWholeBatch(t *testing.T) {
	consumer := newFakeConsumer(
		eventqueue.QueuedEvent{ID: "e1", Event: event.Event{Type: event.SourceClick}},
		eventqueue.QueuedEvent{ID: "e2", Event: event.Event{Type: event.LandView}},
	)
	ch := &fakeCH{fail: true}
	f := eventqueue.NewFlusher(consumer, ch, quietLogger())

	n, err := f.RunOnce(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 2 {
		t.Fatalf("claimed = %d, want 2", n)
	}
	if len(consumer.requeued) != 2 {
		t.Fatalf("requeued = %d, want 2 (the whole failed batch)", len(consumer.requeued))
	}
	if len(consumer.deleted) != 0 {
		t.Fatal("nothing should be deleted on a failed insert")
	}
}

func TestFlusherEmptyClaimIsNoop(t *testing.T) {
	consumer := newFakeConsumer()
	ch := &fakeCH{}
	f := eventqueue.NewFlusher(consumer, ch, quietLogger())

	n, err := f.RunOnce(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 0 {
		t.Fatalf("claimed = %d, want 0", n)
	}
	if len(ch.batches) != 0 {
		t.Fatal("InsertBatch must not be called when nothing was claimed")
	}
}
