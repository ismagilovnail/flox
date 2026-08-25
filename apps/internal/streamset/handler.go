package streamset

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

// Register mounts every /campaigns/{campaignId}/stream-sets route onto
// r. The caller applies tenant.Middleware first — same contract as every
// other handler.
func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Post("/reorder", h.reorder)
	r.Get("/{id}", h.get)
	r.Patch("/{id}", h.update)
	r.Delete("/{id}", h.delete)
	r.Post("/{id}/duplicate", h.duplicate)
}

func campaignID(r *http.Request) string { return chi.URLParam(r, "campaignId") }

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	sets, err := h.svc.List(r.Context(), orgID, campaignID(r))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"streamSets": sets})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	s, err := h.svc.Get(r.Context(), orgID, campaignID(r), chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

type flowRequest struct {
	Name        string          `json:"name"`
	Active      bool            `json:"active"`
	Weight      int             `json:"weight"`
	Landing     FlowLanding     `json:"landing"`
	Pwa         FlowPwa         `json:"pwa"`
	Postlanding FlowPostlanding `json:"postlanding"`
	Destination Destination     `json:"destination"`
}

type createRequest struct {
	Name        string        `json:"name"`
	FallbackURL string        `json:"fallbackUrl"`
	RootFilter  FilterNode    `json:"rootFilter"`
	Flows       []flowRequest `json:"flows"`
}

func toFlowInputs(reqs []flowRequest) []FlowInput {
	out := make([]FlowInput, len(reqs))
	for i, f := range reqs {
		out[i] = FlowInput{
			Name: f.Name, Active: f.Active, Weight: f.Weight,
			Landing: f.Landing, Pwa: f.Pwa, Postlanding: f.Postlanding,
			Destination: f.Destination,
		}
	}
	return out
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())

	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	s, err := h.svc.Create(r.Context(), orgID, campaignID(r), CreateInput{
		Name:        req.Name,
		FallbackURL: req.FallbackURL,
		RootFilter:  req.RootFilter,
		Flows:       toFlowInputs(req.Flows),
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

type updateRequest struct {
	Name        *string        `json:"name"`
	Status      *Status        `json:"status"`
	FallbackURL *string        `json:"fallbackUrl"`
	RootFilter  *FilterNode    `json:"rootFilter"`
	Flows       *[]flowRequest `json:"flows"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())

	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	in := UpdateInput{Name: req.Name, Status: req.Status, FallbackURL: req.FallbackURL, RootFilter: req.RootFilter}
	if req.Flows != nil {
		flows := toFlowInputs(*req.Flows)
		in.Flows = &flows
	}

	s, err := h.svc.Update(r.Context(), orgID, campaignID(r), chi.URLParam(r, "id"), in)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	if err := h.svc.Delete(r.Context(), orgID, campaignID(r), chi.URLParam(r, "id")); err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) duplicate(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	s, err := h.svc.Duplicate(r.Context(), orgID, campaignID(r), chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

type reorderRequest struct {
	OrderedIDs []string `json:"orderedIds"`
}

func (h *Handler) reorder(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())

	var req reorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	sets, err := h.svc.Reorder(r.Context(), orgID, campaignID(r), req.OrderedIDs)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"streamSets": sets})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
