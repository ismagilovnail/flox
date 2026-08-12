// Command api is the FLOX control-plane HTTP server (§33, Phase 16).
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/api/internal/campaign"
	"github.com/ismagilovnail/flox/apps/api/internal/config"
	"github.com/ismagilovnail/flox/apps/api/internal/httpserver"
	"github.com/ismagilovnail/flox/apps/api/internal/logging"
	"github.com/ismagilovnail/flox/apps/api/internal/postgres"
	"github.com/ismagilovnail/flox/apps/api/internal/telemetry"
	"github.com/ismagilovnail/flox/apps/api/internal/tenant"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
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

	srv := httpserver.New(logger, cfg.OTelServiceName, db)

	campaignRepo := campaign.NewRepository(db)
	campaignSvc := campaign.NewService(campaignRepo)
	campaignHandler := campaign.NewHandler(campaignSvc, logger)
	srv.Mux().Route("/campaigns", func(r chi.Router) {
		r.Use(tenant.Middleware)
		campaignHandler.Register(r)
	})

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("http server starting", "addr", cfg.HTTPAddr, "env", cfg.Env)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server failed", "error", err)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutdownCtx)
}
