package referral

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
	// requireWrite gates approve/reject/mark-paid/add-transaction — every
	// role already holds referral.read (migration 00031 grants it to all
	// six), so the dashboard read and requesting a payout carry no
	// permission requirement beyond tenantMiddleware, matching the
	// mock's own "visible to everyone, not gated by canManage" rule for
	// Request Payout.
	requireWrite func(http.Handler) http.Handler
}

func NewHandler(svc *Service, requireWrite func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, requireWrite: requireWrite, logger: logger}
}

func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.dashboard)
	r.Post("/payouts", h.requestPayout)
	r.With(h.requireWrite).Post("/payouts/{id}/approve", h.approvePayout)
	r.With(h.requireWrite).Post("/payouts/{id}/reject", h.rejectPayout)
	r.With(h.requireWrite).Post("/payouts/{id}/mark-paid", h.markPayoutPaid)
	r.With(h.requireWrite).Post("/transactions", h.addTransaction)
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	dash, err := h.svc.Dashboard(r.Context(), orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, dash)
}

type requestPayoutRequest struct {
	Amount float64 `json:"amount"`
}

func (h *Handler) requestPayout(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	var req requestPayoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	payout, err := h.svc.RequestPayout(r.Context(), orgID, userID, req.Amount)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, payout)
}

func (h *Handler) approvePayout(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	payout, err := h.svc.ApprovePayout(r.Context(), orgID, chi.URLParam(r, "id"), userID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, payout)
}

type rejectPayoutRequest struct {
	Note string `json:"note"`
}

func (h *Handler) rejectPayout(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	var req rejectPayoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	payout, err := h.svc.RejectPayout(r.Context(), orgID, chi.URLParam(r, "id"), userID, req.Note)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, payout)
}

func (h *Handler) markPayoutPaid(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	payout, err := h.svc.MarkPayoutPaid(r.Context(), orgID, chi.URLParam(r, "id"), userID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, payout)
}

type addTransactionRequest struct {
	Type            string  `json:"type"`
	Amount          float64 `json:"amount"`
	Description     string  `json:"description"`
	RelatedSignupID *string `json:"relatedSignupId"`
}

func (h *Handler) addTransaction(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	var req addTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	txn, err := h.svc.AddTransaction(r.Context(), orgID, userID, AddTransactionInput{
		Type:            TransactionType(req.Type),
		Amount:          req.Amount,
		Description:     req.Description,
		RelatedSignupID: req.RelatedSignupID,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, txn)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
