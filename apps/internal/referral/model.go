// Package referral implements §30.7 (Phase 14.8) — a real, tenant-scoped
// (org-refers-org) referral program: a persistent code/link per org, real
// attribution of new signups that use it, an append-only ledger of
// accrual/adjustment/payout_paid transactions, and a pending -> approved
// -> paid|rejected payout state machine. See migration 00031's own doc
// comment for the full design rationale.
package referral

import "time"

type SignupStatus string

const (
	StatusTrial          SignupStatus = "trial"
	StatusActiveCustomer SignupStatus = "active_customer"
	StatusChurned        SignupStatus = "churned"
)

type TransactionType string

const (
	TxnAccrual    TransactionType = "accrual"
	TxnAdjustment TransactionType = "adjustment"
	TxnPayoutPaid TransactionType = "payout_paid"
)

func (t TransactionType) mutable() bool {
	return t == TxnAccrual || t == TxnAdjustment
}

type PayoutStatus string

const (
	PayoutPending  PayoutStatus = "pending"
	PayoutApproved PayoutStatus = "approved"
	PayoutPaid     PayoutStatus = "paid"
	PayoutRejected PayoutStatus = "rejected"
)

// Signup is a real row created automatically the moment a NEW org signs up
// using this org's referral code — see auth.Service.Signup and
// Service.RecordSignup. Name/Email snapshot the signing-up user at that
// moment (the mock's own ReferredSignup shape shows a person, e.g. "Marta
// Kowalski" — the referred org's initial owner, not the org itself).
type Signup struct {
	ID                     string       `json:"id"`
	OrganizationID         string       `json:"organizationId"`
	ReferredOrganizationID string       `json:"referredOrganizationId"`
	Name                   string       `json:"name"`
	Email                  string       `json:"email"`
	Status                 SignupStatus `json:"status"`
	SignedUpAt             time.Time    `json:"signedUpAt"`
}

// Transaction is one append-only ledger row. Amount's sign carries meaning
// (positive = credit, negative = debit) — matches the mock's own
// convention exactly, so the frontend needs no sign-flipping logic.
type Transaction struct {
	ID              string          `json:"id"`
	OrganizationID  string          `json:"organizationId"`
	Type            TransactionType `json:"type"`
	Amount          float64         `json:"amount"`
	Currency        string          `json:"currency"`
	Description     string          `json:"description"`
	RelatedSignupID *string         `json:"relatedSignupId,omitempty"`
	RelatedPayoutID *string         `json:"relatedPayoutId,omitempty"`
	CreatedByUserID string          `json:"createdByUserId"`
	CreatedAt       time.Time       `json:"createdAt"`
}

type Payout struct {
	ID                string       `json:"id"`
	OrganizationID    string       `json:"organizationId"`
	Amount            float64      `json:"amount"`
	Currency          string       `json:"currency"`
	Status            PayoutStatus `json:"status"`
	RequestedAt       time.Time    `json:"requestedAt"`
	RequestedByUserID string       `json:"requestedByUserId"`
	ResolvedAt        *time.Time   `json:"resolvedAt,omitempty"`
	ResolvedByUserID  *string      `json:"resolvedByUserId,omitempty"`
	Note              string       `json:"note"`
}

// Balance is always computed by aggregating Transaction+Payout rows —
// never stored — matching the mock's own explicit design commitment.
type Balance struct {
	TotalEarned      float64 `json:"totalEarned"`
	TotalPaid        float64 `json:"totalPaid"`
	PendingPayout    float64 `json:"pendingPayout"`
	AvailableBalance float64 `json:"availableBalance"`
}

// Dashboard hydrates the whole referral page in one call, same "one call
// per page load" precedent tags.ListAssignments already established.
type Dashboard struct {
	Code         string        `json:"code"`
	Link         string        `json:"link"`
	Balance      Balance       `json:"balance"`
	Signups      []Signup      `json:"signups"`
	Transactions []Transaction `json:"transactions"`
	Payouts      []Payout      `json:"payouts"`
}

// AddTransactionInput never accepts TxnPayoutPaid — that type is only
// ever written by Repository.MarkPayoutPaid itself, atomically alongside
// the payout status change.
type AddTransactionInput struct {
	Type            TransactionType
	Amount          float64
	Description     string
	RelatedSignupID *string
}
