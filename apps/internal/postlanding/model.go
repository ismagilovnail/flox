// Package postlanding implements the Postlanding API (§28): handler →
// service → repository, mirroring internal/landing's shape closely — both
// are flat, team-managed entities with no nested children, referenced by
// other domains (flows.postlanding_id, migration 00006 — no CRUD populates
// that column yet, but the schema and this package's own defensive delete
// handling both already account for it). Unlike Landing, there is no
// internal/external split and no server-computed URL: a postlanding's URL
// is always an advertiser/team-owned page the caller supplies directly.
package postlanding

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

// ValidEventTypes is the curated subset of the full §43 event model a
// postlanding can plausibly fire on — matches
// apps/web/src/lib/mock/postlandings.ts's POSTLANDING_EVENT_TYPES exactly
// (frontend source of truth being migrated to lib/api/postlanding.ts in
// this same phase). Not the full canonical event enum: that belongs to
// the Conversions/Postbacks domain, referenced by value here, not
// duplicated in shape.
var ValidEventTypes = []string{
	"PWA_INSTALL",
	"NOTIFICATION_REQUEST",
	"NOTIFICATION_SUBSCRIBE",
	"NOTIFICATION_DECLINE",
	"TG_JOIN",
	"TG_START",
}

func isValidEventType(e string) bool {
	for _, v := range ValidEventTypes {
		if e == v {
			return true
		}
	}
	return false
}

type Postlanding struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	Name           string    `json:"name"`
	URL            string    `json:"url"`
	Events         []string  `json:"events"`
	Status         Status    `json:"status"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type CreateInput struct {
	Name   string
	URL    string
	Events []string
}

// UpdateInput fields are pointers so PATCH can distinguish "not sent" from
// "sent as zero value" — matching landing.UpdateInput. Events is a pointer
// to a slice (not a bare slice) for the same reason: nil means "not sent",
// a non-nil empty slice would mean "clear the list" if that were ever a
// valid request shape (it isn't — Service still requires at least one
// event when Events is sent).
type UpdateInput struct {
	Name   *string
	URL    *string
	Events *[]string
	Status *Status
}
