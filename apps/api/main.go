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
	"github.com/redis/go-redis/v9"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/adaccount/facebookads"
	"github.com/ismagilovnail/flox/apps/internal/adaccount/tiktokads"
	"github.com/ismagilovnail/flox/apps/internal/analytics"
	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/auth"
	"github.com/ismagilovnail/flox/apps/internal/campaign"
	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/conversions"
	"github.com/ismagilovnail/flox/apps/internal/cost"
	"github.com/ismagilovnail/flox/apps/internal/costsync"
	"github.com/ismagilovnail/flox/apps/internal/eventbuf"
	"github.com/ismagilovnail/flox/apps/internal/eventmapping"
	"github.com/ismagilovnail/flox/apps/internal/eventqueue"
	"github.com/ismagilovnail/flox/apps/internal/httpserver"
	"github.com/ismagilovnail/flox/apps/internal/landing"
	"github.com/ismagilovnail/flox/apps/internal/logging"
	"github.com/ismagilovnail/flox/apps/internal/ltv"
	"github.com/ismagilovnail/flox/apps/internal/network"
	"github.com/ismagilovnail/flox/apps/internal/offer"
	"github.com/ismagilovnail/flox/apps/internal/pixel"
	"github.com/ismagilovnail/flox/apps/internal/postback"
	"github.com/ismagilovnail/flox/apps/internal/postbacklog"
	"github.com/ismagilovnail/flox/apps/internal/postbacklogs"
	"github.com/ismagilovnail/flox/apps/internal/postgres"
	"github.com/ismagilovnail/flox/apps/internal/postlanding"
	"github.com/ismagilovnail/flox/apps/internal/pwa"
	"github.com/ismagilovnail/flox/apps/internal/ratelimit"
	"github.com/ismagilovnail/flox/apps/internal/rediscache"
	"github.com/ismagilovnail/flox/apps/internal/routing"
	"github.com/ismagilovnail/flox/apps/internal/routingsimulate"
	"github.com/ismagilovnail/flox/apps/internal/routingstore"
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

	// Redis, connected here (rather than down where it was previously only
	// used, inside the ClickHouse-gated conversion-replay block) so
	// apps/internal/ratelimit can use it too — §54/Phase 30 rate limiting
	// needs to guard /auth/login and /auth/signup, both registered well
	// before that block. Best-effort at startup, same stance as every
	// other Redis-optional path in this codebase (conversion.RedisStore
	// below reuses this exact client instead of opening a second
	// connection): a rate limiter that failed closed on a down Redis
	// would turn a cache outage into a full API outage.
	var rdb *redis.Client
	if cfg.RedisURL != "" {
		redisCtx, cancelRedis := context.WithTimeout(ctx, 5*time.Second)
		client, redisErr := rediscache.NewClient(redisCtx, cfg.RedisURL)
		cancelRedis()
		if redisErr != nil {
			logger.Warn("redis unavailable at startup, rate limiting and conversion progression checks degrade", "error", redisErr)
		} else {
			defer client.Close()
			rdb = client
		}
	} else {
		logger.Warn("REDIS_URL not set, rate limiting and conversion progression checks degrade")
	}
	limiter := ratelimit.New(rdb, logger)

	srv := httpserver.New(logger, cfg.OTelServiceName, db, ch, cfg.AppURL, limiter)

	// Auth/RBAC (§52, Phase 28A). authSvc.ResolveSession implements
	// tenant.SessionResolver — tenantMiddleware below replaces the old
	// X-Organization-Id-header stand-in tenant.Middleware used to be, for
	// every route mounted under it, with zero changes to those routes'
	// own handlers (they still only ever read tenant.OrgID(ctx)).
	// secureCookies is false in development (a Secure cookie is dropped
	// by browsers over plain http, and this dev stack has no TLS
	// termination in front of apps/api).
	authRepo := auth.NewRepository(db)
	authSvc := auth.NewService(authRepo, cfg.AppURL)
	authHandler := auth.NewHandler(authSvc, logger, cfg.Env != "development")
	teamHandler := auth.NewTeamHandler(authSvc, logger)

	// §54/Phase 30: RequireSameOrigin runs OUTSIDE (before) session
	// resolution — rejecting a forged cross-origin mutation is cheaper
	// and safer than first looking up whose session it claims to be.
	// tenantMiddleware keeps its original name/type (func(http.Handler)
	// http.Handler) even though it's now two middlewares composed, so
	// none of this file's 17 r.Use(tenantMiddleware) call sites change.
	csrfCheck := tenant.RequireSameOrigin(cfg.AppURL, logger)
	sessionCheck := tenant.NewMiddleware(authSvc, logger)
	tenantMiddleware := func(next http.Handler) http.Handler { return csrfCheck(sessionCheck(next)) }

	// signup/login/accept-invite are POST (mutating) but pre-session, so
	// they can't go through tenantMiddleware (which requires a cookie
	// that doesn't exist yet) — csrfCheck alone still guards them against
	// login CSRF (forcing a victim's browser to authenticate as an
	// attacker-controlled account, so the victim unknowingly saves real
	// data into it).
	//
	// authRateLimit is deliberately much stricter than httpserver's own
	// general per-IP ceiling (300/min) — these are exactly the routes
	// §54's "brute-force credential stuffing" concern is about
	// (repeatedly trying passwords against /auth/login, or hammering
	// /auth/signup). 20 requests per 15 minutes per IP is generous for a
	// real user who mistypes a password a few times, tight for a script.
	const (
		authRateLimit       = 20
		authRateLimitWindow = 15 * time.Minute
	)
	authLimit := limiter.Middleware("auth", ratelimit.ClientIP, authRateLimit, authRateLimitWindow)
	srv.Mux().Group(func(r chi.Router) {
		r.Use(csrfCheck)
		r.Use(authLimit)
		authHandler.RegisterPublic(r)
	})
	srv.Mux().Group(func(r chi.Router) {
		r.Use(tenantMiddleware)
		authHandler.RegisterAuthenticated(r)
		teamHandler.Register(r)
	})

	campaignRepo := campaign.NewRepository(db)
	campaignSvc := campaign.NewService(campaignRepo)
	campaignHandler := campaign.NewHandler(campaignSvc, logger)
	srv.Mux().Route("/campaigns", func(r chi.Router) {
		r.Use(tenantMiddleware)
		campaignHandler.Register(r)
	})

	trafficSourceHandler := trafficsource.NewHandler(trafficsource.NewService(trafficsource.NewRepository(db)), logger)
	srv.Mux().Route("/traffic-sources", func(r chi.Router) {
		r.Use(tenantMiddleware)
		trafficSourceHandler.Register(r)
	})

	adAccountRepo := adaccount.NewRepository(db)
	adAccountHandler := adaccount.NewHandler(adaccount.NewService(adAccountRepo), logger)

	networkHandler := network.NewHandler(network.NewService(network.NewRepository(db)), logger)
	srv.Mux().Route("/networks", func(r chi.Router) {
		r.Use(tenantMiddleware)
		networkHandler.Register(r)
	})

	landingHandler := landing.NewHandler(landing.NewService(landing.NewRepository(db)), logger)
	srv.Mux().Route("/landings", func(r chi.Router) {
		r.Use(tenantMiddleware)
		landingHandler.Register(r)
	})

	pwaHandler := pwa.NewHandler(pwa.NewService(pwa.NewRepository(db)), logger)
	srv.Mux().Route("/pwas", func(r chi.Router) {
		r.Use(tenantMiddleware)
		pwaHandler.Register(r)
	})

	postlandingHandler := postlanding.NewHandler(postlanding.NewService(postlanding.NewRepository(db)), logger)
	srv.Mux().Route("/postlandings", func(r chi.Router) {
		r.Use(tenantMiddleware)
		postlandingHandler.Register(r)
	})

	pixelHandler := pixel.NewHandler(pixel.NewService(pixel.NewRepository(db)), logger)
	srv.Mux().Route("/pixels", func(r chi.Router) {
		r.Use(tenantMiddleware)
		pixelHandler.Register(r)
	})

	offerHandler := offer.NewHandler(offer.NewService(offer.NewRepository(db)), logger)
	srv.Mux().Route("/offers", func(r chi.Router) {
		r.Use(tenantMiddleware)
		offerHandler.Register(r)
	})

	eventMappingHandler := eventmapping.NewHandler(eventmapping.NewService(eventmapping.NewRepository(db)), logger)
	srv.Mux().Route("/event-mappings", func(r chi.Router) {
		r.Use(tenantMiddleware)
		eventMappingHandler.Register(r)
	})

	costSvc := cost.NewService(cost.NewRepository(db), conversion.NewPostgresFX(db))
	costHandler := cost.NewHandler(costSvc, logger)
	srv.Mux().Route("/campaigns/{campaignId}/cost-entries", func(r chi.Router) {
		r.Use(tenantMiddleware)
		costHandler.Register(r)
	})

	// Ad-spend sync (§74/§27-COST, Phase B): "Sync now" pulls a traffic
	// source's connected ad account's daily campaign spend and writes it
	// into cost_entries. No live Meta/TikTok app credentials exist in
	// this environment, so the adapters below are wired for real but only
	// ever verified structurally (see facebookads_test.go/tiktokads_test.go);
	// nothing prevents pointing them at the real Graph/Business API once
	// real credentials exist. Registered in the same Route() block as
	// adAccountHandler (both mount under /traffic-sources/{id}/connection)
	// — chi panics if two separate Route() calls claim the identical
	// pattern, so this can't be two Mount points even though the two
	// handlers are otherwise independent packages.
	costSyncHandler := costsync.NewHandler(costsync.NewService(adAccountRepo, campaignRepo, costSvc, costsync.Providers{
		FacebookAds: facebookads.New(),
		TikTokAds:   tiktokads.New(),
	}), logger)
	srv.Mux().Route("/traffic-sources/{id}/connection", func(r chi.Router) {
		r.Use(tenantMiddleware)
		adAccountHandler.Register(r)
		costSyncHandler.Register(r)
	})

	streamSetHandler := streamset.NewHandler(streamset.NewService(streamset.NewRepository(db)), logger)
	srv.Mux().Route("/campaigns/{campaignId}/stream-sets", func(r chi.Router) {
		r.Use(tenantMiddleware)
		streamSetHandler.Register(r)
	})

	routingSimulateHandler := routingsimulate.NewHandler(
		routingsimulate.NewService(routingstore.New(db), &routing.Engine{}), logger,
	)
	srv.Mux().Route("/campaigns/{campaignId}/routing/simulate", func(r chi.Router) {
		r.Use(tenantMiddleware)
		routingSimulateHandler.Register(r)
	})

	if ch != nil {
		events := chstore.NewEventStore(ch)

		analyticsSvc := analytics.NewService(events)
		analyticsHandler := analytics.NewHandler(analyticsSvc, logger)
		srv.Mux().Route("/analytics", func(r chi.Router) {
			r.Use(tenantMiddleware)
			analyticsHandler.Register(r)
		})

		ltvSvc := ltv.NewService(events)
		ltvHandler := ltv.NewHandler(ltvSvc, logger)
		srv.Mux().Route("/analytics/ltv", func(r chi.Router) {
			r.Use(tenantMiddleware)
			ltvHandler.Register(r)
		})

		conversionsHandler := conversions.NewHandler(conversions.NewService(events), logger)
		srv.Mux().Route("/conversions", func(r chi.Router) {
			r.Use(tenantMiddleware)
			conversionsHandler.Register(r)
		})

		// Incoming Postback Replay needs the exact dependency graph
		// apps/tracker/main.go's PostbackHandler already builds to call
		// apps/internal/conversion.Service.Record for a real network hit —
		// this used to be deliberately deferred pending exactly this
		// architecture decision (docs/postback-logs.md). Redis and
		// ClickHouse-backed attribution get the same best-effort-at-startup
		// fallback stance tracker uses: a postback-recording rule (§45)
		// must never become a control-plane-availability dependency. This
		// whole block already only runs when ch != nil, so — unlike
		// tracker, which must start even with ClickHouse down —
		// chConn is guaranteed live here; no attribution.NewMemoryResolver
		// fallback is needed on this path.
		convEvents := eventbuf.New(eventqueue.NewSink(eventqueue.NewPostgresQueue(db)), logger, eventbuf.Config{})
		defer convEvents.Close()

		// Reuses rdb (connected once, near the top of run(), so
		// apps/internal/ratelimit can also use it) rather than opening a
		// second Redis connection.
		var convStore conversion.Store = conversion.NewPostgresStore(db)
		if rdb != nil {
			convStore = conversion.NewRedisStore(convStore, rdb, logger)
		} else {
			logger.Warn("redis unavailable, conversion progression checks will read Postgres directly")
		}

		attributionSvc := attribution.NewService(chstore.NewClickResolver(chConn))
		postbackStore := postback.NewPostgresStore(db)
		deliveries := postback.NewEnqueuer(postbackStore, logger)
		attemptLog := postbacklog.NewConversionAttemptLogger(postbacklog.NewPostgresQueue(db, logger))

		convSvc := conversion.NewService(
			conversion.NewPostgresMapper(db),
			convStore,
			conversion.NewPostgresFX(db),
			attributionSvc,
			convEvents,
			deliveries,
			attemptLog,
		)

		postbackLogsHandler := postbacklogs.NewHandler(
			postbacklogs.NewService(
				events,
				conversion.NewPostgresStore(db),
				postback.NewReplayEnqueuer(postbackStore),
				conversion.NewReplayNetworkLookup(conversion.NewPostgresNetworkLookup(db)),
				conversion.NewReplayRecorder(convSvc),
			),
			logger,
		)
		srv.Mux().Route("/postback-logs", func(r chi.Router) {
			r.Use(tenantMiddleware)
			postbackLogsHandler.Register(r)
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
