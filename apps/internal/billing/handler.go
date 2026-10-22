package billing

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

// Handler serves the plan, its usage and its payments. Plans change by
// paying (Checkout) or through apps/cmd/billing.
type Handler struct {
	svc      *Service
	payments *Payments
	// requirePay gates checkout and the payment history (settings.write);
	// Payments.Checkout also requires an Owner.
	requirePay func(http.Handler) http.Handler
	logger     *slog.Logger
}

func NewHandler(svc *Service, payments *Payments, requirePay func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, payments: payments, requirePay: requirePay, logger: logger}
}

func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.status)
	r.Get("/plans", h.plans)
	r.With(h.requirePay).Get("/payments", h.listPayments)
	r.With(h.requirePay).Get("/payments/{id}", h.getPayment)
	r.With(h.requirePay).Post("/checkout", h.checkout)
}

// RegisterNotifications mounts the provider's notification URL — no
// session, outside the tenant routes.
func (h *Handler) RegisterNotifications(r chi.Router) {
	r.Post("/", h.notify)
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	st, err := h.svc.Status(r.Context(), orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (h *Handler) plans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.svc.Plans(r.Context())
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plans": plans})
}

func (h *Handler) listPayments(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	list, err := h.payments.List(r.Context(), orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"payments": list})
}

func (h *Handler) getPayment(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	p, err := h.payments.Get(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type checkoutRequest struct {
	Plan   string `json:"plan"`
	Months int    `json:"months"`
}

func (h *Handler) checkout(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	var req checkoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	p, err := h.payments.Checkout(r.Context(), orgID, userID, req.Plan, req.Months)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// notification is the part of a YooKassa notification FLOX reads: which
// payment. The rest of the body is not trusted — it is not signed.
type notification struct {
	Event  string `json:"event"`
	Object struct {
		ID string `json:"id"`
	} `json:"object"`
}

// notify answers 200 once the payment is handled or is not ours, and 500
// when the provider could not be asked — the sender retries.
func (h *Handler) notify(w http.ResponseWriter, r *http.Request) {
	var n notification
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&n); err != nil || n.Object.ID == "" {
		apierror.Write(w, h.logger, apierror.Validation("not a payment notification", nil))
		return
	}
	if !strings.HasPrefix(n.Event, "payment.") {
		// refund.succeeded and the like name a refund, not a payment;
		// refunds are handled by hand (docs/billing.md).
		h.logger.Info("billing: notification ignored", "event", n.Event, "id", n.Object.ID)
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := h.payments.HandleNotification(r.Context(), n.Object.ID); err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
