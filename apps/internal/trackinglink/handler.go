package trackinglink

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

type Handler struct {
	svc *Service
	// requireWrite is the RBAC middleware every mutating route below is
	// wrapped in (Register). Read routes carry no permission requirement
	// beyond tenantMiddleware — same split custommetrics/reportpresets/
	// referral already use.
	requireWrite func(http.Handler) http.Handler
	logger       *slog.Logger
}

func NewHandler(svc *Service, requireWrite func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, requireWrite: requireWrite, logger: logger}
}

// Register mounts every /campaigns/{campaignId}/tracking-links route onto
// r. The caller applies tenant.Middleware first — same contract as
// streamset.Handler.Register.
func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.list)
	r.With(h.requireWrite).Post("/", h.create)
	r.With(h.requireWrite).Patch("/{id}", h.updateStatus)
	r.With(h.requireWrite).Delete("/{id}", h.delete)
}

// RegisterOrgWide mounts GET /tracking-links?status= — the organization's
// links across campaigns. The caller applies tenant.Middleware and the
// campaign.read gate first.
func (h *Handler) RegisterOrgWide(r chi.Router) {
	r.Get("/", h.listByOrg)
}

func (h *Handler) listByOrg(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	links, err := h.svc.ListByOrg(r.Context(), orgID, Status(r.URL.Query().Get("status")))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"trackingLinks": links})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	campaignID := chi.URLParam(r, "campaignId")
	links, err := h.svc.List(r.Context(), orgID, campaignID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"trackingLinks": links})
}

type createRequest struct {
	DomainID string `json:"domainId"`
	Slug     string `json:"slug"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	campaignID := chi.URLParam(r, "campaignId")

	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	link, err := h.svc.Create(r.Context(), orgID, campaignID, CreateInput{DomainID: req.DomainID, Slug: req.Slug})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

type updateStatusRequest struct {
	Status Status `json:"status"`
}

func (h *Handler) updateStatus(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())

	var req updateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	link, err := h.svc.UpdateStatus(r.Context(), orgID, chi.URLParam(r, "id"), req.Status)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, link)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	if err := h.svc.Delete(r.Context(), orgID, chi.URLParam(r, "id")); err != nil {
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
