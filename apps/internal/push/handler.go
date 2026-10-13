package push

import (
	"encoding/json"
	"log/slog"
	"net/http"

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
		Target: req.Target, URL: req.URL, Filters: req.Filters,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
