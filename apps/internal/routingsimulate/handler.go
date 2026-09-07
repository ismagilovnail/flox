package routingsimulate

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

type Handler struct {
	svc    *Service
	logger *slog.Logger
}

func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Register mounts POST /campaigns/{campaignId}/routing/simulate. The
// caller applies tenant.Middleware first — same contract as every other
// handler.
func (h *Handler) Register(r chi.Router) {
	// POST because it takes a simulated-visit body, but it resolves
	// routing in memory and persists nothing — deliberately not
	// behind campaign.write, so a read-only role can still explain
	// why a click would route where it does (§72).
	r.Post("/", h.simulate)
}

type simulateRequest struct {
	Request map[string]string `json:"request"`
}

func (h *Handler) simulate(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	campaignID := chi.URLParam(r, "campaignId")

	var req simulateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	resp, err := h.svc.Simulate(r.Context(), orgID, campaignID, req.Request)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
