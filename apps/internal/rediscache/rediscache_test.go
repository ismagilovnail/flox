package rediscache

import (
	"context"
	"os"
	"strconv"
	"testing"
)

func TestEvictionProblem(t *testing.T) {
	for _, tc := range []struct {
		policy    string
		maxmemory int64
		wantOK    bool
	}{
		{"noeviction", 256 << 20, false},
		{"", 256 << 20, false},
		{"allkeys-lru", 0, false},
		{"allkeys-lru", 256 << 20, true},
		{"volatile-lru", 256 << 20, true},
		{"allkeys-lfu", 1 << 30, true},
	} {
		if got := evictionProblem(tc.policy, tc.maxmemory); (got == "") != tc.wantOK {
			t.Errorf("evictionProblem(%q, %d) = %q, want ok=%v", tc.policy, tc.maxmemory, got, tc.wantOK)
		}
	}
}

// The dev and test compose files start Redis with maxmemory and
// allkeys-lru; this pins that, against the server REDIS_URL points at.
func TestComposeRedisEvicts(t *testing.T) {
	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL not set, skipping integration test")
	}
	ctx := context.Background()
	client, err := NewClient(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	cfg, err := client.ConfigGet(ctx, "maxmemory*").Result()
	if err != nil {
		t.Fatal(err)
	}
	maxmemory, err := strconv.ParseInt(cfg["maxmemory"], 10, 64)
	if err != nil {
		t.Fatalf("maxmemory %q: %v", cfg["maxmemory"], err)
	}
	if problem := evictionProblem(cfg["maxmemory-policy"], maxmemory); problem != "" {
		t.Fatalf("compose Redis: %s (maxmemory=%d, policy=%q)", problem, maxmemory, cfg["maxmemory-policy"])
	}
}
