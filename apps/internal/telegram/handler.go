package telegram

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/audit"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

// Handler is the /telegram-bots API (settings.write, like integrations).
type Handler struct {
	svc          *Service
	requireWrite func(http.Handler) http.Handler
	logger       *slog.Logger
}

func NewHandler(svc *Service, requireWrite func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, requireWrite: requireWrite, logger: logger}
}

// Register mounts the routes; the caller applies tenant.Middleware and the
// settings.write gate first.
func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.list)
	r.With(h.requireWrite).Post("/", h.connect)
	r.With(h.requireWrite).Delete("/{id}", h.delete)
	r.With(h.requireWrite).Patch("/{id}", h.update)
	r.With(h.requireWrite).Post("/{id}/channels", h.addChannel)
	r.With(h.requireWrite).Delete("/{id}/channels/{channelId}", h.removeChannel)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	bots, err := h.svc.List(r.Context(), orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bots": bots, "configured": h.svc.Configured()})
}

type connectRequest struct {
	Token       string `json:"token"`
	WelcomeText string `json:"welcomeText"`
	Channel     string `json:"channel"`
}

func (h *Handler) connect(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	var req connectRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	b, err := h.svc.Connect(r.Context(), orgID, userID, ConnectInput{Token: req.Token, WelcomeText: req.WelcomeText, Channel: req.Channel})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	audit.Annotate(r.Context(), audit.Entry{Action: "telegram_bot.connected", EntityType: "telegram_bot", EntityID: b.ID})
	writeJSON(w, http.StatusCreated, b)
}

type updateRequest struct {
	WelcomeText *string `json:"welcomeText"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	id, ok := h.id(w, r, "id")
	if !ok {
		return
	}
	var req updateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil || req.WelcomeText == nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", map[string]string{"welcomeText": "required"}))
		return
	}
	b, err := h.svc.UpdateWelcome(r.Context(), orgID, id, *req.WelcomeText)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	audit.Annotate(r.Context(), audit.Entry{Action: "telegram_bot.updated", EntityType: "telegram_bot", EntityID: b.ID})
	writeJSON(w, http.StatusOK, b)
}

type addChannelRequest struct {
	Channel string `json:"channel"`
}

func (h *Handler) addChannel(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	id, ok := h.id(w, r, "id")
	if !ok {
		return
	}
	var req addChannelRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	b, err := h.svc.AddChannel(r.Context(), orgID, id, req.Channel)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	audit.Annotate(r.Context(), audit.Entry{Action: "telegram_bot.channel_added", EntityType: "telegram_bot", EntityID: b.ID})
	writeJSON(w, http.StatusCreated, b)
}

func (h *Handler) removeChannel(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	id, ok := h.id(w, r, "id")
	if !ok {
		return
	}
	channelID, ok := h.id(w, r, "channelId")
	if !ok {
		return
	}
	b, err := h.svc.RemoveChannel(r.Context(), orgID, id, channelID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	audit.Annotate(r.Context(), audit.Entry{Action: "telegram_bot.channel_removed", EntityType: "telegram_bot", EntityID: b.ID})
	writeJSON(w, http.StatusOK, b)
}

// id is the URL parameter name as an id; not one is a 404 (written).
func (h *Handler) id(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	id := chi.URLParam(r, name)
	if !idPattern.MatchString(id) {
		apierror.Write(w, h.logger, apierror.NotFound("not found"))
		return "", false
	}
	return id, true
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	id, ok := h.id(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), orgID, id); err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	audit.Annotate(r.Context(), audit.Entry{Action: "telegram_bot.removed", EntityType: "telegram_bot", EntityID: id})
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
