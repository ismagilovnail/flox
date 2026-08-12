// Package routing is the single source of truth for routing decisions
// (§6-SHARED, §38, Phase 19) — stream-set priority, AND/OR filter
// evaluation, weighted flow selection, sticky assignment. It is consumed
// by apps/tracker and apps/worker (once they exist) and by the
// /routing/simulate HTTP endpoint (Phase 27); there is no second
// implementation anywhere, including the frontend Routing Simulator, which
// runs against a mock of this exact contract until Phase 27 swaps it for
// the real thing (docs/routing.md, ARCHITECTURE.md §6-SHARED decision:
// Strategy A).
//
// Deliberately independent of net/http and of any database driver (§38:
// "Keep routing independent from HTTP handlers"): Resolve is a pure
// function of (already-loaded configuration, already-classified request
// attributes) → decision. Tracking-link resolution, campaign lookup,
// traffic classification (bot/proxy scoring — Phase 20), and in-app
// WebView bounce detection all happen in the caller, before Resolve is
// ever invoked; that boundary is deliberate, not an oversight, and is
// spelled out case-by-case in the conformance fixture (fixture_test.go).
package routing

import "context"

// FilterField mirrors apps/web/src/lib/filters.ts's FilterField exactly —
// the same 31 request attributes a Stream Set's filter tree can reference.
type FilterField string

const (
	FieldCountry         FilterField = "country"
	FieldRegion          FilterField = "region"
	FieldCity            FilterField = "city"
	FieldDevice          FilterField = "device"
	FieldPlatform        FilterField = "platform"
	FieldOS              FilterField = "os"
	FieldOSVersion       FilterField = "os_version"
	FieldBrowser         FilterField = "browser"
	FieldBrowserVersion  FilterField = "browser_version"
	FieldLanguage        FilterField = "language"
	FieldBot             FilterField = "bot"
	FieldProxy           FilterField = "proxy"
	FieldASN             FilterField = "asn"
	FieldConnectionType  FilterField = "connection_type"
	FieldReferrer        FilterField = "referrer"
	FieldUTMSource       FilterField = "utm_source"
	FieldUTMMedium       FilterField = "utm_medium"
	FieldUTMCampaign     FilterField = "utm_campaign"
	FieldUTMContent      FilterField = "utm_content"
	FieldUTMTerm         FilterField = "utm_term"
	FieldSub1            FilterField = "sub1"
	FieldSub2            FilterField = "sub2"
	FieldSub3            FilterField = "sub3"
	FieldSub4            FilterField = "sub4"
	FieldSub5            FilterField = "sub5"
	FieldSub6            FilterField = "sub6"
	FieldSub7            FilterField = "sub7"
	FieldSub8            FilterField = "sub8"
	FieldSub9            FilterField = "sub9"
	FieldSub10           FilterField = "sub10"
	FieldExternalClickID FilterField = "external_click_id"
)

// FilterOperator mirrors lib/filters.ts's FilterOperator exactly.
type FilterOperator string

const (
	OpIs          FilterOperator = "IS"
	OpIsNot       FilterOperator = "IS_NOT"
	OpIn          FilterOperator = "IN"
	OpNotIn       FilterOperator = "NOT_IN"
	OpContains    FilterOperator = "CONTAINS"
	OpNotContains FilterOperator = "NOT_CONTAINS"
	OpStartsWith  FilterOperator = "STARTS_WITH"
	OpEndsWith    FilterOperator = "ENDS_WITH"
	OpMatches     FilterOperator = "MATCHES"
	OpExists      FilterOperator = "EXISTS"
	OpNotExists   FilterOperator = "NOT_EXISTS"
	OpGT          FilterOperator = "GT"
	OpGTE         FilterOperator = "GTE"
	OpLT          FilterOperator = "LT"
	OpLTE         FilterOperator = "LTE"
	OpBetween     FilterOperator = "BETWEEN"
)

