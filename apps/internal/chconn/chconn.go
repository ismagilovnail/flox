// Package chconn owns the ClickHouse connection shared by anything that
// reads or writes the analytics store — mirrors internal/postgres.NewPool
// and internal/rediscache.NewClient.
//
// infra/docker-compose.dev.yml exposes only ClickHouse's HTTP interface
// (port 8123) to the host — the native protocol port stays container-
// internal to avoid colliding with MinIO's API port — so this package
// always dials over HTTP (clickhouse.HTTP), never the native protocol
// reference implementations elsewhere default to.
package chconn

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/ismagilovnail/flox/apps/internal/config"
)

const (
	// dialTimeout: the driver's default is 30 s — a dead ClickHouse would
	// hold every caller that long before failing.
	dialTimeout = 5 * time.Second
	// maxOpenConns per binary: one more than the driver's default idle
	// pool, enough for the API's parallel report queries without letting
	// one binary take all of ClickHouse's max_concurrent_queries.
	maxOpenConns = 20
)

// NewConn opens a ClickHouse connection over HTTP (HTTPS when the URL says
// so) with the binary's query limits (cfg.MaxExecutionTime,
// cfg.MaxMemoryUsage; tech audit CH4), and pings it.
//
// Uncompressed on purpose: with LZ4 over HTTP the driver tries to
// decompress the server's plain-text error on a failed INSERT and loses
// it ("data size should be …"), and chstore.Rejected needs that error's
// code to dead-letter a batch ClickHouse will never accept.
func NewConn(ctx context.Context, cfg config.ClickHouseConfig) (driver.Conn, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("chconn: CLICKHOUSE_URL %q: want http://host:port or https://host:port", cfg.URL)
	}
	opts := &clickhouse.Options{
		Protocol: clickhouse.HTTP,
		Addr:     []string{u.Host},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.User,
			Password: cfg.Password,
		},
		DialTimeout:  dialTimeout,
		MaxOpenConns: maxOpenConns,
		MaxIdleConns: 5,
		Settings:     limits(cfg),
	}
	switch u.Scheme {
	case "https":
		opts.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	case "http", "":
	default:
		return nil, fmt.Errorf("chconn: CLICKHOUSE_URL scheme %q: want http or https", u.Scheme)
	}
	if cfg.MaxExecutionTime > 0 {
		// The client stops waiting a little after the server should have
		// given up, so a server ignoring the limit does not hang the caller.
		opts.ReadTimeout = cfg.MaxExecutionTime + 10*time.Second
	}

	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("chconn: open: %w", err)
	}

	if err := conn.Ping(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("chconn: ping: %w", err)
	}
	return conn, nil
}

// limits are the per-query settings sent with every query.
//
// For a binary with a time limit (apps/api, apps/tracker), a query whose
// caller went away — a report the user navigated off, one the API's
// request timeout already answered — is stopped on ClickHouse too. Over
// HTTP the server keeps running a SELECT whose client disconnected unless
// cancel_http_readonly_queries_on_client_close is on, and it only notices
// the disconnect when it writes: hence a progress header every second.
// Not for apps/worker: its queries run for hours, and a header a second
// would outgrow the HTTP client's 1 MB header limit.
//
// A context deadline also bounds a query: the driver then sends
// max_execution_time = time left + 5 s, in place of the value here.
func limits(cfg config.ClickHouseConfig) clickhouse.Settings {
	s := clickhouse.Settings{}
	if cfg.MaxExecutionTime > 0 {
		s["max_execution_time"] = int(cfg.MaxExecutionTime / time.Second)
		s["cancel_http_readonly_queries_on_client_close"] = 1
		s["send_progress_in_http_headers"] = 1
		s["http_headers_progress_interval_ms"] = 1000
	}
	if cfg.MaxMemoryUsage > 0 {
		s["max_memory_usage"] = cfg.MaxMemoryUsage
	}
	return s
}

// IsQueryTooHeavy reports whether err is ClickHouse stopping a query at
// max_execution_time or max_memory_usage — for apps/api, a report to
// narrow, not a server fault. Over HTTP the driver returns the server's
// message as text; the exception's name is in it.
func IsQueryTooHeavy(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "(TIMEOUT_EXCEEDED)") || strings.Contains(msg, "(MEMORY_LIMIT_EXCEEDED)")
}
