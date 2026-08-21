package chconn_test

import (
	"context"
	"os"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/config"
)

func TestNewConnPingsRealClickHouse(t *testing.T) {
	url := os.Getenv("CLICKHOUSE_URL")
	if url == "" {
		t.Skip("CLICKHOUSE_URL not set, skipping integration test")
	}
	conn, err := chconn.NewConn(context.Background(), config.ClickHouseConfig{
		URL:      url,
		Database: envOr("CLICKHOUSE_DATABASE", "flox"),
		User:     envOr("CLICKHOUSE_USER", "flox"),
		Password: os.Getenv("CLICKHOUSE_PASSWORD"),
	})
	if err != nil {
		t.Fatalf("NewConn: %v", err)
	}
	defer conn.Close()

	var one uint8
	row := conn.QueryRow(context.Background(), "SELECT 1")
	if err := row.Scan(&one); err != nil {
		t.Fatalf("SELECT 1: %v", err)
	}
	if one != 1 {
		t.Fatalf("SELECT 1 = %d, want 1", one)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
