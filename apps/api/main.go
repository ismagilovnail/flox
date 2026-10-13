// Command api is the FLOX control-plane HTTP server (§33).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	// Embedded zone database: the alpine runtime image ships none, and report
	// timezones (Europe/Moscow, ...) are validated with time.LoadLocation.
	_ "time/tzdata"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/adaccount/facebookads"
	"github.com/ismagilovnail/flox/apps/internal/adaccount/googleads"
	"github.com/ismagilovnail/flox/apps/internal/adaccount/tiktokads"
	"github.com/ismagilovnail/flox/apps/internal/alert"
	"github.com/ismagilovnail/flox/apps/internal/analytics"
	"github.com/ismagilovnail/flox/apps/internal/apikey"
	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/auth"
	"github.com/ismagilovnail/flox/apps/internal/campaign"
	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/clicklogs"
	"github.com/ismagilovnail/flox/apps/internal/clientip"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/contentgallery"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/conversions"
	"github.com/ismagilovnail/flox/apps/internal/cost"
	"github.com/ismagilovnail/flox/apps/internal/costsync"
	"github.com/ismagilovnail/flox/apps/internal/custommetrics"
	"github.com/ismagilovnail/flox/apps/internal/domain"
	domaincloudflare "github.com/ismagilovnail/flox/apps/internal/domain/cloudflare"
	domainnamecheap "github.com/ismagilovnail/flox/apps/internal/domain/namecheap"
	"github.com/ismagilovnail/flox/apps/internal/domain/tlsprobe"
	"github.com/ismagilovnail/flox/apps/internal/errtrack"
	"github.com/ismagilovnail/flox/apps/internal/eventmapping"
	"github.com/ismagilovnail/flox/apps/internal/httpserver"
	integrationsengine "github.com/ismagilovnail/flox/apps/internal/integrations"
	"github.com/ismagilovnail/flox/apps/internal/landing"
	"github.com/ismagilovnail/flox/apps/internal/landinglink"
	"github.com/ismagilovnail/flox/apps/internal/logging"
	"github.com/ismagilovnail/flox/apps/internal/ltv"
	"github.com/ismagilovnail/flox/apps/internal/mailer"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/network"
	"github.com/ismagilovnail/flox/apps/internal/offer"
	"github.com/ismagilovnail/flox/apps/internal/offercap"
	"github.com/ismagilovnail/flox/apps/internal/pixel"
	"github.com/ismagilovnail/flox/apps/internal/pixelsend"
	"github.com/ismagilovnail/flox/apps/internal/postback"
	"github.com/ismagilovnail/flox/apps/internal/postbacklog"
	"github.com/ismagilovnail/flox/apps/internal/postbacklogs"
	"github.com/ismagilovnail/flox/apps/internal/postgres"
	"github.com/ismagilovnail/flox/apps/internal/postlanding"
	"github.com/ismagilovnail/flox/apps/internal/push"
	"github.com/ismagilovnail/flox/apps/internal/pwa"
	"github.com/ismagilovnail/flox/apps/internal/ratelimit"
	"github.com/ismagilovnail/flox/apps/internal/rediscache"
	"github.com/ismagilovnail/flox/apps/internal/referral"
	"github.com/ismagilovnail/flox/apps/internal/reportpresets"
	"github.com/ismagilovnail/flox/apps/internal/routing"
	"github.com/ismagilovnail/flox/apps/internal/routingsimulate"
	"github.com/ismagilovnail/flox/apps/internal/routingstore"
	"github.com/ismagilovnail/flox/apps/internal/storage"
	"github.com/ismagilovnail/flox/apps/internal/streamset"
	"github.com/ismagilovnail/flox/apps/internal/tags"
	"github.com/ismagilovnail/flox/apps/internal/telemetry"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
	totpengine "github.com/ismagilovnail/flox/apps/internal/totp"
	"github.com/ismagilovnail/flox/apps/internal/trackinglink"
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

	// Error tracking (internal/errtrack): ERROR log records and panics go to
	// the tracker named by SENTRY_DSN; a no-op without it.
	env := cfg.SentryEnvironment
	if env == "" {
		env = cfg.Env
	}
	reporter, err := errtrack.New(errtrack.Config{DSN: cfg.SentryDSN, Environment: env, Service: "api"})
	if err != nil {
		logger.Error("error tracker configuration failed", "error", err)
		return err
	}
	defer reporter.Flush(2 * time.Second)
	logger = slog.New(errtrack.Handler(logger.Handler(), reporter))

	// clientIP resolves the client address behind TRUSTED_PROXIES; nothing
	// else reads X-Forwarded-For. A malformed entry fails startup rather
	// than silently narrowing or widening who is trusted.
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

	srv, closeAll, err := buildServer(ctx, cfg, logger, reporter, clientIP)
	if err != nil {
		return err
	}
	defer closeAll()

	// Whole-request bounds (tech audit RL5): a client that sends or reads
	// slowly cannot hold a connection open forever. Generous because
	// landing bundles upload up to 50 MiB, and WriteTimeout runs from the
	// end of the headers, so it covers the body upload too; handlers
	// themselves stop at 30 s (httpserver's middleware.Timeout).
	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       2 * time.Minute,
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

