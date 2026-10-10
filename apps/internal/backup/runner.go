package backup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const drillsDir = "drills"

// refreshInterval: how often the metrics are re-read from the bucket.
const refreshInterval = 5 * time.Minute

// Check is one thing a drill verified.
type Check struct {
	Store  string `json:"store"`
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

func pass(store, name, detail string) Check {
	return Check{Store: store, Name: name, OK: true, Detail: detail}
}

func fail(store, name string, err error) Check {
	return Check{Store: store, Name: name, Detail: err.Error()}
}

// Report is one drill, kept as drills/<name>.json.
type Report struct {
	Name       string    `json:"name"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	OK         bool      `json:"ok"`
	Checks     []Check   `json:"checks"`
}

// Runner runs the backups and drills on schedule and keeps the metrics.
type Runner struct {
	cfg        Config
	bucket     *bucket
	postgres   *Postgres // nil: BACKUP_POSTGRES=off
	clickhouse *ClickHouse
	objects    *Objects // nil: no files bucket configured
	logger     *slog.Logger
	metrics    *metrics
}

func NewRunner(cfg Config, pg *Postgres, ch *ClickHouse, objects *Objects, logger *slog.Logger) (*Runner, error) {
	b, err := newBucket(cfg.Target)
	if err != nil {
		return nil, err
	}
	m := newMetrics()
	for store, on := range map[string]bool{StorePostgres: pg != nil, StoreClickHouse: true, StoreObjects: objects != nil} {
		if on {
			m.enabled.WithLabelValues(store).Set(1)
		} else {
			m.enabled.WithLabelValues(store).Set(0)
		}
	}
	return &Runner{cfg: cfg, bucket: b, postgres: pg, clickhouse: ch, objects: objects, logger: logger, metrics: m}, nil
}

// Registry is what /metrics serves.
func (r *Runner) Registry() *prometheus.Registry { return r.metrics.registry }

// Prepare creates the bucket if needed. WAL archiving fails until it
// exists, so the sidecar does this first thing.
func (r *Runner) Prepare(ctx context.Context) error { return r.bucket.ensure(ctx) }

// Serve runs the schedule until ctx is done.
func (r *Runner) Serve(ctx context.Context) {
	r.Refresh(ctx)
	refresh := time.NewTicker(refreshInterval)
	defer refresh.Stop()
	for {
		next := NextRun(time.Now(), r.cfg.At)
		r.logger.Info("next backup", "at", next)
		timer := time.NewTimer(time.Until(next))
	wait:
		for {
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-refresh.C:
				r.Refresh(ctx)
			case <-timer.C:
				break wait
			}
		}
		_ = r.BackupAll(ctx)
		if r.cfg.DrillDay != 0 && time.Now().UTC().Day() == r.cfg.DrillDay {
			_, _ = r.Drill(ctx)
		}
		r.Refresh(ctx)
	}
}

// BackupAll backs up every store. One store failing does not stop the
// others; the error joins every failure.
func (r *Runner) BackupAll(ctx context.Context) error {
	var errs []error
	run := func(store string, f func() error) {
		started := time.Now()
		r.logger.Info("backup starting", "store", store)
		if err := f(); err != nil {
			r.metrics.failures.WithLabelValues(store).Inc()
			r.logger.Error("backup failed", "store", store, "error", err)
			errs = append(errs, fmt.Errorf("%s: %w", store, err))
			return
		}
		r.metrics.duration.WithLabelValues(store).Set(time.Since(started).Seconds())
		r.logger.Info("backup done", "store", store, "took", time.Since(started).Round(time.Second).String())
	}
	if r.postgres != nil {
		run(StorePostgres, func() error {
			if err := r.postgres.Backup(ctx); err != nil {
				return err
			}
			return r.postgres.Prune(ctx, r.cfg.KeepPostgres)
		})
	}
	run(StoreClickHouse, func() error {
		if _, err := r.clickhouse.Backup(ctx, time.Now()); err != nil {
			return err
		}
		gone, err := r.clickhouse.Prune(ctx, r.cfg.KeepClickHouse)
		if len(gone) > 0 {
			r.logger.Info("old clickhouse backups removed", "names", gone)
		}
		return err
	})
	if r.objects != nil {
		run(StoreObjects, func() error {
			m, err := r.objects.Mirror(ctx, time.Now())
			if err == nil {
				r.logger.Info("files mirrored", "files", m.Objects, "copied", m.Copied)
			}
			return err
		})
	}
	return errors.Join(errs...)
}

// Drill restores every store into a scratch copy, checks it, and keeps the
// report.
func (r *Runner) Drill(ctx context.Context) (Report, error) {
	rep := Report{StartedAt: time.Now().UTC()}
	rep.Name = name(rep.StartedAt)
	r.logger.Info("restore drill starting")
	if r.postgres != nil {
		rep.Checks = append(rep.Checks, r.postgres.Drill(ctx)...)
	}
	rep.Checks = append(rep.Checks, r.clickhouse.Drill(ctx)...)
	if r.objects != nil {
		rep.Checks = append(rep.Checks, r.objects.Drill(ctx)...)
	}
	rep.FinishedAt = time.Now().UTC()
	rep.OK = true
	for _, c := range rep.Checks {
		if !c.OK {
			rep.OK = false
			r.logger.Error("restore drill check failed", "store", c.Store, "check", c.Name, "detail", c.Detail)
		}
	}
	r.logger.Info("restore drill done", "ok", rep.OK, "took", rep.FinishedAt.Sub(rep.StartedAt).Round(time.Second).String())
	if err := r.bucket.putJSON(context.WithoutCancel(ctx), drillsDir+"/"+rep.Name+".json", rep); err != nil {
		return rep, err
	}
	r.Refresh(ctx)
	return rep, nil
}

// Status is what the bucket holds, for `flox-backup list` and the metrics.
type Status struct {
	Postgres       []PGBackup
	ClickHouse     []string
	LastClickHouse *ClickHouseManifest
	LastObjects    *ObjectsManifest
	LastDrill      *Report
	LastGoodDrill  *Report
	Archiver       *Archiver
}

// Status reads the backups back. Parts that fail to read are left nil and
// joined into the error.
func (r *Runner) Status(ctx context.Context) (Status, error) {
	var s Status
	var errs []error
	keep := func(err error) {
		if err != nil && !errors.Is(err, errNone) {
			errs = append(errs, err)
		}
	}
	var err error
	if r.postgres != nil {
		s.Postgres, err = r.postgres.List(ctx)
		keep(err)
		if a, err := r.postgres.Archiver(ctx); err == nil {
			s.Archiver = &a
		} else {
			keep(err)
		}
	}
	s.ClickHouse, err = r.clickhouse.List(ctx)
	keep(err)
	if m, err := r.clickhouse.Latest(ctx); err == nil {
		s.LastClickHouse = &m
	} else {
		keep(err)
	}
	if r.objects != nil {
		if m, err := r.objects.Latest(ctx); err == nil {
			s.LastObjects = &m
		} else {
			keep(err)
		}
	}
	drills, err := r.bucket.names(ctx, drillsDir)
	keep(err)
	for i := len(drills) - 1; i >= 0 && s.LastGoodDrill == nil; i-- {
		var rep Report
		if err := r.bucket.getJSON(ctx, drillsDir+"/"+drills[i]+".json", &rep); err != nil {
			keep(err)
			break
		}
		if s.LastDrill == nil {
			s.LastDrill = &rep
		}
		if rep.OK {
			s.LastGoodDrill = &rep
		}
	}
	return s, errors.Join(errs...)
}

// Refresh sets the metrics from the bucket.
func (r *Runner) Refresh(ctx context.Context) {
	s, err := r.Status(ctx)
	if err != nil {
		r.metrics.refreshErrors.Inc()
		r.logger.Warn("reading backup status", "error", err)
	}
	if n := len(s.Postgres); n > 0 {
		last := s.Postgres[n-1]
		r.metrics.lastSuccess.WithLabelValues(StorePostgres).Set(float64(last.FinishTime.Unix()))
		r.metrics.size.WithLabelValues(StorePostgres).Set(float64(last.CompressedSize))
		r.metrics.kept.WithLabelValues(StorePostgres).Set(float64(n))
	}
	if s.LastClickHouse != nil {
		r.metrics.lastSuccess.WithLabelValues(StoreClickHouse).Set(float64(s.LastClickHouse.FinishedAt.Unix()))
		r.metrics.kept.WithLabelValues(StoreClickHouse).Set(float64(len(s.ClickHouse)))
	}
	if s.LastObjects != nil {
		r.metrics.lastSuccess.WithLabelValues(StoreObjects).Set(float64(s.LastObjects.FinishedAt.Unix()))
		r.metrics.size.WithLabelValues(StoreObjects).Set(float64(s.LastObjects.Bytes))
	}
	if s.LastGoodDrill != nil {
		r.metrics.drill.WithLabelValues("success").Set(float64(s.LastGoodDrill.FinishedAt.Unix()))
	}
	if s.LastDrill != nil && !s.LastDrill.OK {
		r.metrics.drill.WithLabelValues("failure").Set(float64(s.LastDrill.FinishedAt.Unix()))
	}
	if a := s.Archiver; a != nil {
		failing := 0.0
		if a.Failing() {
			failing = 1
		}
		r.metrics.walFailing.Set(failing)
		if !a.LastArchived.IsZero() {
			r.metrics.walLastArchived.Set(float64(a.LastArchived.Unix()))
		}
	}
}

type metrics struct {
	registry        *prometheus.Registry
	enabled         *prometheus.GaugeVec
	lastSuccess     *prometheus.GaugeVec
	duration        *prometheus.GaugeVec
	size            *prometheus.GaugeVec
	kept            *prometheus.GaugeVec
	failures        *prometheus.CounterVec
	drill           *prometheus.GaugeVec
	walFailing      prometheus.Gauge
	walLastArchived prometheus.Gauge
	refreshErrors   prometheus.Counter
}

// newMetrics: the sidecar's own registry, so /metrics carries only these
// (internal/metrics would bring every other binary's series at zero).
func newMetrics() *metrics {
	const ns = "flox_backup"
	m := &metrics{
		registry: prometheus.NewRegistry(),
		enabled: prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "store_enabled",
			Help: "1 for each store this sidecar backs up (postgres is 0 with BACKUP_POSTGRES=off)."}, []string{"store"}),
		lastSuccess: prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "last_success_timestamp_seconds",
			Help: "When the newest backup in the bucket finished, by store."}, []string{"store"}),
		duration: prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "last_duration_seconds",
			Help: "How long this sidecar's last successful backup took, by store."}, []string{"store"}),
		size: prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "size_bytes",
			Help: "Size of the newest backup (postgres: compressed base backup; objects: files mirrored)."}, []string{"store"}),
		kept: prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "kept",
			Help: "Backups kept in the bucket, by store."}, []string{"store"}),
		failures: prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: ns, Name: "failures_total",
			Help: "Backups that failed, by store."}, []string{"store"}),
		drill: prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "drill_last_timestamp_seconds",
			Help: "When the last passed (result=success) and the last failed (result=failure) restore drill finished."}, []string{"result"}),
		walFailing: prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "wal_archive_failing",
			Help: "1 when Postgres's most recent WAL archive attempt failed (pg_stat_archiver)."}),
		walLastArchived: prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "wal_last_archived_timestamp_seconds",
			Help: "When Postgres last archived a WAL segment."}),
		refreshErrors: prometheus.NewCounter(prometheus.CounterOpts{Namespace: ns, Name: "status_errors_total",
			Help: "Failures reading the backup status back from the bucket or Postgres."}),
	}
	m.registry.MustRegister(m.enabled, m.lastSuccess, m.duration, m.size, m.kept, m.failures, m.drill, m.walFailing, m.walLastArchived, m.refreshErrors)
	return m
}
