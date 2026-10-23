// Package billing is an organization's plan, what it has used of it this
// month, and the soft limits that follow (audit 2026-10-15, BIZ-1;
// docs/billing.md).
//
// Usage is clicks — SOURCE_CLICK + SOURCE_FILTER — per UTC calendar month,
// read from ClickHouse's per-day rollup. Going over a plan never touches
// the tracker (invariant #9: the redirect stays fast and the client's paid
// traffic keeps flowing). What it does: a warning from 80%, a banner from
// 100%, and from 110% no new campaigns or stream sets until the plan
// changes or the month ends. Count limits (campaigns, domains, members)
// refuse the create that would go past them.
//
// Plans are paid online through a PaymentProvider (YooKassa), a prepaid
// number of months at a time (payments.go), or assigned by hand with
// apps/cmd/billing.
package billing

import "time"

// FreePlan is the plan of an organization with no organization_plans row.
const FreePlan = "free"

// Plan mirrors a plans row. A nil limit is unlimited.
type Plan struct {
	Code          string `json:"code"`
	Name          string `json:"name"`
	MonthlyClicks *int64 `json:"monthlyClicks"`
	MaxCampaigns  *int   `json:"maxCampaigns"`
	MaxDomains    *int   `json:"maxDomains"`
	MaxMembers    *int   `json:"maxMembers"`
	PriceCents    int    `json:"priceCents"`
	Currency      string `json:"currency"`
	// PriceRub is the monthly price in kopecks the plan is sold for
	// online (YooKassa); nil: not sold online.
	PriceRub *int64 `json:"priceRub"`
	// RawRetentionMonths: how long the plan keeps raw clicks (audit
	// DATA-6, docs/retention.md); reports outlive them through the hourly
	// rollup.
	RawRetentionMonths int `json:"rawRetentionMonths"`
}

// Subscription is the organization's current plan. PaidUntil nil: the
// plan is open-ended (Free, or assigned with apps/cmd/billing).
type Subscription struct {
	Plan      Plan
	Since     time.Time
	PaidUntil *time.Time
	// Retention is the organization's organization_retention row (audit
	// DATA-6), nil before internal/retention.Plans first ran for it.
	Retention *RetentionState
}

// RetentionState: the months the organization's raw clicks are kept now,
// and its plan's target since when.
type RetentionState struct {
	Months, Target int
	TargetSince    time.Time
}

// RetentionChange is a shorter raw-click retention that applies at At.
type RetentionChange struct {
	Months int       `json:"months"`
	At     time.Time `json:"at"`
}

// Resource is something a plan limits the creation of.
type Resource string

const (
	ResourceCampaign  Resource = "campaign"
	ResourceStreamSet Resource = "stream_set"
	ResourceDomain    Resource = "domain"
	ResourceMember    Resource = "member"
)

// Level is how far this month's clicks are into the plan.
type Level string

const (
	LevelOK       Level = "ok"
	LevelWarning  Level = "warning"  // from WarnAt
	LevelExceeded Level = "exceeded" // from 100%
	LevelBlocked  Level = "blocked"  // from BlockAt: no new campaigns or stream sets
	LevelUnknown  Level = "unknown"  // clicks could not be read (no ClickHouse)
)

const (
	WarnAt  = 0.8
	BlockAt = 1.1
)

// Counts are what the count limits are checked against: campaigns that are
// not archived, every domain, members that are not suspended (invited ones
// count — they take a seat once they accept).
type Counts struct {
	Campaigns int `json:"campaigns"`
	Domains   int `json:"domains"`
	Members   int `json:"members"`
}

// Usage is this period's clicks (nil when unknown) and the counts.
type Usage struct {
	Clicks *int64 `json:"clicks"`
	Counts
}

// Period is a UTC calendar month: From inclusive, To exclusive.
type Period struct {
	From time.Time
	To   time.Time
}

// Status is GET /billing: the plan, since when, this period and its usage.
type Status struct {
	Plan   Plan      `json:"plan"`
	Since  time.Time `json:"since"`
	Period struct {
		From string `json:"from"` // first day, YYYY-MM-DD
		To   string `json:"to"`   // last day, inclusive
	} `json:"period"`
	Usage Usage `json:"usage"`
	Level Level `json:"level"`
	// PaidUntil: the end of the paid period, nil when open-ended.
	// GraceUntil: when the organization moves to Free if not renewed.
	// Renew: the period ends within RemindBefore, or has ended.
	PaidUntil  *time.Time `json:"paidUntil"`
	GraceUntil *time.Time `json:"graceUntil"`
	Renew      bool       `json:"renew"`
	// CanPay: plans can be paid online on this deployment.
	CanPay bool `json:"canPay"`
	// RawRetentionMonths: how long the organization's raw clicks are kept
	// now (audit DATA-6) — the plan's, except while a shorter one waits
	// (RawRetentionShortens) or before the first pass applied the plan.
	RawRetentionMonths   int              `json:"rawRetentionMonths"`
	RawRetentionShortens *RetentionChange `json:"rawRetentionShortens"`
}

// Change is one organization_plans row, for apps/cmd/billing's history.
type Change struct {
	PlanCode  string
	StartedAt time.Time
	EndedAt   *time.Time
	Note      string
}
