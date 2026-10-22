package chconn

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

// TenantLimits is what one organization may take of ClickHouse at once
// (audit CH-1): its plan's row in Postgres (billing.Repository).
type TenantLimits struct {
	Plan          string // the plan's code, for metrics
	MaxConcurrent int    // queries at once; more wait, then ErrBusy
	MaxThreads    int
	MaxExecution  time.Duration
	MaxMemory     int64 // bytes
	Priority      int   // ClickHouse's priority: lower runs first
}

// LimitsFunc reads an organization's limits.
type LimitsFunc func(ctx context.Context, orgID string) (TenantLimits, error)

// ErrBusy: the organization already runs its plan's number of ClickHouse
// queries and none finished within the wait. apierror answers 429.
var ErrBusy = errors.New("chconn: too many reports running at once for this organization")

// IsBusy reports whether err is ErrBusy.
func IsBusy(err error) bool { return errors.Is(err, ErrBusy) }

const (
	// tenantWait is how long a query waits for one of its organization's
	// slots before ErrBusy.
	tenantWait = 10 * time.Second
	// limitsTTL is how long an organization's limits are kept: a plan
	// change applies within a minute.
	limitsTTL = time.Minute
)

// FallbackLimits apply when an organization's limits cannot be read —
// the tightest plan's, so a Postgres hiccup never lifts isolation.
var FallbackLimits = TenantLimits{Plan: "unknown", MaxConcurrent: 2, MaxThreads: 2, MaxExecution: 15 * time.Second, MaxMemory: 1 << 30, Priority: 4}

// TenantConn is apps/api's ClickHouse connection: every query made on an
// organization's behalf (the organization is in ctx) takes one of that
// organization's slots until its result is read, and runs with its plan's
// settings and quota_key = the organization's id — so one organization's
// heavy reports cannot take the connection pool, or the server, from the
// others. A query without an organization (migrations, background
// monitors) runs as before.
//
// Slots are counted per process: N API replicas allow N× a plan's
// concurrency. quota_key lets a ClickHouse QUOTA KEYED BY client_key
// count per organization on the server as well (docs/deployment.md).
type TenantConn struct {
	driver.Conn
	orgOf  func(context.Context) (string, bool)
	limits LimitsFunc
	logger *slog.Logger
	now    func() time.Time
	wait   time.Duration

	mu     sync.Mutex
	cached map[string]cachedLimits
	slots  map[string]chan struct{}
}

type cachedLimits struct {
	limits TenantLimits
	at     time.Time
}

func NewTenantConn(conn driver.Conn, orgOf func(context.Context) (string, bool), limits LimitsFunc, logger *slog.Logger) *TenantConn {
	return &TenantConn{Conn: conn, orgOf: orgOf, limits: limits, logger: logger, now: time.Now, wait: tenantWait,
		cached: map[string]cachedLimits{}, slots: map[string]chan struct{}{}}
}

// WithWait replaces the 10 s wait for a slot, for tests.
func (c *TenantConn) WithWait(d time.Duration) *TenantConn {
	c.wait = d
	return c
}

type settingsKey struct{}

// Settings adds per-query ClickHouse settings to ctx. Use it instead of
// clickhouse.Context(ctx, clickhouse.WithSettings(...)) in code apps/api
// runs: the driver's WithSettings replaces every earlier setting and
// cannot be read back, so TenantConn would drop the caller's settings or
// the caller the plan's. Settings keeps them in ctx for TenantConn to
// merge, and also sets them on the driver's context, so it works on a
// plain connection too.
func Settings(ctx context.Context, s clickhouse.Settings) context.Context {
	merged := clickhouse.Settings{}
	if prev, ok := ctx.Value(settingsKey{}).(clickhouse.Settings); ok {
		maps.Copy(merged, prev)
	}
	maps.Copy(merged, s)
	ctx = context.WithValue(ctx, settingsKey{}, merged)
	return clickhouse.Context(ctx, clickhouse.WithSettings(merged))
}

