// Command costbench measures what a click costs (audit BIZ-2,
// docs/unit-economics.md): CPU per container and ClickHouse bytes per
// click, under sustained traffic through the real tracker and worker.
//
// It needs a running stack (infra/docker-compose.dev.yml or staging), a
// campaign seeded by cmd/loadtestseed, and vegeta on PATH. `make
// costbench` runs the whole thing: seed, measure, clean up.
//
//	go run ./cmd/costbench -org <orgId> -host <host> -slug <slug> -rate 1000 -duration 25m
//
// Steps:
//  1. Baseline: the containers' CPU over -baseline with no load, so the
//     stack's own idle work (air, Prometheus scrapes, the worker's timers)
//     is subtracted. Measured again after the load, and the lower of the
//     two is used: a merge or mutation still running from earlier work
//     inflates one of them (a first run's ClickHouse came out at 0).
//  2. Load: vegeta at -rate for -duration, fed a stream of clicks shaped
//     like paid social traffic (varied IPs, user agents, fbclid/ttclid,
//     utm and subs). Every target is new, so nothing compresses better
//     than real traffic would through repetition.
//  3. Drain: wait until ClickHouse holds every click that got a redirect,
//     then -settle more for the worker and ClickHouse to finish.
//  4. CPU: the containers' cgroup usage over the window, minus baseline.
//     The raw CPU and both idle rates are in report.json.
//  5. Storage: OPTIMIZE … FINAL on the event tables (merged parts are
//     what a table holds after a while), then each table's growth.
//  6. Cleanup: the organization's rows leave ClickHouse (-keep to skip).
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type options struct {
	org, host, slug, tracker string
	rate                     int
	duration, baseline       time.Duration
	settle                   time.Duration
	containers               []string
	chURL, chDB, chUser      string
	chPassword               string
	out                      string
	keep                     bool
}

func main() {
	var o options
	var containers string
	flag.StringVar(&o.org, "org", "", "organization id from loadtestseed")
	flag.StringVar(&o.host, "host", "", "tracking domain from loadtestseed")
	flag.StringVar(&o.slug, "slug", "loadtest", "tracking link slug")
	flag.StringVar(&o.tracker, "tracker", "http://localhost:8081", "tracker base URL")
	flag.IntVar(&o.rate, "rate", 1000, "clicks per second")
	flag.DurationVar(&o.duration, "duration", 25*time.Minute, "load duration")
	flag.DurationVar(&o.baseline, "baseline", 2*time.Minute, "idle window measured before the load")
	flag.DurationVar(&o.settle, "settle", time.Minute, "extra time after the queue drains")
	flag.StringVar(&containers, "containers", "infra-tracker-1,infra-worker-1,infra-clickhouse-1,infra-nats-1,infra-postgres-1,infra-redis-1", "containers whose CPU is measured")
	flag.StringVar(&o.chURL, "clickhouse", envOr("CLICKHOUSE_URL", "http://localhost:8123"), "ClickHouse HTTP URL")
	flag.StringVar(&o.chDB, "clickhouse-db", envOr("CLICKHOUSE_DATABASE", "flox"), "ClickHouse database")
	flag.StringVar(&o.chUser, "clickhouse-user", envOr("CLICKHOUSE_USER", "flox"), "ClickHouse user")
	flag.StringVar(&o.out, "out", "costbench-out", "directory for vegeta results and the report")
	flag.BoolVar(&o.keep, "keep", false, "keep the organization's rows in ClickHouse")
	flag.Parse()
	o.chPassword = os.Getenv("CLICKHOUSE_PASSWORD")
	o.containers = strings.Split(containers, ",")
	if o.org == "" || o.host == "" {
		fmt.Fprintln(os.Stderr, "-org and -host are required (from: go run ./cmd/loadtestseed seed)")
		os.Exit(2)
	}
	if err := run(context.Background(), o); err != nil {
		fmt.Fprintln(os.Stderr, "costbench:", err)
		os.Exit(1)
	}
}

// Report is what one run measured; report.json holds it.
type Report struct {
	StartedAt   time.Time      `json:"startedAt"`
	Rate        int            `json:"rate"`
	Duration    string         `json:"duration"`
	Sent        int64          `json:"sent"`
	Redirected  int64          `json:"redirected"`
	Stored      int64          `json:"stored"`
	LatencyP50  string         `json:"latencyP50"`
	LatencyP99  string         `json:"latencyP99"`
	WindowSec   float64        `json:"windowSec"`
	CPU         []ContainerCPU `json:"cpu"`
	Tables      []TableGrowth  `json:"tables"`
	BytesPerClk float64        `json:"compressedBytesPerClick"`
	// Columns: the costliest columns of the tables that grew. Those with a
	// column TTL (ip, user_agent, referrer) are emptied after 90 days;
	// the rows themselves stay.
	Columns []ColumnGrowth     `json:"columns"`
	Notes   map[string]float64 `json:"notes,omitempty"`
}

