package campaign

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/pagination"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

type Handler struct {
	svc *Service
	// requireWrite is the RBAC middleware every mutating route below is
	// wrapped in (Register). Read routes carry no permission requirement
	// beyond tenantMiddleware — same split custommetrics/reportpresets/
	// referral already use.
	requireWrite func(http.Handler) http.Handler
	// requireManageAll gates reassigning a campaign (campaign.manage_all).
	requireManageAll func(http.Handler) http.Handler
	logger           *slog.Logger
}

func NewHandler(svc *Service, requireWrite, requireManageAll func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, requireWrite: requireWrite, requireManageAll: requireManageAll, logger: logger}
}

// Register mounts every /campaigns route onto r. The caller is responsible
// for applying tenant.Middleware to r first — this handler only reads
// tenant.OrgID from context, it doesn't validate that it's present.
func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.list)
	r.With(h.requireWrite).Post("/", h.create)
	r.Get("/{id}", h.get)
	r.With(h.requireWrite).Patch("/{id}", h.update)
	r.With(h.requireWrite).Delete("/{id}", h.delete)
	r.With(h.requireWrite).Post("/{id}/duplicate", h.duplicate)
	r.With(h.requireWrite).Post("/{id}/pause", h.pause)
	r.With(h.requireWrite).Post("/{id}/activate", h.activate)
	r.With(h.requireManageAll).Put("/{id}/owner", h.setOwner)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())

	page, err := pagination.FromRequest(r)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	filter := ListFilter{Limit: page.Limit, Offset: page.Offset}

	result, err := h.svc.List(r.Context(), orgID, filter)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"campaigns": result.Campaigns, "total": result.Total})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	c, err := h.svc.Get(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

type createRequest struct {
	TrafficSourceID    string `json:"trafficSourceId"`
	Name               string `json:"name"`
	FallbackURL        string `json:"fallbackUrl"`
	Notes              string `json:"notes"`
	ExternalCampaignID string `json:"externalCampaignId"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())

	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	c, err := h.svc.Create(r.Context(), orgID, CreateInput{
		TrafficSourceID:    req.TrafficSourceID,
		Name:               req.Name,
		FallbackURL:        req.FallbackURL,
		Notes:              req.Notes,
		ExternalCampaignID: req.ExternalCampaignID,
		OwnerUserID:        userIDOf(r),
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

type updateRequest struct {
	Name               *string `json:"name"`
	TrafficSourceID    *string `json:"trafficSourceId"`
	FallbackURL        *string `json:"fallbackUrl"`
	Notes              *string `json:"notes"`
	Status             *Status `json:"status"`
	ExternalCampaignID *string `json:"externalCampaignId"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())

	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	c, err := h.svc.Update(r.Context(), orgID, chi.URLParam(r, "id"), UpdateInput{
		Name:               req.Name,
		TrafficSourceID:    req.TrafficSourceID,
		FallbackURL:        req.FallbackURL,
		Notes:              req.Notes,
		Status:             req.Status,
		ExternalCampaignID: req.ExternalCampaignID,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	if err := h.svc.Delete(r.Context(), orgID, chi.URLParam(r, "id")); err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) duplicate(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	c, err := h.svc.Duplicate(r.Context(), orgID, chi.URLParam(r, "id"), userIDOf(r))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *Handler) pause(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	c, err := h.svc.Pause(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) activate(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	c, err := h.svc.Activate(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type ownerRequest struct {
	UserID string `json:"userId"`
}

// setOwner answers PUT /campaigns/{id}/owner {"userId"}: assign the
// campaign to a member, or to nobody with "".
func (h *Handler) setOwner(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req ownerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	c, err := h.svc.SetOwner(r.Context(), orgID, chi.URLParam(r, "id"), req.UserID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// userIDOf is the signed-in member; "" for an API key, whose campaigns
// nobody owns.
func userIDOf(r *http.Request) string {
	if _, isKey := tenant.APIKeyScope(r.Context()); isKey {
		return ""
	}
	id, _ := tenant.UserID(r.Context())
	return id
}
