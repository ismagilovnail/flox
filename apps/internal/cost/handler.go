package cost

import (
	"encoding/json"
	"net/http"
	"time"

	"log/slog"

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

// Register mounts every /campaigns/{campaignId}/cost-entries route onto
// r. The caller applies tenant.Middleware first, same contract as
// campaign.Handler.Register.
func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.list)
	r.Post("/", h.upsert)
	r.Delete("/{id}", h.delete)
	// Colocated with the CRUD it summarizes rather than under /analytics:
	// unlike click/revenue analytics, spend never depends on ClickHouse
	// being up (see this package's doc comment), so it shouldn't share
	// apps/api's `if ch != nil` mount guard for the rest of /analytics.
	r.Get("/daily", h.daily)
}

type entryResponse struct {
	ID              string   `json:"id"`
	CampaignID      string   `json:"campaignId"`
	TrafficSourceID *string  `json:"trafficSourceId"`
	EntryDate       string   `json:"entryDate"`
	Amount          float64  `json:"amount"`
	Currency        string   `json:"currency"`
	AmountUSD       *float64 `json:"amountUsd"`
	Source          Source   `json:"source"`
	CreatedAt       string   `json:"createdAt"`
	UpdatedAt       string   `json:"updatedAt"`
}

func toResponse(e Entry) entryResponse {
	return entryResponse{
		ID:              e.ID,
		CampaignID:      e.CampaignID,
		TrafficSourceID: e.TrafficSourceID,
		EntryDate:       e.EntryDate.Format(dateLayout),
		Amount:          e.Amount,
		Currency:        e.Currency,
		AmountUSD:       e.AmountUSD,
		Source:          e.Source,
		CreatedAt:       e.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       e.UpdatedAt.Format(time.RFC3339),
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	campaignID := chi.URLParam(r, "campaignId")

	from, to, ok := parseRange(w, r, h.logger)
	if !ok {
		return
	}

	entries, err := h.svc.List(r.Context(), orgID, campaignID, ListFilter{From: from, To: to})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}

	resp := make([]entryResponse, len(entries))
	for i, e := range entries {
		resp[i] = toResponse(e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": resp})
}

type upsertRequest struct {
	TrafficSourceID *string `json:"trafficSourceId"`
	EntryDate       string  `json:"entryDate"`
	Amount          float64 `json:"amount"`
	Currency        string  `json:"currency"`
}

func (h *Handler) upsert(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	campaignID := chi.URLParam(r, "campaignId")

	var req upsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	entryDate, err := time.Parse(dateLayout, req.EntryDate)
	if err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid cost entry", map[string]string{"entryDate": "must be YYYY-MM-DD"}))
		return
	}

	e, err := h.svc.Upsert(r.Context(), orgID, campaignID, UpsertInput{
		TrafficSourceID: req.TrafficSourceID,
		EntryDate:       entryDate,
		Amount:          req.Amount,
		Currency:        req.Currency,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, toResponse(e))
}

type dailySpendResponse struct {
	Day          string  `json:"day"`
	AmountUSD    float64 `json:"amountUsd"`
	AllConverted bool    `json:"allConverted"`
}

func (h *Handler) daily(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	campaignID := chi.URLParam(r, "campaignId")

	from, to, ok := parseRange(w, r, h.logger)
	if !ok {
		return
	}

	spend, err := h.svc.DailyCampaignSpend(r.Context(), orgID, campaignID, from, to)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}

	resp := make([]dailySpendResponse, len(spend))
	for i, s := range spend {
		resp[i] = dailySpendResponse{Day: s.Day.Format(dateLayout), AmountUSD: s.AmountUSD, AllConverted: s.AllConverted}
	}
	writeJSON(w, http.StatusOK, map[string]any{"spend": resp})
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	campaignID := chi.URLParam(r, "campaignId")
	if err := h.svc.Delete(r.Context(), orgID, campaignID, chi.URLParam(r, "id")); err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// parseRange defaults to the last 30 days, matching the campaign
// overview's own fixed window (hooks/use-campaign-analytics.ts).
func parseRange(w http.ResponseWriter, r *http.Request, logger *slog.Logger) (from, to time.Time, ok bool) {
	to = time.Now().UTC()
	from = to.AddDate(0, 0, -29)
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

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
