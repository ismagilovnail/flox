// Package metrics is the single place every §53 Prometheus
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
	// tracking link or inactive campaign), "rate_limited" (over the
	// per-IP click limit, sent on unrecorded), or "error".
	TrackingRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "tracking_requests_total",
		Help:      "Total apps/tracker redirect requests, by outcome.",
	}, []string{"outcome"})

	// UntrustedForwardedTotal counts requests carrying X-Forwarded-For from
	// a peer outside TRUSTED_PROXIES (internal/clientip): the header is
	// ignored and the peer is taken as the client. A few are direct
	// visitors behind their own proxy; most of a service's traffic means
	// it sits behind a proxy or CDN that TRUSTED_PROXIES leaves out, and
	// every visitor shares the proxy's address — one per-IP click limit
	// for all of them (audit REL-2, alert UntrustedProxyInFront).
	UntrustedForwardedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "untrusted_forwarded_requests_total",
		Help:      "Requests with X-Forwarded-For from a peer outside TRUSTED_PROXIES (the header was ignored).",
	})

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
	// QueueReapedTotal counts rows apps/worker's reaper returned to a queue
	// after their claim was abandoned (internal/queuereaper). Any increase
	// means a worker died mid-batch; a steady one means it keeps dying.
	QueueReapedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "queue_reaped_total",
		Help:      "Abandoned 'processing' queue rows returned to their queue, by queue.",
	}, []string{"queue"})

	// FXSyncTotal counts apps/worker's reference-rate syncs (internal/fx),
	// by outcome (ok|error).
	FXSyncTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "fx_sync_total",
		Help:      "FX reference-rate sync runs, by outcome.",
	}, []string{"outcome"})

	// FXLatestRateAgeSeconds is how old the newest stored rate is. Reference
	// rates skip weekends and holidays, so a few days is normal; past
	// fx.MaxStaleDays new non-USD money stops getting a USD value.
	FXLatestRateAgeSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "fx_latest_rate_age_seconds",
		Help:      "Age of the newest FX rate on file, refreshed after each sync.",
	})

	// RoutingCacheRequestsTotal counts apps/tracker's routing cache reads
	// (internal/routingcache), by table and result: hit, stale (served while
	// refreshing), miss (loaded from Postgres), stale_on_error, error,
	// refresh_error.
	RoutingCacheRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "routing_cache_requests_total",
		Help:      "apps/tracker routing cache reads, by table and result.",
	}, []string{"table", "result"})

	// RoutingCacheInvalidationsTotal counts full cache flushes (a routing
	// table changed, or the change listener reconnected).
	RoutingCacheInvalidationsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "routing_cache_invalidations_total",
		Help:      "apps/tracker routing cache flushes on configuration change.",
	})

	// QueueDeadLetteredTotal counts rows moved to 'dead' because ClickHouse
	// kept rejecting their batch (pgqueue.Fail). Not lost — kept in
	// Postgres to be put back once the cause is fixed — but not in reports.
	QueueDeadLetteredTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "queue_dead_lettered_total",
		Help:      "Queue rows dead-lettered after ClickHouse repeatedly rejected their batch, by queue.",
	}, []string{"queue"})

	// BrowserConversionsTotal counts conversions from a page's script
	// (apps/tracker /c/{networkId}, audit UNI-1 part 2) by result: the
	// engine's, unknown_click (fresh, retried) or a refusal before it
	// (unknown_network, forbidden_origin, bad_request, rate_limited,
	// internal_error).
	BrowserConversionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "browser_conversions_total",
		Help:      "Conversions reported by a page's script, by result.",
	}, []string{"result"})

	// OfflineConversionsTotal counts conversions reported to an ad network
	// (internal/offlineconv, audit UNI-2 part 2) by result: sent, retry
	// (to be sent again), failed (refused, or out of attempts).
	OfflineConversionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "offline_conversions_total",
		Help:      "Conversions reported to an ad network, by result.",
	}, []string{"result"})

	// ShopWebhooksTotal counts online stores' order webhooks (apps/tracker
	// /shopify/, /woocommerce/ and /tilda/{networkId}, audit UNI-4) by
	// platform, topic (orders/paid, refunds/create, order.created,
	// order.updated, form, other) and result: the engine's, or why it never got
	// there (bad_signature, unknown_network, ping, ignored_*,
	// internal_error).
	ShopWebhooksTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "shop_webhooks_total",
		Help:      "Online stores' order webhooks, by platform, topic and result.",
	}, []string{"platform", "topic", "result"})

	// PostbackIncomingTotal counts incoming network postbacks (apps/tracker
	// /postback/{networkId}) by result: the conversion engine's own
	// (success, duplicate, ignored, error) or a request refused before it
	// (unknown_network, unauthorized, bad_request, internal_error).
	PostbackIncomingTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "postback_incoming_total",
		Help:      "Incoming network postbacks, by result.",
	}, []string{"result"})

	PostbackIncomingLatencySeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "postback_incoming_latency_seconds",
		Help:      "Incoming postback handling latency (mapping, attribution, the record transaction).",
		Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
	})

	// StickyCookiesRejectedTotal counts sticky cookies the tracker ignored
	// (apps/tracker/sticky.go, tech audit T5): unsigned (issued before
	// signing, or forged), bad_signature (forged or another key), malformed
	// (signed but not ULIDs — a bug). Steady bad_signature is someone
	// trying.
	StickyCookiesRejectedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "sticky_cookies_rejected_total",
		Help:      "Sticky cookies ignored, by reason.",
	}, []string{"reason"})

	// AttributionOutcomesTotal counts how conversions were attributed
	// (attribution.Outcome: attributed, unknown_click, ambiguous, …). A
	// rising unknown_click share means clicks are not reaching ClickHouse
	// in time, or networks send ids FLOX never issued.
	AttributionOutcomesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "attribution_outcomes_total",
		Help:      "Conversion attribution outcomes.",
	}, []string{"outcome"})

	// RetentionDeletedTotal counts rows the retention job deleted
	// (internal/retention), by table.
	RetentionDeletedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "retention_deleted_total",
		Help:      "Rows deleted by the retention job, by table.",
	}, []string{"table"})

	// ReattributionsTotal counts unknown_click conversions the worker
	// looked up again (internal/reattribute): attributed — the click turned
	// up and the conversion was rewritten; gave_up — still no click after
	// the last retry, unknown_click stays.
	ReattributionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "reattributions_total",
		Help:      "Retried attributions of unknown_click conversions, by outcome.",
	}, []string{"outcome"})

	// AttributionRetriesPending is how many conversions wait for their
	// click to be looked up again or their event to be rewritten.
	AttributionRetriesPending = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "attribution_retries_pending",
		Help:      "Conversions waiting for a retried attribution.",
	})

	// QueueOldestDueAgeSeconds is how long the oldest due row of each
	// worker queue has been waiting past its due time — catches a stalled
	// queue that is too small for a depth alert.
	QueueOldestDueAgeSeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "queue_oldest_due_age_seconds",
		Help:      "Seconds the oldest due row of a worker queue has waited past its due time (0 when none is due).",
	}, []string{"queue"})

	// QueueRows counts a worker queue's open rows by status (queued,
	// retrying, processing, dead — never the kept success rows).
	QueueRows = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "queue_rows",
		Help:      "Open rows of a worker queue, by status.",
	}, []string{"queue", "status"})

	// FXRateMissingTotal counts conversions and cost entries stored without
	// a USD value because no rate was on file. No currency label: the code
	// comes from the postback request and would be unbounded.
	FXRateMissingTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "fx_rate_missing_total",
		Help:      "Amounts stored without a USD value because no FX rate was on file.",
	})

	// RedisErrorsTotal counts Redis calls that failed and fell through
	// (Redis is cache and rate limits only, so failures are survivable —
	// but silent without this). op: conversion_last_status, ratelimit.
	RedisErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "redis_errors_total",
		Help:      "Failed Redis calls that fell back to the durable path, by operation.",
	}, []string{"op"})

	// ReconciliationMismatchedBuckets is how many (organization, day,
	// status) buckets of the last reconciliation run differ between Postgres
	// postbacks and ClickHouse conversion_events, by kind (count, usd).
	ReconciliationMismatchedBuckets = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "reconciliation_mismatched_buckets",
		Help:      "Buckets where Postgres and ClickHouse conversions differ, by kind (count|usd), as of the last run.",
	}, []string{"kind"})

	// ReconciliationRepairedTotal counts ClickHouse conversion events the
	// reconciliation rewrote from Postgres, by action (reemitted, deleted).
	ReconciliationRepairedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "reconciliation_repaired_events_total",
		Help:      "ClickHouse conversion events rewritten from Postgres by the reconciliation, by action.",
	}, []string{"action"})

	ReconciliationRunsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "reconciliation_runs_total",
		Help:      "Postgres/ClickHouse reconciliation runs, by outcome.",
	}, []string{"outcome"})

	ReconciliationLastSuccessSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "reconciliation_last_success_timestamp_seconds",
		Help:      "Unix time of the last successful Postgres/ClickHouse reconciliation run.",
	})

	PostbackDeliveriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "postback_deliveries_total",
		Help:      "Outgoing postback delivery attempts, by outcome (success|retrying|dead).",
	}, []string{"outcome"})

	// PixelDeliveriesTotal: apps/internal/pixelsend.Deliverer — events
	// sent to ad platforms, by provider and outcome (success|retrying|dead).
	PixelDeliveriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "pixel_deliveries_total",
		Help:      "Pixel event delivery attempts to ad platforms, by provider and outcome (success|retrying|dead).",
	}, []string{"provider", "outcome"})

	// PushDeliveriesTotal: apps/internal/pushsend.Deliverer — push
	// notification send attempts by outcome (delivered|gone|retry|failed).
	PushDeliveriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "push_deliveries_total",
		Help:      "Push notification send attempts, by outcome (delivered|gone|retry|failed).",
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
