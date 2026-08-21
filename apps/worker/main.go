// Command worker is FLOX's async background processor (§46, Phase 24). It
// is a separate binary from apps/api and apps/tracker, same Go module, so
// it can be deployed and scaled independently of the redirect hot path —
// exactly the same reasoning apps/tracker's own doc comment gives.
//
// Phase 24's job is outgoing postback delivery only (internal/postback):
// claim due rows from postback_deliveries and dispatch them, with
// exponential backoff and a dead-letter state after repeated failure.
// Consuming the tracker's event queue into ClickHouse is a LATER role for
// this binary (§47/§48, Phases 25-26) — apps/worker/README.md describes
// that eventual full scope; this file only stands up what Phase 24 itself
// specifies, per CLAUDE.md's "work strictly one phase at a time."
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/logging"
	"github.com/ismagilovnail/flox/apps/internal/postback"
	"github.com/ismagilovnail/flox/apps/internal/postgres"
	"github.com/ismagilovnail/flox/apps/internal/telemetry"
)

// pollBatchSize is how many due deliveries one poll claims at a time.
const pollBatchSize = 20

// pollIdle is how long the poll loop waits after a batch smaller than
// pollBatchSize before polling again — no point hammering Postgres when the
// queue is empty or nearly so.
const pollIdle = 5 * time.Second

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadWorker()
	if err != nil {
		return err
	}

	logger := logging.New(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := telemetry.Setup(ctx, cfg.OTelServiceName, cfg.OTelExporterOTLPEndpoint)
	if err != nil {
		logger.Error("telemetry setup failed", "error", err)
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTelemetry(shutdownCtx); err != nil {
			logger.Error("telemetry shutdown failed", "error", err)
		}
	}()

	db, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		return err
	}
	defer db.Close()

	deliverer := postback.NewDeliverer(postback.NewPostgresStore(db), http.DefaultClient, logger)

	// A bare health endpoint for orchestration liveness probes — this
	// binary has no inbound routing otherwise, its work is the poll loop
	// below.
	healthSrv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		logger.Info("worker health endpoint starting", "addr", cfg.HTTPAddr, "env", cfg.Env)
		if err := healthSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("worker health endpoint failed", "error", err)
		}
	}()

	logger.Info("postback delivery poll loop starting", "batch_size", pollBatchSize, "idle", pollIdle)
	go deliverer.PollLoop(ctx, pollBatchSize, pollIdle)

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return healthSrv.Shutdown(shutdownCtx)
}
