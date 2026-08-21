package conversions

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

const dateLayout = "2006-01-02"

type Handler struct {
	svc    *Service
	logger *slog.Logger
}

func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Register mounts every /conversions route onto r. The caller applies
// tenant.Middleware first — same contract as every other handler.
func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.list)
	r.Get("/{clickId}", h.timeline)
}

// list answers GET /conversions?from=&to=&limit=&offset= — org-wide, not
// scoped to one campaign, matching the frontend's top-level Conversions
// nav item.
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())

	to := time.Now().UTC()
	from := to.AddDate(0, 0, -7)
	if v := r.URL.Query().Get("from"); v != "" {
		parsed, err := time.Parse(dateLayout, v)
		if err != nil {
			apierror.Write(w, h.logger, apierror.Validation("invalid from date, want YYYY-MM-DD", nil))
			return
		}
		from = parsed
	}
	if v := r.URL.Query().Get("to"); v != "" {
		parsed, err := time.Parse(dateLayout, v)
		if err != nil {
			apierror.Write(w, h.logger, apierror.Validation("invalid to date, want YYYY-MM-DD", nil))
			return
		}
		// This package queries raw event_at timestamps, not day-granularity
		// aggregates (unlike internal/analytics, where a date-only `to` is
		// safe as-is because the stored column is already a DATE) — parsed
		// as midnight, a same-day `to` would exclude every event recorded
		// later that day. End-of-day makes ?to=<today> actually include
		// today's events.
		to = parsed.Add(24*time.Hour - time.Nanosecond)
	}

	limit, offset := 0, 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = n
		}
	}

	result, err := h.svc.List(r.Context(), orgID, from, to, limit, offset)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// timeline answers GET /conversions/{clickId} — the detail page's whole
// payload, funnel + conversion events merged and sorted.
func (h *Handler) timeline(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	clickID := chi.URLParam(r, "clickId")

	result, err := h.svc.Timeline(r.Context(), orgID, clickID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
