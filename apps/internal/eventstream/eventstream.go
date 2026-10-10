// Package eventstream carries tracker events to the worker through NATS
// JetStream instead of the Postgres event_queue. The end-to-end load test
// (docs/performance.md, 2026-10-10) found the Postgres queue to be the
// ceiling: at 8k events/s Postgres spent a full core on the queue alone,
// at 12k/s the tracker's buffer overflowed. The stream takes that write
// off Postgres.
//
// What goes through it: the tracker's buffered events (clicks, views,
// installs — everything eventbuf writes). What does not: conversions. A
// CPA event commits in one Postgres transaction with its postback row and
// outgoing delivery (audit D1, the dedup and progression rules of
// CLAUDE.md #3), so it stays in event_queue; the worker reads both.
//
// One message is one tracker batch (at most MaxMessageBytes), with a
// Nats-Msg-Id derived from its events. That id makes every retry safe
// twice over: JetStream drops a republished message within its duplicate
// window, and the worker inserts each message into ClickHouse under the
// deduplication token "nats:<id>" (audit D2), so a redelivery after a lost
// ack adds no rows either. Re-batching messages in the worker would break
// the second guarantee — a redelivered message could land in a different
// block under a different token.
//
// The stream uses work-queue retention: a message stays until the worker
// acknowledges it, whether or not a consumer exists yet. A second reader
// of the same events (e.g. a bot detector) needs its own stream sourcing
// this one, not a second consumer here.
package eventstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/ismagilovnail/flox/apps/internal/event"
)

const (
	// StreamName, Subject and ConsumerName are the stream's fixed names.
	StreamName   = "FLOX_EVENTS"
	Subject      = "flox.events"
	ConsumerName = "clickhouse-writer"

	// MaxMessageBytes caps one message, under the NATS server's default
	// 1 MB max_payload; a larger batch is split.
	MaxMessageBytes = 512 << 10

	// duplicateWindow is how long JetStream remembers a message id. The
	// tracker retries an unacknowledged publish within seconds; ten minutes
	// covers a long NATS outage with eventbuf's backoff.
	duplicateWindow = 10 * time.Minute

	// ackWait is how long the worker has to write one message before
	// JetStream redelivers it.
	ackWait = time.Minute
)

// Topology names and sizes the stream. Zero values take the defaults
// (tests use their own names, apart from the running worker's).
type Topology struct {
	Stream   string
	Subject  string
	Consumer string
	// Replicas: 1 for a single server; 3 in a production cluster.
	Replicas int
	// MaxBytes bounds the stream on disk. When full, new publishes are
	// refused and the tracker falls back to Postgres — never are unread
	// events discarded.
	MaxBytes int64
	// MaxAge drops events nobody consumed for this long.
	MaxAge time.Duration
}

func (t Topology) withDefaults() Topology {
	if t.Stream == "" {
		t.Stream = StreamName
	}
	if t.Subject == "" {
		t.Subject = Subject
	}
	if t.Consumer == "" {
		t.Consumer = ConsumerName
	}
	if t.Replicas <= 0 {
		t.Replicas = 1
	}
	if t.MaxBytes <= 0 {
		t.MaxBytes = 10 << 30
	}
	if t.MaxAge <= 0 {
		t.MaxAge = 7 * 24 * time.Hour
	}
	return t
}

// Connect opens a connection that keeps reconnecting in the background,
// also when NATS is down at startup. Publishes never wait in a reconnect
// buffer: while disconnected they fail at once, and the tracker writes to
// Postgres instead of holding the batch.
func Connect(url, name string, logger *slog.Logger) (*nats.Conn, jetstream.JetStream, error) {
	nc, err := nats.Connect(url,
		nats.Name(name),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(time.Second),
		nats.ReconnectBufSize(-1),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			logger.Warn("nats disconnected", "error", err)
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			logger.Info("nats reconnected", "url", c.ConnectedUrlRedacted())
		}),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to nats: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("jetstream: %w", err)
	}
	return nc, js, nil
}

