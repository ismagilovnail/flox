package costsync

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

// defaultLookbackDays mirrors cost/handler.go's own parseRange default
// (29 days back + today = 30 days) — ad platforms commonly revise very
// recent days' reported spend, so a "Sync now" without explicit dates
// should re-pull that whole window, not just today.
const defaultLookbackDays = 29

const dateLayout = "2006-01-02"

type Handler struct {
	svc    *Service
	logger *slog.Logger
}

func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Register mounts the single "sync now" action onto r, expected to be
// nested under /traffic-sources/{id}/connection alongside adaccount's
// own GET/PATCH/DELETE (same {id} URL param, same tenant.Middleware).
func (h *Handler) Register(r chi.Router) {
	r.Post("/sync", h.sync)
}

func (h *Handler) sync(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	trafficSourceID := chi.URLParam(r, "id")

	from, to, ok := parseRange(w, r, h.logger)
	if !ok {
		return
	}

	result, err := h.svc.Sync(r.Context(), orgID, trafficSourceID, from, to)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}

func parseRange(w http.ResponseWriter, r *http.Request, logger *slog.Logger) (from, to time.Time, ok bool) {
	to = time.Now().UTC()
	from = to.AddDate(0, 0, -defaultLookbackDays)
	if v := r.URL.Query().Get("from"); v != "" {
		parsed, err := time.Parse(dateLayout, v)
		if err != nil {
			apierror.Write(w, logger, apierror.Validation("invalid from date, want YYYY-MM-DD", nil))
			return time.Time{}, time.Time{}, false
		}
		from = parsed
	}
	if v := r.URL.Query().Get("to"); v != "" {
		parsed, err := time.Parse(dateLayout, v)
		if err != nil {
			apierror.Write(w, logger, apierror.Validation("invalid to date, want YYYY-MM-DD", nil))
			return time.Time{}, time.Time{}, false
		}
		to = parsed
	}
	return from, to, true
}
