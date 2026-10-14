package apikey

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
	svc    *Service
	logger *slog.Logger
	// requireWrite gates create/revoke — reuses auth.Service's own
	// settings.write permission (the vocabulary team.ts/migration 00020
	// already reserved for exactly this Settings panel), passed in from
	// main.go rather than importing the auth package directly.
	requireWrite func(http.Handler) http.Handler
}

func NewHandler(svc *Service, requireWrite func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, requireWrite: requireWrite, logger: logger}
}

func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.list)
	r.With(h.requireWrite).Post("/", h.create)
	r.With(h.requireWrite).Delete("/{id}", h.revoke)
	r.With(h.requireWrite).Post("/{id}/rotate", h.rotate)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	keys, err := h.svc.List(r.Context(), orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"apiKeys": keys})
}

type createRequest struct {
	Name  string `json:"name"`
	Scope string `json:"scope"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	result, err := h.svc.Create(r.Context(), orgID, userID, CreateInput{Name: req.Name, Scope: Scope(req.Scope)})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	audit.Annotate(r.Context(), audit.Entry{Action: "api_key.created", EntityType: "api_key", EntityID: result.ApiKey.ID})
	writeJSON(w, http.StatusCreated, map[string]any{
		"apiKey":  result.ApiKey,
		"fullKey": result.FullKey,
	})
}

// rotate answers POST /api-keys/{id}/rotate with the replacement key and
// its full secret (shown once); the old key works for RotationGrace more.
func (h *Handler) rotate(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	id := chi.URLParam(r, "id")
	result, err := h.svc.Rotate(r.Context(), orgID, id, userID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	audit.Annotate(r.Context(), audit.Entry{Action: "api_key.rotated", EntityType: "api_key", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{
		"apiKey":  result.ApiKey,
		"fullKey": result.FullKey,
	})
}

func (h *Handler) revoke(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.svc.Revoke(r.Context(), orgID, id); err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	audit.Annotate(r.Context(), audit.Entry{Action: "api_key.revoked", EntityType: "api_key", EntityID: id})
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
