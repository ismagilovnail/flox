package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/routingcache"
	"github.com/ismagilovnail/flox/apps/internal/routingstore"
)

// latencyProxy forwards TCP to Postgres and holds every reply for delay —
// a slow network or an overloaded database, as the client sees it.
type latencyProxy struct {
	ln    net.Listener
	delay atomic.Int64 // time.Duration
	mu    sync.Mutex
	conns []net.Conn
}

func startLatencyProxy(t *testing.T, upstream string) *latencyProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &latencyProxy{ln: ln}
	go func() {
		for {
			client, err := ln.Accept()
			if err != nil {
				return
			}
			server, err := net.Dial("tcp", upstream)
			if err != nil {
				_ = client.Close()
				continue
			}
			p.mu.Lock()
			p.conns = append(p.conns, client, server)
			p.mu.Unlock()
			go func() { _, _ = io.Copy(server, client) }()
			go p.slowCopy(client, server)
		}
	}()
	return p
}

func (p *latencyProxy) slowCopy(dst, src net.Conn) {
	buf := make([]byte, 32<<10)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			time.Sleep(time.Duration(p.delay.Load()))
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			_ = dst.Close()
			return
		}
	}
}

func (p *latencyProxy) Close() {
	_ = p.ln.Close()
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.conns {
		_ = c.Close()
	}
}

// Tech audit 2026-10-06, §16 (RL1): Postgres answering after 5 s. A link
// the tracker has served keeps redirecting from memory within the latency
// budget (p95 ≤ 50 ms); one it never saw fails at LoadTimeout, not after
// the database's 5 s.
func TestRedirectsStayFastWhilePostgresIsSlow(t *testing.T) {
	direct := testPool(t)
	seen := seedFlowFixture(t, direct, nil, nil)
	unseen := seedFlowFixture(t, direct, nil, nil)

	cfg := direct.Config().Copy()
	proxy := startLatencyProxy(t, net.JoinHostPort(cfg.ConnConfig.Host, strconv.Itoa(int(cfg.ConnConfig.Port))))
	cfg.ConnConfig.Host = "127.0.0.1"
	cfg.ConnConfig.Port = uint16(proxy.ln.Addr().(*net.TCPAddr).Port)
	slow, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Cleanups run last-first: the proxy closes before the pool, so the
	// pool is not left waiting on held replies.
	t.Cleanup(slow.Close)
	t.Cleanup(proxy.Close)

	const loadTimeout = 300 * time.Millisecond
	h, _ := cachedHandler(t, routingstore.New(slow), routingcache.Config{TTL: time.Millisecond, LoadTimeout: loadTimeout})
	assertRedirect(t, get(h, seen.host, "/t/go"), landingTestOffer)

	proxy.delay.Store(int64(5 * time.Second))
	time.Sleep(5 * time.Millisecond) // the entry is stale from here on

	const clicks = 200
	took := make([]time.Duration, 0, clicks)
	for i := 0; i < clicks; i++ {
		start := time.Now()
		rec := get(h, seen.host, "/t/go")
		took = append(took, time.Since(start))
		assertRedirect(t, rec, landingTestOffer)
	}
	sort.Slice(took, func(i, j int) bool { return took[i] < took[j] })
	if p95 := took[clicks*95/100]; p95 > 50*time.Millisecond {
		t.Fatalf("redirect p95 = %v with Postgres answering after 5s, want ≤ 50ms", p95)
	}

	start := time.Now()
	rec := get(h, unseen.host, "/t/go")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("never-seen link with Postgres slow: %d, want 502", rec.Code)
	}
	elapsed := time.Since(start)
	t.Logf("p50 %v, p95 %v, max %v; never-seen link failed after %v", took[clicks/2], took[clicks*95/100], took[clicks-1], elapsed)
	if elapsed < loadTimeout || elapsed > loadTimeout+time.Second {
		t.Fatalf("never-seen link failed after %v, want about LoadTimeout (%v): sooner means the delay did not apply, later that the timeout did not", elapsed, loadTimeout)
	}
}
