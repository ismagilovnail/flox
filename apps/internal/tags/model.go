// Package tags implements §30.6 — a cross-entity color-label system
// spanning Campaigns, CPA Networks, Offers, Flows, Traffic Sources, PWA
// apps, and Landing Pages. Two tables: a generic `tags` table and a
// polymorphic `taggables` join (see migration 00028's own doc comment
// for the tenant-isolation and referential-integrity reasoning).
package tags

import "time"

// EntityType is the fixed whitelist of taggable entities — matches the
// migration's own CHECK constraint and the frontend's TaggableEntityType
// union exactly. Postlanding/Pixels/Conversions/Domains/Team are NOT
// taggable per spec.
type EntityType string

const (
	EntityCampaign      EntityType = "campaign"
	EntityNetwork       EntityType = "network"
	EntityOffer         EntityType = "offer"
	EntityFlow          EntityType = "flow"
	EntityTrafficSource EntityType = "traffic_source"
	EntityPwa           EntityType = "pwa"
	EntityLanding       EntityType = "landing"
)

func (t EntityType) valid() bool {
	switch t {
	case EntityCampaign, EntityNetwork, EntityOffer, EntityFlow, EntityTrafficSource, EntityPwa, EntityLanding:
		return true
	}
	return false
}

// entityTable maps an EntityType to the real table it's checked against
// for ownership — the only place this mapping exists, so adding an
// eighth taggable entity later means one new line here (plus the
// migration's CHECK and the frontend union).
var entityTable = map[EntityType]string{
	EntityCampaign:      "campaigns",
	EntityNetwork:       "networks",
	EntityOffer:         "offers",
	EntityFlow:          "flows",
	EntityTrafficSource: "traffic_sources",
	EntityPwa:           "pwas",
	EntityLanding:       "landings",
}

// Colors is the fixed decorative palette a tag's `color` must be one of
// — mirrors the frontend's own TAG_COLORS id list exactly. Validated
// server-side too (CLAUDE.md: never trust client-side-only validation).
var Colors = map[string]bool{
	"red": true, "orange": true, "amber": true, "green": true, "teal": true,
	"blue": true, "indigo": true, "purple": true, "pink": true, "gray": true,
}

type Tag struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	Name           string    `json:"name"`
	Color          string    `json:"color"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type Assignment struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organizationId"`
	TagID          string     `json:"tagId"`
	EntityType     EntityType `json:"entityType"`
	EntityID       string     `json:"entityId"`
	CreatedAt      time.Time  `json:"createdAt"`
}

type CreateTagInput struct {
	Name  string
	Color string
}

type UpdateTagInput struct {
	Name  *string
	Color *string
}

type SetEntityTagsInput struct {
	EntityType EntityType
	EntityID   string
	TagIDs     []string
}

// BulkEditInput mirrors §30.6's bulk semantics exactly: ToAdd tags are
// assigned to every entity in EntityIDs (no-op where already present);
// ToRemove tags are unassigned from every entity in EntityIDs (no-op
// where absent). Tags untouched by the caller are left alone on every
// entity, even ones only some of the selection had.
type BulkEditInput struct {
	EntityType EntityType
	EntityIDs  []string
	ToAdd      []string
	ToRemove   []string
}
