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

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/analytics"
	"github.com/ismagilovnail/flox/apps/internal/campaign"
	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/cost"
	"github.com/ismagilovnail/flox/apps/internal/httpserver"
	"github.com/ismagilovnail/flox/apps/internal/logging"
	"github.com/ismagilovnail/flox/apps/internal/ltv"
	"github.com/ismagilovnail/flox/apps/internal/network"
	"github.com/ismagilovnail/flox/apps/internal/offer"
	"github.com/ismagilovnail/flox/apps/internal/postgres"
	"github.com/ismagilovnail/flox/apps/internal/streamset"
	"github.com/ismagilovnail/flox/apps/internal/telemetry"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
	"github.com/ismagilovnail/flox/apps/internal/trafficsource"
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

	// ClickHouse is best-effort at startup, matching the tracker/worker's
	// own "a dependency the redirect/queue path never touches must not
	// become a startup-availability requirement" stance: apps/api's
	// campaign endpoints don't need it, only /analytics does, so a down
	// ClickHouse degrades one route group and shows up on /ready — it
	// doesn't take the whole control-plane API down.
	var ch driver.Conn
	chConn, chErr := chconn.NewConn(ctx, cfg.ClickHouse)
	if chErr != nil {
		logger.Warn("clickhouse unavailable at startup, /analytics will fail until it recovers", "error", chErr)
	} else if migrateErr := chstore.Migrate(ctx, chConn); migrateErr != nil {
		// Idempotent (every statement is CREATE ... IF NOT EXISTS), so this
		// is safe to run from both apps/api and apps/worker regardless of
		// which one starts first — see chstore.Migrate's doc.
		logger.Warn("clickhouse schema migration failed, /analytics will fail until it recovers", "error", migrateErr)
		_ = chConn.Close()
	} else {
		defer chConn.Close()
		ch = chConn
	}

	srv := httpserver.New(logger, cfg.OTelServiceName, db, ch, cfg.AppURL)

	campaignRepo := campaign.NewRepository(db)
	campaignSvc := campaign.NewService(campaignRepo)
	campaignHandler := campaign.NewHandler(campaignSvc, logger)
	srv.Mux().Route("/campaigns", func(r chi.Router) {
		r.Use(tenant.Middleware)
		campaignHandler.Register(r)
	})

	trafficSourceHandler := trafficsource.NewHandler(trafficsource.NewService(trafficsource.NewRepository(db)), logger)
	srv.Mux().Route("/traffic-sources", func(r chi.Router) {
		r.Use(tenant.Middleware)
		trafficSourceHandler.Register(r)
	})

	networkHandler := network.NewHandler(network.NewService(network.NewRepository(db)), logger)
	srv.Mux().Route("/networks", func(r chi.Router) {
		r.Use(tenant.Middleware)
		networkHandler.Register(r)
	})

	offerHandler := offer.NewHandler(offer.NewService(offer.NewRepository(db)), logger)
	srv.Mux().Route("/offers", func(r chi.Router) {
		r.Use(tenant.Middleware)
		offerHandler.Register(r)
	})

	costHandler := cost.NewHandler(cost.NewService(cost.NewRepository(db), conversion.NewPostgresFX(db)), logger)
	srv.Mux().Route("/campaigns/{campaignId}/cost-entries", func(r chi.Router) {
		r.Use(tenant.Middleware)
		costHandler.Register(r)
	})

	streamSetHandler := streamset.NewHandler(streamset.NewService(streamset.NewRepository(db)), logger)
	srv.Mux().Route("/campaigns/{campaignId}/stream-sets", func(r chi.Router) {
		r.Use(tenant.Middleware)
		streamSetHandler.Register(r)
	})

	if ch != nil {
		events := chstore.NewEventStore(ch)

		analyticsSvc := analytics.NewService(events)
		analyticsHandler := analytics.NewHandler(analyticsSvc, logger)
		srv.Mux().Route("/analytics", func(r chi.Router) {
			r.Use(tenant.Middleware)
			analyticsHandler.Register(r)
		})

		ltvSvc := ltv.NewService(events)
		ltvHandler := ltv.NewHandler(ltvSvc, logger)
		srv.Mux().Route("/analytics/ltv", func(r chi.Router) {
			r.Use(tenant.Middleware)
			ltvHandler.Register(r)
		})
	}

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