type ContainerCPU struct {
	Name string `json:"name"`
	// RawSec: CPU seconds over the window; IdleBefore/IdleAfter: idle CPU
	// seconds per wall second around it.
	RawSec     float64 `json:"rawSec"`
	IdleBefore float64 `json:"idleBefore"`
	IdleAfter  float64 `json:"idleAfter"`
	// CPUSecPerMillion: CPU seconds per million clicks, baseline removed.
	CPUSecPerMillion float64 `json:"cpuSecPerMillionClicks"`
	// CoresAt1000: cores busy at 1,000 clicks/s.
	CoresAt1000 float64 `json:"coresAt1000rps"`
	PeakMemMB   float64 `json:"peakMemMB"`
}

type ColumnGrowth struct {
	Table         string  `json:"table"`
	Column        string  `json:"column"`
	TTL           bool    `json:"ttl"`
	BytesPerClick float64 `json:"compressedBytesPerClick"`
}

type TableGrowth struct {
	Table             string  `json:"table"`
	OrgRows           int64   `json:"orgRows"`
	CompressedBytes   int64   `json:"compressedBytes"`
	UncompressedBytes int64   `json:"uncompressedBytes"`
	BytesPerClick     float64 `json:"compressedBytesPerClick"`
}

func run(ctx context.Context, o options) error {
	if err := os.MkdirAll(o.out, 0o755); err != nil {
		return err
	}
	ch := &clickhouse{url: o.chURL, db: o.chDB, user: o.chUser, password: o.chPassword}
	cg, err := cgroupsOf(o.containers)
	if err != nil {
		return err
	}
	rep := Report{StartedAt: time.Now().UTC(), Rate: o.rate, Duration: o.duration.String(), Notes: map[string]float64{}}

	logf("baseline: %s idle", o.baseline)
	idleBefore := cg.idle(o.baseline)

	partsBefore, err := ch.parts(ctx)
	if err != nil {
		return err
	}
	colsBefore, err := ch.columns(ctx)
	if err != nil {
		return err
	}
	u0, w0 := cg.usage(), time.Now()

	logf("load: %d/s for %s", o.rate, o.duration)
	peak := cg.sampleMemory(ctx)
	results := filepath.Join(o.out, "results.bin")
	if err := attack(ctx, o, results); err != nil {
		return err
	}
	metrics, err := vegetaReport(results)
	if err != nil {
		return err
	}
	rep.Sent, rep.Redirected = metrics.Requests, metrics.StatusCodes["302"]
	rep.LatencyP50, rep.LatencyP99 = time.Duration(metrics.Latencies.P50).String(), time.Duration(metrics.Latencies.P99).String()

	logf("drain: waiting for %d clicks in ClickHouse", rep.Redirected)
	rep.Stored, err = ch.waitFor(ctx, o.org, rep.Redirected, 5*time.Minute)
	if err != nil {
		return err
	}
	time.Sleep(o.settle)
	u1, w1 := cg.usage(), time.Now()
	peakMem := peak()
	rep.WindowSec = w1.Sub(w0).Seconds()
	logf("baseline: %s idle again", o.baseline)
	idleAfter := cg.idle(o.baseline)

	clicks := float64(rep.Stored)
	for _, n := range o.containers {
		raw := float64(u1[n]-u0[n]) / 1e6
		busy := raw - min(idleBefore[n], idleAfter[n])*rep.WindowSec
		if busy < 0 {
			busy = 0
		}
		perMillion := busy / clicks * 1e6
		rep.CPU = append(rep.CPU, ContainerCPU{Name: n, RawSec: round(raw, 1),
			IdleBefore: round(idleBefore[n], 4), IdleAfter: round(idleAfter[n], 4), CPUSecPerMillion: round(perMillion, 1),
			CoresAt1000: round(perMillion/1e6*1000, 3), PeakMemMB: round(peakMem[n]/(1<<20), 0)})
	}

	logf("storage: OPTIMIZE FINAL, then table growth")
	if err := ch.optimize(ctx, partsBefore); err != nil {
		return err
	}
	partsAfter, err := ch.parts(ctx)
	if err != nil {
		return err
	}
	var total int64
	for table, after := range partsAfter {
		before := partsBefore[table]
		rows, err := ch.orgRows(ctx, table, o.org)
		if err != nil {
			return err
		}
		g := TableGrowth{Table: table, OrgRows: rows, CompressedBytes: after.compressed - before.compressed,
			UncompressedBytes: after.uncompressed - before.uncompressed}
		if g.CompressedBytes <= 0 && rows == 0 {
			continue
		}
		g.BytesPerClick = round(float64(g.CompressedBytes)/clicks, 1)
		total += g.CompressedBytes
		rep.Tables = append(rep.Tables, g)
	}
	sort.Slice(rep.Tables, func(i, j int) bool { return rep.Tables[i].CompressedBytes > rep.Tables[j].CompressedBytes })
	colsAfter, err := ch.columns(ctx)
	if err != nil {
		return err
	}
	for k, after := range colsAfter {
		if grown := after.bytes - colsBefore[k].bytes; float64(grown)/clicks >= 1 {
			rep.Columns = append(rep.Columns, ColumnGrowth{Table: k.table, Column: k.column, TTL: after.ttl,
				BytesPerClick: round(float64(grown)/clicks, 1)})
		}
	}
	sort.Slice(rep.Columns, func(i, j int) bool { return rep.Columns[i].BytesPerClick > rep.Columns[j].BytesPerClick })
	rep.BytesPerClk = round(float64(total)/clicks, 1)

	if !o.keep {
		logf("cleanup: deleting the organization's rows from ClickHouse")
		if err := ch.deleteOrg(ctx, o.org); err != nil {
			return err
		}
	}

	buf, _ := json.MarshalIndent(rep, "", "  ")
	if err := os.WriteFile(filepath.Join(o.out, "report.json"), buf, 0o644); err != nil {
		return err
	}
	printMarkdown(os.Stdout, rep)
	return nil
}

