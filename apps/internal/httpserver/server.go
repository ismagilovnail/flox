// Package httpserver builds apps/api's chi router: request ID, structured
// request logging, panic recovery, OTel instrumentation, and the health/
// readiness/metrics endpoints (§33, §53). Route registration for real
// resources (campaigns, offers, ...) lives in each domain package
// (internal/campaign/handler.go, ...) and is mounted onto Server.Mux()
// from cmd/api/main.go — this file only owns cross-cutting middleware and
// the endpoints every phase needs.
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
func New(logger *slog.Logger, serviceName string, db Pinger, ch Pinger, appURL string) *Server {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(responseRequestID)
	r.Use(requestLogger(logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{appURL},
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/health", healthHandler)
	r.Get("/ready", readyHandler(db, ch))
	// §53/Phase 29: unauthenticated, same convention as /health — a
	// scrape target on an internal network, never a route a browser hits.
	r.Handle("/metrics", metrics.Handler())

	return &Server{mux: r, router: otelhttp.NewHandler(r, serviceName)}
}

// Mux is where domain packages mount their own route groups, e.g.:
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
