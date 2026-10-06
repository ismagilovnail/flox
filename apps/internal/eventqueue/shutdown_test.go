package eventqueue_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/eventqueue"
)

// ctxConsumer fails Delete/Requeue on a cancelled context, the way a real
// pgx call does — the fake consumer alone ignores ctx and would hide the bug.
type ctxConsumer struct{ *fakeConsumer }

func (c ctxConsumer) Delete(ctx context.Context, ids []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.fakeConsumer.Delete(ctx, ids)
}

func (c ctxConsumer) Fail(ctx context.Context, ids []string, rejected bool, lastError string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return c.fakeConsumer.Fail(ctx, ids, rejected, lastError)
}

func (c ctxConsumer) Requeue(ctx context.Context, ids []string, next time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.fakeConsumer.Requeue(ctx, ids, next)
}

// shutdownCH simulates SIGTERM arriving while the batch is being inserted.
type shutdownCH struct {
	stop func()
	fail bool
}

func (s shutdownCH) InsertBatch(ctx context.Context, _ []event.Event) error {
	s.stop()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.fail {
		return errors.New("clickhouse unavailable")
	}
	return nil
}

// Audit 2026-10-06, Q1: a shutdown mid-batch must not leave the claimed rows
// 'processing' — the batch is either delivered (deleted) or requeued.
func TestFlusherFinishesClaimedBatchAfterShutdown(t *testing.T) {
	for _, tc := range []struct {
		name         string
		insertFails  bool
		wantDeleted  int
		wantRequeued int
	}{
		{"insert succeeds", false, 2, 0},
		{"insert fails", true, 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			consumer := ctxConsumer{newFakeConsumer(
				eventqueue.QueuedEvent{ID: "e1"},
				eventqueue.QueuedEvent{ID: "e2"},
			)}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := eventqueue.NewFlusher(consumer, shutdownCH{stop: cancel, fail: tc.insertFails}, quietLogger())

			if _, err := f.RunOnce(ctx, 10); err != nil {
				t.Fatalf("RunOnce: %v", err)
			}
			if got := len(consumer.deleted); got != tc.wantDeleted {
				t.Errorf("deleted = %d, want %d", got, tc.wantDeleted)
			}
			if got := len(consumer.requeued); got != tc.wantRequeued {
				t.Errorf("requeued = %d, want %d", got, tc.wantRequeued)
			}
			if len(consumer.claimed) != 0 {
				t.Errorf("rows left claimed after shutdown: %v", consumer.claimed)
			}
		})
	}
}
