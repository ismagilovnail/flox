package chconn_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

type orgKey struct{}

func withOrg(ctx context.Context, org string) context.Context {
	return context.WithValue(ctx, orgKey{}, org)
}

func orgOf(ctx context.Context) (string, bool) {
	org, ok := ctx.Value(orgKey{}).(string)
	return org, ok
}

// fakeConn runs nothing: Query blocks until its rows are read, so a test
// can hold a slot for as long as it likes.
type fakeConn struct {
	driver.Conn
	running atomic.Int32
	peak    atomic.Int32
}

func (f *fakeConn) enter() {
	n := f.running.Add(1)
	for {
		p := f.peak.Load()
		if n <= p || f.peak.CompareAndSwap(p, n) {
			return
		}
	}
}

func (f *fakeConn) Query(context.Context, string, ...any) (driver.Rows, error) {
	f.enter()
	return &fakeRows{conn: f, left: 1}, nil
}

func (f *fakeConn) QueryRow(context.Context, string, ...any) driver.Row {
	f.enter()
	return &fakeRow{conn: f}
}

func (f *fakeConn) Select(context.Context, any, string, ...any) error {
	f.enter()
	f.running.Add(-1)
	return nil
}

func (f *fakeConn) Exec(context.Context, string, ...any) error {
	f.enter()
	f.running.Add(-1)
	return nil
}

type fakeRows struct {
	driver.Rows
	conn *fakeConn
	left int
	done sync.Once
}

func (r *fakeRows) Next() bool {
	if r.left == 0 {
		r.done.Do(func() { r.conn.running.Add(-1) })
		return false
	}
	r.left--
	return true
}
func (r *fakeRows) Close() error {
	r.done.Do(func() { r.conn.running.Add(-1) })
	return nil
}
func (r *fakeRows) Err() error { return nil }

type fakeRow struct {
	driver.Row
	conn *fakeConn
}

func (r *fakeRow) Scan(...any) error { r.conn.running.Add(-1); return nil }

func limitsOf(n int, calls *atomic.Int32) chconn.LimitsFunc {
	return func(context.Context, string) (chconn.TenantLimits, error) {
		if calls != nil {
			calls.Add(1)
		}
		return chconn.TenantLimits{Plan: "start", MaxConcurrent: n, MaxThreads: 4, MaxExecution: 30 * time.Second, MaxMemory: 1 << 30, Priority: 3}, nil
	}
}

func TestTenantSlotsWaitThenBusy(t *testing.T) {
	fake := &fakeConn{}
	c := chconn.NewTenantConn(fake, orgOf, limitsOf(2, nil), quiet).WithWait(100 * time.Millisecond)
	ctxA := withOrg(context.Background(), "org-a")

	// Two queries hold A's two slots; a third is refused after the wait.
	r1, err := c.Query(ctxA, "q")
	if err != nil {
		t.Fatal(err)
	}
	r2, err := c.Query(ctxA, "q")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := c.Query(ctxA, "q"); !chconn.IsBusy(err) {
		t.Fatalf("third query = %v, want ErrBusy", err)
	}
	if waited := time.Since(start); waited < 90*time.Millisecond {
		t.Errorf("refused after %v, want after the wait", waited)
	}
	if err := c.QueryRow(ctxA, "q").Scan(); !chconn.IsBusy(err) {
		t.Errorf("QueryRow while full = %v, want ErrBusy", err)
	}
	if err := c.Select(ctxA, nil, "q"); !chconn.IsBusy(err) {
		t.Errorf("Select while full = %v, want ErrBusy", err)
	}

	// Another organization is not held up by A.
	if err := c.Select(withOrg(context.Background(), "org-b"), nil, "q"); err != nil {
		t.Errorf("org B while A is full = %v", err)
	}
	// Nor is a query without an organization.
	if err := c.Exec(context.Background(), "q"); err != nil {
		t.Errorf("no organization = %v", err)
	}

	// Reading a result to the end gives its slot back; Close does too.
	for r1.Next() {
	}
	if err := c.Select(ctxA, nil, "q"); err != nil {
		t.Errorf("after one result was read = %v", err)
	}
	_ = r2.Close()
	_ = r2.Close() // twice: one slot back, not two
	row := c.QueryRow(ctxA, "q")
	if err := row.Scan(); err != nil {
		t.Errorf("QueryRow after Close = %v", err)
	}
}

func TestTenantWaitingQueryGetsTheFreedSlot(t *testing.T) {
	fake := &fakeConn{}
	c := chconn.NewTenantConn(fake, orgOf, limitsOf(1, nil), quiet).WithWait(2 * time.Second)
	ctx := withOrg(context.Background(), "org-a")
	held, err := c.Query(ctx, "q")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = held.Close()
	}()
	if err := c.Select(ctx, nil, "q"); err != nil {
		t.Fatalf("waiting query = %v, want the freed slot", err)
	}

	// A cancelled caller stops waiting with its context's error.
	held, _ = c.Query(ctx, "q")
	defer held.Close()
	cctx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	if err := c.Select(cctx, nil, "q"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("cancelled wait = %v, want the context's error", err)
	}
}

