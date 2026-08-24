package postbacklogs

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

// Register mounts every /postback-logs route onto r. The caller applies
// tenant.Middleware first — same contract as every other handler.
func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.list)
	r.Post("/replay-outgoing", h.replayOutgoing)
}

// list answers GET /postback-logs?from=&to=&limit=&offset= — org-wide,
// both directions mixed, matching the frontend's single Logs table.
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
		// A date-only `to` parses to midnight UTC — pushed to end-of-day so
		// it includes that day's own events (raw event_at timestamps, not
		// the day-granularity aggregates internal/analytics queries; see
		// apps/internal/conversions' own handler for the same fix and why).
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

// replayOutgoing answers POST /postback-logs/replay-outgoing — re-enqueues
// a fresh delivery for a past outgoing attempt, off the exact fields a
// PostbackLog row the browser already has.
func (h *Handler) replayOutgoing(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())

	var in ReplayOutgoingInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("could not parse request body", nil))
		return
	}

	result, err := h.svc.ReplayOutgoing(r.Context(), orgID, in)
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
