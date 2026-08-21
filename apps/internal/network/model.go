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
