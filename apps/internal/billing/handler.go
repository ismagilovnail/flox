package billing

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

// Handler serves the plan and its usage, read-only: plans change through
// apps/cmd/billing until a payment provider lands.
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.status)
	r.Get("/plans", h.plans)
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	st, err := h.svc.Status(r.Context(), orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, st)
}

func (h *Handler) plans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.svc.Plans(r.Context())
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, map[string]any{"plans": plans})
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}
