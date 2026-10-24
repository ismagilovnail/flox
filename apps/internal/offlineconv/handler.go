package offlineconv

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/audit"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

type Handler struct {
	svc          *Service
	requireWrite func(http.Handler) http.Handler
	logger       *slog.Logger
}

func NewHandler(svc *Service, requireWrite func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, requireWrite: requireWrite, logger: logger}
}

// Register mounts GET/PUT /offline-conversions under
// /traffic-sources/{id}/connection (apps/api, next to adaccount's routes).
func (h *Handler) Register(r chi.Router) {
	r.Get("/offline-conversions", h.get)
	r.With(h.requireWrite).Put("/offline-conversions", h.update)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	st, err := h.svc.Get(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

type updateRequest struct {
	Enabled            bool   `json:"enabled"`
	LoginAccountID     string `json:"loginAccountId"`
	LeadActionID       string `json:"leadActionId"`
	PurchaseActionID   string `json:"purchaseActionId"`
	RepurchaseActionID string `json:"repurchaseActionId"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	st, err := h.svc.Update(r.Context(), orgID, chi.URLParam(r, "id"), Settings{
		Enabled: req.Enabled, LoginAccountID: req.LoginAccountID,
		LeadActionID: req.LeadActionID, PurchaseActionID: req.PurchaseActionID, RepurchaseActionID: req.RepurchaseActionID,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	audit.Annotate(r.Context(), audit.Entry{Action: "offline_conversions.updated", EntityType: "traffic_source", EntityID: chi.URLParam(r, "id")})
	writeJSON(w, http.StatusOK, st)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