// attack runs vegeta with a lazily generated stream of targets.
func attack(ctx context.Context, o options, results string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, "vegeta", "attack", "-lazy", "-format=json",
		"-rate="+strconv.Itoa(o.rate)+"/1s", "-duration="+o.duration.String(), "-redirects=-1",
		"-timeout=10s", "-output="+results)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting vegeta (is it on PATH?): %w", err)
	}
	go func() {
		defer stdin.Close()
		gen := newTraffic(rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 7)))
		w := bufio.NewWriterSize(stdin, 1<<16)
		enc := json.NewEncoder(w)
		target := strings.TrimRight(o.tracker, "/") + "/t/" + url.PathEscape(o.slug)
		for ctx.Err() == nil {
			if err := enc.Encode(gen.next(target, o.host)); err != nil {
				return // vegeta finished and closed its stdin
			}
		}
	}()
	return cmd.Wait()
}

type vegetaMetrics struct {
	Requests    int64            `json:"requests"`
	StatusCodes map[string]int64 `json:"status_codes"`
	Latencies   struct {
		P50 int64 `json:"50th"`
		P99 int64 `json:"99th"`
	} `json:"latencies"`
}

func vegetaReport(results string) (vegetaMetrics, error) {
	var m vegetaMetrics
	out, err := exec.Command("vegeta", "report", "-type=json", results).Output()
	if err != nil {
		return m, fmt.Errorf("vegeta report: %w", err)
	}
	return m, json.Unmarshal(out, &m)
}

// ---- cgroups (v2): per-container CPU and memory ----

type cgroups map[string]string // container name → cgroup directory

func cgroupsOf(names []string) (cgroups, error) {
	cg := cgroups{}
	for _, n := range names {
		id, err := exec.Command("docker", "inspect", "-f", "{{.Id}}", n).Output()
		if err != nil {
			return nil, fmt.Errorf("docker inspect %s: %w", n, err)
		}
		dir := "/sys/fs/cgroup/system.slice/docker-" + strings.TrimSpace(string(id)) + ".scope"
		if _, err := os.Stat(filepath.Join(dir, "cpu.stat")); err != nil {
			return nil, fmt.Errorf("%s: no cgroup v2 cpu.stat at %s", n, dir)
		}
		cg[n] = dir
	}
	return cg, nil
}

