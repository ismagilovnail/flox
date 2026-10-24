// Package network implements the Network API (§27):
// handler → service → repository, mirroring internal/trafficsource's
// shape closely — both are flat, team-managed entities with no nested
// children, referenced by other domains (campaigns reference traffic
// sources; offers/flows reference networks).
package network

import "time"

type Status string

const (
	StatusActive   Status = "active"
	StatusPaused   Status = "paused"
	StatusArchived Status = "archived"
)

func (s Status) Valid() bool {
	switch s {
	case StatusActive, StatusPaused, StatusArchived:
		return true
	default:
		return false
	}
}

type Network struct {
	ID               string `json:"id"`
	OrganizationID   string `json:"organizationId"`
	Name             string `json:"name"`
	PostbackURL      string `json:"postbackUrl"`
	AcceptDuplicates bool   `json:"acceptDuplicates"`
	// Currency is the payout currency: a postback with revenue but no
	// currency is taken to be in it (ISO 4217, upper case; USD default).
	Currency string `json:"currency"`
	// TemplateKey is the affiliate-platform template the network was set
	// up from (apps/web lib/network-templates.ts; audit INT-3); "" = none.
	TemplateKey string `json:"templateKey"`
	// BrowserConversions: apps/tracker also takes conversions from a page's
	// script at /c/{id} — no secret, only from BrowserDomains, only
	// statuses that map to CPA_HOLD or CPA_ACCEPT, once each per click
	// (audit UNI-1 part 2, docs/networks-offers.md "Own site without a
	// server"). BrowserDomains are host names; their subdomains count too.
	BrowserConversions bool     `json:"browserConversions"`
	BrowserDomains     []string `json:"browserDomains"`
	// IncomingPostbackURL is where the network sends its postbacks:
	// TRACKER_PUBLIC_URL + /postback/{id}, without the query (the secret is
	// shown once and only the client composes it in). Empty when
	// TRACKER_PUBLIC_URL is not configured — never a guessed host (audit
	// 2026-10-15, UX-1).
	IncomingPostbackURL string    `json:"incomingPostbackUrl"`
	Status              Status    `json:"status"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

type CreateInput struct {
	Name             string
	PostbackURL      string
	AcceptDuplicates bool
	// Currency empty means USD.
	Currency    string
	TemplateKey string
}

// UpdateInput fields are pointers so PATCH can distinguish "not sent"
// from "sent as zero value" — matching trafficsource.UpdateInput.
type UpdateInput struct {
	Name             *string
	PostbackURL      *string
	AcceptDuplicates *bool
	Currency         *string
	TemplateKey      *string
	Status           *Status
	// BrowserDomains nil: unchanged.
	BrowserConversions *bool
	BrowserDomains     []string
}

// CreateResult is Service.Create's return shape — PostbackSecret appears
// here, once, and nowhere else: Network itself (returned by every other
// read — List/Get/Update/Duplicate) never carries it, matching the one-
// way-hash-at-rest precedent this shares with apps/internal/auth's
// session/invite tokens (§54).
type CreateResult struct {
	Network        Network
	PostbackSecret string
}

// DiagnosticsWindow is how far back Diagnostics looks.
const DiagnosticsWindow = 30 * 24 * time.Hour

// Diagnostics is what a network's own postbacks say about its setup.
type Diagnostics struct {
	WindowDays      int             `json:"windowDays"`
	RedepWithoutTxn RedepWithoutTxn `json:"redepWithoutTxn"`
}

// RedepWithoutTxn counts redeposits that came without txn_id. The dedup
// key of a REDEP is (click_id, CPA_REDEP, txn_id), so without the id every
// repeat deposit of a click after the first is refused as a duplicate
// (§45; audit 2026-10-15, DATA-2). Duplicates is what was lost.
type RedepWithoutTxn struct {
	Received   int        `json:"received"`
	Duplicates int        `json:"duplicates"`
	LastAt     *time.Time `json:"lastAt"`
}
