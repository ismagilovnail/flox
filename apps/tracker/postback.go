package main

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// PostbackHandler is Phase 23's conversion engine endpoint (§45):
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
	ctx := r.Context()
	networkID := chi.URLParam(r, "networkId")

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

	p := conversion.Postback{
		OrganizationID:   network.OrganizationID,
		NetworkID:        network.ID,
		AcceptDuplicates: network.AcceptDuplicates,
		ClickID:          r.FormValue("click_id"),
		ExternalClickID:  r.FormValue("external_click_id"),
		RawStatus:        r.FormValue("status"),
		Currency:         r.FormValue("currency"),
		NetworkTxnID:     firstNonEmpty(r.FormValue("txn_id"), r.FormValue("transaction_id")),
		OccurredAt:       time.Now().UTC(),
	}

	if raw := r.FormValue("revenue"); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, postbackResponse{Result: "error", Message: "invalid revenue"})
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
