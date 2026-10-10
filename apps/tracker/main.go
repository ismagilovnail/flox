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
	"github.com/ismagilovnail/flox/apps/internal/classifier"
	"github.com/ismagilovnail/flox/apps/internal/classifier/maxmind"
	"github.com/ismagilovnail/flox/apps/internal/clientip"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/errtrack"
	"github.com/ismagilovnail/flox/apps/internal/eventbuf"
	"github.com/ismagilovnail/flox/apps/internal/eventqueue"
	"github.com/ismagilovnail/flox/apps/internal/eventstream"
	"github.com/ismagilovnail/flox/apps/internal/landinglink"
	"github.com/ismagilovnail/flox/apps/internal/logging"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/offercap"
	"github.com/ismagilovnail/flox/apps/internal/postback"
	"github.com/ismagilovnail/flox/apps/internal/postbacklog"
	"github.com/ismagilovnail/flox/apps/internal/postgres"
	"github.com/ismagilovnail/flox/apps/internal/ratelimit"
	"github.com/ismagilovnail/flox/apps/internal/rediscache"
	"github.com/ismagilovnail/flox/apps/internal/routing"
	"github.com/ismagilovnail/flox/apps/internal/routingcache"
	"github.com/ismagilovnail/flox/apps/internal/routingstore"
	"github.com/ismagilovnail/flox/apps/internal/storage"
	"github.com/ismagilovnail/flox/apps/internal/telemetry"
	"github.com/ismagilovnail/flox/apps/internal/tlsask"
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

	// Error tracking (internal/errtrack): ERROR log records and panics go to
	// the tracker named by SENTRY_DSN; a no-op without it.
	env := cfg.SentryEnvironment
	if env == "" {
		env = cfg.Env
	}
	reporter, err := errtrack.New(errtrack.Config{DSN: cfg.SentryDSN, Environment: env, Service: "tracker"})
	if err != nil {
		logger.Error("error tracker configuration failed", "error", err)
		return err
	}
	defer reporter.Flush(2 * time.Second)
	logger = slog.New(errtrack.Handler(logger.Handler(), reporter))

	// Refused up front: without the key, sticky routing would either stop
	// (re-bucketing every returning visitor) or trust forged cookies.
	sticky, err := newStickySigner(cfg.StickyCookieKey)
	if err != nil {
		logger.Error("sticky cookie key", "error", err)
		return err
	}

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
	metrics.RegisterPgxPool(db, "tracker")

	// §43's durable queue (apps/worker's ClickHouse consumer, Phase 25)
	// replaces the structured-log stand-in this used through Phase 24 —
	// exactly the one-line swap that design promised, because everything
	// upstream only ever sees the eventbuf.Sink interface.
	//
	// With NATS_URL set, events go to the JetStream stream instead and the
	// queue only takes what NATS refuses (internal/eventstream). The
	// connection opens first so it closes after the buffer's final flush.
	var sink eventbuf.Sink = eventqueue.NewSink(eventqueue.NewPostgresQueue(db))
	if cfg.NATSURL != "" {
		nc, js, err := eventstream.Connect(cfg.NATSURL, "flox-tracker", logger)
		if err != nil {
			logger.Error("nats connection failed", "error", err)
			return err
		}
		defer nc.Close()
		topology := eventstream.Topology{Replicas: cfg.NATSStreamReplicas, MaxBytes: int64(cfg.NATSStreamMaxMB) << 20}
		go func() {
			if _, err := eventstream.EnsureTopologyLoop(ctx, js, topology, logger); err == nil {
				logger.Info("nats event stream ready", "stream", eventstream.StreamName)
			}
		}()
		sink = eventstream.NewSink(js, topology, sink, logger)
	} else {
		logger.Warn("NATS_URL not set, events go through the postgres event_queue")
	}
	events := eventbuf.New(sink, logger, eventbuf.Config{})
	defer events.Close()
	metrics.RegisterEventBufStats(events)

	// Geo/ASN are best-effort, same stance as ClickHouse/Redis below
	// (CLAUDE.md #9): an unset or bad GeoIP path must never block the
	// redirect path or prevent the tracker from starting. A configured
	// path is watched instead: until a file is there lookups come back
	// empty (as classifier.New's Noop defaults would), and a file
	// replaced on disk — a new month — is swapped in within a minute,
	// no restart (docs/geoip.md).
	var geoProvider classifier.GeoProvider
	var asnProvider classifier.ASNProvider
	var geoDBs []geoIPDB
	if cfg.GeoIPCityDBPath != "" {
		p := maxmind.LazyGeoProvider(cfg.GeoIPCityDBPath)
		defer p.Close()
		geoProvider = p
		geoDBs = append(geoDBs, geoIPDB{cfg.GeoIPCityDBPath, p})
	}
	if cfg.GeoIPASNDBPath != "" {
		p := maxmind.LazyASNProvider(cfg.GeoIPASNDBPath)
		defer p.Close()
		asnProvider = p
		geoDBs = append(geoDBs, geoIPDB{cfg.GeoIPASNDBPath, p})
	}
	if len(geoDBs) > 0 {
		reloadGeoIP(ctx, geoDBs, logger)
	}

	// Routing reads come from memory (audit 2026-10-06, RL1/A1): Postgres
	// slow or down no longer stalls or fails clicks for campaigns already
	// seen, and every edit flushes the cache through LISTEN/NOTIFY.
	routes := routingcache.New(routingstore.New(db), routingcache.Config{}, logger)
	go routes.Listen(ctx, cfg.DatabaseURL)

	handler := &Handler{
		sticky:     sticky,
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

	// ClickHouse is connected in the background: a down analytics store
	// must not block the redirect path (CLAUDE.md #9) or keep the tracker
	// from starting. Until it connects, postbacks are recorded unknown_click
	// and retried by the worker (audit 2026-10-06, T1), and no offer is
	// capped.
	clickHouse := &lateClickHouse{}
	go clickHouse.connect(ctx, db, cfg.ClickHouse, logger)
	var clickResolver attribution.ClickResolver = clickHouse
	// Daily offer caps: a background snapshot, never a query on the
	// redirect path.
	offerCaps := offercap.NewMonitor(offercap.NewPostgresSource(db), clickHouse, logger)
	go offerCaps.Run(ctx)
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
	// Without Redis the refused-postback limit counts in this process.
	limiter := ratelimit.New(nil, logger)
	if cfg.RedisURL != "" {
		redisCtx, cancelRedis := context.WithTimeout(ctx, 5*time.Second)
		rdb, err := rediscache.NewClient(redisCtx, cfg.RedisURL)
		cancelRedis()
		if err != nil {
			logger.Warn("redis unavailable at startup, conversion progression checks will read Postgres directly", "error", err)
		} else {
			defer rdb.Close()
			rediscache.CheckEviction(ctx, rdb, logger)
			convStore = conversion.NewRedisStore(convStore, rdb, logger)
			limiter = ratelimit.New(rdb, logger)
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
		failures: limiter.Failures("postback-failures", postbackFailureLimit, postbackFailureWindow),
		clientIP: clientIP,
		logger:   logger,
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
	r.Use(errtrack.Middleware(reporter, logger))
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
	// The TLS proxy's on-demand certificate check (internal/tlsask).
	r.Get(tlsask.Path, tlsask.Handler(db, logger))
	handler.Register(r)
	postbackHandler.Register(r)

	// Whole-request bounds (tech audit RL5). Requests here are a redirect,
	// a landing or PWA page, or a postback — small, so short: a slow client
	// gives up its connection after 15 s of upload or 30 s overall.
	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
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

// geoIPDB is one GeoIP database file the tracker watches.
type geoIPDB struct {
	path string
	geoIPReloader
}

type geoIPReloader interface {
	Reload() (changed bool, err error)
	Built() time.Time
}

// reloadGeoIP loads the GeoIP databases now, then checks every minute for
// a file replaced on disk. A missing or broken file is logged once until
// it changes; the database already loaded stays in use.
func reloadGeoIP(ctx context.Context, dbs []geoIPDB, logger *slog.Logger) {
	failing := make([]string, len(dbs))
	check := func() {
		for i, db := range dbs {
			changed, err := db.Reload()
			switch {
			case err != nil:
				if msg := err.Error(); msg != failing[i] {
					failing[i] = msg
					logger.Warn("geoip database unavailable, keeping the one loaded (if any)", "path", db.path, "error", err)
				}
			case changed:
				failing[i] = ""
				logger.Info("geoip database loaded", "path", db.path, "built", db.Built().Format(time.DateOnly))
			}
		}
	}
	check()
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				check()
			}
		}
	}()
}
