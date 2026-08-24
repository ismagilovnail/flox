// Package landing implements the Landing API (§28): handler → service →
// repository, mirroring internal/network's shape closely — both are flat,
// team-managed entities with no nested children, referenced by other
// domains (flows.landing_id, migration 00006 — no CRUD populates that
// column yet, but the schema and this package's own defensive delete
// handling both already account for it).
package landing

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

type Type string

const (
	TypeInternal Type = "internal"
	TypeExternal Type = "external"
)

func (t Type) Valid() bool {
	switch t {
	case TypeInternal, TypeExternal:
		return true
	default:
		return false
	}
}

type Landing struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
	Type           Type   `json:"type"`
	// URL is resolved — hosted on our CDN for TypeInternal (Service
	// computes it from Name, never trusts a client-supplied value for
	// this type), the advertiser's own URL for TypeExternal.
	URL       string    `json:"url"`
	Content   string    `json:"content"` // TypeInternal only; empty for external
	Status    Status    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type CreateInput struct {
	Name    string
	Type    Type
	URL     string // TypeExternal only; ignored (recomputed) for TypeInternal
	Content string // TypeInternal only
}

// UpdateInput fields are pointers so PATCH can distinguish "not sent"
// from "sent as zero value" — matching network.UpdateInput.
type UpdateInput struct {
	Name    *string
	Type    *Type
	URL     *string
	Content *string
	Status  *Status
}
