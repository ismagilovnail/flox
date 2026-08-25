package pixel

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

// Register mounts every /pixels route onto r. The caller applies
// tenant.Middleware first — same contract as every other handler.
func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Get("/{id}", h.get)
	r.Patch("/{id}", h.update)
	r.Delete("/{id}", h.delete)
	r.Post("/{id}/duplicate", h.duplicate)
	r.Post("/{id}/pause", h.pause)
	r.Post("/{id}/activate", h.activate)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	pixels, err := h.svc.List(r.Context(), orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pixels": pixels})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	p, err := h.svc.Get(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type createRequest struct {
	Name     string   `json:"name"`
	Provider Provider `json:"provider"`
	PixelID  string   `json:"pixelId"`
	Events   []string `json:"events"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())

	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	p, err := h.svc.Create(r.Context(), orgID, CreateInput{
		Name:     req.Name,
		Provider: req.Provider,
		PixelID:  req.PixelID,
		Events:   req.Events,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

type updateRequest struct {
	Name     *string   `json:"name"`
	Provider *Provider `json:"provider"`
	PixelID  *string   `json:"pixelId"`
	Events   *[]string `json:"events"`
	Status   *Status   `json:"status"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())

	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	p, err := h.svc.Update(r.Context(), orgID, chi.URLParam(r, "id"), UpdateInput{
		Name:     req.Name,
		Provider: req.Provider,
		PixelID:  req.PixelID,
		Events:   req.Events,
		Status:   req.Status,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
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
	p, err := h.svc.Duplicate(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *Handler) pause(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	p, err := h.svc.Pause(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handler) activate(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	p, err := h.svc.Activate(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
