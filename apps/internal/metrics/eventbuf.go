package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/ismagilovnail/flox/apps/internal/eventbuf"
)

// RegisterEventBufStats exposes an eventbuf.Writer's own cumulative
// Stats() counters as Prometheus CounterFuncs — read at scrape time
// rather than incremented at eventbuf's own call sites, so eventbuf's
// internals stay untouched and there is exactly one source of truth for
// these numbers (eventbuf's atomic counters), not two that could drift.
//
// Call once per process, right after constructing the Writer
// (apps/tracker/main.go — eventbuf.Writer is tracker-only today).
// EventsEnqueuedTotal is the tracker-side half of §53's "event_loss
// (enqueued vs persisted)" pair; EventsPersistedTotal (metrics.go, this
// package) is the apps/worker-side other half.
func RegisterEventBufStats(w *eventbuf.Writer) {
	promauto.NewCounterFunc(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "events_enqueued_total",
		Help:      "Events accepted into apps/tracker's in-memory event buffer.",
	}, func() float64 { return float64(w.Stats().Enqueued) })

	promauto.NewCounterFunc(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "events_buffer_dropped_total",
		Help:      "Events dropped because the in-memory event buffer was full — genuinely lost, never retried.",
	}, func() float64 { return float64(w.Stats().Dropped) })

	promauto.NewCounterFunc(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "events_queue_written_total",
		Help:      "Events successfully written from the in-memory buffer into Postgres event_queue.",
	}, func() float64 { return float64(w.Stats().Written) })

	promauto.NewCounterFunc(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "events_queue_write_failed_total",
		Help:      "Batches that failed writing into Postgres event_queue — genuinely lost, never retried.",
	}, func() float64 { return float64(w.Stats().Failed) })
}
