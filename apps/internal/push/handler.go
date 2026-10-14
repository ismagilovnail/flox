package push

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/pagination"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

type Handler struct {
	svc          *Service
	requireWrite func(http.Handler) http.Handler
	logger       *slog.Logger
}

func NewHandler(svc *Service, requireWrite func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, requireWrite: requireWrite, logger: logger}
}

// Register mounts the /push routes. The caller applies tenant.Middleware
// and the push.read gate first.
func (h *Handler) Register(r chi.Router) {
	r.Get("/summary", h.summary)
	r.Get("/subscriptions", h.subscriptions)
	r.Post("/audience", h.audience)
	r.Get("/broadcasts", h.broadcasts)
	r.Get("/broadcasts/{id}", h.broadcast)
	r.With(h.requireWrite).Post("/broadcasts", h.createBroadcast)
	r.With(h.requireWrite).Post("/broadcasts/{id}/cancel", h.cancelBroadcast)
	r.With(h.requireWrite).Post("/broadcasts/{id}/pause", h.setTriggerActive(false))
	r.With(h.requireWrite).Post("/broadcasts/{id}/resume", h.setTriggerActive(true))
	r.Get("/stats", h.stats)
}

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	s, err := h.svc.Summary(r.Context(), orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": s, "configured": h.svc.Configured()})
}

func (h *Handler) subscriptions(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	page, err := pagination.FromRequest(r)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	q := r.URL.Query()
	f := SubscriptionFilter{Status: q.Get("status"), PwaID: q.Get("pwaId"), CampaignID: q.Get("campaignId"), Country: q.Get("country")}
	if f.Status != "" && f.Status != StatusActive && f.Status != StatusUnsubscribed {
		apierror.Write(w, h.logger, apierror.Validation("invalid status", map[string]string{"status": "active or unsubscribed"}))
		return
	}
	items, total, err := h.svc.ListSubscriptions(r.Context(), orgID, f, page)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscriptions": items, "total": total})
}

func (h *Handler) audience(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var f Filters
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	n, err := h.svc.CountAudience(r.Context(), orgID, f)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": n})
}

func (h *Handler) broadcasts(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	page, err := pagination.FromRequest(r)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	items, total, err := h.svc.ListBroadcasts(r.Context(), orgID, page)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"broadcasts": items, "total": total})
}

func (h *Handler) broadcast(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	b, err := h.svc.GetBroadcast(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

type createBroadcastRequest struct {
	Name     string  `json:"name"`
	Title    string  `json:"title"`
	Body     string  `json:"body"`
	IconURL  string  `json:"iconUrl"`
	ImageURL string  `json:"imageUrl"`
	Target   Target  `json:"target"`
	URL      string  `json:"url"`
	Filters  Filters `json:"filters"`
	// SendAt (RFC 3339) schedules the broadcast; absent or null sends now.
	SendAt *time.Time `json:"sendAt"`
	// Trigger makes it a triggered broadcast; not with sendAt.
	Trigger *Trigger `json:"trigger"`
}

func (h *Handler) createBroadcast(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	var req createBroadcastRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	b, err := h.svc.CreateBroadcast(r.Context(), orgID, userID, CreateBroadcastInput{
		Name: req.Name, Title: req.Title, Body: req.Body, IconURL: req.IconURL, ImageURL: req.ImageURL,
		Target: req.Target, URL: req.URL, Filters: req.Filters, SendAt: req.SendAt, Trigger: req.Trigger,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (h *Handler) cancelBroadcast(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	b, err := h.svc.CancelBroadcast(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// setTriggerActive pauses or resumes a triggered broadcast.
func (h *Handler) setTriggerActive(active bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID, _ := tenant.OrgID(r.Context())
		b, err := h.svc.SetTriggerActive(r.Context(), orgID, chi.URLParam(r, "id"), active)
		if err != nil {
			apierror.Write(w, h.logger, err)
			return
		}
		writeJSON(w, http.StatusOK, b)
	}
}

// stats answers GET /push/stats?from=&to=&timezone=&groupBy= with rows of
// push metrics keyed as the custom-metric catalog names them (push_sent,
// push_delivered, push_opened, push_ctr) plus failed/unsubscribed, and the
// totals.
func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	q := r.URL.Query()
	from, errFrom := time.Parse(time.DateOnly, q.Get("from"))
	to, errTo := time.Parse(time.DateOnly, q.Get("to"))
	if errFrom != nil || errTo != nil {
		apierror.Write(w, h.logger, apierror.Validation("from and to are YYYY-MM-DD dates", map[string]string{"from": "YYYY-MM-DD", "to": "YYYY-MM-DD"}))
		return
	}
	group := StatsGroup(q.Get("groupBy"))
	if group == "" {
		group = StatsByDay
	}
	rows, total, err := h.svc.Stats(r.Context(), orgID, from, to, q.Get("timezone"), group)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	out := make([]statsRowResponse, len(rows))
	for i, row := range rows {
		out[i] = statsRowResponse{Key: row.Key, Label: row.Label, Metrics: metricsOf(row.StatsCounts)}
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": out, "totals": metricsOf(total)})
}

type statsRowResponse struct {
	Key     string         `json:"key"`
	Label   string         `json:"label,omitempty"`
	Metrics map[string]any `json:"metrics"`
}

// metricsOf names the counts as the custom-metric catalog does, so a push
// formula evaluates on a row as it is. push_ctr is opened ÷ delivered in
// percent, null when nothing was delivered.
func metricsOf(c StatsCounts) map[string]any {
	var ctr any
	if c.Delivered > 0 {
		ctr = float64(c.Opened) / float64(c.Delivered) * 100
	}
	return map[string]any{
		"push_sent": c.Sent, "push_delivered": c.Delivered, "push_opened": c.Opened, "push_ctr": ctr,
		"push_failed": c.Failed, "push_unsubscribed": c.Unsubscribed,
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
