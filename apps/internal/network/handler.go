package network

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/audit"
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
	logger       *slog.Logger
}

func NewHandler(svc *Service, requireWrite func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, requireWrite: requireWrite, logger: logger}
}

// Register mounts every /networks route onto r. The caller applies
// tenant.Middleware first — same contract as every other handler.
func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.list)
	r.With(h.requireWrite).Post("/", h.create)
	r.Get("/{id}", h.get)
	r.Get("/{id}/diagnostics", h.diagnostics)
	r.With(h.requireWrite).Patch("/{id}", h.update)
	r.With(h.requireWrite).Delete("/{id}", h.delete)
	r.With(h.requireWrite).Post("/{id}/duplicate", h.duplicate)
	r.With(h.requireWrite).Post("/{id}/pause", h.pause)
	r.With(h.requireWrite).Post("/{id}/activate", h.activate)
	r.With(h.requireWrite).Post("/{id}/regenerate-postback-secret", h.regeneratePostbackSecret)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	page, err := pagination.FromRequest(r)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	networks, total, err := h.svc.List(r.Context(), orgID, page)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"networks": networks, "total": total})
}

func (h *Handler) diagnostics(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	d, err := h.svc.Diagnostics(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	n, err := h.svc.Get(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

type createRequest struct {
	Name             string `json:"name"`
	PostbackURL      string `json:"postbackUrl"`
	AcceptDuplicates bool   `json:"acceptDuplicates"`
	Currency         string `json:"currency"`
	TemplateKey      string `json:"templateKey"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())

	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	result, err := h.svc.Create(r.Context(), orgID, CreateInput{
		Name:             req.Name,
		PostbackURL:      req.PostbackURL,
		AcceptDuplicates: req.AcceptDuplicates,
		Currency:         req.Currency,
		TemplateKey:      req.TemplateKey,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, createResponse{Network: result.Network, PostbackSecret: result.PostbackSecret})
}

// createResponse embeds Network's own fields alongside postbackSecret —
// the ONE response that ever carries the plaintext secret (§54:
// CreateResult's own doc comment). regeneratePostbackSecret below returns
// just the secret string, deliberately not re-echoing the whole network.
type createResponse struct {
	Network
	PostbackSecret string `json:"postbackSecret"`
}

type updateRequest struct {
	Name             *string `json:"name"`
	PostbackURL      *string `json:"postbackUrl"`
	AcceptDuplicates *bool   `json:"acceptDuplicates"`
	Currency         *string `json:"currency"`
	TemplateKey      *string `json:"templateKey"`
	Status           *Status `json:"status"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())

	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	n, err := h.svc.Update(r.Context(), orgID, chi.URLParam(r, "id"), UpdateInput{
		Name:             req.Name,
		PostbackURL:      req.PostbackURL,
		AcceptDuplicates: req.AcceptDuplicates,
		Currency:         req.Currency,
		TemplateKey:      req.TemplateKey,
		Status:           req.Status,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
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
	n, err := h.svc.Duplicate(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (h *Handler) pause(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	n, err := h.svc.Pause(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (h *Handler) regeneratePostbackSecret(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	secret, err := h.svc.RegeneratePostbackSecret(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	audit.Annotate(r.Context(), audit.Entry{Action: "network.postback_secret_regenerated", EntityType: "network", EntityID: chi.URLParam(r, "id")})
	writeJSON(w, http.StatusOK, struct {
		PostbackSecret string `json:"postbackSecret"`
	}{secret})
}

func (h *Handler) activate(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	n, err := h.svc.Activate(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
