package billing

import (
	"context"
	"errors"
	"time"
)

// PaymentProvider takes a payment for a plan (invariant #11: behind an
// interface; internal/billing/yookassa is the one in use). Create returns
// where to send the payer; Get is the truth about a payment — a
// notification only says which payment to read again.
type PaymentProvider interface {
	Name() string
	CreatePayment(ctx context.Context, req PaymentRequest) (ProviderPayment, error)
	GetPayment(ctx context.Context, id string) (ProviderPayment, error)
}

// PaymentRequest is one checkout. Amount is in minor units (kopecks).
type PaymentRequest struct {
	IdempotenceKey string
	Amount         int64
	Currency       string
	Description    string
	ReturnURL      string
	Metadata       map[string]string
	Receipt        *Receipt // nil: no receipt is sent with the payment
}

// Receipt is a 54-FZ receipt sent with a payment: one service item.
type Receipt struct {
	Email         string
	Item          string
	VATCode       int
	TaxSystemCode int // 0: not sent
}

// Provider payment statuses, as Payment.Status holds them.
const (
	PaymentPending   = "pending"
	PaymentSucceeded = "succeeded"
	PaymentCanceled  = "canceled"
)

// ProviderPayment is a payment as the provider reports it.
type ProviderPayment struct {
	ID                 string
	Status             string // PaymentPending, PaymentSucceeded or PaymentCanceled
	Amount             int64
	Currency           string
	ConfirmationURL    string
	CancellationReason string
	Metadata           map[string]string
}

// ErrProvider wraps a provider failure (network, 5xx): retry later.
var ErrProvider = errors.New("payment provider unavailable")

// Payment is a billing_payments row.
type Payment struct {
	ID                 string     `json:"id"`
	PlanCode           string     `json:"planCode"`
	Months             int        `json:"months"`
	Amount             int64      `json:"amount"` // minor units
	Currency           string     `json:"currency"`
	Status             string     `json:"status"`
	CancellationReason string     `json:"cancellationReason"`
	ConfirmationURL    string     `json:"confirmationUrl,omitempty"`
	AppliedAt          *time.Time `json:"appliedAt"`
	CreatedAt          time.Time  `json:"createdAt"`

	OrganizationID    string `json:"-"`
	ProviderPaymentID string `json:"-"`
}

// Months a plan can be paid for.
var PayableMonths = map[int]bool{1: true, 3: true, 12: true}

// GracePeriod is how long a plan stays on after its paid period, and
// RemindBefore how early its owners are asked to renew.
const (
	GracePeriod  = 3 * 24 * time.Hour
	RemindBefore = 3 * 24 * time.Hour
)
