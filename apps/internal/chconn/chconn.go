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
	"fmt"
	"net/url"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/ismagilovnail/flox/apps/internal/config"
)

// NewConn opens a ClickHouse connection over HTTP and pings it.
func NewConn(ctx context.Context, cfg config.ClickHouseConfig) (driver.Conn, error) {
	addr, err := hostPort(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("chconn: parsing CLICKHOUSE_URL: %w", err)
	}

	conn, err := clickhouse.Open(&clickhouse.Options{
		Protocol: clickhouse.HTTP,
		Addr:     []string{addr},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.User,
			Password: cfg.Password,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("chconn: open: %w", err)
	}

	if err := conn.Ping(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("chconn: ping: %w", err)
	}
	return conn, nil
}

// hostPort extracts "host:port" from a URL like "http://localhost:8123" —
// clickhouse-go's Addr wants the authority only, not a scheme.
func hostPort(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if u.Host == "" {
		return "", fmt.Errorf("no host in %q", rawURL)
	}
	return u.Host, nil
}
