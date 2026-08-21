// Package streamset implements the write path for Stream Sets, their
// recursive AND/OR filter trees, and their weighted Flows (§21/§39,
// Phase 7-9). The read path — loading this exact schema into the pure
// decision engine's types and evaluating a route — already exists
// (apps/internal/routingstore + apps/internal/routing, used by the
// tracker's hot path) and stays the single source of truth for routing
// decisions (CLAUDE.md #1); this package only ever writes rows for that
// existing reader to later load, never re-implements matching or
// weighted selection itself.
//
// FilterField/FilterOperator/Joiner/DestinationKind/StreamSetStatus are
// reused directly from internal/routing rather than redefined — one
// enum, not two copies that could drift.
package streamset

import (
	"time"

	"github.com/ismagilovnail/flox/apps/internal/routing"
)

type Status = routing.StreamSetStatus

const (
	StatusActive Status = routing.StreamSetActive
	StatusPaused Status = routing.StreamSetPaused
)

// FilterNodeKind mirrors the frontend's FilterNode discriminated union
// (lib/filters.ts: FilterCondition | FilterGroupNode) as a single
// flattened struct — same "small fixed set of variants, flatten it" call
// internal/routing/trace.go already made for its own Trace type.
type FilterNodeKind string

const (
	NodeCondition FilterNodeKind = "condition"
	NodeGroup     FilterNodeKind = "group"
)

// FilterNode is either a condition (Kind == NodeCondition) or a group
// (Kind == NodeGroup) — never both, enforced by validation, not by the
// Go type system, matching the frontend's own runtime-checked union.
type FilterNode struct {
	Kind FilterNodeKind `json:"type"`

	// condition fields
	Field    routing.FilterField    `json:"field,omitempty"`
	Operator routing.FilterOperator `json:"operator,omitempty"`
	Value    string                 `json:"value,omitempty"`
	ValueTo  string                 `json:"valueTo,omitempty"`

	// group fields
	Joiner   routing.Joiner `json:"joiner,omitempty"`
	Children []FilterNode   `json:"children,omitempty"`
}

type Destination struct {
	Kind routing.DestinationKind `json:"kind"`
	// offer only
	NetworkID string `json:"networkId,omitempty"`
	OfferID   string `json:"offerId,omitempty"`
	// redirect only
	URL string `json:"url,omitempty"`
}

type Flow struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Active      bool        `json:"active"`
	Weight      int         `json:"weight"`
	Destination Destination `json:"destination"`
}

// FlowInput omits ID — Flows are replaced wholesale on every write
// (matching internal/offer's own offer_links precedent and the frontend
// form's whole-array submission), so the server always mints fresh ids.
type FlowInput struct {
	Name        string
	Active      bool
	Weight      int
	Destination Destination
}

type StreamSet struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organizationId"`
	CampaignID     string     `json:"campaignId"`
	Name           string     `json:"name"`
	Priority       int        `json:"priority"`
	Status         Status     `json:"status"`
	FallbackURL    string     `json:"fallbackUrl"`
	RootFilter     FilterNode `json:"rootFilter"`
	Flows          []Flow     `json:"flows"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type CreateInput struct {
	Name        string
	Priority    int
	FallbackURL string
	RootFilter  FilterNode
	Flows       []FlowInput
}

// UpdateInput fields are pointers so PATCH can distinguish "not sent"
// from "sent" — matching every other domain's UpdateInput this session.
// RootFilter/Flows, when present, replace the whole tree/array — there is
// no partial filter-tree or per-flow PATCH, same reasoning as offer_links.
type UpdateInput struct {
	Name        *string
	Priority    *int
	Status      *Status
	FallbackURL *string
	RootFilter  *FilterNode
	Flows       *[]FlowInput
}
