package chconn_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

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

// The binary's limits reach every query, and a query stopped at one is
// recognized (tech audit CH4).
func TestLimitsStopAHeavyQuery(t *testing.T) {
	url := os.Getenv("CLICKHOUSE_URL")
	if url == "" {
		t.Skip("CLICKHOUSE_URL not set, skipping integration test")
	}
	ctx := context.Background()
	for name, cfg := range map[string]config.ClickHouseConfig{
		"time":   {MaxExecutionTime: time.Second},
		"memory": {MaxMemoryUsage: 10 << 20},
	} {
		cfg.URL, cfg.Database, cfg.User = url, envOr("CLICKHOUSE_DATABASE", "flox"), envOr("CLICKHOUSE_USER", "flox")
		conn, err := chconn.NewConn(ctx, cfg)
		if err != nil {
			t.Fatalf("%s: NewConn: %v", name, err)
		}
		var n uint64
		query := "SELECT count() FROM numbers(100000000000) WHERE sipHash64(number) % 7 = 3"
		if name == "memory" {
			query = "SELECT length(groupArray(number)) FROM numbers(10000000)"
		}
		started := time.Now()
		err = conn.QueryRow(ctx, query).Scan(&n)
		conn.Close()
		if !chconn.IsQueryTooHeavy(err) {
			t.Errorf("%s: err = %v, want a query stopped at its limit", name, err)
		}
		if took := time.Since(started); took > 10*time.Second {
			t.Errorf("%s: took %s", name, took)
		}
	}
}

func TestIsQueryTooHeavy(t *testing.T) {
	for msg, want := range map[string]bool{
		"ClickHouse exception: Code: 159. DB::Exception: Timeout exceeded: elapsed 1 seconds, maximum: 1. (TIMEOUT_EXCEEDED)": true,
		"Code: 241. DB::Exception: Memory limit (for query) exceeded. (MEMORY_LIMIT_EXCEEDED)":                                true,
		"Code: 60. DB::Exception: Table flox.x does not exist. (UNKNOWN_TABLE)":                                               false,
	} {
		if got := chconn.IsQueryTooHeavy(errors.New(msg)); got != want {
			t.Errorf("IsQueryTooHeavy(%q) = %v", msg, got)
		}
	}
	if chconn.IsQueryTooHeavy(nil) {
		t.Error("nil is not an error")
	}
}

func TestNewConnRefusesAnUnknownScheme(t *testing.T) {
	if _, err := chconn.NewConn(context.Background(), config.ClickHouseConfig{URL: "tcp://localhost:9000"}); err == nil || !strings.Contains(err.Error(), "scheme") {
		t.Fatalf("err = %v, want the scheme refused", err)
	}
}

// A query whose caller gave up stops on the server too: over HTTP,
// ClickHouse otherwise runs it to the end (tech audit CH4).
func TestCancelledQueryStopsOnTheServer(t *testing.T) {
	url := os.Getenv("CLICKHOUSE_URL")
	if url == "" {
		t.Skip("CLICKHOUSE_URL not set, skipping integration test")
	}
	// A binary with a time limit (apps/api's 30 s): the query below would
	// run far longer than the 1 s after which its caller gives up.
	conn, err := chconn.NewConn(context.Background(), config.ClickHouseConfig{
		URL: url, Database: envOr("CLICKHOUSE_DATABASE", "flox"), User: envOr("CLICKHOUSE_USER", "flox"),
		MaxExecutionTime: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	marker := "cancel-test-" + time.Now().Format("150405.000000")
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(time.Second, cancel)
	var n uint64
	_ = conn.QueryRow(ctx, "SELECT count() FROM numbers(100000000000) WHERE sipHash64(number) % 7 = 3 AND '"+marker+"' != ''").Scan(&n)

	// Still running a few seconds later means it was not cancelled.
	deadline := time.Now().Add(5 * time.Second)
	for {
		var running uint64
		if err := conn.QueryRow(context.Background(),
			"SELECT count() FROM system.processes WHERE query LIKE ? AND query NOT LIKE '%system.processes%'", "%"+marker+"%").Scan(&running); err != nil {
			t.Fatal(err)
		}
		if running == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the query is still running on ClickHouse after its context was cancelled")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// With progress headers on, a query longer than their interval still
// returns its result.
func TestBoundedQueryWithProgressHeadersReturns(t *testing.T) {
	url := os.Getenv("CLICKHOUSE_URL")
	if url == "" {
		t.Skip("CLICKHOUSE_URL not set, skipping integration test")
	}
	conn, err := chconn.NewConn(context.Background(), config.ClickHouseConfig{
		URL: url, Database: envOr("CLICKHOUSE_DATABASE", "flox"), User: envOr("CLICKHOUSE_USER", "flox"),
		MaxExecutionTime: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	started := time.Now()
	var n uint64
	if err := conn.QueryRow(context.Background(), "SELECT count() FROM numbers(200000000) WHERE sipHash64(number) % 7 = 3").Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n == 0 {
		t.Fatal("no result")
	}
	t.Logf("took %s", time.Since(started).Round(time.Millisecond))
}
