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

type Trace struct {
	Kind   TraceKind
	Passed bool

	// condition fields (Kind == TraceCondition)
	Field        FilterField
	Operator     FilterOperator
	Value        string
	ValueTo      string
	RequestValue string

	// group fields (Kind == TraceGroup)
	Joiner   Joiner
	Children []Trace
}

// StreamSetEvaluation mirrors the frontend's StreamSetEvaluation — one
// entry per stream set considered, in priority order, whether or not it
// matched.
type StreamSetEvaluation struct {
	StreamSetID      string
	Name             string
	Priority         int
	Status           StreamSetStatus
	Matched          bool
	ReasonNotMatched string // empty when Matched
	Trace            Trace
}

// FlowCandidate mirrors the frontend's FlowCandidate — every flow
// considered for weighted selection, whether or not it was picked.
type FlowCandidate struct {
	FlowID            string
	Name              string
	Weight            int
	NormalizedPercent float64
	Selected          bool
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
}
