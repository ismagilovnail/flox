package config

import (
	"testing"
	"time"
)

// Each binary's ClickHouse query limits (tech audit CH4): reports in the
// API are bounded, attribution in the tracker tighter, the worker's long
// jobs not at all; the environment overrides any of them.
func TestClickHouseLimitsPerBinary(t *testing.T) {
	t.Setenv("CLICKHOUSE_MAX_EXECUTION_TIME", "")
	t.Setenv("CLICKHOUSE_MAX_MEMORY_USAGE", "")
	for name, c := range map[string]struct {
		load   func() (Config, error)
		exec   time.Duration
		memory int64
	}{
		"api":     {Load, 30 * time.Second, 4 << 30},
		"tracker": {LoadTracker, 10 * time.Second, 1 << 30},
		"worker":  {LoadWorker, 0, 0},
	} {
		cfg, err := c.load()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if cfg.ClickHouse.MaxExecutionTime != c.exec || cfg.ClickHouse.MaxMemoryUsage != c.memory {
			t.Errorf("%s: limits %v / %d, want %v / %d", name, cfg.ClickHouse.MaxExecutionTime, cfg.ClickHouse.MaxMemoryUsage, c.exec, c.memory)
		}
	}

	t.Setenv("CLICKHOUSE_MAX_EXECUTION_TIME", "120")
	t.Setenv("CLICKHOUSE_MAX_MEMORY_USAGE", "0")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClickHouse.MaxExecutionTime != 2*time.Minute || cfg.ClickHouse.MaxMemoryUsage != 0 {
		t.Errorf("overridden: %v / %d", cfg.ClickHouse.MaxExecutionTime, cfg.ClickHouse.MaxMemoryUsage)
	}

	t.Setenv("CLICKHOUSE_MAX_EXECUTION_TIME", "-1")
	if _, err := Load(); err == nil {
		t.Error("a negative limit was accepted")
	}
}

// The JetStream stream's size must fit the server's max_file_store, so it
// is configurable (a single small NATS server refused the 10 GiB default).
func TestNATSStreamMaxMB(t *testing.T) {
	t.Setenv("NATS_STREAM_MAX_MB", "")
	cfg, err := LoadWorker()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.NATSStreamMaxMB != 10240 {
		t.Errorf("default %d MB, want 10240", cfg.NATSStreamMaxMB)
	}

	t.Setenv("NATS_STREAM_MAX_MB", "2048")
	if cfg, err = LoadTracker(); err != nil || cfg.NATSStreamMaxMB != 2048 {
		t.Errorf("overridden: %d MB, %v", cfg.NATSStreamMaxMB, err)
	}

	t.Setenv("NATS_STREAM_MAX_MB", "lots")
	if _, err := LoadWorker(); err == nil {
		t.Error("a non-number was accepted")
	}
}
