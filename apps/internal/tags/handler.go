package tags

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

// Register mounts every /tags route onto r. The caller applies
// tenant.Middleware first — same contract as every other handler.
func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Patch("/{id}", h.update)
	r.Delete("/{id}", h.delete)

	r.Get("/assignments", h.listAssignments)
	r.Post("/assignments/set", h.setEntityTags)
	r.Post("/assignments/bulk", h.bulkEdit)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	list, err := h.svc.List(r.Context(), orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": list})
}

type createRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	t, err := h.svc.Create(r.Context(), orgID, CreateTagInput{Name: req.Name, Color: req.Color})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

type updateRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	t, err := h.svc.Update(r.Context(), orgID, chi.URLParam(r, "id"), UpdateTagInput{Name: req.Name, Color: req.Color})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	if err := h.svc.Delete(r.Context(), orgID, chi.URLParam(r, "id")); err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listAssignments(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	entityType := EntityType(r.URL.Query().Get("entityType"))
	list, err := h.svc.ListAssignments(r.Context(), orgID, entityType)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"assignments": list})
}

type setEntityTagsRequest struct {
	EntityType string   `json:"entityType"`
	EntityID   string   `json:"entityId"`
	TagIDs     []string `json:"tagIds"`
}

func (h *Handler) setEntityTags(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req setEntityTagsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	err := h.svc.SetEntityTags(r.Context(), orgID, SetEntityTagsInput{
		EntityType: EntityType(req.EntityType),
		EntityID:   req.EntityID,
		TagIDs:     req.TagIDs,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type bulkEditRequest struct {
	EntityType string   `json:"entityType"`
	EntityIDs  []string `json:"entityIds"`
	ToAdd      []string `json:"toAdd"`
	ToRemove   []string `json:"toRemove"`
}

func (h *Handler) bulkEdit(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req bulkEditRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	err := h.svc.BulkEdit(r.Context(), orgID, BulkEditInput{
		EntityType: EntityType(req.EntityType),
		EntityIDs:  req.EntityIDs,
		ToAdd:      req.ToAdd,
		ToRemove:   req.ToRemove,
	})
	if err != nil {
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
