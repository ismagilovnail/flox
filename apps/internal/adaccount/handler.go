package adaccount

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

// Register mounts every /traffic-sources/{id}/connection route onto r.
// The caller applies tenant.Middleware first — same contract as every
// other handler. A single-resource sub-route (no {connectionId} in the
// path) since a traffic source has at most one connection — GET reads
// it, PATCH connects/reconnects (matching this codebase's convention of
// PATCH for "update/replace in place," same as Stream Sets' whole-array
// Flows replacement), DELETE disconnects.
func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.get)
	r.Patch("/", h.connect)
	r.Delete("/", h.disconnect)
}

func trafficSourceID(r *http.Request) string { return chi.URLParam(r, "id") }

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	c, err := h.svc.Get(r.Context(), orgID, trafficSourceID(r))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

type connectRequest struct {
	AdAccountID string `json:"adAccountId"`
	AccessToken string `json:"accessToken"`
}

func (h *Handler) connect(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())

	var req connectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	c, err := h.svc.Connect(r.Context(), orgID, trafficSourceID(r), ConnectInput{
		AdAccountID: req.AdAccountID,
		AccessToken: req.AccessToken,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) disconnect(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	if err := h.svc.Disconnect(r.Context(), orgID, trafficSourceID(r)); err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
