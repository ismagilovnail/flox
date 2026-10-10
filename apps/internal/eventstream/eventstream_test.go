package eventstream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/eventbuf"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// events makes n events whose encoded size is about size bytes each.
func events(n, size int) []event.Event {
	out := make([]event.Event, n)
	for i := range out {
		out[i] = event.Event{
			ID:             idgen.New(),
			OrganizationID: "org",
			Type:           event.SourceClick,
			UserAgent:      strings.Repeat("u", size),
		}
	}
	return out
}

func TestSplitIsDeterministicAndBounded(t *testing.T) {
	batch := events(500, 3000) // ~1.5 MB: several messages
	chunks, err := split(batch)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 3 {
		t.Fatalf("got %d chunks, want the batch split", len(chunks))
	}
	total := 0
	for _, c := range chunks {
		if len(c.data) > MaxMessageBytes {
			t.Fatalf("chunk of %d bytes over the %d limit", len(c.data), MaxMessageBytes)
		}
		decoded, err := decode(c.data)
		if err != nil || len(decoded) != len(c.events) {
			t.Fatalf("chunk does not decode back: %v", err)
		}
		total += len(c.events)
	}
	if total != len(batch) {
		t.Fatalf("chunks hold %d events, want %d", total, len(batch))
	}

	// A retry of the rest of the batch from a chunk boundary yields the
	// same messages, so JetStream and ClickHouse recognise them.
	again, err := split(rest(chunks[1:]))
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range again {
		if c.id != chunks[i+1].id || string(c.data) != string(chunks[i+1].data) {
			t.Fatalf("retried chunk %d differs from the original", i)
		}
	}
}

