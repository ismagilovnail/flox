package integrations

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
	// requireWrite gates connect/verify/disconnect — reuses auth.
	// Service's own settings.write permission (the same one Organization/
	// API Keys/Security already use for this Settings panel), passed in
	// from main.go rather than importing the auth package directly.
	requireWrite func(http.Handler) http.Handler
}

func NewHandler(svc *Service, requireWrite func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, requireWrite: requireWrite, logger: logger}
}

func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.summary)
	r.With(h.requireWrite).Post("/{provider}/connect", h.connect)
	r.With(h.requireWrite).Post("/{provider}/verify", h.verify)
	r.With(h.requireWrite).Delete("/{provider}", h.disconnect)
}

func provider(r *http.Request) Provider { return Provider(chi.URLParam(r, "provider")) }

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	sum, err := h.svc.Summary(r.Context(), orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

func (h *Handler) connect(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())

	var fields map[string]string
	if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	integration, err := h.svc.Connect(r.Context(), orgID, ConnectInput{Provider: provider(r), Fields: fields})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, integration)
}

func (h *Handler) verify(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	integration, err := h.svc.Verify(r.Context(), orgID, provider(r))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, integration)
}

func (h *Handler) disconnect(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	if err := h.svc.Disconnect(r.Context(), orgID, provider(r)); err != nil {
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
