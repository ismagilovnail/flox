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

// Register mounts GET/PUT /offline-conversions (Google Ads) and
// GET/PUT/DELETE /metrica (Yandex Metrica) under
// /traffic-sources/{id}/connection (apps/api, next to adaccount's routes).
func (h *Handler) Register(r chi.Router) {
	r.Get("/offline-conversions", h.get)
	r.With(h.requireWrite).Put("/offline-conversions", h.update)
	r.Get("/metrica", h.getMetrica)
	r.With(h.requireWrite).Put("/metrica", h.putMetrica)
	r.With(h.requireWrite).Delete("/metrica", h.deleteMetrica)
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

func (h *Handler) getMetrica(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	st, err := h.svc.GetMetrica(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

type metricaRequest struct {
	Enabled        bool   `json:"enabled"`
	CounterID      string `json:"counterId"`
	Token          string `json:"token"`
	Currency       string `json:"currency"`
	LeadGoal       string `json:"leadGoal"`
	PurchaseGoal   string `json:"purchaseGoal"`
	RepurchaseGoal string `json:"repurchaseGoal"`
}

func (h *Handler) putMetrica(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req metricaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	st, err := h.svc.PutMetrica(r.Context(), orgID, chi.URLParam(r, "id"), MetricaInput{
		Token: req.Token,
		MetricaSettings: MetricaSettings{
			Enabled: req.Enabled, CounterID: req.CounterID, Currency: req.Currency,
			LeadGoal: req.LeadGoal, PurchaseGoal: req.PurchaseGoal, RepurchaseGoal: req.RepurchaseGoal,
		},
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	audit.Annotate(r.Context(), audit.Entry{Action: "metrica_connection.updated", EntityType: "traffic_source", EntityID: chi.URLParam(r, "id")})
	writeJSON(w, http.StatusOK, st)
}

func (h *Handler) deleteMetrica(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	if err := h.svc.DeleteMetrica(r.Context(), orgID, chi.URLParam(r, "id")); err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	audit.Annotate(r.Context(), audit.Entry{Action: "metrica_connection.deleted", EntityType: "traffic_source", EntityID: chi.URLParam(r, "id")})
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
