package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

// OrganizationHandler serves /organization — this org's own settings
// (§30: name/timezone/currency, real columns since migration 00001).
// Read is open to any authenticated member (no distinct
// "organization.read"/"settings.read" permission exists in the
// vocabulary team.ts/migration 00020 already established — everyone in
// an org can see its own name/timezone/currency); writes require
// settings.write, the same permission key that vocabulary already
// reserved for exactly this and grants Owner/Admin only.
type OrganizationHandler struct {
	svc    *Service
	logger *slog.Logger
}

func NewOrganizationHandler(svc *Service, logger *slog.Logger) *OrganizationHandler {
	return &OrganizationHandler{svc: svc, logger: logger}
}

func (h *OrganizationHandler) Register(r chi.Router) {
	requireWrite := h.svc.RequirePermission("settings.write", h.logger)
	r.Get("/organization", h.get)
	r.With(requireWrite).Patch("/organization", h.update)
}

type organizationSettingsResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
	Currency string `json:"currency"`
}

func toOrganizationSettingsResponse(o OrganizationSettings) organizationSettingsResponse {
	return organizationSettingsResponse{ID: o.ID, Name: o.Name, Timezone: o.Timezone, Currency: o.Currency}
}

func (h *OrganizationHandler) get(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	o, err := h.svc.GetOrganizationSettings(r.Context(), orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, toOrganizationSettingsResponse(o))
}

type updateOrganizationRequest struct {
	Name     *string `json:"name"`
	Timezone *string `json:"timezone"`
	Currency *string `json:"currency"`
}

func (h *OrganizationHandler) update(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	var req updateOrganizationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	o, err := h.svc.UpdateOrganizationSettings(r.Context(), orgID, userID, UpdateOrganizationSettingsInput{
		Name: req.Name, Timezone: req.Timezone, Currency: req.Currency,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, toOrganizationSettingsResponse(o))
}