// usage: each container's cumulative CPU time in µs.
func (cg cgroups) usage() map[string]int64 {
	out := map[string]int64{}
	for n, dir := range cg {
		b, _ := os.ReadFile(filepath.Join(dir, "cpu.stat"))
		for _, line := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(line, "usage_usec "); ok {
				out[n], _ = strconv.ParseInt(v, 10, 64)
			}
		}
	}
	return out
}

// idle: each container's CPU seconds per wall second over d.
func (cg cgroups) idle(d time.Duration) map[string]float64 {
	u0, t0 := cg.usage(), time.Now()
	time.Sleep(d)
	u1, t1 := cg.usage(), time.Now()
	out := map[string]float64{}
	for n := range u0 {
		out[n] = float64(u1[n]-u0[n]) / 1e6 / t1.Sub(t0).Seconds()
	}
	return out
}

// sampleMemory samples anonymous memory every 2 s until the returned func
// is called, which reports each container's peak in bytes.
func (cg cgroups) sampleMemory(ctx context.Context) func() map[string]float64 {
	ctx, cancel := context.WithCancel(ctx)
	var mu sync.Mutex
	peak := map[string]float64{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			for n, dir := range cg {
				// anon: the processes' own memory, without the page cache
				// (which memory.current counts, the dev images' build
				// cache included).
				var v float64
				b, _ := os.ReadFile(filepath.Join(dir, "memory.stat"))
				for _, line := range strings.Split(string(b), "\n") {
					if s, ok := strings.CutPrefix(line, "anon "); ok {
						v, _ = strconv.ParseFloat(s, 64)
					}
				}
				mu.Lock()
				peak[n] = max(peak[n], v)
				mu.Unlock()
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	return func() map[string]float64 {
		cancel()
		<-done
		return peak
	}
}

// ---- ClickHouse over HTTP ----

type clickhouse struct{ url, db, user, password string }

func (c *clickhouse) query(ctx context.Context, q string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/?database="+url.QueryEscape(c.db), strings.NewReader(q))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-ClickHouse-User", c.user)
	req.Header.Set("X-ClickHouse-Key", c.password)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("clickhouse: %s: %s", strings.TrimSpace(string(body)), q)
	}
	return body, nil
}

type partSize struct{ compressed, uncompressed int64 }

func (c *clickhouse) parts(ctx context.Context) (map[string]partSize, error) {
	body, err := c.query(ctx, `SELECT table, sum(data_compressed_bytes), sum(data_uncompressed_bytes)
		FROM system.parts WHERE active AND database = currentDatabase() GROUP BY table FORMAT TSV`)
	if err != nil {
		return nil, err
	}
	out := map[string]partSize{}
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			continue
		}
		cmp, _ := strconv.ParseInt(f[1], 10, 64)
		unc, _ := strconv.ParseInt(f[2], 10, 64)
		out[f[0]] = partSize{cmp, unc}
	}
	return out, nil
}

type columnKey struct{ table, column string }

type columnSize struct {
	bytes int64
	ttl   bool
}

// columns: every column's compressed size, and whether it has a TTL of
// its own (in the table's DDL as "`col` Type TTL …").
func (c *clickhouse) columns(ctx context.Context) (map[columnKey]columnSize, error) {
	body, err := c.query(ctx, `SELECT col.table, col.name, col.data_compressed_bytes,
		position(t.create_table_query, concat('`+"`"+`', col.name, '`+"`"+` ', col.type, ' TTL ')) > 0
		FROM system.columns col JOIN system.tables t ON t.database = col.database AND t.name = col.table
		WHERE col.database = currentDatabase() FORMAT TSV`)
	if err != nil {
		return nil, err
	}
	out := map[columnKey]columnSize{}
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 4 {
			continue
		}
		n, _ := strconv.ParseInt(f[2], 10, 64)
		out[columnKey{f[0], f[1]}] = columnSize{bytes: n, ttl: f[3] == "1"}
	}
	return out, nil
}

// orgTables: the MergeTree tables with an organization_id column.
func (c *clickhouse) orgTables(ctx context.Context) ([]string, error) {
	body, err := c.query(ctx, `SELECT t.name FROM system.tables t JOIN system.columns col
		ON col.database = t.database AND col.table = t.name
		WHERE t.database = currentDatabase() AND t.engine LIKE '%MergeTree' AND col.name = 'organization_id' FORMAT TSV`)
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(body)), nil
}

