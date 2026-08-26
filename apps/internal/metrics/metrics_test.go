package metrics_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/eventbuf"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

// TestHandlerExposesRegisteredMetrics is a smoke test that GET /metrics
// (metrics.Handler()) actually serves the Prometheus text format and
// includes a representative sample of every §53 metric this package
// defines — catching a typo'd Name/Namespace that would otherwise only
// surface by eyeballing a live scrape.
func TestHandlerExposesRegisteredMetrics(t *testing.T) {
	// Touch every collector once so it's guaranteed to have at least one
	// series before scraping — an untouched CounterVec/HistogramVec
	// contributes no lines to the exposition format at all (Prometheus
	// clients only emit series that have actually been observed), which
	// would make this test's substring checks fail for reasons unrelated
	// to what it's actually verifying.
	metrics.TrackingRequestsTotal.WithLabelValues("redirected").Add(0)
	metrics.TrackingLatencySeconds.Observe(0)
	metrics.RoutingLatencySeconds.Observe(0)
	metrics.EventProcessingLatencySeconds.Observe(0)
	metrics.EventsPersistedTotal.Add(0)
	metrics.EventsRequeuedTotal.Add(0)
	metrics.EventQueueDepth.Set(0)
	metrics.PostbackDeliveriesTotal.WithLabelValues("success").Add(0)
	metrics.AnalyticsQueryLatencySeconds.WithLabelValues("campaign_daily").Observe(0)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()

	for _, name := range []string{
		"flox_tracking_requests_total",
		"flox_tracking_latency_seconds",
		"flox_routing_latency_seconds",
		"flox_event_processing_latency_seconds",
		"flox_events_persisted_total",
		"flox_events_requeued_total",
		"flox_event_queue_depth",
		"flox_postback_deliveries_total",
		"flox_analytics_query_latency_seconds",
	} {
		if !strings.Contains(body, name) {
			t.Errorf("scraped output missing metric %q", name)
		}
	}
}

type fakeSink struct {
	mu      sync.Mutex
	written int
	fail    bool
}

func (s *fakeSink) Write(ctx context.Context, batch []event.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("simulated sink failure")
	}
	s.written += len(batch)
	return nil
}

// TestRegisterEventBufStatsTracksWriterCounters exercises
// RegisterEventBufStats against a real eventbuf.Writer — enqueue enough
// events to force at least one flush, then confirm the CounterFuncs it
// registers read back exactly what the Writer's own Stats() reports.
// Only one test in this package may call RegisterEventBufStats: it
// registers new Prometheus collectors each call, and a second call in
// the same test binary would panic on duplicate registration.
func TestRegisterEventBufStatsTracksWriterCounters(t *testing.T) {
	sink := &fakeSink{}
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	writer := eventbuf.New(sink, logger, eventbuf.Config{BatchSize: 3, FlushInterval: time.Hour})

	metrics.RegisterEventBufStats(writer)

	for i := 0; i < 3; i++ {
		if !writer.Enqueue(event.Event{Type: event.SourceClick}) {
			t.Fatalf("Enqueue %d returned false, want true (buffer has plenty of room)", i)
		}
	}
	// BatchSize=3 flushes synchronously inside the writer's own goroutine
	// as soon as the third event arrives — Close() waits for that
	// goroutine to finish draining, so by the time it returns the flush
	// is guaranteed to have happened.
	writer.Close()

	if got := scrapeCounter(t, "flox_events_enqueued_total"); got != 3 {
		t.Fatalf("flox_events_enqueued_total = %v, want 3", got)
	}
	if got := scrapeCounter(t, "flox_events_queue_written_total"); got != 3 {
		t.Fatalf("flox_events_queue_written_total = %v, want 3", got)
	}
	if got := scrapeCounter(t, "flox_events_buffer_dropped_total"); got != 0 {
		t.Fatalf("flox_events_buffer_dropped_total = %v, want 0", got)
	}
	if sink.written != 3 {
		t.Fatalf("fakeSink received %d events, want 3", sink.written)
	}
}

// scrapeCounter reads a single metric's current value straight from a
// real /metrics scrape — CounterFunc (used by RegisterEventBufStats)
// doesn't implement prometheus.Counter, so testutil.ToFloat64 (which
// wants a Collector) can't read it directly; going through the same
// exposition format a real Prometheus server would parse is the more
// end-to-end check anyway.
func scrapeCounter(t *testing.T, name string) float64 {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, req)
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if after, ok := strings.CutPrefix(line, name+" "); ok {
			v, err := strconv.ParseFloat(strings.TrimSpace(after), 64)
			if err != nil {
				t.Fatalf("parsing value of %q from line %q: %v", name, line, err)
			}
			return v
		}
	}
	t.Fatalf("metric %q not found in scrape output", name)
	return 0
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
