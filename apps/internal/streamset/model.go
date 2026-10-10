// Package streamset implements the write path for Stream Sets, their
// recursive AND/OR filter trees, and their weighted Flows (§21/§39). The
// read path — apps/internal/routingstore loading them for
// apps/internal/routing — is the single source of truth for routing
// decisions (CLAUDE.md #1); this package only writes rows for it, never
// re-implements matching or weighted selection.
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

	// group fields. Children has no omitempty: the frontend's
	// ApiFilterGroup always requires a children array (hydrateFilterNode
	// calls .map() on it unconditionally) — an empty top-level group ("no
	// filters, matches all traffic") is a real, UI-supported
	// configuration, and omitempty would encode its Children as JSON
	// null instead of [], crashing the frontend on load. See
	// docs/routing-simulate.md.
	Joiner   routing.Joiner `json:"joiner,omitempty"`
	Children []FilterNode   `json:"children"`
}

type Destination struct {
	Kind routing.DestinationKind `json:"kind"`
	// offer only
	NetworkID string `json:"networkId,omitempty"`
	OfferID   string `json:"offerId,omitempty"`
	// redirect only
	URL string `json:"url,omitempty"`
}

// PwaType is the per-Flow display mode for the PWA stage (how *this* flow
// shows the PWA: an internal page, an external redirect, or an iOS
// app-store link) — independent of which pwa manifest (pwa_id) is
// selected. Mirrors the flows table's own pwa_type CHECK constraint
// (migration 00006) and apps/web's PWA_TYPES.
type PwaType string

const (
	PwaTypeInternal PwaType = "internal"
	PwaTypeExternal PwaType = "external"
	PwaTypeIOSApp   PwaType = "ios_app"
)

func (t PwaType) Valid() bool {
	switch t {
	case PwaTypeInternal, PwaTypeExternal, PwaTypeIOSApp:
		return true
	default:
		return false
	}
}

// FlowLanding/FlowPwa/FlowPostlanding are the funnel stages a Flow can
// optionally run before its Destination (§25's canonical funnel:
// Landing -> PWA -> Postlanding -> Destination). Each carries its own
// Enabled flag independent of whether an id is set — matching the flows
// table's own landing_enabled/pwa_enabled/postlanding_enabled columns,
// which are separate from the nullable *_id columns specifically so an
// operator can toggle a stage off without losing their previous pick.
//
// No `omitempty` on the id/type fields: they must always be present on
// the wire as "" when unset, never an absent key. apps/web's ApiFlowLanding/
// ApiFlowPwa/ApiFlowPostlanding types (and the zod schemas built on them)
// declare landingId/pwaId/pwaType/postlandingId as required string/enum
// fields, not optional — an omitted key deserializes to `undefined` in
// JS, which z.string()/z.enum([...,""]) both reject as invalid. That
// silently failed client-side validation on every Update whenever any
// flow had a disabled stage (handleSubmit's default onInvalid is a
// no-op: no error shown, no request sent) — caught via manual browser
// testing in the Stream Set <-> Pixel attachment phase, but the bug
// predates it (introduced when these stages were added). Fixed here,
// not by loosening the frontend schema, since "" is the actual
// meaningful unset value the rest of this package already treats it as
// (see nullIfEmpty in repository.go).
type FlowLanding struct {
	Enabled   bool   `json:"enabled"`
	LandingID string `json:"landingId"`
	// AsPwa: show this landing as an installable PWA shell. Independent
	// of the Pwa stage below (its own pwa_id/pwa_type pick a *separate*
	// PWA manifest to install after the landing, if that stage is also
	// enabled) — this flag only affects how the landing itself renders.
	AsPwa bool `json:"asPwa"`
}

type FlowPwa struct {
	Enabled bool    `json:"enabled"`
	PwaID   string  `json:"pwaId"`
	PwaType PwaType `json:"pwaType"`
}

type FlowPostlanding struct {
	Enabled       bool   `json:"enabled"`
	PostlandingID string `json:"postlandingId"`
}