func TestTenantConcurrencyNeverExceedsThePlan(t *testing.T) {
	fake := &fakeConn{}
	c := chconn.NewTenantConn(fake, orgOf, limitsOf(3, nil), quiet).WithWait(5 * time.Second)
	ctx := withOrg(context.Background(), "org-a")
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rows, err := c.Query(ctx, "q")
			if err != nil {
				t.Error(err)
				return
			}
			time.Sleep(2 * time.Millisecond)
			for rows.Next() {
			}
		}()
	}
	wg.Wait()
	if p := fake.peak.Load(); p > 3 || p == 0 {
		t.Errorf("peak concurrency = %d, want at most the plan's 3", p)
	}
}

func TestTenantLimitsAreCachedAndFallBack(t *testing.T) {
	var calls atomic.Int32
	c := chconn.NewTenantConn(&fakeConn{}, orgOf, limitsOf(2, &calls), quiet)
	ctx := withOrg(context.Background(), "org-a")
	for i := 0; i < 5; i++ {
		if err := c.Select(ctx, nil, "q"); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Errorf("limits read %d times for 5 queries, want once (cached)", calls.Load())
	}

	failing := func(context.Context, string) (chconn.TenantLimits, error) {
		return chconn.TenantLimits{}, errors.New("postgres down")
	}
	fake := &fakeConn{}
	c = chconn.NewTenantConn(fake, orgOf, failing, quiet).WithWait(50 * time.Millisecond)
	var held []driver.Rows
	for i := 0; i < chconn.FallbackLimits.MaxConcurrent; i++ {
		r, err := c.Query(ctx, "q")
		if err != nil {
			t.Fatalf("query %d with the fallback = %v", i+1, err)
		}
		held = append(held, r)
	}
	if _, err := c.Query(ctx, "q"); !chconn.IsBusy(err) {
		t.Errorf("past the fallback's concurrency = %v, want ErrBusy (isolation kept)", err)
	}
	for _, r := range held {
		_ = r.Close()
	}
}

// Against a real ClickHouse: the plan's settings and the quota key reach
// the server, merged with the caller's own (chconn.Settings) and with
// the connection's (cancel on client close), as system.query_log shows.
func TestTenantSettingsReachClickHouse(t *testing.T) {
	url := os.Getenv("CLICKHOUSE_URL")
	if url == "" {
		t.Skip("CLICKHOUSE_URL not set, skipping integration test")
	}
	ctx := context.Background()
	raw, err := chconn.NewConn(ctx, config.ClickHouseConfig{
		URL: url, Database: envOr("CLICKHOUSE_DATABASE", "flox"), User: envOr("CLICKHOUSE_USER", "flox"),
		Password: os.Getenv("CLICKHOUSE_PASSWORD"), MaxExecutionTime: 30 * time.Second, MaxMemoryUsage: 4 << 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	org := idgen.New()
	limits := func(context.Context, string) (chconn.TenantLimits, error) {
		return chconn.TenantLimits{Plan: "free", MaxConcurrent: 2, MaxThreads: 2, MaxExecution: 15 * time.Second, MaxMemory: 1 << 30, Priority: 4}, nil
	}
	c := chconn.NewTenantConn(raw, orgOf, limits, quiet)

	marker := "tenant_settings_" + idgen.New()
	qctx := chconn.Settings(withOrg(ctx, org), clickhouse.Settings{"max_result_rows": 1000})
	var n uint64
	if err := c.QueryRow(qctx, fmt.Sprintf("SELECT count() FROM numbers(10) WHERE '%s' != ''", marker)).Scan(&n); err != nil || n != 10 {
		t.Fatalf("query = %d, %v", n, err)
	}
	if err := raw.Exec(ctx, "SYSTEM FLUSH LOGS"); err != nil {
		t.Fatal(err)
	}
	var quotaKey string
	var settings map[string]string
	if err := raw.QueryRow(ctx, `
		SELECT quota_key, Settings FROM system.query_log
		WHERE type = 'QueryFinish' AND query LIKE ? AND query NOT LIKE '%query_log%'
		ORDER BY event_time DESC LIMIT 1`, "%"+marker+"%").Scan(&quotaKey, &settings); err != nil {
		t.Fatalf("finding the query in query_log: %v", err)
	}
	if quotaKey != org {
		t.Errorf("quota_key = %q, want the organization %q", quotaKey, org)
	}
	for k, want := range map[string]string{
		"max_threads": "2", "max_memory_usage": "1073741824", "priority": "4", "max_execution_time": "15",
		"max_result_rows": "1000", "cancel_http_readonly_queries_on_client_close": "1",
	} {
		if settings[k] != want {
			t.Errorf("setting %s = %q, want %q (all: %v)", k, settings[k], want, settings)
		}
	}
}
