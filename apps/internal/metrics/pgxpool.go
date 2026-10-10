package metrics

import (
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// RegisterPgxPool exposes a Postgres pool's own statistics, labelled by
// the binary it belongs to. Pool exhaustion is the first thing to break
// under load (tech audit §14).
//
// A second pool for the same service replaces the first: a test that
// builds the API more than once in one process must not panic on a
// duplicate registration.
func RegisterPgxPool(pool *pgxpool.Pool, service string) {
	pgxPoolsMu.Lock()
	defer pgxPoolsMu.Unlock()
	if old, ok := pgxPools[service]; ok {
		prometheus.Unregister(old)
	}
	c := &pgxPoolCollector{pool: pool, service: service}
	prometheus.MustRegister(c)
	pgxPools[service] = c
}

var (
	pgxPoolsMu sync.Mutex
	pgxPools   = map[string]*pgxPoolCollector{}
)

type pgxPoolCollector struct {
	pool    *pgxpool.Pool
	service string
}

var (
	pgPoolConns = prometheus.NewDesc(prometheus.BuildFQName(namespace, "pg_pool", "conns"),
		"Postgres pool connections by state (acquired, idle, total) and the configured max.",
		[]string{"service", "state"}, nil)
	pgPoolAcquires = prometheus.NewDesc(prometheus.BuildFQName(namespace, "pg_pool", "acquires_total"),
		"Postgres pool acquires: all, those that had to wait for a free connection (empty), and canceled.",
		[]string{"service", "kind"}, nil)
	pgPoolAcquireWait = prometheus.NewDesc(prometheus.BuildFQName(namespace, "pg_pool", "acquire_wait_seconds_total"),
		"Total time spent waiting to acquire a Postgres connection.",
		[]string{"service"}, nil)
)

func (c *pgxPoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- pgPoolConns
	ch <- pgPoolAcquires
	ch <- pgPoolAcquireWait
}

func (c *pgxPoolCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(pgPoolConns, prometheus.GaugeValue, float64(s.AcquiredConns()), c.service, "acquired")
	ch <- prometheus.MustNewConstMetric(pgPoolConns, prometheus.GaugeValue, float64(s.IdleConns()), c.service, "idle")
	ch <- prometheus.MustNewConstMetric(pgPoolConns, prometheus.GaugeValue, float64(s.TotalConns()), c.service, "total")
	ch <- prometheus.MustNewConstMetric(pgPoolConns, prometheus.GaugeValue, float64(s.MaxConns()), c.service, "max")
	ch <- prometheus.MustNewConstMetric(pgPoolAcquires, prometheus.CounterValue, float64(s.AcquireCount()), c.service, "all")
	ch <- prometheus.MustNewConstMetric(pgPoolAcquires, prometheus.CounterValue, float64(s.EmptyAcquireCount()), c.service, "empty")
	ch <- prometheus.MustNewConstMetric(pgPoolAcquires, prometheus.CounterValue, float64(s.CanceledAcquireCount()), c.service, "canceled")
	ch <- prometheus.MustNewConstMetric(pgPoolAcquireWait, prometheus.CounterValue, s.AcquireDuration().Seconds(), c.service)
}