// EnsureTopology creates or updates the stream and the worker's durable
// consumer. Both binaries call it, so whichever starts first leaves the
// stream ready; it is idempotent.
func EnsureTopology(ctx context.Context, js jetstream.JetStream, t Topology) (jetstream.Consumer, error) {
	t = t.withDefaults()
	if _, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:       t.Stream,
		Subjects:   []string{t.Subject},
		Retention:  jetstream.WorkQueuePolicy,
		Discard:    jetstream.DiscardNew,
		Storage:    jetstream.FileStorage,
		Replicas:   t.Replicas,
		MaxBytes:   t.MaxBytes,
		MaxAge:     t.MaxAge,
		MaxMsgSize: MaxMessageBytes,
		Duplicates: duplicateWindow,
	}); err != nil {
		return nil, fmt.Errorf("ensuring stream %s: %w", t.Stream, err)
	}
	cons, err := js.CreateOrUpdateConsumer(ctx, t.Stream, jetstream.ConsumerConfig{
		Durable:       t.Consumer,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       ackWait,
		MaxDeliver:    -1,
		MaxAckPending: 1000,
		FilterSubject: t.Subject,
	})
	if err != nil {
		return nil, fmt.Errorf("ensuring consumer %s: %w", t.Consumer, err)
	}
	return cons, nil
}

// EnsureTopologyLoop retries EnsureTopology until it succeeds or ctx ends,
// for a binary that started while NATS was down. The tracker publishes
// meanwhile anyway: without a stream the publish is refused and the batch
// goes to Postgres.
func EnsureTopologyLoop(ctx context.Context, js jetstream.JetStream, t Topology, logger *slog.Logger) (jetstream.Consumer, error) {
	wait := time.Second
	for attempt := 1; ; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		cons, err := EnsureTopology(attemptCtx, js, t)
		cancel()
		if err == nil {
			return cons, nil
		}
		if attempt == 1 || attempt%10 == 0 {
			logger.Warn("nats stream not ready, retrying", "error", err, "attempt", attempt)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
		wait = min(wait*2, 30*time.Second)
	}
}

// chunk is one message: its events, their encoded batch and its id.
type chunk struct {
	events []event.Event
	data   []byte
	id     string
}

// split encodes batch into messages of at most MaxMessageBytes, greedily
// from the start. Splitting is deterministic: a suffix that starts on a
// chunk boundary splits into exactly the same chunks again, so a retry of
// the rest of a batch republishes the same message ids.
func split(batch []event.Event) ([]chunk, error) {
	var chunks []chunk
	var cur chunk
	cur.data = append(cur.data, '[')
	for _, e := range batch {
		b, err := json.Marshal(e)
		if err != nil {
			return nil, fmt.Errorf("encoding event %s: %w", e.ID, err)
		}
		if len(cur.events) > 0 && len(cur.data)+1+len(b)+1 > MaxMessageBytes {
			chunks = append(chunks, cur.close())
			cur = chunk{data: []byte{'['}}
		}
		if len(cur.events) > 0 {
			cur.data = append(cur.data, ',')
		}
		cur.data = append(cur.data, b...)
		cur.events = append(cur.events, e)
	}
	if len(cur.events) > 0 {
		chunks = append(chunks, cur.close())
	}
	return chunks, nil
}

func (c chunk) close() chunk {
	c.data = append(c.data, ']')
	c.id = messageID(c.events)
	return c
}

// messageID names a message by its first event (a ULID, unique) and its
// length: the same events always get the same id.
func messageID(events []event.Event) string {
	return events[0].ID + "-" + strconv.Itoa(len(events))
}

// decode reads one message back into its events.
func decode(data []byte) ([]event.Event, error) {
	var events []event.Event
	if err := json.Unmarshal(data, &events); err != nil {
		return nil, err
	}
	return events, nil
}

// refused reports whether a publish error means the stream certainly did
// not store the message: no connection, no stream, or the stream answered
// with an error (full, too large). Anything else — a timeout above all —
// leaves it unknown whether the message was stored.
func refused(err error) bool {
	var apiErr *jetstream.APIError
	return errors.As(err, &apiErr) ||
		errors.Is(err, jetstream.ErrNoStreamResponse) ||
		errors.Is(err, nats.ErrNoResponders) ||
		errors.Is(err, nats.ErrNoServers) ||
		errors.Is(err, nats.ErrConnectionClosed) ||
		errors.Is(err, nats.ErrDisconnected) ||
		errors.Is(err, nats.ErrConnectionReconnecting) ||
		errors.Is(err, nats.ErrReconnectBufExceeded) ||
		errors.Is(err, nats.ErrMaxPayload) ||
		// Before the first successful connect the client knows no server,
		// so a message with headers (the msg id) is refused unsent.
		errors.Is(err, nats.ErrHeadersNotSupported)
}
