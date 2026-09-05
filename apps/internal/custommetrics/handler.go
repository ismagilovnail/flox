package custommetrics

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
	// requireWrite gates create/update/delete/duplicate — mounted per
	// route below, same pattern as auth.TeamHandler. Read routes carry
	// no permission requirement beyond tenantMiddleware: every role has
	// custom_metric.read (migration 00029), and Service.List/Get already
	// narrow what a read-only caller actually sees.
	requireWrite func(http.Handler) http.Handler
}

func NewHandler(svc *Service, requireWrite func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, requireWrite: requireWrite, logger: logger}
}

func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.list)
	r.Get("/{id}", h.get)
	r.With(h.requireWrite).Post("/", h.create)
	r.With(h.requireWrite).Patch("/{id}", h.update)
	r.With(h.requireWrite).Delete("/{id}", h.delete)
	r.With(h.requireWrite).Post("/{id}/duplicate", h.duplicate)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	list, err := h.svc.List(r.Context(), orgID, userID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"metrics": list})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	m, err := h.svc.Get(r.Context(), orgID, userID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

type createRequest struct {
	Name      string   `json:"name"`
	GroupName string   `json:"group"`
	Formula   string   `json:"formula"`
	Format    string   `json:"format"`
	Status    string   `json:"status"`
	Targets   []string `json:"targets"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	m, err := h.svc.Create(r.Context(), orgID, userID, CreateInput{
		Name: req.Name, GroupName: req.GroupName, Formula: req.Formula,
		Format: Format(req.Format), Status: Status(req.Status), Targets: req.Targets,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

type updateRequest struct {
	Name      *string   `json:"name"`
	GroupName *string   `json:"group"`
	Formula   *string   `json:"formula"`
	Format    *string   `json:"format"`
	Status    *string   `json:"status"`
	Targets   *[]string `json:"targets"`
	Active    *bool     `json:"active"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	in := UpdateInput{Name: req.Name, GroupName: req.GroupName, Formula: req.Formula, Targets: req.Targets, Active: req.Active}
	if req.Format != nil {
		f := Format(*req.Format)
		in.Format = &f
	}
	if req.Status != nil {
		st := Status(*req.Status)
		in.Status = &st
	}
	m, err := h.svc.Update(r.Context(), orgID, userID, chi.URLParam(r, "id"), in)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	if err := h.svc.Delete(r.Context(), orgID, userID, chi.URLParam(r, "id")); err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) duplicate(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	m, err := h.svc.Duplicate(r.Context(), orgID, userID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
