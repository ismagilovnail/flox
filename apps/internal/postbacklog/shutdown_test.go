package postbacklog_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/postbacklog"
)

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

type shutdownCH struct{ stop func() }

func (s shutdownCH) InsertPostbackAttempts(ctx context.Context, _ []chstore.PostbackAttempt) error {
	s.stop()
	return ctx.Err()
}

// Audit 2026-10-06, Q1 — see eventqueue's TestFlusherFinishesClaimedBatchAfterShutdown.
func TestFlusherFinishesClaimedBatchAfterShutdown(t *testing.T) {
	consumer := ctxConsumer{newFakeConsumer(
		postbacklog.QueuedAttempt{ID: "a1"},
		postbacklog.QueuedAttempt{ID: "a2"},
	)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := postbacklog.NewFlusher(consumer, shutdownCH{stop: cancel}, quietLogger())

	if _, err := f.RunOnce(ctx, 10); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(consumer.deleted) != 2 {
		t.Fatalf("deleted = %d, want 2 (batch abandoned on shutdown)", len(consumer.deleted))
	}
}