func TestRefused(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{jetstream.ErrNoStreamResponse, true},
		{fmt.Errorf("nats: %w", &jetstream.APIError{Code: 503, ErrorCode: 10077, Description: "maximum bytes exceeded"}), true},
		{nats.ErrConnectionReconnecting, true},
		{nats.ErrReconnectBufExceeded, true},
		{nats.ErrConnectionClosed, true},
		{nats.ErrHeadersNotSupported, true},
		{context.DeadlineExceeded, false},
		{nats.ErrTimeout, false},
		{errors.New("something else"), false},
	} {
		if got := refused(tc.err); got != tc.want {
			t.Errorf("refused(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

// fakePublisher answers each publish with the next error (nil = stored).
type fakePublisher struct {
	mu   sync.Mutex
	errs []error
	ids  []string
}

func (p *fakePublisher) PublishMsg(_ context.Context, msg *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var err error
	if len(p.errs) > 0 {
		err, p.errs = p.errs[0], p.errs[1:]
	}
	return &jetstream.PubAck{}, err
}

type memSink struct {
	mu     sync.Mutex
	events []event.Event
	err    error
}

func (s *memSink) Write(_ context.Context, batch []event.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.events = append(s.events, batch...)
	return nil
}

func (s *memSink) EnqueueBatch(ctx context.Context, batch []event.Event) error {
	return s.Write(ctx, batch)
}

func (s *memSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

func TestSinkFallsBackOnlyWhenNATSCertainlyRefused(t *testing.T) {
	batch := events(10, 10)

	// Refused: Postgres takes the batch at once.
	pg := &memSink{}
	sink := NewSink(&fakePublisher{errs: []error{jetstream.ErrNoStreamResponse}}, Topology{}, pg, quiet)
	if err := sink.Write(context.Background(), batch); err != nil {
		t.Fatalf("refused publish: %v, want the batch written to postgres", err)
	}
	if pg.count() != 10 {
		t.Fatalf("postgres got %d events, want 10", pg.count())
	}

	// Unknown outcome: retried on NATS, Postgres untouched — even when the
	// next attempt is refused, until unknownGrace has passed.
	pg = &memSink{}
	now := time.Now()
	pub := &fakePublisher{errs: []error{context.DeadlineExceeded, jetstream.ErrNoStreamResponse, jetstream.ErrNoStreamResponse}}
	sink = NewSink(pub, Topology{}, pg, quiet)
	sink.now = func() time.Time { return now }
	for attempt := 1; attempt <= 2; attempt++ {
		err := sink.Write(context.Background(), batch)
		var se *eventbuf.SinkError
		if !errors.As(err, &se) || len(se.Retry) != 10 {
			t.Fatalf("attempt %d: err = %v, want all 10 events to retry", attempt, err)
		}
		if pg.count() != 0 {
			t.Fatalf("attempt %d: a maybe-stored message went to postgres", attempt)
		}
	}
	now = now.Add(unknownGrace)
	if err := sink.Write(context.Background(), batch); err != nil {
		t.Fatalf("after the grace period: %v, want postgres to take it", err)
	}
	if pg.count() != 10 {
		t.Fatalf("postgres got %d events after the grace period, want 10", pg.count())
	}

	// Postgres failing too: the whole batch is retried.
	pg = &memSink{err: errors.New("postgres down")}
	sink = NewSink(&fakePublisher{errs: []error{nats.ErrConnectionReconnecting}}, Topology{}, pg, quiet)
	var se *eventbuf.SinkError
	if err := sink.Write(context.Background(), batch); !errors.As(err, &se) || len(se.Retry) != 10 {
		t.Fatalf("both down: err = %v, want all 10 to retry", err)
	}
}

// --- against a real NATS server (NATS_URL) ---

func mustJetStream(t *testing.T) (*nats.Conn, jetstream.JetStream, Topology) {
	t.Helper()
	url := envOrSkip(t)
	nc, js, err := Connect(url, "eventstream-test", quiet)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	name := "TEST_" + idgen.New()
	topo := Topology{Stream: name, Subject: "test." + strings.ToLower(name), Consumer: "test-writer", MaxBytes: 64 << 20}
	t.Cleanup(func() { _ = js.DeleteStream(context.Background(), name) })
	return nc, js, topo
}

func envOrSkip(t *testing.T) string {
	t.Helper()
	url := os.Getenv("NATS_URL")
	if url == "" {
		t.Skip("NATS_URL not set, skipping integration test")
	}
	return url
}

// recorder is a ClickHouse stand-in failing the first failFor inserts of
// each message.
type recorder struct {
	mu      sync.Mutex
	failFor int
	tries   map[string]int
	events  map[string]int
}

func newRecorder(failFor int) *recorder {
	return &recorder{failFor: failFor, tries: map[string]int{}, events: map[string]int{}}
}

func (r *recorder) InsertBatch(_ context.Context, batch []event.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := messageID(batch)
	r.tries[key]++
	if r.tries[key] <= r.failFor {
		return errors.New("clickhouse unavailable")
	}
	for _, e := range batch {
		r.events[e.ID]++
	}
	return nil
}

func (r *recorder) written() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func runConsumer(t *testing.T, cons jetstream.Consumer, ch Inserter, handoff Handoff, cfg ConsumerConfig) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		NewConsumer(cons, ch, handoff, cfg, quiet).Run(ctx)
		close(done)
	}()
	t.Cleanup(func() { cancel(); <-done })
}

func TestStreamCarriesEveryEventOnce(t *testing.T) {
	_, js, topo := mustJetStream(t)
	ctx := context.Background()
	cons, err := EnsureTopology(ctx, js, topo)
	if err != nil {
		t.Fatal(err)
	}
	pg := &memSink{}
	sink := NewSink(js, topo, pg, quiet)
	batch := events(1200, 1500) // several messages
	if err := sink.Write(ctx, batch); err != nil {
		t.Fatal(err)
	}
	// The tracker retrying a batch it already published (a lost ack)
	// stores nothing new: same message ids, inside the duplicate window.
	if err := sink.Write(ctx, batch); err != nil {
		t.Fatal(err)
	}
	stream, err := js.Stream(ctx, topo.Stream)
	if err != nil {
		t.Fatal(err)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	chunks, _ := split(batch)
	if int(info.State.Msgs) != len(chunks) {
		t.Fatalf("stream holds %d messages, want %d (the republish must be deduplicated)", info.State.Msgs, len(chunks))
	}

	ch := newRecorder(0)
	runConsumer(t, cons, ch, pg, ConsumerConfig{})
	waitFor(t, "all events in clickhouse", func() bool { return ch.written() == len(batch) })
	for id, n := range ch.events {
		if n != 1 {
			t.Fatalf("event %s written %d times", id, n)
		}
	}
	if pg.count() != 0 {
		t.Fatalf("postgres got %d events, want none", pg.count())
	}
	// Acknowledged messages leave the work queue.
	waitFor(t, "stream empty", func() bool {
		info, err := stream.Info(ctx)
		return err == nil && info.State.Msgs == 0
	})
}

func TestFailedInsertIsRedelivered(t *testing.T) {
	_, js, topo := mustJetStream(t)
	ctx := context.Background()
	cons, err := EnsureTopology(ctx, js, topo)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewSink(js, topo, &memSink{}, quiet).Write(ctx, events(20, 10)); err != nil {
		t.Fatal(err)
	}
	ch := newRecorder(2)
	runConsumer(t, cons, ch, &memSink{}, ConsumerConfig{})
	waitFor(t, "the third delivery to land", func() bool { return ch.written() == 20 })
}

func TestRepeatedFailureMovesTheMessageToPostgres(t *testing.T) {
	_, js, topo := mustJetStream(t)
	ctx := context.Background()
	cons, err := EnsureTopology(ctx, js, topo)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewSink(js, topo, &memSink{}, quiet).Write(ctx, events(20, 10)); err != nil {
		t.Fatal(err)
	}
	pg := &memSink{}
	runConsumer(t, cons, newRecorder(1_000), pg, ConsumerConfig{HandoffAfter: 2})
	waitFor(t, "the handoff to postgres", func() bool { return pg.count() == 20 })
	stream, _ := js.Stream(ctx, topo.Stream)
	waitFor(t, "the message acknowledged", func() bool {
		info, err := stream.Info(ctx)
		return err == nil && info.State.Msgs == 0
	})
}

func TestMissingStreamFallsBackToPostgres(t *testing.T) {
	_, js, topo := mustJetStream(t) // topology never created
	pg := &memSink{}
	if err := NewSink(js, topo, pg, quiet).Write(context.Background(), events(5, 10)); err != nil {
		t.Fatal(err)
	}
	if pg.count() != 5 {
		t.Fatalf("postgres got %d events, want 5", pg.count())
	}
}

func TestNATSDownFallsBackToPostgres(t *testing.T) {
	envOrSkip(t)
	// Nothing listens here; the connection keeps retrying in the background.
	nc, js, err := Connect("nats://127.0.0.1:1", "eventstream-test", quiet)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	pg := &memSink{}
	start := time.Now()
	if err := NewSink(js, Topology{}, pg, quiet).Write(context.Background(), events(5, 10)); err != nil {
		t.Fatal(err)
	}
	if pg.count() != 5 {
		t.Fatalf("postgres got %d events, want 5", pg.count())
	}
	if took := time.Since(start); took > time.Second {
		t.Fatalf("falling back took %s, want it immediate", took)
	}
}
