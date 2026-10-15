package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/clientip"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/ratelimit"
)

// A client IP with this many refused postbacks (unknown network, wrong
// secret) within the window gets 429 for the rest of it, before any
// Postgres lookup (tech audit P4). Accepted postbacks never
// count, so a network's replay of any size is not limited.
const (
	postbackFailureLimit  = 30
	postbackFailureWindow = 10 * time.Minute
)

// PostbackHandler is the conversion engine's endpoint (§45):
//
//	GET/POST /postback/{networkId}
//
// {networkId} — not a request body field or header — is where
// OrganizationID comes from (CLAUDE.md #5): PostgresNetworkLookup resolves
// it before a conversion.Postback is even constructed, so nothing
// downstream of this handler can source a tenant scope from anywhere else.
//
// This is deliberately not on the same latency budget as Handler.track
// (non-negotiable #9 is specifically the redirect path); a postback does
// real work — mapping, attribution, an FX lookup, a durable write — before
// it can answer.
type PostbackHandler struct {
	networks conversion.NetworkLookup
	service  *conversion.Service
	// failures and clientIP limit refused postbacks per IP; nil failures
	// turns the limit off.
	failures *ratelimit.Failures
	clientIP *clientip.Resolver
	logger   *slog.Logger
}

func (h *PostbackHandler) Register(r chi.Router) {
	r.Get("/postback/{networkId}", h.handle)
	r.Post("/postback/{networkId}", h.handle)
}

type postbackResponse struct {
	Result  string `json:"result"`
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
}

func (h *PostbackHandler) handle(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rw := &postbackResultWriter{ResponseWriter: w, result: "internal_error"}
	w = rw
	defer func() {
		metrics.PostbackIncomingLatencySeconds.Observe(time.Since(start).Seconds())
		metrics.PostbackIncomingTotal.WithLabelValues(rw.result).Inc()
	}()
	ctx := r.Context()
	networkID := chi.URLParam(r, "networkId")

	if h.failures != nil {
		ip := h.clientIP.String(r)
		if h.failures.Blocked(ctx, ip) {
			writeJSON(w, http.StatusTooManyRequests, postbackResponse{Result: "error", Message: "too many refused postbacks, try again later"})
			return
		}
		defer func() {
			if rw.result == "unknown_network" || rw.result == "unauthorized" {
				h.failures.Record(context.WithoutCancel(ctx), ip)
			}
		}()
	}

	if !idgen.IsValid(networkID) {
		writeJSON(w, http.StatusNotFound, postbackResponse{Result: "error", Message: "unknown network"})
		return
	}

	network, err := h.networks.ByID(ctx, networkID)
	switch {
	case errors.Is(err, conversion.ErrNetworkNotFound):
		writeJSON(w, http.StatusNotFound, postbackResponse{Result: "error", Message: "unknown network"})
		return
	case err != nil:
		h.logger.Error("looking up network", "error", err, "network_id", networkID)
		writeJSON(w, http.StatusInternalServerError, postbackResponse{Result: "error", Message: "internal error"})
		return
	}

	// FormValue reads both query params (the common GET-postback shape)
	// and a POST's urlencoded body through one call, matching §45's
	// "GET /postback / POST /postback" without duplicating parsing.
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, postbackResponse{Result: "error", Message: "could not parse request"})
		return
	}

	// §54: {networkId} alone is guessable, so the incoming postback URL
	// FLOX hands the network operator carries a secret (migration 00021,
	// network.Service.Create/RegeneratePostbackSecret). An empty stored
	// hash can never match any hashed input, so it rejects every postback
	// without a separate check.
	if !validPostbackSecret(r.FormValue("secret"), network.PostbackSecretHash) {
		writeJSON(w, http.StatusUnauthorized, postbackResponse{Result: "error", Message: "invalid or missing secret"})
		return
	}

	p := conversion.Postback{
		OrganizationID:   network.OrganizationID,
		NetworkID:        network.ID,
		AcceptDuplicates: network.AcceptDuplicates,
		ClickID:          r.FormValue("click_id"),
		ExternalClickID:  r.FormValue("external_click_id"),
		RawStatus:        r.FormValue("status"),
		Currency:         r.FormValue("currency"),
		NetworkCurrency:  network.Currency,
		NetworkTxnID:     firstNonEmpty(r.FormValue("txn_id"), r.FormValue("transaction_id")),
		PostbackURL:      network.PostbackURL,
		OccurredAt:       time.Now().UTC(),
	}

	// When the conversion happened at the network (tech audit T3); without
	// it, the moment the postback arrived.
	if raw := firstNonEmpty(r.FormValue("event_time"), r.FormValue("ts")); raw != "" {
		at, err := conversion.ParseEventTime(raw, p.OccurredAt)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, postbackResponse{Result: "error", Message: strings.TrimPrefix(err.Error(), "conversion: ")})
			return
		}
		p.OccurredAt = at
	}

	if raw := r.FormValue("revenue"); raw != "" {
		v, err := conversion.ParseRevenue(raw)
		if err != nil {
			// 400, not 500: the same request will never become valid, so
			// the network must not retry it.
			writeJSON(w, http.StatusBadRequest, postbackResponse{Result: "error", Message: strings.TrimPrefix(err.Error(), "conversion: ")})
			return
		}
		p.Revenue = &v
	}

	result, err := h.service.Record(ctx, p)
	if err != nil {
		// An infrastructure failure, not an understood-and-handled
		// outcome (conversion.Service reserves error returns for exactly
		// this). 500 tells the network to retry rather than silently
		// swallowing a conversion.
		h.logger.Error("recording conversion", "error", err, "network_id", networkID)
		writeJSON(w, http.StatusInternalServerError, postbackResponse{Result: "error", Message: "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, postbackResponse{
		Result:  string(result.Kind),
		Status:  string(result.Status),
		Message: result.Message,
	})
}

// validPostbackSecret hashes the incoming request's secret and compares
// it against the network's stored hash using a constant-time comparison
// (subtle.ConstantTimeCompare) — an early-exit == comparison here would
// leak how many leading bytes matched through response timing, letting an
// attacker recover the secret byte-by-byte.
func validPostbackSecret(provided, storedHash string) bool {
	if provided == "" || storedHash == "" {
		return false
	}
	sum := sha256.Sum256([]byte(provided))
	providedHash := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(providedHash), []byte(storedHash)) == 1
}

// postbackResultWriter remembers the result label of the response written,
// for postback_incoming_total — from the status code before the engine ran,
// from the engine's own result after.
type postbackResultWriter struct {
	http.ResponseWriter
	result string
}

func (w *postbackResultWriter) label(status int, body postbackResponse) {
	switch status {
	case http.StatusOK:
		w.result = body.Result
	case http.StatusNotFound:
		w.result = "unknown_network"
	case http.StatusUnauthorized:
		w.result = "unauthorized"
	case http.StatusBadRequest:
		w.result = "bad_request"
	case http.StatusTooManyRequests:
		w.result = "rate_limited"
	default:
		w.result = "internal_error"
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	if rw, ok := w.(*postbackResultWriter); ok {
		if body, ok := v.(postbackResponse); ok {
			rw.label(status, body)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