type Flow struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Active      bool            `json:"active"`
	Weight      int             `json:"weight"`
	Landing     FlowLanding     `json:"landing"`
	Pwa         FlowPwa         `json:"pwa"`
	Postlanding FlowPostlanding `json:"postlanding"`
	Destination Destination     `json:"destination"`
}

// FlowSummary is one row of the org-wide, cross-campaign Flows list
// (ROADMAP.md: "Flows have no top-level list view") — Flow's own fields
// plus the campaign/stream-set context that page needs but a flow
// nested inside its own stream set's editor already has for free from
// its surrounding page. UpdatedAt is the flow row's own last-write time
// — the same moment its ID (below) was last reminted, since Update
// deletes and reinserts every flow in a stream set whenever its Flows
// array is saved (see Repository.Update / insertFlows) rather than
// diffing and patching individual rows in place.
//
// That reminting is the reason this ID is NOT a stable, permanent
// identity the way StreamSet.ID or Campaign.ID are — but only a real
// editor save triggers it, not every write: apps/web's own
// onToggleStatus (stream-set-list.tsx) PATCHes {status} alone, leaving
// UpdateInput.Flows nil, which Repository.Update's own "else" branch
// just re-reads unchanged — quick pause/activate/reorder actions never
// touch a flow's id. Opening the full stream-set-editor.tsx form and
// clicking "Save changes" does, though, even having changed nothing
// about the flows themselves: that form is a single unified submit, not
// per-field patches, so it always resubmits the complete flows array,
// setting UpdateInput.Flows non-nil and hitting the delete-then-reinsert
// path (see TestListFlowsSpansCampaignsAndCarriesContext's own two
// subtests proving each case directly). apps/tracker still writes
// exactly this ID as stream_set_id's own DimFlow counterpart
// (routing.Engine's RouteResult.FlowID = flow.ID, apps/internal/routing/
// engine.go), so a flow's "View statistics" link keeps working, but
// only for events recorded since its current ID was minted — the next
// full editor save resets that flow's visible history to zero. Disclosed
// explicitly in the frontend (flow-columns.tsx's own comment + a UI
// hint), not silently shipped as if IDs were permanent — the same
// "document the gap, don't hide it" standard applied to Stream Set's
// own raw-ULID-in-filter-chip tradeoff.
type FlowSummary struct {
	Flow
	StreamSetID   string    `json:"streamSetId"`
	StreamSetName string    `json:"streamSetName"`
	CampaignID    string    `json:"campaignId"`
	CampaignName  string    `json:"campaignName"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// FlowInput omits ID — Flows are replaced wholesale on every write
// (matching internal/offer's own offer_links precedent and the frontend
// form's whole-array submission), so the server always mints fresh ids.
type FlowInput struct {
	Name        string
	Active      bool
	Weight      int
	Landing     FlowLanding
	Pwa         FlowPwa
	Postlanding FlowPostlanding
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
	// PixelIDs: which of the org's Pixels (apps/internal/pixel) this
	// Stream Set fires — a many-to-many via stream_set_pixels (migration
	// 00008), never a per-Flow concern. A pixel's own `events` field (not
	// referenced here) decides which §43 events actually trigger it; this
	// list is only "eligible to fire for traffic that matched this set."
	PixelIDs  []string  `json:"pixelIds"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type CreateInput struct {
	Name        string
	Priority    int
	FallbackURL string
	RootFilter  FilterNode
	Flows       []FlowInput
	PixelIDs    []string
}

// UpdateInput fields are pointers so PATCH can distinguish "not sent"
// from "sent" — matching every other domain's UpdateInput this session.
// RootFilter/Flows/PixelIDs, when present, replace the whole tree/array —
// there is no partial filter-tree/per-flow/per-pixel PATCH, same
// reasoning as offer_links.
type UpdateInput struct {
	Name        *string
	Priority    *int
	Status      *Status
	FallbackURL *string
	RootFilter  *FilterNode
	Flows       *[]FlowInput
	PixelIDs    *[]string
}
