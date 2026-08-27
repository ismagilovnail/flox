// Package network implements the Network API (§27/Phase 11):
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
	ID               string    `json:"id"`
	OrganizationID   string    `json:"organizationId"`
	Name             string    `json:"name"`
	PostbackURL      string    `json:"postbackUrl"`
	AcceptDuplicates bool      `json:"acceptDuplicates"`
	Status           Status    `json:"status"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type CreateInput struct {
	Name             string
	PostbackURL      string
	AcceptDuplicates bool
}

// UpdateInput fields are pointers so PATCH can distinguish "not sent"
// from "sent as zero value" — matching trafficsource.UpdateInput.
type UpdateInput struct {
	Name             *string
	PostbackURL      *string
	AcceptDuplicates *bool
	Status           *Status
}

// CreateResult is Service.Create's return shape — PostbackSecret appears
// here, once, and nowhere else: Network itself (returned by every other
// read — List/Get/Update/Duplicate) never carries it, matching the one-
// way-hash-at-rest precedent this shares with apps/internal/auth's
// session/invite tokens (§54/Phase 30).
type CreateResult struct {
	Network        Network
	PostbackSecret string
}
