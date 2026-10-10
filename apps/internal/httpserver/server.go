// Package httpserver builds apps/api's chi router: request ID, structured
// request logging, panic recovery, OTel instrumentation, a general rate
// limit, and the health/readiness/metrics endpoints (§33, §53, §54).
// Route registration for real resources (campaigns, offers, ...) lives in
// each domain package (internal/campaign/handler.go, ...) and is mounted
// onto Server.Mux() from cmd/api/main.go — this file only owns cross-
// cutting middleware and the endpoints every phase needs.
package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/ratelimit"
)

// generalRateLimit/generalRateLimitWindow: a broad ceiling across every
// domain route (§54/Phase 30) — generous enough not to bother a real
// dashboard session (TanStack Query re-fetching several resources, ⌘K
// searches, ...) while still bounding a scripted abuse attempt. Keyed
// per-organization for API-key-authenticated traffic, per-IP otherwise
// (New's own generalLimitKey param, built in apps/api/main.go) — not
// purely per-IP the way it originally was, so one high-volume
// integration no longer shares a bucket with unrelated traffic behind
// the same address. Auth endpoints layer a much stricter, separate
// limit on top of this one (apps/api/main.go) — this general limit
// alone isn't tuned for brute-force protection.
const (
	generalRateLimit       = 300
	generalRateLimitWindow = time.Minute
)

// Pinger is satisfied by *pgxpool.Pool — kept as a narrow interface here so
// this package doesn't need to import pgx just to define /ready.
type Pinger interface {
	Ping(ctx context.Context) error
}

type Server struct {
	mux    *chi.Mux
	router http.Handler
}

// ch may be nil when the caller has no ClickHouse connection to offer —
// /ready simply skips that check rather than reporting it unavailable.
// appURL is apps/web's own origin, the one CORS-allowed origin (Phase 27:
// the browser calls this API directly from a different origin in dev).
// AllowCredentials must be true (§52/Phase 28): the session cookie
// tenant.NewMiddleware now requires only travels on a cross-origin fetch
// when the browser is told the response allows credentialed requests —
// paired with a single explicit AllowedOrigins entry, never "*", which
// the fetch spec forbids combining with credentials anyway.
// identify is tenant.PeekIdentity, built in apps/api/main.go (this
// package deliberately doesn't import apps/internal/tenant, to keep
// cross-cutting infra and tenancy concerns from tangling) — mounted
// globally, before the general rate limit, so that generalLimitKey (also
// built in main.go, closing over both tenant's context helpers and a
// clientip.Resolver) can key an API-key-authenticated request by its
// organization instead of by client IP — see PeekIdentity's own doc
// comment for why this ordering matters and what it fixes (ROADMAP.md:
// "General rate limit stays IP-keyed").
// Option adjusts New.
type Option func(*options)

type options struct {
	recoverer func(http.Handler) http.Handler
	hsts      bool
}

// WithHSTS adds Strict-Transport-Security to every response — only once
// TLS terminates in front of the API (apps/api passes NODE_ENV !=
// development, the same gate as its Secure session cookie): a browser
// that has seen it refuses plain HTTP to this host for a year.
func WithHSTS(on bool) Option {
	return func(o *options) { o.hsts = on }
}

// securityHeaders hardens every API response (tech audit S2). The API
// serves JSON only, so its CSP allows nothing to load or run and nothing
// to frame it: a response opened directly in a tab, or embedded by another
// site, can do nothing. nosniff keeps a browser from reading JSON as HTML.
func securityHeaders(hsts bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
			h.Set("X-Frame-Options", "DENY")
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "no-referrer")
			if hsts {
				h.Set("Strict-Transport-Security", "max-age=31536000")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// WithRecoverer replaces chi's Recoverer — apps/api passes errtrack's, which
// also reports the panic to the error tracker.
func WithRecoverer(mw func(http.Handler) http.Handler) Option {
	return func(o *options) { o.recoverer = mw }
}

func New(logger *slog.Logger, serviceName string, db Pinger, ch Pinger, appURL string, limiter *ratelimit.Limiter, generalLimitKey func(r *http.Request) string, identify func(http.Handler) http.Handler, opts ...Option) *Server {
	o := options{recoverer: middleware.Recoverer}
	for _, opt := range opts {
		opt(&o)
	}
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(responseRequestID)
	r.Use(securityHeaders(o.hsts))
	r.Use(requestLogger(logger))
	r.Use(o.recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{appURL},
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// identify must run before the general limiter (next line) — it's
	// what populates the context the limiter's own generalLimitKey reads
	// from. Both must come before health/ready/metrics below — chi panics
	// if Use is called after any route is registered on this mux ("all
	// middlewares must be defined before routes") — exemptPaths is what
	// actually keeps an orchestrator's liveness probe or a Prometheus
	// scrape from ever tripping a rate limit meant for abuse on domain
	// routes, not registration order.
	r.Use(identify)
	r.Use(exemptPaths(limiter.Middleware("api", generalLimitKey, generalRateLimit, generalRateLimitWindow), "/health", "/ready", "/metrics"))

	r.Get("/health", healthHandler)
	r.Get("/ready", readyHandler(db, ch))
	// §53/Phase 29: unauthenticated, same convention as /health — a
	// scrape target on an internal network, never a route a browser hits.
	r.Handle("/metrics", metrics.Handler())

	// The API itself is versioned (tech audit 2026-10-06, API3): every
	// domain route lives under /v1, so a breaking change can ship as /v2
	// beside it once integrations call the API with their keys. Health,
	// readiness and metrics are infrastructure, not API, and stay at the
	// root where probes and scrapers look for them.
	v1 := chi.NewRouter()
	r.Mount(APIPrefix, v1)

	return &Server{mux: v1, router: otelhttp.NewHandler(r, serviceName)}
}

// APIPrefix is the path every domain route is served under.
const APIPrefix = "/v1"

// Mux is where domain packages mount their own route groups — under
// APIPrefix, e.g. GET /v1/campaigns:
//
//	srv.Mux().Route("/campaigns", func(r chi.Router) {
//	    r.Use(tenantMiddleware)
//	    campaignHandler.Register(r)
//	})
func (s *Server) Mux() chi.Router {
	return s.mux
}

func (s *Server) Handler() http.Handler {
	return s.router
}

// exemptPaths wraps mw so an exact-match path bypasses it entirely — used
// to keep infra health checks and metrics scrapes off the general rate
// limit without depending on chi route-registration order.
func exemptPaths(mw func(http.Handler) http.Handler, paths ...string) func(http.Handler) http.Handler {
	exempt := make(map[string]bool, len(paths))
	for _, p := range paths {
		exempt[p] = true
	}
	return func(next http.Handler) http.Handler {
		wrapped := mw(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if exempt[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}
			wrapped.ServeHTTP(w, r)
		})
	}
}

// responseRequestID echoes chi's generated request ID back as a response
// header, so a client (or a support engineer pasting a request ID from a
// bug report) can correlate their request with server-side logs/traces.
func responseRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := middleware.GetReqID(r.Context()); id != "" {
			w.Header().Set("X-Request-Id", id)
		}
		next.ServeHTTP(w, r)
	})
}

// requestLogger logs one structured line per request, tagged with chi's
// request ID so it can be correlated with the matching OTel span.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			logger.Info("http_request",
				"request_id", middleware.GetReqID(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}
