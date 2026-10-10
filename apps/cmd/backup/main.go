// Command flox-backup backs up Postgres, ClickHouse and the files bucket
// and runs the restore drill (internal/backup, docs/backup.md).
//
//	flox-backup serve   nightly backups (BACKUP_AT), monthly drill (BACKUP_DRILL_DAY), /metrics
//	flox-backup now     back up every store once
//	flox-backup drill   restore every store into a scratch copy and check it
//	flox-backup list    what the bucket holds
//
// It runs in the backup sidecar (apps/cmd/backup/Dockerfile): it needs
// wal-g, pg_ctl, and the live Postgres data directory mounted read-only.
// With BACKUP_POSTGRES=off (a managed Postgres, its provider's PITR) it
// backs up ClickHouse and the files bucket only, and needs none of them
// (target backup-remote, infra/k8s/components/backup).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/ismagilovnail/flox/apps/internal/backup"
	"github.com/ismagilovnail/flox/apps/internal/chconn"
)

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	if err := run(cmd); err != nil {
		fmt.Fprintln(os.Stderr, "flox-backup:", err)
		os.Exit(1)
	}
}

func run(cmd string) error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := backup.ConfigFromEnv()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var pg *backup.Postgres
	if cfg.Postgres {
		if pg, err = backup.NewPostgres(cfg.DatabaseURL, cfg.PGData, cfg.DrillDir, cfg.Target); err != nil {
			return err
		}
	}
	conn, err := chconn.NewConn(ctx, cfg.ClickHouse)
	if err != nil {
		return err
	}
	defer conn.Close()
	ch, err := backup.NewClickHouse(conn, cfg.ClickHouse.Database, cfg.Target, cfg.ClickHouseS3Endpoint)
	if err != nil {
		return err
	}
	var objects *backup.Objects
	if cfg.Objects.Bucket != "" {
		if objects, err = backup.NewObjects(cfg.Objects, cfg.Target); err != nil {
			return err
		}
	}
	r, err := backup.NewRunner(cfg, pg, ch, objects, logger)
	if err != nil {
		return err
	}
	if err := r.Prepare(ctx); err != nil {
		return err
	}

	switch cmd {
	case "serve":
		return serve(ctx, cfg, r, logger)
	case "now":
		return r.BackupAll(ctx)
	case "drill":
		rep, err := r.Drill(ctx)
		printReport(rep)
		if err != nil {
			return err
		}
		if !rep.OK {
			return errors.New("restore drill failed")
		}
		return nil
	case "list":
		s, err := r.Status(ctx)
		printStatus(s, cfg.Postgres)
		return err
	default:
		return fmt.Errorf("unknown command %q (serve, now, drill, list)", cmd)
	}
}

func serve(ctx context.Context, cfg backup.Config, r *backup.Runner, logger *slog.Logger) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.Handle("/metrics", promhttp.HandlerFor(r.Registry(), promhttp.HandlerOpts{}))
	// Health and metrics only (tech audit RL5).
	srv := &http.Server{
		Addr: cfg.HTTPAddr, Handler: mux,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: time.Minute,
	}
	go func() {
		logger.Info("backup sidecar listening", "addr", cfg.HTTPAddr, "daily_at_utc", fmtClock(cfg.At), "drill_day", cfg.DrillDay)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server", "error", err)
		}
	}()
	r.Serve(ctx)
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}

func fmtClock(d time.Duration) string {
	return fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

func printReport(rep backup.Report) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, c := range rep.Checks {
		mark := "✓"
		if !c.OK {
			mark = "✗"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", mark, c.Store, c.Name, c.Detail)
	}
	_ = w.Flush()
	verdict := "PASSED"
	if !rep.OK {
		verdict = "FAILED"
	}
	fmt.Printf("\nrestore drill %s in %s (report drills/%s.json)\n", verdict, rep.FinishedAt.Sub(rep.StartedAt).Round(time.Second), rep.Name)
}

func printStatus(s backup.Status, withPostgres bool) {
	if withPostgres {
		fmt.Println("postgres base backups (PITR from the oldest to the last archived WAL):")
		for _, b := range s.Postgres {
			fmt.Printf("  %s  %s  %s\n", b.FinishTime.UTC().Format(time.DateTime), b.Name, mb(b.CompressedSize))
		}
		if len(s.Postgres) == 0 {
			fmt.Println("  none")
		}
		if a := s.Archiver; a != nil {
			state := "ok"
			if a.Failing() {
				state = "FAILING since " + a.LastFailed.UTC().Format(time.DateTime)
			}
			fmt.Printf("  wal archiving: %s, last archived %s at %s\n", state, orNone(a.LastArchivedWAL), stamp(a.LastArchived))
		}
	} else {
		fmt.Println("postgres: not backed up here (BACKUP_POSTGRES=off, the provider's PITR)")
	}
	fmt.Println("clickhouse backups:")
	fmt.Printf("  %s\n", orNone(strings.Join(s.ClickHouse, ", ")))
	if s.LastObjects != nil {
		fmt.Printf("files mirror: %d files, %s, at %s\n", s.LastObjects.Objects, mb(s.LastObjects.Bytes), stamp(s.LastObjects.FinishedAt))
	}
	switch {
	case s.LastDrill == nil:
		fmt.Println("restore drill: never run")
	case s.LastDrill.OK:
		fmt.Printf("restore drill: passed at %s\n", stamp(s.LastDrill.FinishedAt))
	default:
		fmt.Printf("restore drill: FAILED at %s (drills/%s.json)\n", stamp(s.LastDrill.FinishedAt), s.LastDrill.Name)
	}
}

func mb(n int64) string { return fmt.Sprintf("%.1f MB", float64(n)/(1<<20)) }

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.UTC().Format(time.DateTime) + " UTC"
}