func (c *clickhouse) orgRows(ctx context.Context, table, org string) (int64, error) {
	tables, err := c.orgTables(ctx)
	if err != nil {
		return 0, err
	}
	for _, t := range tables {
		if t == table {
			body, err := c.query(ctx, fmt.Sprintf("SELECT count() FROM `%s` WHERE organization_id = '%s' FORMAT TSV", t, escape(org)))
			if err != nil {
				return 0, err
			}
			return strconv.ParseInt(strings.TrimSpace(string(body)), 10, 64)
		}
	}
	return 0, nil
}

func (c *clickhouse) waitFor(ctx context.Context, org string, want int64, timeout time.Duration) (int64, error) {
	deadline := time.Now().Add(timeout)
	var last int64 = -1
	stable := 0
	for {
		n, err := c.orgRows(ctx, "click_events", org)
		if err != nil {
			return 0, err
		}
		if n >= want {
			return n, nil
		}
		if n == last {
			stable++
		} else {
			stable = 0
		}
		// No growth for 30 s: report what arrived instead of waiting on.
		if stable >= 15 || time.Now().After(deadline) {
			logf("drain: %d of %d arrived, no longer growing", n, want)
			return n, nil
		}
		last = n
		time.Sleep(2 * time.Second)
	}
}

// optimize merges every table that grew during the run.
func (c *clickhouse) optimize(ctx context.Context, before map[string]partSize) error {
	now, err := c.parts(ctx)
	if err != nil {
		return err
	}
	for table, p := range now {
		if p == before[table] {
			continue
		}
		if _, err := c.query(ctx, fmt.Sprintf("OPTIMIZE TABLE `%s` FINAL", table)); err != nil {
			return err
		}
	}
	return nil
}

func (c *clickhouse) deleteOrg(ctx context.Context, org string) error {
	tables, err := c.orgTables(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, t := range tables {
		_, err := c.query(ctx, fmt.Sprintf("ALTER TABLE `%s` DELETE WHERE organization_id = '%s' SETTINGS mutations_sync = 1", t, escape(org)))
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func escape(s string) string { return strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) }

// ---- output ----

func printMarkdown(w io.Writer, r Report) {
	var b bytes.Buffer
	fmt.Fprintf(&b, "\n## costbench %s — %d/s for %s\n\n", r.StartedAt.Format(time.DateTime), r.Rate, r.Duration)
	fmt.Fprintf(&b, "Sent %d, redirected %d, stored %d. Latency p50 %s, p99 %s. Window %.0f s.\n\n",
		r.Sent, r.Redirected, r.Stored, r.LatencyP50, r.LatencyP99, r.WindowSec)
	fmt.Fprintln(&b, "| Container | CPU s / 1M clicks | Cores at 1,000/s | Peak memory, MB |")
	fmt.Fprintln(&b, "|---|---:|---:|---:|")
	var sumSec, sumCores float64
	for _, c := range r.CPU {
		fmt.Fprintf(&b, "| %s | %.1f | %.3f | %.0f |\n", c.Name, c.CPUSecPerMillion, c.CoresAt1000, c.PeakMemMB)
		sumSec += c.CPUSecPerMillion
		sumCores += c.CoresAt1000
	}
	fmt.Fprintf(&b, "| **total** | **%.1f** | **%.3f** | |\n\n", sumSec, sumCores)
	fmt.Fprintln(&b, "| ClickHouse table | Rows of the org | Compressed, MB | Bytes / click |")
	fmt.Fprintln(&b, "|---|---:|---:|---:|")
	for _, t := range r.Tables {
		fmt.Fprintf(&b, "| %s | %d | %.1f | %.1f |\n", t.Table, t.OrgRows, float64(t.CompressedBytes)/(1<<20), t.BytesPerClick)
	}
	fmt.Fprintf(&b, "| **total** | | | **%.1f** |\n\n", r.BytesPerClk)
	fmt.Fprintln(&b, "| Column | 90-day TTL | Bytes / click |")
	fmt.Fprintln(&b, "|---|---|---:|")
	for i, c := range r.Columns {
		if i == 15 {
			break
		}
		ttl := ""
		if c.TTL {
			ttl = "yes"
		}
		fmt.Fprintf(&b, "| %s.%s | %s | %.1f |\n", c.Table, c.Column, ttl, c.BytesPerClick)
	}
	_, _ = w.Write(b.Bytes())
}

func round(v float64, digits int) float64 {
	p := 1.0
	for range digits {
		p *= 10
	}
	return float64(int64(v*p+0.5)) / p
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, time.Now().Format("15:04:05")+" "+format+"\n", args...)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
