// Package tlsask answers a TLS proxy's question "may I get a certificate
// for this hostname?" — Caddy's on_demand_tls `ask` endpoint
// (docs/deployment.md). Marketer audit 2026-10-06, #4.
//
// The answer is yes only for a hostname some organization has added on the
// Domains page. Without this check anyone could point a name at the
// tracker and make the proxy request certificates for it until the CA's
// rate limits lock out the real domains.
package tlsask

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Path is where the tracker serves the check. The proxy calls it on the
// tracker's internal address; it should not be proxied to the public.
const Path = "/internal/tls-ask"

func Handler(db *pgxpool.Pool, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain")))
		if host == "" {
			http.Error(w, "domain is required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		var known bool
		// Only an active (ownership-verified) domain gets a certificate:
		// anyone can add a name, only its DNS owner can verify it (tech
		// audit T6).
		err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM domains WHERE lower(domain) = $1 AND status = 'active')`, host).Scan(&known)
		if err != nil {
			logger.Error("tls ask: looking up domain", "domain", host, "error", err)
			http.Error(w, "lookup failed", http.StatusServiceUnavailable)
			return
		}
		if !known {
			http.Error(w, "unknown domain", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}
