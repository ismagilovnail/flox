// Command tracker is the FLOX hot-path click/redirect service (§41,
// Phase 21). It is a separate binary from apps/api so the redirect path
// can be deployed and scaled independently, but shares
// apps/internal/routing and apps/internal/classifier with it — there is
// exactly one implementation of routing and classification in the system
// (ARCHITECTURE.md, CLAUDE.md non-negotiable #1).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/classifier"
	"github.com/ismagilovnail/flox/apps/internal/classifier/maxmind"
	"github.com/ismagilovnail/flox/apps/internal/clientip"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/eventbuf"
	"github.com/ismagilovnail/flox/apps/internal/eventqueue"
	"github.com/ismagilovnail/flox/apps/internal/landinglink"
	"github.com/ismagilovnail/flox/apps/internal/logging"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/offercap"
	"github.com/ismagilovnail/flox/apps/internal/postback"
	"github.com/ismagilovnail/flox/apps/internal/postbacklog"
	"github.com/ismagilovnail/flox/apps/internal/postgres"
	"github.com/ismagilovnail/flox/apps/internal/rediscache"
	"github.com/ismagilovnail/flox/apps/internal/routing"
	"github.com/ismagilovnail/flox/apps/internal/routingcache"
	"github.com/ismagilovnail/flox/apps/internal/routingstore"
	"github.com/ismagilovnail/flox/apps/internal/storage"
	"github.com/ismagilovnail/flox/apps/internal/telemetry"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadTracker()
	if err != nil {
		return err
	}

	logger := logging.New(cfg.LogLevel)

	// See apps/api/main.go's identical construction — same TRUSTED_PROXIES
	// value, same reasoning (security audit finding #2). apps/tracker's
	// resolved IP feeds classifier.Input (geo/ASN) and the visit
	// fingerprint (§38's deterministic flow pick), so a spoofable one here
	// let a visitor choose its own A/B arm and defeat geo targeting, not
	// just bypass a rate limit.
	clientIP, err := clientip.New(clientip.ParseList(cfg.TrustedProxies), logger)
	if err != nil {
		logger.Error("trusted proxy configuration failed", "error", err)
		return err
	}

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

	// §43's durable queue (apps/worker's ClickHouse consumer, Phase 25)
	// replaces the structured-log stand-in this used through Phase 24 —
	// exactly the one-line swap that design promised, because everything
	// upstream only ever sees the eventbuf.Sink interface.
	events := eventbuf.New(eventqueue.NewSink(eventqueue.NewPostgresQueue(db)), logger, eventbuf.Config{})
	defer events.Close()
	metrics.RegisterEventBufStats(events)

	// Geo/ASN are best-effort at startup, same stance as ClickHouse/Redis
	// below (CLAUDE.md #9): an unset or bad GeoIP path must never block
	// the redirect path or prevent the tracker from starting — it just
	// falls back to classifier.New's own Noop defaults, exactly the
	// pre-existing behavior when nothing is configured.
	var geoProvider classifier.GeoProvider
	if cfg.GeoIPCityDBPath != "" {
		p, err := maxmind.NewGeoProvider(cfg.GeoIPCityDBPath)
		if err != nil {
			logger.Warn("geoip city database unavailable, geo classification disabled", "path", cfg.GeoIPCityDBPath, "error", err)
		} else {
			defer p.Close()
			geoProvider = p
		}
	}
	var asnProvider classifier.ASNProvider
	if cfg.GeoIPASNDBPath != "" {
		p, err := maxmind.NewASNProvider(cfg.GeoIPASNDBPath)
		if err != nil {
			logger.Warn("geoip asn database unavailable, asn classification disabled", "path", cfg.GeoIPASNDBPath, "error", err)
		} else {
			defer p.Close()
			asnProvider = p
		}
	}

	// Routing reads come from memory (audit 2026-10-06, RL1/A1): Postgres
	// slow or down no longer stalls or fails clicks for campaigns already
	// seen, and every edit flushes the cache through LISTEN/NOTIFY.
	routes := routingcache.New(routingstore.New(db), routingcache.Config{}, logger)
	go routes.Listen(ctx, cfg.DatabaseURL)

	handler := &Handler{
		store:      routes,
		classifier: classifier.New(geoProvider, asnProvider, nil),
		engine:     &routing.Engine{},
		events:     events,
		logger:     logger,
		clientIP:   clientIP,
	}

	// Internal landings (docs/landings.md, "Serving visitors"). Both
	// dependencies are optional: without a signing key internal landing
	// stages are skipped (straight to the destination, as before), without
	// storage pages are served with no <base href>.
	if key, err := landinglink.ParseKey(cfg.LandingLinkKey); err != nil {
		logger.Warn("internal landings disabled: flows with one redirect straight to their destination", "error", err)
	} else {
		handler.links = landinglink.NewSigner(key)
	}
	if assets, err := storage.NewClient(cfg.S3); err != nil {
		logger.Warn("object storage not configured, internal landings are served without asset base URLs", "error", err)
	} else {
		handler.assets = assets
	}

	// ClickHouse is best-effort at startup, same stance as Redis below: a
	// down analytics store must not block the redirect path (CLAUDE.md #9)
	// or prevent the tracker from starting at all. Falling back to
	// MemoryResolver degrades attribution to "always unattributed" — worse
	// than real matching, but no worse than every phase before this one.
	var clickResolver attribution.ClickResolver = attribution.NewMemoryResolver()
	var offerCaps *offercap.Monitor
	chConn, chErr := chconn.NewConn(ctx, cfg.ClickHouse)
	if chErr != nil {
		logger.Warn("clickhouse unavailable at startup, attribution will not match real clicks", "error", chErr)
	} else if migrateErr := chstore.Migrate(ctx, chConn); migrateErr != nil {
		logger.Warn("clickhouse schema migration failed, attribution will not match real clicks", "error", migrateErr)
		_ = chConn.Close()
	} else {
		defer chConn.Close()
		clickResolver = chstore.NewClickResolver(chConn)
		// Daily offer caps: a background snapshot, never a query on the
		// redirect path. Without ClickHouse no offer is ever capped.
		offerCaps = offercap.NewMonitor(offercap.NewPostgresSource(db), chstore.NewEventStore(chConn), logger)
		go offerCaps.Run(ctx)
	}
	handler.caps = offerCaps
	attributionSvc := attribution.NewService(clickResolver)

	// Redis is best-effort at startup, matching §45's "REDIS UNAVAILABLE:
	// fall through and record the event" at runtime: a postback correctness
	// rule must not become a tracker-availability dependency, especially
	// since the redirect path (this binary's actual latency budget) never
	// touches Redis at all. conversion.PostgresStore alone is already
	// correct — the cache only saves a round trip.
	// The store records a conversion together with its CPA event and its
	// outgoing postback in one transaction (audit 2026-10-06, D1).
	var convStore conversion.Store = conversion.NewPostgresStore(db).WithDeliveries(postback.NewPostgresStore(db))
	if cfg.RedisURL != "" {
		redisCtx, cancelRedis := context.WithTimeout(ctx, 5*time.Second)
		rdb, err := rediscache.NewClient(redisCtx, cfg.RedisURL)
		cancelRedis()
		if err != nil {
			logger.Warn("redis unavailable at startup, conversion progression checks will read Postgres directly", "error", err)
		} else {
			defer rdb.Close()
			convStore = conversion.NewRedisStore(convStore, rdb, logger)
		}
	} else {
		logger.Warn("REDIS_URL not set, conversion progression checks will read Postgres directly")
	}

	attemptLog := postbacklog.NewConversionAttemptLogger(postbacklog.NewPostgresQueue(db, logger))

	postbackHandler := &PostbackHandler{
		networks: conversion.NewPostgresNetworkLookup(db),
		service: conversion.NewService(
			conversion.NewPostgresMapper(db),
			convStore,
			conversion.NewPostgresFX(db),
			attributionSvc,
			attemptLog,
		),
		logger: logger,
	}

	r := chi.NewRouter()
	// RequestID + echoing it back is a plain context.WithValue and one
	// response header write — negligible next to the hot path's real
	// costs (DB reads, routing). It is NOT the per-request logging
	// middleware apps/api uses (requestLogger, apps/internal/httpserver):
	// that would mean one synchronous slog.Logger.Info call per click,
	// which IS duplicate work (§53 request-ID correlation is satisfied by
	// the header alone; the durable record of the click is the async
	// event, not a log line).
	r.Use(middleware.RequestID)
	r.Use(echoRequestID)
	r.Use(middleware.Recoverer)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	// Readiness, unlike /health: can this instance serve clicks? Yes when
	// Postgres answers, or when routing is cached (it then serves every
	// campaign it has seen through an outage). No while shutting down, so
	// a load balancer stops sending new clicks before the listener closes.
	var draining atomic.Bool
	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if draining.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"draining"}`))
			return
		}
		pingCtx, cancel := context.WithTimeout(r.Context(), 300*time.Millisecond)
		defer cancel()
		dbErr := db.Ping(pingCtx)
		if dbErr != nil && !routes.Warm() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unavailable","reason":"postgres unreachable and routing cache empty"}`))
			return
		}
		if dbErr != nil {
			_, _ = w.Write([]byte(`{"status":"degraded","reason":"postgres unreachable, serving cached routing"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})
	r.Handle("/metrics", metrics.Handler())
	handler.Register(r)
	postbackHandler.Register(r)

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("tracker starting", "addr", cfg.HTTPAddr, "env", cfg.Env)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("tracker failed", "error", err)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")
	draining.Store(true)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	// Drain buffered events after the server stops accepting requests, so
	// a clean shutdown doesn't discard clicks it already served.
	events.Close()
	logStats(logger, events)
	return nil
}

func logStats(logger *slog.Logger, events *eventbuf.Writer) {
	s := events.Stats()
	logger.Info("event writer stats",
		"enqueued", s.Enqueued, "written", s.Written, "dropped", s.Dropped, "failed", s.Failed)
}

// echoRequestID mirrors apps/internal/httpserver's own responseRequestID —
// duplicated rather than imported, since that package is documented as
// apps/api's own chi router builder and pulling it in here for one six-line
// helper would be a heavier coupling than just repeating it.
func echoRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := middleware.GetReqID(r.Context()); id != "" {
			w.Header().Set("X-Request-Id", id)
		}
		next.ServeHTTP(w, r)
	})
}
