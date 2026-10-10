package eventstream

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

// Inserter writes events to ClickHouse (chstore.EventStore).
type Inserter interface {
	InsertBatch(ctx context.Context, events []event.Event) error
}

// Handoff takes events the stream could not get into ClickHouse (the
// Postgres event_queue's producer).
type Handoff interface {
	EnqueueBatch(ctx context.Context, events []event.Event) error
}

// ConsumerConfig tunes Consumer. Zero values take the defaults.
type ConsumerConfig struct {
	// Workers is how many messages are written to ClickHouse at once.
	Workers int
	// FetchBatch is how many messages one worker pulls per request.
	FetchBatch int
	// HandoffAfter: a message whose insert failed this many deliveries in
	// a row moves to the Postgres event_queue, whose flusher splits a
	// failing batch and dead-letters the event ClickHouse keeps rejecting
	// (audit Q2) — one bad event must not hold its whole message forever.
	HandoffAfter int
	// InsertTimeout bounds one ClickHouse insert.
	InsertTimeout time.Duration
}

func (c ConsumerConfig) withDefaults() ConsumerConfig {
	if c.Workers <= 0 {
		c.Workers = 4
	}
	if c.FetchBatch <= 0 {
		c.FetchBatch = 8
	}
	if c.HandoffAfter <= 0 {
		c.HandoffAfter = 10
	}
	if c.InsertTimeout <= 0 {
		c.InsertTimeout = 30 * time.Second
	}
	return c
}

// Consumer writes the stream into ClickHouse in apps/worker.
type Consumer struct {
	cons    jetstream.Consumer
	ch      Inserter
	handoff Handoff
	cfg     ConsumerConfig
	logger  *slog.Logger
}

func NewConsumer(cons jetstream.Consumer, ch Inserter, handoff Handoff, cfg ConsumerConfig, logger *slog.Logger) *Consumer {
	return &Consumer{cons: cons, ch: ch, handoff: handoff, cfg: cfg.withDefaults(), logger: logger}
}

// Run pulls and writes until ctx ends. A message already pulled is still
// written and acknowledged after ctx ends (its insert has its own timeout);
// anything unacknowledged at exit is redelivered after the ack wait.
func (c *Consumer) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for range c.cfg.Workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.loop(ctx)
		}()
	}
	wg.Wait()
}

func (c *Consumer) loop(ctx context.Context) {
	for ctx.Err() == nil {
		batch, err := c.cons.Fetch(c.cfg.FetchBatch, jetstream.FetchMaxWait(2*time.Second))
		if err != nil {
			c.logger.Warn("nats fetch failed", "error", err)
			sleep(ctx, time.Second)
			continue
		}
		for msg := range batch.Messages() {
			c.handle(ctx, msg)
		}
		if err := batch.Error(); err != nil && !errors.Is(err, nats.ErrTimeout) && !errors.Is(err, context.DeadlineExceeded) {
			c.logger.Warn("nats fetch ended with an error", "error", err)
			sleep(ctx, time.Second)
		}
	}
}

func (c *Consumer) handle(ctx context.Context, msg jetstream.Msg) {
	events, err := decode(msg.Data())
	if err != nil || len(events) == 0 {
		metrics.EventStreamUndecodableTotal.Inc()
		c.logger.Error("nats message is not an event batch, dropping it", "error", err, "bytes", len(msg.Data()))
		_ = msg.TermWithReason("undecodable")
		return
	}
	id := msg.Headers().Get(jetstream.MsgIDHeader)
	if id == "" {
		id = messageID(events)
	}
	delivered := uint64(1)
	if meta, err := msg.Metadata(); err == nil {
		delivered = meta.NumDelivered
	}

	// The write and the ack outlive a shutdown: cutting them off would only
	// make the message come back.
	work, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.cfg.InsertTimeout)
	defer cancel()
	start := time.Now()
	err = c.ch.InsertBatch(chstore.WithBatchID(work, "nats:"+id), events)
	if err == nil {
		metrics.EventProcessingLatencySeconds.Observe(time.Since(start).Seconds())
		metrics.EventStreamConsumedTotal.Add(float64(len(events)))
		// A lost ack only redelivers the message; ClickHouse drops the
		// repeated block by its token.
		if ackErr := msg.Ack(); ackErr != nil {
			c.logger.Warn("nats ack failed, message will be redelivered", "error", ackErr, "message_id", id)
		}
		return
	}

	if delivered >= uint64(c.cfg.HandoffAfter) {
		herr := c.handoff.EnqueueBatch(work, events)
		if herr == nil {
			metrics.EventStreamHandedOffTotal.Add(float64(len(events)))
			c.logger.Error("clickhouse kept rejecting a nats message, moved it to event_queue",
				"error", err, "message_id", id, "deliveries", delivered, "events", len(events))
			_ = msg.Ack()
			return
		}
		c.logger.Error("could not move a failing nats message to event_queue", "error", herr, "message_id", id)
	}
	metrics.EventStreamRetriedTotal.Inc()
	c.logger.Warn("clickhouse insert failed, nats will redeliver",
		"error", err, "message_id", id, "deliveries", delivered)
	_ = msg.NakWithDelay(redeliveryDelay(delivered))
}

// redeliveryDelay backs off 1s, 2s, 4s … up to a minute.
func redeliveryDelay(delivered uint64) time.Duration {
	if delivered > 7 {
		return time.Minute
	}
	return min(time.Second<<(delivered-1), time.Minute)
}

// PollPending keeps metrics.EventStreamPending current until ctx ends.
func PollPending(ctx context.Context, cons jetstream.Consumer, interval time.Duration, logger *slog.Logger) {
	for {
		infoCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		info, err := cons.Info(infoCtx)
		cancel()
		if err == nil {
			metrics.EventStreamPending.Set(float64(info.NumPending) + float64(info.NumAckPending))
		} else if ctx.Err() == nil {
			logger.Debug("nats consumer info unavailable", "error", err)
		}
		if !sleep(ctx, interval) {
			return
		}
	}
}

func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
