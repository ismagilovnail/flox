package reportpresets

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
	// requireWrite gates create/update/delete — every role already holds
	// report_preset.read (migration 00030 grants it to all six), so list/
	// get carry no permission requirement beyond tenantMiddleware, same
	// pattern custommetrics.Handler established.
	requireWrite func(http.Handler) http.Handler
}

func NewHandler(svc *Service, requireWrite func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, requireWrite: requireWrite, logger: logger}
}

func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.list)
	r.Get("/{id}", h.get)
	r.With(h.requireWrite).Post("/", h.create)
	r.With(h.requireWrite).Patch("/{id}", h.update)
	r.With(h.requireWrite).Delete("/{id}", h.delete)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	presets, err := h.svc.List(r.Context(), orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"presets": presets})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	preset, err := h.svc.Get(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, preset)
}

type periodRequest struct {
	Type PeriodType `json:"type"`
	Days *int       `json:"days,omitempty"`
	From *string    `json:"from,omitempty"`
	To   *string    `json:"to,omitempty"`
}

func (p periodRequest) toPeriod() Period {
	return Period{Type: p.Type, Days: p.Days, From: p.From, To: p.To}
}

type createRequest struct {
	Name          string         `json:"name"`
	Dimensions    []string       `json:"dimensions"`
	Metrics       []string       `json:"metrics"`
	Filters       []Filter       `json:"filters"`
	ColumnOrder   []string       `json:"columnOrder"`
	ColumnWidths  map[string]int `json:"columnWidths"`
	HiddenColumns []string       `json:"hiddenColumns"`
	GroupBy       string         `json:"groupBy"`
	Period        periodRequest  `json:"period"`
	Timezone      string         `json:"timezone"`
	DateBasis     string         `json:"dateBasis"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	preset, err := h.svc.Create(r.Context(), orgID, userID, CreateInput{
		Name:          req.Name,
		Dimensions:    req.Dimensions,
		Metrics:       req.Metrics,
		Filters:       req.Filters,
		ColumnOrder:   req.ColumnOrder,
		ColumnWidths:  req.ColumnWidths,
		HiddenColumns: req.HiddenColumns,
		GroupBy:       req.GroupBy,
		Period:        req.Period.toPeriod(),
		Timezone:      req.Timezone,
		DateBasis:     req.DateBasis,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, preset)
}

type updateRequest struct {
	Name          *string        `json:"name"`
	Dimensions    []string       `json:"dimensions"`
	Metrics       []string       `json:"metrics"`
	Filters       []Filter       `json:"filters"`
	ColumnOrder   []string       `json:"columnOrder"`
	ColumnWidths  map[string]int `json:"columnWidths"`
	HiddenColumns []string       `json:"hiddenColumns"`
	GroupBy       *string        `json:"groupBy"`
	Period        *periodRequest `json:"period"`
	Timezone      *string        `json:"timezone"`
	DateBasis     *string        `json:"dateBasis"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	in := UpdateInput{
		Name:          req.Name,
		Dimensions:    req.Dimensions,
		Metrics:       req.Metrics,
		Filters:       req.Filters,
		ColumnOrder:   req.ColumnOrder,
		ColumnWidths:  req.ColumnWidths,
		HiddenColumns: req.HiddenColumns,
		GroupBy:       req.GroupBy,
		Timezone:      req.Timezone,
		DateBasis:     req.DateBasis,
	}
	if req.Period != nil {
		p := req.Period.toPeriod()
		in.Period = &p
	}
	preset, err := h.svc.Update(r.Context(), orgID, chi.URLParam(r, "id"), in)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, preset)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	if err := h.svc.Delete(r.Context(), orgID, chi.URLParam(r, "id")); err != nil {
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
