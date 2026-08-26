// Package metrics is the single place every §53/Phase 29 Prometheus
// collector is defined — apps/api, apps/tracker, and apps/worker each
// import it and expose Handler() at GET /metrics on their own port; each
// binary is a separate OS process with its own default Prometheus
// registry, so a metric recorded in one process never mixes with another
// (Prometheus's own "job" scrape label is what tells them apart later).
//
// "event_loss (enqueued vs persisted)" from §53's tracked-metric list is
// deliberately NOT one metric here — it is two counters
// (EventsEnqueuedTotal, from apps/tracker's eventbuf.Writer; and
// EventsPersistedTotal, from apps/worker's eventqueue.Flusher) that a
// dashboard/alert derives loss from via PromQL (rate(enqueued) -
// rate(persisted), or similar). Storing a single pre-subtracted "loss"
// gauge would require one process to know both sides' counts, which
// span two different binaries — standard Prometheus practice is to keep
// raw counters and let queries compute derived values, never the reverse.
//
// Likewise "postback_success"/"postback_failure" (§53's two named
// bullets) are one CounterVec, PostbackDeliveriesTotal, labeled by
// outcome — "success" maps directly, but §53's single "failure" bucket
// is split into "retrying" (will be attempted again, not yet lost) and
// "dead" (exhausted MaxAttempts, genuinely lost) since that distinction
// already exists in apps/internal/postback's own DeliveryStatus enum and
// collapsing it would throw away information the dashboard can always
// re-collapse (sum by outcome!="success") but never recover once gone.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "flox"

// hotPathBuckets fits apps/tracker's own latency budget (§41, CLAUDE.md
// non-negotiable #9: p50 < 20ms, p95 < 50ms) — finer resolution below
// 50ms than prometheus.DefBuckets offers, since every interesting bucket
// boundary for this metric lives well under DefBuckets' first bucket
// (5ms) through its middle (100ms).
var hotPathBuckets = []float64{.001, .002, .005, .01, .02, .05, .1, .25, .5, 1}

var (
	// TrackingRequestsTotal / TrackingLatencySeconds: apps/tracker's
	// redirect handler (§41). outcome is one of "redirected", "blocked"
	// (SOURCE_FILTER — no destination matched), "not_found" (unknown
	// tracking link or inactive campaign), or "error".
	TrackingRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "tracking_requests_total",
		Help:      "Total apps/tracker redirect requests, by outcome.",
	}, []string{"outcome"})

	TrackingLatencySeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "tracking_latency_seconds",
		Help:      "apps/tracker redirect handler end-to-end latency (§41 budget: p50<20ms, p95<50ms).",
		Buckets:   hotPathBuckets,
	})

	// RoutingLatencySeconds: internal/routing.Engine.Resolve — recorded
	// wherever it's called (apps/tracker's hot path, and apps/api's
	// /routing/simulate, each in their own process's registry).
	RoutingLatencySeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "routing_latency_seconds",
		Help:      "internal/routing.Engine.Resolve decision latency.",
		Buckets:   hotPathBuckets,
	})

	// EventsEnqueuedTotal / EventsDroppedTotal: apps/tracker's
	// eventbuf.Writer — see RegisterEventBufStats below. Not vars here
	// because their value comes from an existing atomic counter read at
	// scrape time (CounterFunc), not an increment call site.

	// EventProcessingLatencySeconds / EventsPersistedTotal /
	// EventsRequeuedTotal: apps/worker's eventqueue.Flusher — one
	// ClickHouse InsertBatch call per RunOnce.
	EventProcessingLatencySeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "event_processing_latency_seconds",
		Help:      "eventqueue.Flusher: ClickHouse InsertBatch duration per claimed batch.",
		Buckets:   prometheus.DefBuckets,
	})

	EventsPersistedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "events_persisted_total",
		Help:      "Events successfully flushed from event_queue into ClickHouse (apps/worker's eventqueue.Flusher).",
	})

	EventsRequeuedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "events_requeued_total",
		Help:      "Events whose ClickHouse insert failed and were requeued for retry (not yet lost).",
	})

	// EventQueueDepth: polled periodically from event_queue's own row
	// count — see eventqueue.PollDepth.
	EventQueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "event_queue_depth",
		Help:      "Rows currently queued in Postgres event_queue, polled periodically by apps/worker.",
	})

	// PostbackDeliveriesTotal: apps/internal/postback.Deliverer — see
	// this file's own package doc comment for the outcome label values.
	PostbackDeliveriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "postback_deliveries_total",
		Help:      "Outgoing postback delivery attempts, by outcome (success|retrying|dead).",
	}, []string{"outcome"})

	// AnalyticsQueryLatencySeconds: apps/internal/analytics — endpoint
	// names the specific query (e.g. "campaign_daily").
	AnalyticsQueryLatencySeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "analytics_query_latency_seconds",
		Help:      "ClickHouse analytics query latency, by endpoint.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"endpoint"})
)

// Handler serves the Prometheus text exposition format for whatever this
// process has registered — mount at GET /metrics, unauthenticated (a
// scrape target on an internal network, same convention as GET /health).
func Handler() http.Handler {
	return promhttp.Handler()
}
