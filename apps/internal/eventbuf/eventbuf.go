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
//
// A failed sink write is retried with backoff, not thrown away (audit
// 2026-10-06, RL2: one Postgres blip used to lose the whole batch). While a
// batch is being retried, new events wait in the buffer; only if the
// outage outlasts the buffer do they start being dropped. On Close, retries
// get ShutdownRetryBudget and whatever is still unwritten is counted as
// failed.
package eventbuf

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
)

// Sink is where flushed batches go. The real implementation (Phase 24's
// worker consuming a durable queue into ClickHouse) replaces the default
// without the tracker changing at all.
//
// A Sink writes a batch atomically: a plain error means none of it was
// written and the whole batch is retried. A Sink that wrote part of a batch,
// or refused events that can never succeed, returns a *SinkError instead.
type Sink interface {
	Write(ctx context.Context, batch []event.Event) error
}

// SinkError reports a batch a Sink could not fully write: Retry are the
// events worth trying again, Dropped how many were refused for good (e.g.
// their organization was deleted). Events of the batch in neither were
// written.
type SinkError struct {
	Retry   []event.Event
	Dropped int
	Err     error
}

func (e *SinkError) Error() string { return e.Err.Error() }
func (e *SinkError) Unwrap() error { return e.Err }

type Config struct {
	// BufferSize is the channel depth — how many events may be waiting
	// when the sink is slow before new ones start being dropped.
	BufferSize int
	// BatchSize flushes as soon as this many events have accumulated.
	BatchSize int
	// FlushInterval flushes a partial batch this often, so low-traffic
	// events aren't stuck in the buffer indefinitely.
	FlushInterval time.Duration
	// RetryMin/RetryMax bound the exponential backoff between attempts to
	// write a failed batch.
	RetryMin, RetryMax time.Duration
	// ShutdownRetryBudget is how long Close keeps retrying a failing
	// batch before counting it as failed, so a dead sink cannot hang
	// shutdown.
	ShutdownRetryBudget time.Duration
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
	if c.RetryMin <= 0 {
		c.RetryMin = 100 * time.Millisecond
	}
	if c.RetryMax <= 0 {
		c.RetryMax = 5 * time.Second
	}
	if c.ShutdownRetryBudget <= 0 {
		c.ShutdownRetryBudget = 5 * time.Second
	}
	return c
}

type Writer struct {
	cfg    Config
	sink   Sink
	logger *slog.Logger

	ch        chan event.Event
	done      chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once
	closed    atomic.Bool

	enqueued atomic.Uint64
	dropped  atomic.Uint64
	written  atomic.Uint64
	failed   atomic.Uint64
	retried  atomic.Uint64
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
	if w.closed.Load() {
		// Nothing drains the channel any more: accepting the event would
		// count it as enqueued and lose it silently.
		w.dropped.Add(1)
		return false
	}
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

// writeTimeout bounds one attempt to write a batch.
const writeTimeout = 10 * time.Second

// write delivers batch, retrying what the sink reports as retryable until it
// is written — or, once Close has been called, until ShutdownRetryBudget
// runs out. It blocks the flush loop meanwhile; new events wait in the
// channel (and are dropped only when it is full), which keeps memory
// bounded and the batch order intact.
func (w *Writer) write(batch []event.Event) {
	backoff := w.cfg.RetryMin
	var shutdownDeadline time.Time
	for attempt := 1; ; attempt++ {
		timeout := writeTimeout
		if w.closed.Load() {
			if shutdownDeadline.IsZero() {
				shutdownDeadline = time.Now().Add(w.cfg.ShutdownRetryBudget)
			}
			// Shutdown has a fixed budget: no attempt or pause may outrun it.
			timeout = min(timeout, max(time.Until(shutdownDeadline), 50*time.Millisecond))
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		err := w.sink.Write(ctx, batch)
		cancel()
		if err == nil {
			w.written.Add(uint64(len(batch)))
			return
		}

		retry := batch
		var se *SinkError
		if errors.As(err, &se) {
			retry = se.Retry
			w.failed.Add(uint64(se.Dropped))
			w.written.Add(uint64(len(batch) - len(se.Retry) - se.Dropped))
			if se.Dropped > 0 {
				w.logger.Error("event sink refused events for good", "error", err, "dropped", se.Dropped)
			}
		}
		if len(retry) == 0 {
			return
		}
		batch = retry

		if w.closed.Load() {
			if shutdownDeadline.IsZero() {
				shutdownDeadline = time.Now().Add(w.cfg.ShutdownRetryBudget)
			}
			if !time.Now().Before(shutdownDeadline) {
				w.failed.Add(uint64(len(batch)))
				w.logger.Error("event sink still failing at shutdown, giving up on batch",
					"error", err, "batch_size", len(batch), "attempts", attempt)
				return
			}
		}
		w.retried.Add(1)
		if attempt == 1 || attempt%10 == 0 {
			w.logger.Warn("event sink write failed, retrying",
				"error", err, "batch_size", len(batch), "attempt", attempt, "backoff", backoff)
		}
		pause := backoff
		if !shutdownDeadline.IsZero() {
			pause = min(pause, max(time.Until(shutdownDeadline), 0))
		}
		time.Sleep(pause)
		backoff = min(backoff*2, w.cfg.RetryMax)
	}
}

// Close stops accepting new events, drains the buffer, and waits for the
// final flush. Safe to call more than once: apps/tracker closes explicitly
// before logging its stats and again in a defer for early returns, and the
// second close of done used to panic every clean shutdown (audit
// 2026-10-06, RL3).
func (w *Writer) Close() {
	w.closeOnce.Do(func() {
		w.closed.Store(true)
		close(w.done)
	})
	w.wg.Wait()
}

type Stats struct {
	Enqueued uint64
	Dropped  uint64
	Written  uint64
	// Failed counts events lost after the sink failed: refused for good,
	// or still failing when Close's retry budget ran out.
	Failed uint64
	// Retried counts failed write attempts that were retried.
	Retried uint64
}

func (w *Writer) Stats() Stats {
	return Stats{
		Enqueued: w.enqueued.Load(),
		Dropped:  w.dropped.Load(),
		Written:  w.written.Load(),
		Failed:   w.failed.Load(),
		Retried:  w.retried.Load(),
	}
}