// begin takes a slot for ctx's organization and returns the context to
// query with and the release; ok false: no organization, query as is.
func (c *TenantConn) begin(ctx context.Context) (context.Context, func(), error) {
	org, ok := c.orgOf(ctx)
	if !ok || org == "" {
		return ctx, func() {}, nil
	}
	lim := c.limitsFor(ctx, org)
	slot := c.slot(org, lim.MaxConcurrent)

	start := c.now()
	select {
	case slot <- struct{}{}:
	default:
		timer := time.NewTimer(c.wait)
		defer timer.Stop()
		select {
		case slot <- struct{}{}:
		case <-timer.C:
			metrics.ClickHouseTenantWaitsTotal.WithLabelValues(lim.Plan, "busy").Inc()
			return nil, nil, ErrBusy
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
		metrics.ClickHouseTenantWaitSeconds.Observe(c.now().Sub(start).Seconds())
		metrics.ClickHouseTenantWaitsTotal.WithLabelValues(lim.Plan, "waited").Inc()
	}

	var once sync.Once
	release := func() { once.Do(func() { <-slot }) }

	s := clickhouse.Settings{}
	if prev, ok := ctx.Value(settingsKey{}).(clickhouse.Settings); ok {
		maps.Copy(s, prev)
	}
	s["max_threads"] = lim.MaxThreads
	s["max_memory_usage"] = lim.MaxMemory
	s["priority"] = lim.Priority
	if lim.MaxExecution > 0 {
		s["max_execution_time"] = int(lim.MaxExecution / time.Second)
	}
	return clickhouse.Context(ctx, clickhouse.WithSettings(s), clickhouse.WithQuotaKey(org)), release, nil
}

func (c *TenantConn) limitsFor(ctx context.Context, org string) TenantLimits {
	c.mu.Lock()
	cl, ok := c.cached[org]
	c.mu.Unlock()
	if ok && c.now().Sub(cl.at) < limitsTTL {
		return cl.limits
	}
	lim, err := c.limits(ctx, org)
	if err != nil || lim.MaxConcurrent <= 0 {
		if err != nil {
			c.logger.Warn("chconn: reading an organization's report limits, using the fallback", "organization_id", org, "error", err)
		}
		lim = FallbackLimits
	}
	c.mu.Lock()
	c.cached[org] = cachedLimits{limits: lim, at: c.now()}
	c.mu.Unlock()
	return lim
}

// slot is the organization's semaphore, made again when its plan's
// concurrency changes (queries holding the old one finish on it).
func (c *TenantConn) slot(org string, n int) chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.slots[org]
	if !ok || cap(s) != n {
		s = make(chan struct{}, n)
		c.slots[org] = s
	}
	return s
}

func (c *TenantConn) Query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	qctx, release, err := c.begin(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := c.Conn.Query(qctx, query, args...)
	if err != nil {
		release()
		return nil, err
	}
	return &tenantRows{Rows: rows, release: release}, nil
}

func (c *TenantConn) QueryRow(ctx context.Context, query string, args ...any) driver.Row {
	qctx, release, err := c.begin(ctx)
	if err != nil {
		return errRow{err}
	}
	return &tenantRow{Row: c.Conn.QueryRow(qctx, query, args...), release: release}
}

func (c *TenantConn) Select(ctx context.Context, dest any, query string, args ...any) error {
	qctx, release, err := c.begin(ctx)
	if err != nil {
		return err
	}
	defer release()
	return c.Conn.Select(qctx, dest, query, args...)
}

func (c *TenantConn) Exec(ctx context.Context, query string, args ...any) error {
	qctx, release, err := c.begin(ctx)
	if err != nil {
		return err
	}
	defer release()
	return c.Conn.Exec(qctx, query, args...)
}

// tenantRows gives the slot back once the result is read or closed.
type tenantRows struct {
	driver.Rows
	release func()
}

func (r *tenantRows) Next() bool {
	if r.Rows.Next() {
		return true
	}
	r.release()
	return false
}

func (r *tenantRows) Close() error {
	defer r.release()
	return r.Rows.Close()
}

// tenantRow gives the slot back once the row is scanned.
type tenantRow struct {
	driver.Row
	release func()
}

func (r *tenantRow) Scan(dest ...any) error {
	defer r.release()
	return r.Row.Scan(dest...)
}

func (r *tenantRow) ScanStruct(dest any) error {
	defer r.release()
	return r.Row.ScanStruct(dest)
}

// errRow is a row whose query never ran.
type errRow struct{ err error }

func (r errRow) Err() error           { return r.err }
func (r errRow) Scan(...any) error    { return r.err }
func (r errRow) ScanStruct(any) error { return r.err }
