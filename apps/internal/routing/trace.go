package routing

// Trace mirrors the frontend's ConditionTrace | GroupTrace union
// (lib/routing-simulate.ts) as a single flattened struct rather than a
// tagged pointer pair — same "small fixed set of variants, flatten it"
// call already made for Flow.Destination and the Postgres flows table;
// keeps this trivially JSON-serializable for the future /routing/simulate
// response with no extra marshaling code.
type TraceKind string

const (
	TraceCondition TraceKind = "condition"
	TraceGroup     TraceKind = "group"
)

// JSON tags below match the frontend's ConditionTrace/GroupTrace/
// StreamSetEvaluation/FlowCandidate field names exactly (lib/routing-
// simulate.ts, retired by Phase 27 in favor of these types going over
// the wire directly) — the doc comment above was written with exactly
// this reuse in mind, so /routing/simulate needs no separate DTO layer
// for these nested shapes.
type Trace struct {
	Kind   TraceKind `json:"kind"`
	Passed bool      `json:"passed"`

	// condition fields (Kind == TraceCondition)
	Field        FilterField    `json:"field,omitempty"`
	Operator     FilterOperator `json:"operator,omitempty"`
	Value        string         `json:"value,omitempty"`
	ValueTo      string         `json:"valueTo,omitempty"`
	RequestValue string         `json:"requestValue,omitempty"`

	// group fields (Kind == TraceGroup). Children has no omitempty:
	// Go's encoding/json omits a slice under omitempty whenever len==0,
	// nil or not — an empty top-level group ("no filters — matches all
	// traffic") has a real, non-nil, zero-length Children, and the
	// frontend's GroupTrace.children is called unconditionally
	// (.length, .map(...)); omitting the key crashes it. See
	// docs/routing-simulate.md and streamset.FilterNode's identical fix.
	Joiner   Joiner  `json:"joiner,omitempty"`
	Children []Trace `json:"children"`
}

// StreamSetEvaluation mirrors the frontend's StreamSetEvaluation — one
// entry per stream set considered, in priority order, whether or not it
// matched.
type StreamSetEvaluation struct {
	StreamSetID      string          `json:"streamSetId"`
	Name             string          `json:"name"`
	Priority         int             `json:"priority"`
	Status           StreamSetStatus `json:"status"`
	Matched          bool            `json:"matched"`
	ReasonNotMatched string          `json:"reasonNotMatched,omitempty"` // empty when Matched
	Trace            Trace           `json:"trace"`
}

// FlowCandidate mirrors the frontend's FlowCandidate — every flow
// considered for weighted selection, whether or not it was picked.
type FlowCandidate struct {
	FlowID            string  `json:"flowId"`
	Name              string  `json:"name"`
	Weight            int     `json:"weight"`
	NormalizedPercent float64 `json:"normalizedPercent"`
	Selected          bool    `json:"selected"`
}

// Explanation is the full "why" behind a RouteResult — everything §72
// asks a route decision to be able to answer, at the depth the frontend
// Routing Simulator (Phase 10) already renders. Router.Explain returns
// this alongside the spec-exact RouteResult; Resolve (the interface method
// §38 specifies) only returns RouteResult; both share one evaluation pass
// under the hood, so nothing here can drift from what Resolve actually
// decided.
type Explanation struct {
	StreamSetEvaluations []StreamSetEvaluation
	MatchedStreamSetID   string // empty if none matched
	FlowCandidates       []FlowCandidate
	StickyNote           string

	// DestinationLabel is resolveDestination's human label ("Offer",
	// "Redirect", "Stream Set fallback", "Campaign fallback", "No
	// destination configured") for the same RouteResult.Destination URL —
	// computed once, at the same call site that already resolves the URL,
	// so it can never disagree with what was actually picked.
	DestinationLabel string
}
