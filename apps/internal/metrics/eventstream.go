package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// NATS JetStream event stream (internal/eventstream): the tracker publishes
// event batches, the worker consumes them into ClickHouse.
var (
	// EventStreamPublishedTotal: events the stream acknowledged.
	EventStreamPublishedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "eventstream_published_total",
		Help:      "Events JetStream acknowledged from the tracker.",
	})

	// EventStreamFallbackTotal: events written to the Postgres event_queue
	// because the stream certainly did not take them (NATS down, stream
	// missing or full).
	EventStreamFallbackTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "eventstream_fallback_total",
		Help:      "Events the tracker wrote to the Postgres event_queue because NATS refused them.",
	})

	// EventStreamPublishErrorsTotal by kind: "refused" (not stored, falls
	// back to Postgres) or "unknown" (maybe stored, retried on NATS).
	EventStreamPublishErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "eventstream_publish_errors_total",
		Help:      "Failed JetStream publishes by kind: refused (falls back to Postgres) or unknown (retried on NATS).",
	}, []string{"kind"})

	// EventStreamConsumedTotal: events the worker wrote to ClickHouse from
	// the stream.
	EventStreamConsumedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "eventstream_consumed_total",
		Help:      "Events the worker wrote from JetStream to ClickHouse.",
	})

	// EventStreamRetriedTotal: messages whose ClickHouse insert failed and
	// were handed back to JetStream for redelivery.
	EventStreamRetriedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "eventstream_retried_total",
		Help:      "JetStream messages whose ClickHouse insert failed and will be redelivered.",
	})

	// EventStreamHandedOffTotal: events moved to the Postgres event_queue
	// after repeated failures, where its split-and-dead-letter handling
	// takes over.
	EventStreamHandedOffTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "eventstream_handed_off_total",
		Help:      "Events moved from JetStream to the Postgres event_queue after repeated ClickHouse failures.",
	})

	// EventStreamUndecodableTotal: messages that are not an event batch at
	// all; terminated, never retried.
	EventStreamUndecodableTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "eventstream_undecodable_total",
		Help:      "JetStream messages that could not be decoded as an event batch (terminated).",
	})

	// EventStreamPending: messages waiting for the worker — not yet
	// delivered plus delivered and not yet acknowledged.
	EventStreamPending = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "eventstream_pending_messages",
		Help:      "JetStream messages not yet written to ClickHouse (pending + awaiting ack).",
	})
)
