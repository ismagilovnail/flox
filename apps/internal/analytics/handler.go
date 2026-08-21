package analytics

import (
	"encoding/json"
	"log/slog"
	"net/http"
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

// Register mounts every /analytics route onto r. The caller applies
// tenant.Middleware first — same contract as campaign.Handler.Register.
func (h *Handler) Register(r chi.Router) {
	r.Get("/campaigns/{campaignId}/daily", h.campaignDaily)
}

type dailyCountResponse struct {
	Day        string `json:"day"`
	Type       string `json:"type"`
	EventCount uint64 `json:"eventCount"`
}

// campaignDaily answers GET /analytics/campaigns/{campaignId}/daily?from=YYYY-MM-DD&to=YYYY-MM-DD.
// from/to default to the last 7 days when omitted, matching a dashboard's
// most common first load.
func (h *Handler) campaignDaily(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	campaignID := chi.URLParam(r, "campaignId")

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
		to = parsed
	}

	counts, err := h.svc.CampaignDaily(r.Context(), orgID, campaignID, from, to)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}

	resp := make([]dailyCountResponse, len(counts))
	for i, c := range counts {
		resp[i] = dailyCountResponse{Day: c.Day.Format(dateLayout), Type: string(c.Type), EventCount: c.EventCount}
	}
	writeJSON(w, http.StatusOK, map[string]any{"counts": resp})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
