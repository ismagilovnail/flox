// Command worker is FLOX's async background processor. It is a separate
// binary from apps/api and apps/tracker, same Go module, so it can be
// deployed and scaled independently of the redirect hot path — exactly the
// same reasoning apps/tracker's own doc comment gives.
//
// Two poll loops run here:
//   - internal/postback (§46, Phase 24): outgoing postback delivery —
//     claim due rows from postback_deliveries and dispatch them, with
//     exponential backoff and a dead-letter state after repeated failure.
//   - internal/eventqueue (§43/§47, Phase 25): the tracker's event queue,
//     claimed in batches and written to ClickHouse's minimal `events` table
//     (internal/chstore) — a deliberately disposable single-table schema;
//     the real five-table design lands in Phase 26 (§48), which this file
//     does not build ahead of. See docs/analytics-pipeline.md.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/eventqueue"
	"github.com/ismagilovnail/flox/apps/internal/logging"
	"github.com/ismagilovnail/flox/apps/internal/postback"
	"github.com/ismagilovnail/flox/apps/internal/postgres"
	"github.com/ismagilovnail/flox/apps/internal/telemetry"
)

// postbackPollBatchSize/postbackPollIdle: how many due postback deliveries
// one poll claims, and how long to wait after a partial batch.
const (
	postbackPollBatchSize = 20
	postbackPollIdle      = 5 * time.Second
)

// eventPollBatchSize/eventPollIdle: same shape, sized for click/tracking
// event volume, which runs far higher than outgoing postback deliveries —
// a bigger batch and a shorter idle wait keeps the queue from backing up
// under normal traffic.
const (
	eventPollBatchSize = 500
	eventPollIdle      = 2 * time.Second
)

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

	ch, err := chconn.NewConn(ctx, cfg.ClickHouse)
	if err != nil {
		logger.Error("clickhouse connection failed", "error", err)
		return err
	}
	defer ch.Close()
	if err := chstore.Migrate(ctx, ch); err != nil {
		logger.Error("clickhouse schema migration failed", "error", err)
		return err
	}

	deliverer := postback.NewDeliverer(postback.NewPostgresStore(db), http.DefaultClient, logger)
	flusher := eventqueue.NewFlusher(eventqueue.NewPostgresQueue(db), chstore.NewEventStore(ch), logger)

	// A bare health endpoint for orchestration liveness probes — this
	// binary has no inbound routing otherwise, its work is the two poll
	// loops below.
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

	logger.Info("postback delivery poll loop starting", "batch_size", postbackPollBatchSize, "idle", postbackPollIdle)
	go deliverer.PollLoop(ctx, postbackPollBatchSize, postbackPollIdle)

	logger.Info("event flush poll loop starting", "batch_size", eventPollBatchSize, "idle", eventPollIdle)
	go flusher.PollLoop(ctx, eventPollBatchSize, eventPollIdle)

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return healthSrv.Shutdown(shutdownCtx)
}
