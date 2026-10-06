package eventqueue_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/eventqueue"
)

// failRecorder records how the flusher classified each failure.
type failRecorder struct {
	*fakeConsumer
	rejected []bool
}

func (c *failRecorder) Fail(ctx context.Context, ids []string, rejected bool, lastError string) (bool, error) {
	c.rejected = append(c.rejected, rejected)
	return c.fakeConsumer.Fail(ctx, ids, rejected, lastError)
}

type errCH struct{ err error }

func (e errCH) InsertBatch(context.Context, []event.Event) error { return e.err }

// Audit 2026-10-06, Q2: only a ClickHouse server rejection counts toward
// dead-lettering; an outage never does.
func TestFlusherClassifiesFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"server rejected the data", errors.New(`[HTTP 500] response body: "Code: 53. DB::Exception: Type mismatch"`), true},
		{"clickhouse unreachable", errors.New("dial tcp: connect: connection refused"), false},
		{"server overloaded", errors.New(`Code: 241. DB::Exception: Memory limit exceeded`), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			consumer := &failRecorder{fakeConsumer: newFakeConsumer(eventqueue.QueuedEvent{ID: "e1"})}
			if _, err := eventqueue.NewFlusher(consumer, errCH{tc.err}, quietLogger()).RunOnce(context.Background(), 10); err != nil {
				t.Fatalf("RunOnce: %v", err)
			}
			if len(consumer.rejected) != 1 || consumer.rejected[0] != tc.want {
				t.Fatalf("Fail called with rejected=%v, want [%v]", consumer.rejected, tc.want)
			}
		})
	}
}
