package eventstream

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/eventbuf"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

// publishTimeout bounds one publish. A stream on the same network answers
// in well under a millisecond; past this the outcome is unknown.
const publishTimeout = 2 * time.Second

// unknownGrace is how long a message whose publish outcome is unknown is
// retried on NATS only. Until then a refusal is not taken at its word: the
// earlier attempt may have stored the message, and writing it to Postgres
// too would count its clicks twice. After it, NATS has been failing for
// long enough that losing the events (the tracker's buffer fills) is the
// worse outcome, and the message goes to Postgres anyway.
const unknownGrace = 30 * time.Second

// Publisher is the slice of jetstream.JetStream Sink uses.
type Publisher interface {
	PublishMsg(ctx context.Context, msg *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error)
}

// Sink is the tracker's eventbuf.Sink when NATS is configured: each batch
// is published as one or more messages; what NATS certainly refused goes to
// fallback (the Postgres event_queue) instead.
type Sink struct {
	js       Publisher
	topology Topology
	fallback eventbuf.Sink
	logger   *slog.Logger
	now      func() time.Time

	mu sync.Mutex
	// unknown holds the messages whose last publish ended without an
	// answer, with when that first happened.
	unknown map[string]time.Time
}

// NewSink publishes to t's stream (zero Topology: the default names).
func NewSink(js Publisher, t Topology, fallback eventbuf.Sink, logger *slog.Logger) *Sink {
	return &Sink{js: js, topology: t.withDefaults(), fallback: fallback, logger: logger, now: time.Now, unknown: map[string]time.Time{}}
}

var _ eventbuf.Sink = (*Sink)(nil)

// Write publishes batch message by message. It returns nil when every
// event is in the stream or in Postgres, and an *eventbuf.SinkError
// whose Retry is the rest of the batch from the first message it could not
// place — a suffix starting on a message boundary, so the retry republishes
// the same message ids (see split).
func (s *Sink) Write(ctx context.Context, batch []event.Event) error {
	chunks, err := split(batch)
	if err != nil {
		// json.Marshal of an event does not fail in practice; if it ever
		// does, Postgres encodes the same events and reports the bad one.
		return s.fallback.Write(ctx, batch)
	}
	for i, c := range chunks {
		err := s.publish(ctx, c)
		if err == nil {
			metrics.EventStreamPublishedTotal.Add(float64(len(c.events)))
			s.settle(c.id)
			continue
		}
		if !refused(err) || !s.mayFallBack(c.id) {
			metrics.EventStreamPublishErrorsTotal.WithLabelValues("unknown").Inc()
			s.markUnknown(c.id)
			return &eventbuf.SinkError{Retry: rest(chunks[i:]), Err: err}
		}
		metrics.EventStreamPublishErrorsTotal.WithLabelValues("refused").Inc()
		if ferr := s.fallback.Write(ctx, c.events); ferr != nil {
			if se, ok := ferr.(*eventbuf.SinkError); ok {
				return &eventbuf.SinkError{
					Retry:   append(append([]event.Event{}, se.Retry...), rest(chunks[i+1:])...),
					Dropped: se.Dropped,
					Err:     ferr,
				}
			}
			return &eventbuf.SinkError{Retry: rest(chunks[i:]), Err: ferr}
		}
		metrics.EventStreamFallbackTotal.Add(float64(len(c.events)))
		s.settle(c.id)
		s.logger.Warn("nats refused events, wrote them to postgres", "error", err, "events", len(c.events))
	}
	return nil
}

func (s *Sink) publish(ctx context.Context, c chunk) error {
	ctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	_, err := s.js.PublishMsg(ctx, &nats.Msg{Subject: s.topology.Subject, Data: c.data},
		jetstream.WithMsgID(c.id),
		jetstream.WithExpectStream(s.topology.Stream),
		// No waiting out a missing stream: Postgres takes the batch now.
		jetstream.WithRetryAttempts(0),
	)
	return err
}

// mayFallBack: a message never published with an unknown outcome may go to
// Postgres at once; one that was must wait out unknownGrace.
func (s *Sink) mayFallBack(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	since, ok := s.unknown[id]
	return !ok || s.now().Sub(since) >= unknownGrace
}

func (s *Sink) markUnknown(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.unknown[id]; !ok {
		s.unknown[id] = s.now()
	}
}

func (s *Sink) settle(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.unknown, id)
}

func rest(chunks []chunk) []event.Event {
	var events []event.Event
	for _, c := range chunks {
		events = append(events, c.events...)
	}
	return events
}