// buildServer connects the API's stores and mounts every route. The
// returned close releases the connections; it is also called when
// buildServer fails part way. Split from run so a test can walk the real
// router (tenant_isolation_routes_test.go).
func buildServer(ctx context.Context, cfg config.Config, logger *slog.Logger, reporter errtrack.Reporter, clientIP *clientip.Resolver) (srv *httpserver.Server, closeAll func(), err error) {
	var closers []func()
	closeAll = func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}
	defer func() {
		if err != nil {
			closeAll()
		}
	}()

	db, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		return nil, nil, err
	}
	closers = append(closers, db.Close)
	metrics.RegisterPgxPool(db, "api")

	// ClickHouse is best-effort at startup, matching the tracker/worker's
	// own "a dependency the redirect/queue path never touches must not
	// become a startup-availability requirement" stance: apps/api's
	// campaign endpoints don't need it, only /analytics does, so a down
	// ClickHouse degrades one route group and shows up on /ready — it
	// doesn't take the whole control-plane API down.
	var ch driver.Conn
	var offerCaps *offercap.Monitor
	chConn, chErr := chconn.NewConn(ctx, cfg.ClickHouse)
	if chErr != nil {
		logger.Warn("clickhouse unavailable at startup, /analytics will fail until it recovers", "error", chErr)
	} else if migrateErr := chstore.MigrateLocked(ctx, db, chConn); migrateErr != nil {
		// api, tracker and worker all migrate at start; the lock lets
		// exactly one apply — see chstore.MigrateLocked.
		logger.Warn("clickhouse schema migration failed, /analytics will fail until it recovers", "error", migrateErr)
		_ = chConn.Close()
	} else {
		closers = append(closers, func() { _ = chConn.Close() })
		ch = chConn
		// Same snapshot the tracker routes with, so the simulator answers
		// "what would happen right now" including offers at their daily cap.
		offerCaps = offercap.NewMonitor(offercap.NewPostgresSource(db), chstore.NewEventStore(chConn), logger)
		go offerCaps.Run(ctx)
	}

	// Redis backs the rate limits (§54) and the conversion cache below, one
	// client for both. Optional: a limiter failing closed on a down Redis
	// would turn a cache outage into an API outage (/auth/* counts in
	// process instead, ratelimit.StrictMiddleware).
	var rdb *redis.Client
	if cfg.RedisURL != "" {
		redisCtx, cancelRedis := context.WithTimeout(ctx, 5*time.Second)
		client, redisErr := rediscache.NewClient(redisCtx, cfg.RedisURL)
		cancelRedis()
		if redisErr != nil {
			logger.Warn("redis unavailable at startup, rate limiting and conversion progression checks degrade", "error", redisErr)
		} else {
			closers = append(closers, func() { _ = client.Close() })
			rediscache.CheckEviction(ctx, client, logger)
			rdb = client
		}
	} else {
		logger.Warn("REDIS_URL not set, rate limiting and conversion progression checks degrade")
	}
	limiter := ratelimit.New(rdb, logger)

	// Object storage (apps/internal/contentgallery's only consumer so
	// far) — best-effort at startup, same "a down optional dependency
	// degrades its own route group, not the whole API" stance as
	// ClickHouse/Redis above: /content-gallery upload/list return
	// apierror.Unavailable until minio recovers, nothing else is
	// affected.
	storageClient, storageErr := storage.NewClient(cfg.S3)
	if storageErr != nil {
		logger.Warn("object storage not configured, Content Gallery will be unavailable", "error", storageErr)
		storageClient = nil
	} else if ensureErr := storageClient.EnsureBucket(ctx, cfg.S3.Region); ensureErr != nil {
		logger.Warn("object storage unreachable at startup, Content Gallery will be unavailable until it recovers", "error", ensureErr)
	}

	// httpserver.New itself is constructed further down, once authSvc/
	// apiKeySvc exist — it needs tenant.PeekIdentity built from both (see
	// the comment right before that call).

	// Auth/RBAC (§52): authSvc resolves sessions for tenantMiddleware below;
	// routes only ever read tenant.OrgID(ctx). Cookies are Secure outside
	// development (a Secure cookie is dropped over plain http). mailer.New
	// only parses SMTP_URL, so an unreachable relay surfaces on the first
	// send, not at startup.
	mail, err := mailer.New(cfg.SMTPURL, cfg.MailFrom)
	if err != nil {
		logger.Error("mailer configuration failed", "error", err)
		return nil, nil, err
	}

	authRepo := auth.NewRepository(db)
	authSvc := auth.NewService(authRepo, cfg.AppURL, mail)
	authHandler := auth.NewHandler(authSvc, logger, cfg.Env != "development", clientIP)
	teamHandler := auth.NewTeamHandler(authSvc, logger)
	organizationHandler := auth.NewOrganizationHandler(authSvc, logger)
	securityHandler := auth.NewSecurityHandler(authSvc, logger, clientIP)
	totpHandler := auth.NewTOTPHandler(authSvc, logger, cfg.Env != "development")

	referralSvc := referral.NewService(referral.NewRepository(db))
	authSvc.SetReferralRecorder(referralSvc)

	// A misconfigured/missing TOTP key must fail startup loudly, not run
	// real 2FA against a well-known or empty key — same "fail loudly at
	// boot" bar as every other required-secret parse in this file.
	totpKey, err := totpengine.ParseKey(cfg.TOTPEncryptionKey)
	if err != nil {
		logger.Error("TOTP encryption key configuration failed", "error", err)
		return nil, nil, err
	}
	authSvc.SetTOTPKey(totpKey)

	// Same "fail loudly at boot" bar as TOTP's own key, above — a
	// misconfigured/missing key must never silently run Settings >
	// Integrations credential storage with a well-known key.
	integrationKey, err := integrationsengine.ParseKey(cfg.IntegrationEncryptionKey)
	if err != nil {
		logger.Error("integration encryption key configuration failed", "error", err)
		return nil, nil, err
	}

	// Same bar again — Google Ads' developer token/client secret/refresh
	// token (apps/internal/adaccount) must never run with a missing or
	// well-known key.
	adAccountKey, err := adaccount.ParseKey(cfg.AdAccountEncryptionKey)
	if err != nil {
		logger.Error("ad account encryption key configuration failed", "error", err)
		return nil, nil, err
	}

	// Pixels' access tokens (apps/internal/pixel), the same bar.
	pixelKey, err := pixel.ParseKey(cfg.PixelEncryptionKey)
	if err != nil {
		logger.Error("pixel encryption key configuration failed", "error", err)
		return nil, nil, err
	}

	// One apikey.Service both authenticates Bearer requests (tenant
	// middleware) and serves the Settings > API keys routes below.
	apiKeySvc := apikey.NewService(apikey.NewRepository(db), logger)

	// RequireSameOrigin runs before session resolution (§54): refusing a
	// forged cross-origin mutation is cheaper and safer than looking up
	// whose session it claims. Bearer requests carry no Origin and pass.
	// tenantMiddleware is the two composed.
	csrfCheck := tenant.RequireSameOrigin(cfg.AppURL, logger)
	sessionCheck := tenant.NewMiddleware(authSvc, apiKeySvc, logger, cfg.Env != "development")
	tenantMiddleware := func(next http.Handler) http.Handler { return csrfCheck(sessionCheck(next)) }

	// identify puts the org/API-key identity into context, best-effort,
	// before the general limiter (§54, tenant.PeekIdentity). The limiter
	// keys API-key traffic by organization, so one integration does not
	// share a bucket with everything behind its IP; browser and
	// unauthenticated traffic is keyed by client IP.
	identify := tenant.PeekIdentity(authSvc, apiKeySvc, logger)
	generalLimitKey := func(r *http.Request) string {
		if scope, ok := tenant.APIKeyScope(r.Context()); ok && scope != "" {
			if orgID, ok := tenant.OrgID(r.Context()); ok {
				return "org:" + orgID
			}
		}
		return "ip:" + clientIP.String(r)
	}
	srv = httpserver.New(logger, cfg.OTelServiceName, db, ch, cfg.AppURL, limiter, generalLimitKey, identify,
		httpserver.WithRecoverer(errtrack.Middleware(reporter, logger)),
		httpserver.WithHSTS(cfg.Env != "development"))

	// §52 RBAC: every mutating route needs its write permission, every
	// tenant route group its read permission (readGate) — the frontend's
	// hidden buttons are never the only guard (rbac_coverage_test.go,
	// rbac_read_coverage_test.go).
	requireCampaignWrite := authSvc.RequirePermission("campaign.write", logger)
	requireOfferWrite := authSvc.RequirePermission("offer.write", logger)
	requireSourceWrite := authSvc.RequirePermission("source.write", logger)
	requireContentWrite := authSvc.RequirePermission("content.write", logger)
	requireSettingsWrite := authSvc.RequirePermission("settings.write", logger)
	// readGate guards a whole route group with its read permission (the
	// Team page's "View …" rows). Writes inside still need their own
	// write permission; every role holding one also holds the read. Before
	// this only team.read was checked, so a Viewer without offer.read or
	// source.read still read offers and sources.
	readGate := func(permission string) func(http.Handler) http.Handler {
		return authSvc.RequirePermission(permission, logger)
	}

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
	// Strict: still limited, in process, while Redis is down (tech audit
	// API2); the per-account limit in Postgres (internal/auth) holds too.
	authLimit := limiter.StrictMiddleware("auth", clientIP.String, authRateLimit, authRateLimitWindow)
	srv.Mux().Group(func(r chi.Router) {
		r.Use(csrfCheck)
		r.Use(authLimit)
		authHandler.RegisterPublic(r)
		totpHandler.RegisterPublic(r)
	})
	srv.Mux().Group(func(r chi.Router) {
		r.Use(tenantMiddleware)
		authHandler.RegisterAuthenticated(r)
		teamHandler.Register(r)
		organizationHandler.Register(r)
		securityHandler.Register(r)
		totpHandler.RegisterAuthenticated(r)
	})

	campaignRepo := campaign.NewRepository(db)
	campaignSvc := campaign.NewService(campaignRepo)
	campaignHandler := campaign.NewHandler(campaignSvc, requireCampaignWrite, logger)
	srv.Mux().Route("/campaigns", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("campaign.read"))
		campaignHandler.Register(r)
	})

	trafficSourceHandler := trafficsource.NewHandler(trafficsource.NewService(trafficsource.NewRepository(db)), requireSourceWrite, logger)
	srv.Mux().Route("/traffic-sources", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("source.read"))
		trafficSourceHandler.Register(r)
	})

	trackingLinkHandler := trackinglink.NewHandler(trackinglink.NewService(trackinglink.NewRepository(db)), requireCampaignWrite, logger)
	srv.Mux().Route("/campaigns/{campaignId}/tracking-links", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("campaign.read"))
		trackingLinkHandler.Register(r)
	})

	adAccountRepo := adaccount.NewRepository(db, adAccountKey)
	adAccountHandler := adaccount.NewHandler(adaccount.NewService(adAccountRepo), requireSourceWrite, logger)

	integrationsSvc := integrationsengine.NewService(integrationsengine.NewRepository(db), integrationKey, adAccountRepo)
	integrationsHandler := integrationsengine.NewHandler(
		integrationsSvc,
		requireSettingsWrite,
		logger,
	)
	srv.Mux().Route("/integrations", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("settings.write"))
		integrationsHandler.Register(r)
	})

	// Registrar/DNS automation (§30): domain.Service reads Namecheap and
	// Cloudflare credentials through integrationsSvc. GoDaddy, Route53,
	// cloudflare_registrar and unmanaged have no adapter and stay
	// operator-declared.
	domainHandler := domain.NewHandler(
		domain.NewService(domain.NewRepository(db), integrationsSvc, domainnamecheap.New(), domaincloudflare.New(), cfg.TrackerDNSTarget, &tlsprobe.Prober{}),
		requireSettingsWrite,
		logger,
	)
	srv.Mux().Route("/domains", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("campaign.read"))
		domainHandler.Register(r)
	})

	networkHandler := network.NewHandler(network.NewService(network.NewRepository(db)), requireOfferWrite, logger)
	srv.Mux().Route("/networks", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("offer.read"))
		networkHandler.Register(r)
	})

	landingHandler := landing.NewHandler(landing.NewService(landing.NewRepository(db), storageClient, logger), requireContentWrite, logger)
	srv.Mux().Route("/landings", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("content.read"))
		landingHandler.Register(r)
	})

	pwaSvc := pwa.NewService(pwa.NewRepository(db))
	// The PWA constructor's "open test page" needs the same click-link key
	// apps/tracker verifies with, and the tracker's browser-reachable URL.
	// Either missing → the button answers Unavailable; nothing else is
	// affected.
	if key, err := landinglink.ParseKey(cfg.LandingLinkKey); err != nil || cfg.TrackerPublicURL == "" {
		logger.Warn("PWA test links disabled", "error", err, "tracker_public_url_set", cfg.TrackerPublicURL != "")
	} else {
		pwaSvc.SetTestLinks(pwa.TestLinks{Signer: landinglink.NewSigner(key), TrackerURL: cfg.TrackerPublicURL, AppURL: cfg.AppURL})
	}
	pwaHandler := pwa.NewHandler(pwaSvc, requireContentWrite, logger)
	srv.Mux().Route("/pwas", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("content.read"))
		pwaHandler.Register(r)
	})

	contentGalleryHandler := contentgallery.NewHandler(contentgallery.NewService(contentgallery.NewRepository(db), storageClient), requireContentWrite, logger)
	srv.Mux().Route("/content-gallery", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("content.read"))
		contentGalleryHandler.Register(r)
	})

	// Alerts (marketer audit #10): rules under Settings, the bell's feed.
	alertHandler := alert.NewHandler(alert.NewService(alert.NewRepository(db)), requireSettingsWrite, logger)
	srv.Mux().Route("/alert-rules", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("settings.write"))
		alertHandler.RegisterRules(r)
	})
	srv.Mux().Route("/notifications", func(r chi.Router) {
		r.Use(tenantMiddleware)
		alertHandler.RegisterNotifications(r)
	})

	tagsHandler := tags.NewHandler(tags.NewService(tags.NewRepository(db)), requireContentWrite, logger)
	srv.Mux().Route("/tags", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("content.read"))
		tagsHandler.Register(r)
	})

	customMetricsHandler := custommetrics.NewHandler(
		custommetrics.NewService(custommetrics.NewRepository(db)),
		authSvc.RequirePermission("custom_metric.write", logger),
		logger,
	)
	srv.Mux().Route("/custom-metrics", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("custom_metric.read"))
		customMetricsHandler.Register(r)
	})

	// Web Push (internal/push, docs/push.md): subscribers and broadcasts.
	// The VAPID keys live with the worker; the api only needs to know a
	// pair exists, to refuse broadcasts nothing could send.
	pushHandler := push.NewHandler(
		push.NewService(push.NewRepository(db), cfg.PushVAPIDPublicKey != ""),
		authSvc.RequirePermission("push.write", logger),
		logger,
	)
	srv.Mux().Route("/push", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("push.read"))
		pushHandler.Register(r)
	})

	apiKeyHandler := apikey.NewHandler(
		apiKeySvc,
		requireSettingsWrite,
		logger,
	)
	srv.Mux().Route("/api-keys", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("settings.write"))
		apiKeyHandler.Register(r)
	})

	reportPresetsHandler := reportpresets.NewHandler(
		reportpresets.NewService(reportpresets.NewRepository(db)),
		authSvc.RequirePermission("report_preset.write", logger),
		logger,
	)
	srv.Mux().Route("/report-presets", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("report_preset.read"))
		reportPresetsHandler.Register(r)
	})

	referralHandler := referral.NewHandler(
		referralSvc,
		authSvc.RequirePermission("referral.write", logger),
		logger,
	)
	srv.Mux().Route("/referral", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("referral.read"))
		referralHandler.Register(r)
	})

	postlandingHandler := postlanding.NewHandler(postlanding.NewService(postlanding.NewRepository(db)), requireContentWrite, logger)
	srv.Mux().Route("/postlandings", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("content.read"))
		postlandingHandler.Register(r)
	})

	pixelHandler := pixel.NewHandler(pixel.NewService(pixel.NewRepository(db, pixelKey)), requireContentWrite, logger)
	srv.Mux().Route("/pixels", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("content.read"))
		pixelHandler.Register(r)
	})

	offerHandler := offer.NewHandler(offer.NewService(offer.NewRepository(db)), requireOfferWrite, logger)
	srv.Mux().Route("/offers", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("offer.read"))
		offerHandler.Register(r)
	})

	// Today's count per capped offer, from the snapshot routing uses — the
	// offers list's "cap reached" badge.
	srv.Mux().With(tenantMiddleware, readGate("offer.read")).Get("/offer-caps", offercap.Handler(offerCaps))

	eventMappingHandler := eventmapping.NewHandler(eventmapping.NewService(eventmapping.NewRepository(db)), requireOfferWrite, logger)
	srv.Mux().Route("/event-mappings", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("offer.read"))
		eventMappingHandler.Register(r)
	})

	costSvc := cost.NewService(cost.NewRepository(db), conversion.NewPostgresFX(db))
	costHandler := cost.NewHandler(costSvc, requireCampaignWrite, logger)
	srv.Mux().Route("/campaigns/{campaignId}/cost-entries", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("campaign.read"))
		costHandler.Register(r)
	})

	// Ad-spend sync (§74/§27-COST): "Sync now" pulls a traffic source's
	// connected ad account's daily campaign spend and writes it into
	// cost_entries. The adapters are tested against fake servers only. Registered in the same Route() block as
	// adAccountHandler (both mount under /traffic-sources/{id}/connection)
	// — chi panics if two separate Route() calls claim the identical
	// pattern, so this can't be two Mount points even though the two
	// handlers are otherwise independent packages.
	costSyncHandler := costsync.NewHandler(costsync.NewService(adAccountRepo, campaignRepo, costSvc, costsync.Providers{
		FacebookAds: facebookads.New(),
		TikTokAds:   tiktokads.New(),
		GoogleAds:   googleads.New(),
	}), requireSourceWrite, logger)
	srv.Mux().Route("/traffic-sources/{id}/connection", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("source.read"))
		adAccountHandler.Register(r)
		costSyncHandler.Register(r)
	})

	streamSetHandler := streamset.NewHandler(streamset.NewService(streamset.NewRepository(db)), requireCampaignWrite, logger)
	srv.Mux().Route("/campaigns/{campaignId}/stream-sets", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("campaign.read"))
		streamSetHandler.Register(r)
	})
	srv.Mux().Route("/flows", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("campaign.read"))
		streamSetHandler.RegisterFlows(r)
	})

	routingSimulateHandler := routingsimulate.NewHandler(
		routingsimulate.NewService(routingstore.New(db), &routing.Engine{}).WithCaps(offerCaps), logger,
	)
	srv.Mux().Route("/campaigns/{campaignId}/routing/simulate", func(r chi.Router) {
		r.Use(tenantMiddleware)
		r.Use(readGate("campaign.read"))
		routingSimulateHandler.Register(r)
	})

	if ch != nil {
		events := chstore.NewEventStore(ch)

		analyticsSvc := analytics.NewService(events, costSvc)
		analyticsHandler := analytics.NewHandler(analyticsSvc, logger)
		srv.Mux().Route("/analytics", func(r chi.Router) {
			r.Use(tenantMiddleware)
			r.Use(readGate("analytics.read"))
			analyticsHandler.Register(r)
		})

		ltvSvc := ltv.NewService(events)
		ltvHandler := ltv.NewHandler(ltvSvc, logger)
		srv.Mux().Route("/analytics/ltv", func(r chi.Router) {
			r.Use(tenantMiddleware)
			r.Use(readGate("analytics.read"))
			ltvHandler.Register(r)
		})

		conversionsHandler := conversions.NewHandler(conversions.NewService(events), logger)
		srv.Mux().Route("/conversions", func(r chi.Router) {
			r.Use(tenantMiddleware)
			r.Use(readGate("analytics.read"))
			conversionsHandler.Register(r)
		})

		clickLogsHandler := clicklogs.NewHandler(clicklogs.NewService(events), logger)
		srv.Mux().Route("/click-log", func(r chi.Router) {
			r.Use(tenantMiddleware)
			r.Use(readGate("analytics.read"))
			clickLogsHandler.Register(r)
		})

		// Incoming postback replay records through conversion.Service, built
		// as apps/tracker builds it (docs/postback-logs.md). Redis stays
		// optional, as in the tracker: recording a postback (§45) must not
		// depend on it. The block runs only with ClickHouse connected, so
		// attribution needs no fallback. rdb is the client from above.
		postbackStore := postback.NewPostgresStore(db)
		var convStore conversion.Store = conversion.NewPostgresStore(db).WithDeliveries(postbackStore).WithPixels(pixelsend.NewPostgresStore(db))
		if rdb != nil {
			convStore = conversion.NewRedisStore(convStore, rdb, logger)
		} else {
			logger.Warn("redis unavailable, conversion progression checks will read Postgres directly")
		}

		attributionSvc := attribution.NewService(chstore.NewClickResolver(chConn))
		attemptLog := postbacklog.NewConversionAttemptLogger(postbacklog.NewPostgresQueue(db, logger))

		convSvc := conversion.NewService(
			conversion.NewPostgresMapper(db),
			convStore,
			conversion.NewPostgresFX(db),
			attributionSvc,
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
			requireOfferWrite,
			logger,
		)
		srv.Mux().Route("/postback-logs", func(r chi.Router) {
			r.Use(tenantMiddleware)
			r.Use(readGate("offer.read"))
			postbackLogsHandler.Register(r)
		})
	}

	return srv, closeAll, nil
}
