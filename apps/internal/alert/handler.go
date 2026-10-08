package alert

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

type Handler struct {
	svc *Service
	// requireWrite guards rule changes (settings.write). Reading rules and
	// notifications, and marking one's own notifications read, is open to
	// every member.
	requireWrite func(http.Handler) http.Handler
	logger       *slog.Logger
}

func NewHandler(svc *Service, requireWrite func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, requireWrite: requireWrite, logger: logger}
}

// RegisterRules mounts /alert-rules; RegisterNotifications mounts
// /notifications. The caller applies tenant.Middleware first.
func (h *Handler) RegisterRules(r chi.Router) {
	r.Get("/", h.listRules)
	r.With(h.requireWrite).Post("/", h.createRule)
	r.With(h.requireWrite).Put("/{id}", h.updateRule)
	r.With(h.requireWrite).Delete("/{id}", h.deleteRule)
}

func (h *Handler) RegisterNotifications(r chi.Router) {
	r.Get("/", h.listNotifications)
	r.Post("/read", h.markRead)
}

type ruleRequest struct {
	Kind        Kind     `json:"kind"`
	CampaignID  *string  `json:"campaignId"`
	Threshold   *float64 `json:"threshold"`
	MinSpend    *float64 `json:"minSpend"`
	NotifySlack bool     `json:"notifySlack"`
	Enabled     *bool    `json:"enabled"`
}

func (req ruleRequest) input() RuleInput {
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	return RuleInput{Kind: req.Kind, CampaignID: req.CampaignID, Threshold: req.Threshold, MinSpend: req.MinSpend, NotifySlack: req.NotifySlack, Enabled: enabled}
}

func (h *Handler) listRules(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	rules, err := h.svc.ListRules(r.Context(), orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": rules})
}

func (h *Handler) createRule(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req ruleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	rule, err := h.svc.CreateRule(r.Context(), orgID, req.input())
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, rule)
}

func (h *Handler) updateRule(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req ruleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	rule, err := h.svc.UpdateRule(r.Context(), orgID, chi.URLParam(r, "id"), req.input())
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (h *Handler) deleteRule(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	if err := h.svc.DeleteRule(r.Context(), orgID, chi.URLParam(r, "id")); err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listNotifications(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	list, err := h.svc.Notifications(r.Context(), orgID, userID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, ok := tenant.UserID(r.Context())
	if !ok || userID == "" {
		// An API key has no user to keep read state for.
		apierror.Write(w, h.logger, apierror.Validation("notifications are marked read per user; sign in to do it", nil))
		return
	}
	if err := h.svc.MarkAllRead(r.Context(), orgID, userID); err != nil {
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