// Attributes is the classified/raw request context filters evaluate
// against — mirrors the frontend's SimulateRequest (Record<FilterField,
// string>) exactly, right down to being a plain string map: every field
// is compared as text (numeric comparison for GT/GTE/LT/LTE/BETWEEN is a
// property of the operator, not the storage type).
type Attributes map[FilterField]string

// FilterNode is either a FilterCondition or a FilterGroup — the recursive
// AND/OR tree mirroring lib/filters.ts's FilterNode union
// (FilterCondition | FilterGroupNode).
type FilterNode interface {
	evaluate(attrs Attributes) Trace
}

type FilterCondition struct {
	Field    FilterField
	Operator FilterOperator
	Value    string
	ValueTo  string // BETWEEN's upper bound only
}

type FilterGroup struct {
	Joiner   Joiner
	Children []FilterNode
}

type Joiner string

const (
	JoinAND Joiner = "AND"
	JoinOR  Joiner = "OR"
)

// Flow is one weighted destination candidate under a Stream Set — mirrors
// apps/web's Flow type (stream-sets.ts) and the flows table (Phase 17
// migrations), minus the landing/pwa/postlanding "stage" fields, which
// affect what the tracker serves *after* routing decides a flow, not the
// routing decision itself.
type Flow struct {
	ID          string
	Name        string
	Active      bool
	Weight      int
	Destination Destination
}

type DestinationKind string

const (
	DestinationOffer    DestinationKind = "offer"
	DestinationRedirect DestinationKind = "redirect"
)

// Destination mirrors the frontend's discriminated union (kind: "offer" |
// "redirect"). OfferActive is not part of the frontend mock today — the
// mock resolves an offer's URL once at flow-authoring time and never
// re-checks whether the offer is still active. The Go engine is
// deliberately stricter (§58 requires an "inactive offers" test case): an
// offer destination whose OfferActive is false is treated exactly like a
// missing destination, falling through to the stream set's/campaign's
// fallback.
type Destination struct {
	Kind        DestinationKind
	URL         string // redirect: the URL itself. offer: the offer's resolved link URL.
	OfferActive bool   // only meaningful when Kind == DestinationOffer
}

type StreamSetStatus string

const (
	StreamSetActive StreamSetStatus = "active"
	StreamSetPaused StreamSetStatus = "paused"
)

type StreamSet struct {
	ID          string
	Name        string
	Priority    int
	Status      StreamSetStatus
	RootFilter  FilterGroup
	Flows       []Flow
	FallbackURL string
}

// RoutingConfig is the campaign's already-loaded, versioned configuration
// (§39's "load configuration (versioned)" step is the caller's job — a
// repository/cache read — not this package's).
type RoutingConfig struct {
	CampaignID    string
	ConfigVersion int64
	FallbackURL   string
	StreamSets    []StreamSet

	// Sticky flags (§39-STICKY). StickyFlowKeepClickID is deliberately
	// absent — it only affects whether the caller reuses a click_id for
	// attribution, which has zero effect on which flow/destination gets
	// selected, so it never needs to reach this package. See
	// docs/routing.md.
	StickyFlow             bool
	StickyFlowSkipInactive bool
}

// StickyState is the caller's already-parsed sf_{campaignId} cookie value
// (setId:flowId[:clickId] — parsing the raw cookie string is an HTTP-layer
// concern, out of scope here). Nil means no cookie was present.
type StickyState struct {
	StreamSetID string
	FlowID      string
}

type RequestContext struct {
	Attributes Attributes
	Config     RoutingConfig
	Sticky     *StickyState
}

// RouteResult is §38's exact spec'd shape — the production hot-path return
// value. Reason is a terse, human-readable summary; Router.Explain (below)
// returns the full structured trace the future /routing/simulate endpoint
// and this package's own conformance tests need for deeper "why" answers
// (§72) without growing this struct beyond what §38 specifies.
type RouteResult struct {
	CampaignID    string
	StreamSetID   string
	FlowID        string
	Destination   string
	Reason        string
	StickyApplied bool
	ConfigVersion int64
}

// Router is §38's exact interface.
type Router interface {
	Resolve(ctx context.Context, req RequestContext) (RouteResult, error)
}
