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
	"github.com/ismagilovnail/flox/apps/internal/eventmapping"
	"github.com/ismagilovnail/flox/apps/internal/httpserver"
	integrationsengine "github.com/ismagilovnail/flox/apps/internal/integrations"
	"github.com/ismagilovnail/flox/apps/internal/landing"
	"github.com/ismagilovnail/flox/apps/internal/landinglink"
	"github.com/ismagilovnail/flox/apps/internal/logging"
	"github.com/ismagilovnail/flox/apps/internal/ltv"
	"github.com/ismagilovnail/flox/apps/internal/mailer"
	"github.com/ismagilovnail/flox/apps/internal/network"
	"github.com/ismagilovnail/flox/apps/internal/offer"
	"github.com/ismagilovnail/flox/apps/internal/offercap"
	"github.com/ismagilovnail/flox/apps/internal/pixel"
	"github.com/ismagilovnail/flox/apps/internal/postback"
	"github.com/ismagilovnail/flox/apps/internal/postbacklog"
	"github.com/ismagilovnail/flox/apps/internal/postbacklogs"
	"github.com/ismagilovnail/flox/apps/internal/postgres"
	"github.com/ismagilovnail/flox/apps/internal/postlanding"
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

	// clientIP resolves the real client address behind TRUSTED_PROXIES,
	// replacing every previously-unconditional X-Forwarded-For read
	// (ratelimit.ClientIP, chi's middleware.RealIP) — security audit
	// finding #2. A malformed TRUSTED_PROXIES entry fails startup rather
	// than silently narrowing or widening who gets trusted, the same
	// stance cfg.TOTPEncryptionKey already takes on a bad key.
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
	} else if migrateErr := chstore.Migrate(ctx, chConn); migrateErr != nil {
		// Idempotent (every statement is CREATE ... IF NOT EXISTS), so this
		// is safe to run from both apps/api and apps/worker regardless of
		// which one starts first — see chstore.Migrate's doc.
		logger.Warn("clickhouse schema migration failed, /analytics will fail until it recovers", "error", migrateErr)
		_ = chConn.Close()
	} else {
		defer chConn.Close()
		ch = chConn
		// Same snapshot the tracker routes with, so the simulator answers
		// "what would happen right now" including offers at their daily cap.
		offerCaps = offercap.NewMonitor(offercap.NewPostgresSource(db), chstore.NewEventStore(chConn), logger)
		go offerCaps.Run(ctx)
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

	// Auth/RBAC (§52, Phase 28A). authSvc.ResolveSession implements
	// tenant.SessionResolver — tenantMiddleware below replaces the old
	// X-Organization-Id-header stand-in tenant.Middleware used to be, for
	// every route mounted under it, with zero changes to those routes'
	// own handlers (they still only ever read tenant.OrgID(ctx)).
	// secureCookies is false in development (a Secure cookie is dropped
	// by browsers over plain http, and this dev stack has no TLS
	// termination in front of apps/api).
	// Signup's email-verification link is the one place this codebase
	// sends a real outbound email (§ auth.Service.Signup's own comment) —
	// mailer.New only parses SMTP_URL, it doesn't dial anything at
	// startup, so an unreachable relay surfaces on the first send
	// attempt, not here.
	mail, err := mailer.New(cfg.SMTPURL, cfg.MailFrom)
	if err != nil {
		logger.Error("mailer configuration failed", "error", err)
		return err
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
		return err
	}
	authSvc.SetTOTPKey(totpKey)

	// Same "fail loudly at boot" bar as TOTP's own key, above — a
	// misconfigured/missing key must never silently run Settings >
	// Integrations credential storage with a well-known key.
	integrationKey, err := integrationsengine.ParseKey(cfg.IntegrationEncryptionKey)
	if err != nil {
		logger.Error("integration encryption key configuration failed", "error", err)
		return err
	}

	// Same bar again — Google Ads' developer token/client secret/refresh
	// token (apps/internal/adaccount) must never run with a missing or
	// well-known key.
	adAccountKey, err := adaccount.ParseKey(cfg.AdAccountEncryptionKey)
	if err != nil {
		logger.Error("ad account encryption key configuration failed", "error", err)
		return err
	}

	// apiKeySvc is constructed here, ahead of tenant.NewMiddleware, so the
	// same instance both authenticates every tenant-scoped request's
	// Authorization: Bearer header (security audit finding #4's real-auth
	// follow-up) and serves the Settings > API keys panel's own CRUD
	// routes further down — one Service, not two.
	apiKeySvc := apikey.NewService(apikey.NewRepository(db), logger)

	// §54/Phase 30: RequireSameOrigin runs OUTSIDE (before) session
	// resolution — rejecting a forged cross-origin mutation is cheaper
	// and safer than first looking up whose session it claims to be. A
	// Bearer-token request naturally has no Origin header (RequireSameOrigin's
	// own doc comment: absence means a non-browser client), so this needs
	// no special-casing for API keys.
	// tenantMiddleware keeps its original name/type (func(http.Handler)
	// http.Handler) even though it's now two middlewares composed, so
	// none of this file's r.Use(tenantMiddleware) call sites change.
	csrfCheck := tenant.RequireSameOrigin(cfg.AppURL, logger)
	sessionCheck := tenant.NewMiddleware(authSvc, apiKeySvc, logger, cfg.Env != "development")
	tenantMiddleware := func(next http.Handler) http.Handler { return csrfCheck(sessionCheck(next)) }

	// §54: closes the "general rate limit stays IP-keyed" gap
	// (ROADMAP.md) — identify resolves a request's org/API-key identity
	// into context, best-effort, before the general limiter below ever
	// runs (tenant.PeekIdentity's own doc comment has the full reasoning
	// and why NewMiddleware, above, doesn't pay for it twice).
	// generalLimitKey is what actually uses that identity: an API-key-
	// authenticated request keys by its organization; everything else
	// (unauthenticated, or session-cookie browser traffic) still keys by
	// client IP, same as before this phase — a browser session isn't the
	// "many unrelated requests sharing one IP" case this is fixing.
	identify := tenant.PeekIdentity(authSvc, apiKeySvc, logger)
	generalLimitKey := func(r *http.Request) string {
		if scope, ok := tenant.APIKeyScope(r.Context()); ok && scope != "" {
			if orgID, ok := tenant.OrgID(r.Context()); ok {
				return "org:" + orgID
			}
		}
		return "ip:" + clientIP.String(r)
	}
	srv := httpserver.New(logger, cfg.OTelServiceName, db, ch, cfg.AppURL, limiter, generalLimitKey, identify)

	// §52 RBAC. Until the 2026-09-06 audit, RequirePermission gated only
	// /team, /security and the four features whose own phases wired it
	// (integrations, custom metrics, API keys, report presets, referral) —
	// so the role -> permission policy migrations 00001/00020 seed was
	// almost entirely unenforced: any member of any role, Viewer included,
	// could create, edit and delete campaigns, offers, networks, traffic
	// sources, domains and stream sets (i.e. rewrite live routing) through
	// the API, with the frontend's hidden buttons the only thing in the
	// way. These five cover every remaining mutating route; reads stay open
	// to any member of the org, matching this codebase's existing split.
	requireCampaignWrite := authSvc.RequirePermission("campaign.write", logger)
	requireOfferWrite := authSvc.RequirePermission("offer.write", logger)
	requireSourceWrite := authSvc.RequirePermission("source.write", logger)
	requireContentWrite := authSvc.RequirePermission("content.write", logger)
	requireSettingsWrite := authSvc.RequirePermission("settings.write", logger)

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
	authLimit := limiter.Middleware("auth", clientIP.String, authRateLimit, authRateLimitWindow)
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
		campaignHandler.Register(r)
	})

	trafficSourceHandler := trafficsource.NewHandler(trafficsource.NewService(trafficsource.NewRepository(db)), requireSourceWrite, logger)
	srv.Mux().Route("/traffic-sources", func(r chi.Router) {
		r.Use(tenantMiddleware)
		trafficSourceHandler.Register(r)
	})

	trackingLinkHandler := trackinglink.NewHandler(trackinglink.NewService(trackinglink.NewRepository(db)), requireCampaignWrite, logger)
	srv.Mux().Route("/campaigns/{campaignId}/tracking-links", func(r chi.Router) {
		r.Use(tenantMiddleware)
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
		integrationsHandler.Register(r)
	})

	// Registrar/DNS automation (§30): domain.Service reads real
	// Namecheap/Cloudflare credentials through integrationsSvc rather
	// than a second copy of them — GoDaddy/Route53/cloudflare_registrar/
	// unmanaged have no real adapter and stay plain operator-declared,
	// same as before this phase.
	domainHandler := domain.NewHandler(
		domain.NewService(domain.NewRepository(db), integrationsSvc, domainnamecheap.New(), domaincloudflare.New(), cfg.TrackerDNSTarget),
		requireSettingsWrite,
		logger,
	)
	srv.Mux().Route("/domains", func(r chi.Router) {
		r.Use(tenantMiddleware)
		domainHandler.Register(r)
	})

	networkHandler := network.NewHandler(network.NewService(network.NewRepository(db)), requireOfferWrite, logger)
	srv.Mux().Route("/networks", func(r chi.Router) {
		r.Use(tenantMiddleware)
		networkHandler.Register(r)
	})

	landingHandler := landing.NewHandler(landing.NewService(landing.NewRepository(db), storageClient, logger), requireContentWrite, logger)
	srv.Mux().Route("/landings", func(r chi.Router) {
		r.Use(tenantMiddleware)
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
		pwaHandler.Register(r)
	})

	contentGalleryHandler := contentgallery.NewHandler(contentgallery.NewService(contentgallery.NewRepository(db), storageClient), requireContentWrite, logger)
	srv.Mux().Route("/content-gallery", func(r chi.Router) {
		r.Use(tenantMiddleware)
		contentGalleryHandler.Register(r)
	})

	tagsHandler := tags.NewHandler(tags.NewService(tags.NewRepository(db)), requireContentWrite, logger)
	srv.Mux().Route("/tags", func(r chi.Router) {
		r.Use(tenantMiddleware)
		tagsHandler.Register(r)
	})

	customMetricsHandler := custommetrics.NewHandler(
		custommetrics.NewService(custommetrics.NewRepository(db)),
		authSvc.RequirePermission("custom_metric.write", logger),
		logger,
	)
	srv.Mux().Route("/custom-metrics", func(r chi.Router) {
		r.Use(tenantMiddleware)
		customMetricsHandler.Register(r)
	})

	apiKeyHandler := apikey.NewHandler(
		apiKeySvc,
		requireSettingsWrite,
		logger,
	)
	srv.Mux().Route("/api-keys", func(r chi.Router) {
		r.Use(tenantMiddleware)
		apiKeyHandler.Register(r)
	})

	reportPresetsHandler := reportpresets.NewHandler(
		reportpresets.NewService(reportpresets.NewRepository(db)),
		authSvc.RequirePermission("report_preset.write", logger),
		logger,
	)
	srv.Mux().Route("/report-presets", func(r chi.Router) {
		r.Use(tenantMiddleware)
		reportPresetsHandler.Register(r)
	})

	referralHandler := referral.NewHandler(
		referralSvc,
		authSvc.RequirePermission("referral.write", logger),
		logger,
	)
	srv.Mux().Route("/referral", func(r chi.Router) {
		r.Use(tenantMiddleware)
		referralHandler.Register(r)
	})

	postlandingHandler := postlanding.NewHandler(postlanding.NewService(postlanding.NewRepository(db)), requireContentWrite, logger)
	srv.Mux().Route("/postlandings", func(r chi.Router) {
		r.Use(tenantMiddleware)
		postlandingHandler.Register(r)
	})

	pixelHandler := pixel.NewHandler(pixel.NewService(pixel.NewRepository(db)), requireContentWrite, logger)
	srv.Mux().Route("/pixels", func(r chi.Router) {
		r.Use(tenantMiddleware)
		pixelHandler.Register(r)
	})

	offerHandler := offer.NewHandler(offer.NewService(offer.NewRepository(db)), requireOfferWrite, logger)
	srv.Mux().Route("/offers", func(r chi.Router) {
		r.Use(tenantMiddleware)
		offerHandler.Register(r)
	})

	eventMappingHandler := eventmapping.NewHandler(eventmapping.NewService(eventmapping.NewRepository(db)), requireOfferWrite, logger)
	srv.Mux().Route("/event-mappings", func(r chi.Router) {
		r.Use(tenantMiddleware)
		eventMappingHandler.Register(r)
	})

	costSvc := cost.NewService(cost.NewRepository(db), conversion.NewPostgresFX(db))
	costHandler := cost.NewHandler(costSvc, requireCampaignWrite, logger)
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
		GoogleAds:   googleads.New(),
	}), requireSourceWrite, logger)
	srv.Mux().Route("/traffic-sources/{id}/connection", func(r chi.Router) {
		r.Use(tenantMiddleware)
		adAccountHandler.Register(r)
		costSyncHandler.Register(r)
	})

	streamSetHandler := streamset.NewHandler(streamset.NewService(streamset.NewRepository(db)), requireCampaignWrite, logger)
	srv.Mux().Route("/campaigns/{campaignId}/stream-sets", func(r chi.Router) {
		r.Use(tenantMiddleware)
		streamSetHandler.Register(r)
	})
	srv.Mux().Route("/flows", func(r chi.Router) {
		r.Use(tenantMiddleware)
		streamSetHandler.RegisterFlows(r)
	})

	routingSimulateHandler := routingsimulate.NewHandler(
		routingsimulate.NewService(routingstore.New(db), &routing.Engine{}).WithCaps(offerCaps), logger,
	)
	srv.Mux().Route("/campaigns/{campaignId}/routing/simulate", func(r chi.Router) {
		r.Use(tenantMiddleware)
		routingSimulateHandler.Register(r)
	})

	if ch != nil {
		events := chstore.NewEventStore(ch)

		analyticsSvc := analytics.NewService(events, costSvc)
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

		clickLogsHandler := clicklogs.NewHandler(clicklogs.NewService(events), logger)
		srv.Mux().Route("/click-log", func(r chi.Router) {
			r.Use(tenantMiddleware)
			clickLogsHandler.Register(r)
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
		//
		// Reuses rdb (connected once, near the top of run(), so
		// apps/internal/ratelimit can also use it) rather than opening a
		// second Redis connection.
		postbackStore := postback.NewPostgresStore(db)
		var convStore conversion.Store = conversion.NewPostgresStore(db).WithDeliveries(postbackStore)
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
