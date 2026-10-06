// Command reconcile compares (and, unless -report-only, repairs) ClickHouse
// conversions against Postgres over any date range — the worker only checks
// the last three days every hour (internal/reconcile).
//
// Use it for history: after a long outage, after FX rates arrive for old
// non-USD conversions (fx.RepairUSD fixes Postgres; this rewrites their
// ClickHouse events), or to audit a period.
//
//	go run ./cmd/reconcile -from 2026-09-01 -to 2026-10-01 [-report-only]
//
// Same env as apps/worker (DATABASE_URL, CLICKHOUSE_*). Works one UTC day at
// a time, repeating a day until a run repairs it without hitting the
// per-run cap.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/postgres"
	"github.com/ismagilovnail/flox/apps/internal/reconcile"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "reconcile:", err)
		os.Exit(1)
	}
}

func run() error {
	fromFlag := flag.String("from", "", "first UTC day, YYYY-MM-DD (required)")
	toFlag := flag.String("to", "", "day after the last one, YYYY-MM-DD (required)")
	reportOnly := flag.Bool("report-only", false, "list differences, change nothing")
	flag.Parse()
	from, err := time.Parse(time.DateOnly, *fromFlag)
	if err != nil {
		return fmt.Errorf("-from: %w", err)
	}
	to, err := time.Parse(time.DateOnly, *toFlag)
	if err != nil {
		return fmt.Errorf("-to: %w", err)
	}
	if !to.After(from) {
		return fmt.Errorf("-to must be after -from")
	}

	cfg, err := config.LoadWorker()
	if err != nil {
		return err
	}
	ctx := context.Background()
	pg, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pg.Close()
	ch, err := chconn.NewConn(ctx, cfg.ClickHouse)
	if err != nil {
		return err
	}
	defer ch.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	r := reconcile.New(pg, ch, logger)

	var buckets, reemitted, deleted, unmatched int
	for day := from; day.Before(to); day = day.AddDate(0, 0, 1) {
		next := day.AddDate(0, 0, 1)
		for {
			mismatches, err := r.Compare(ctx, day, next)
			if err != nil {
				return err
			}
			if len(mismatches) == 0 || *reportOnly {
				buckets += len(mismatches)
				for _, m := range mismatches {
					fmt.Printf("%s %s %-11s postgres %d / %.2f USD   clickhouse %d / %.2f USD\n",
						m.Day, m.OrganizationID, m.Status, m.Postgres.Count, m.Postgres.USD, m.ClickHouse.Count, m.ClickHouse.USD)
				}
				break
			}
			got, err := r.Repair(ctx, mismatches, day, next)
			if err != nil {
				return err
			}
			reemitted += got.Reemitted
			deleted += got.Deleted
			if got.Skipped == 0 {
				// What remains differs only through rows without an event id.
				unmatched += got.Unmatched
				break
			}
		}
	}
	if *reportOnly {
		fmt.Printf("%d differing buckets\n", buckets)
	} else {
		fmt.Printf("re-emitted %d, deleted %d, unmatched (no event id) %d\n", reemitted, deleted, unmatched)
	}
	return nil
}
