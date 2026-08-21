package postbacklog_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/postbacklog"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeConsumer struct {
	mu       sync.Mutex
	pending  []postbacklog.QueuedAttempt
	claimed  map[string]postbacklog.QueuedAttempt
	deleted  []string
	requeued []string
}

func newFakeConsumer(attempts ...postbacklog.QueuedAttempt) *fakeConsumer {
	return &fakeConsumer{pending: attempts, claimed: map[string]postbacklog.QueuedAttempt{}}
}

func (c *fakeConsumer) ClaimDue(_ context.Context, limit int) ([]postbacklog.QueuedAttempt, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := limit
	if n > len(c.pending) {
		n = len(c.pending)
	}
	claimed := c.pending[:n]
	c.pending = c.pending[n:]
	for _, a := range claimed {
		c.claimed[a.ID] = a
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
		if a, ok := c.claimed[id]; ok {
			c.pending = append(c.pending, a)
			delete(c.claimed, id)
		}
	}
	return nil
}

type fakeCH struct {
	fail    bool
	batches [][]chstore.PostbackAttempt
	mu      sync.Mutex
}

func (f *fakeCH) InsertPostbackAttempts(_ context.Context, attempts []chstore.PostbackAttempt) error {
	if f.fail {
		return errors.New("clickhouse unavailable")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.batches = append(f.batches, attempts)
	return nil
}

func TestFlusherSuccessDeletesClaimedAttempts(t *testing.T) {
	consumer := newFakeConsumer(
		postbacklog.QueuedAttempt{ID: "a1", Attempt: chstore.PostbackAttempt{Direction: "incoming"}},
		postbacklog.QueuedAttempt{ID: "a2", Attempt: chstore.PostbackAttempt{Direction: "outgoing"}},
	)
	ch := &fakeCH{}
	f := postbacklog.NewFlusher(consumer, ch, quietLogger())

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
}

func TestFlusherFailureRequeuesWholeBatch(t *testing.T) {
	consumer := newFakeConsumer(
		postbacklog.QueuedAttempt{ID: "a1", Attempt: chstore.PostbackAttempt{Direction: "incoming"}},
	)
	ch := &fakeCH{fail: true}
	f := postbacklog.NewFlusher(consumer, ch, quietLogger())

	n, err := f.RunOnce(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 1 {
		t.Fatalf("claimed = %d, want 1", n)
	}
	if len(consumer.requeued) != 1 {
		t.Fatalf("requeued = %d, want 1", len(consumer.requeued))
	}
	if len(consumer.deleted) != 0 {
		t.Fatal("nothing should be deleted on a failed insert")
	}
}

func TestFlusherEmptyClaimIsNoop(t *testing.T) {
	consumer := newFakeConsumer()
	ch := &fakeCH{}
	f := postbacklog.NewFlusher(consumer, ch, quietLogger())

	n, err := f.RunOnce(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 0 {
		t.Fatalf("claimed = %d, want 0", n)
	}
	if len(ch.batches) != 0 {
		t.Fatal("InsertPostbackAttempts must not be called when nothing was claimed")
	}
}
