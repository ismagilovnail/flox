// Package eventbuf is the tracker's buffered batch event writer (§41:
// "record click/event asynchronously (buffered batch writer → queue)…
// Do not block the redirect on event persistence").
//
// The hot path calls Enqueue, which is non-blocking by construction: it
// does a single non-blocking channel send and returns immediately. A
// background goroutine drains the channel into batches and hands each
// batch to a Sink. The redirect therefore never waits on a network write,
// a slow queue, or a stalled sink — CLAUDE.md non-negotiable #9
// (tracking p50 < 20ms / p95 < 50ms).
//
// The deliberate consequence: when the buffer is full, events are dropped
// rather than blocking the user's redirect. Dropping is the correct
// trade-off for a redirect service — a lost analytics row is recoverable,
// a hung redirect is a lost click — but it must be *visible*, so drops are
// counted and logged rather than silently swallowed.
package eventbuf

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
)

// Sink is where flushed batches go. The real implementation (Phase 24's
// worker consuming a durable queue into ClickHouse) replaces the default
// without the tracker changing at all.
type Sink interface {
	Write(ctx context.Context, batch []event.Event) error
}

type Config struct {
	// BufferSize is the channel depth — how many events may be waiting
	// when the sink is slow before new ones start being dropped.
	BufferSize int
	// BatchSize flushes as soon as this many events have accumulated.
	BatchSize int
	// FlushInterval flushes a partial batch this often, so low-traffic
	// events aren't stuck in the buffer indefinitely.
	FlushInterval time.Duration
}

func (c Config) withDefaults() Config {
	if c.BufferSize <= 0 {
		c.BufferSize = 10_000
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 500
	}
	if c.FlushInterval <= 0 {
		c.FlushInterval = time.Second
	}
	return c
}

type Writer struct {
	cfg    Config
	sink   Sink
	logger *slog.Logger

	ch   chan event.Event
	done chan struct{}
	wg   sync.WaitGroup

	enqueued atomic.Uint64
	dropped  atomic.Uint64
	written  atomic.Uint64
	failed   atomic.Uint64
}

func New(sink Sink, logger *slog.Logger, cfg Config) *Writer {
	cfg = cfg.withDefaults()
	w := &Writer{
		cfg:    cfg,
		sink:   sink,
		logger: logger,
		ch:     make(chan event.Event, cfg.BufferSize),
		done:   make(chan struct{}),
	}
	w.wg.Add(1)
	go w.run()
	return w
}

// Enqueue never blocks and never returns an error the hot path has to
// handle — the redirect must not care whether persistence succeeded.
// Returns false only so tests (and metrics) can observe drops.
func (w *Writer) Enqueue(e event.Event) bool {
	select {
	case w.ch <- e:
		w.enqueued.Add(1)
		return true
	default:
		// Buffer full: drop rather than block the redirect. Counted, and
		// logged at Warn so it can never be silent.
		n := w.dropped.Add(1)
		if n == 1 || n%1000 == 0 {
			w.logger.Warn("event buffer full, dropping events",
				"dropped_total", n, "buffer_size", w.cfg.BufferSize)
		}
		return false
	}
}

func (w *Writer) run() {
	defer w.wg.Done()

	batch := make([]event.Event, 0, w.cfg.BatchSize)
	ticker := time.NewTicker(w.cfg.FlushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		w.write(batch)
		batch = make([]event.Event, 0, w.cfg.BatchSize)
	}

	for {
		select {
		case e := <-w.ch:
			batch = append(batch, e)
			if len(batch) >= w.cfg.BatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-w.done:
			// Drain whatever is still buffered before exiting, so a clean
			// shutdown doesn't discard events that were already accepted.
			for {
				select {
				case e := <-w.ch:
					batch = append(batch, e)
					if len(batch) >= w.cfg.BatchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

func (w *Writer) write(batch []event.Event) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := w.sink.Write(ctx, batch); err != nil {
		w.failed.Add(uint64(len(batch)))
		w.logger.Error("event sink write failed", "error", err, "batch_size", len(batch))
		return
	}
	w.written.Add(uint64(len(batch)))
}

// Close stops accepting new events, drains the buffer, and waits for the
// final flush. Safe to call once.
func (w *Writer) Close() {
	close(w.done)
	w.wg.Wait()
}

type Stats struct {
	Enqueued uint64
	Dropped  uint64
	Written  uint64
	Failed   uint64
}

func (w *Writer) Stats() Stats {
	return Stats{
		Enqueued: w.enqueued.Load(),
		Dropped:  w.dropped.Load(),
		Written:  w.written.Load(),
		Failed:   w.failed.Load(),
	}
}
