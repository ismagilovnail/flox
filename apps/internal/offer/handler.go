package offer

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

type Handler struct {
	svc    *Service
	logger *slog.Logger
}

func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Register mounts every /offers route onto r. The caller applies
// tenant.Middleware first — same contract as every other handler.
func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Get("/{id}", h.get)
	r.Patch("/{id}", h.update)
	r.Delete("/{id}", h.delete)
	r.Post("/{id}/duplicate", h.duplicate)
	r.Post("/{id}/pause", h.pause)
	r.Post("/{id}/activate", h.activate)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	offers, err := h.svc.List(r.Context(), orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"offers": offers})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	o, err := h.svc.Get(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

type linkRequest struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type createRequest struct {
	NetworkID string        `json:"networkId"`
	Name      string        `json:"name"`
	Countries []string      `json:"countries"`
	Payout    float64       `json:"payout"`
	Currency  string        `json:"currency"`
	Cap       *int          `json:"cap"`
	Links     []linkRequest `json:"links"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())

	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	links := make([]LinkInput, len(req.Links))
	for i, l := range req.Links {
		links[i] = LinkInput{Label: l.Label, URL: l.URL}
	}

	o, err := h.svc.Create(r.Context(), orgID, CreateInput{
		NetworkID: req.NetworkID,
		Name:      req.Name,
		Countries: req.Countries,
		Payout:    req.Payout,
		Currency:  req.Currency,
		Cap:       req.Cap,
		Links:     links,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

type updateRequest struct {
	NetworkID *string        `json:"networkId"`
	Name      *string        `json:"name"`
	Countries *[]string      `json:"countries"`
	Payout    *float64       `json:"payout"`
	Currency  *string        `json:"currency"`
	Cap       *OptionalCap   `json:"cap"`
	Status    *Status        `json:"status"`
	Links     *[]linkRequest `json:"links"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())

	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	in := UpdateInput{
		NetworkID: req.NetworkID,
		Name:      req.Name,
		Countries: req.Countries,
		Payout:    req.Payout,
		Currency:  req.Currency,
		Cap:       req.Cap,
		Status:    req.Status,
	}
	if req.Links != nil {
		links := make([]LinkInput, len(*req.Links))
		for i, l := range *req.Links {
			links[i] = LinkInput{Label: l.Label, URL: l.URL}
		}
		in.Links = &links
	}

	o, err := h.svc.Update(r.Context(), orgID, chi.URLParam(r, "id"), in)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	if err := h.svc.Delete(r.Context(), orgID, chi.URLParam(r, "id")); err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) duplicate(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	o, err := h.svc.Duplicate(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

func (h *Handler) pause(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	o, err := h.svc.Pause(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (h *Handler) activate(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	o, err := h.svc.Activate(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
