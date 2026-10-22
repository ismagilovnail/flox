package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ClickHouseTenantWaitsTotal / ClickHouseTenantWaitSeconds:
// apps/internal/chconn.TenantConn (audit CH-1) — a report query that found
// its organization's slots taken: "waited" got one, "busy" gave up after
// the wait (429). Labelled by plan, never by organization.
var (
	ClickHouseTenantWaitsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "clickhouse_tenant_waits_total",
		Help:      "Report queries that waited for one of their organization's ClickHouse slots, by plan and outcome (waited|busy).",
	}, []string{"plan", "outcome"})

	ClickHouseTenantWaitSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "clickhouse_tenant_wait_seconds",
		Help:      "How long a report query waited for one of its organization's ClickHouse slots.",
		Buckets:   []float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10},
	})
)
