// Package routingsimulate is the thin HTTP-facing wrapper around
// routingstore.LoadRoutingConfig + routing.Router.Explain that backs
// POST /campaigns/{campaignId}/routing/simulate (Phase 27, §6-SHARED
// Strategy A). It contains no routing decision logic of its own — every
// question the frontend Routing Simulator asks ("why did/didn't this
// stream set match", "which flow won the weighted draw") is answered by
// the exact same evaluation apps/tracker's hot path runs, via
// routing.Engine.Explain. This package only reshapes that answer into
// the wire response the frontend expects and translates known sentinel
// errors into apierror's envelope.
package routingsimulate

import "github.com/ismagilovnail/flox/apps/internal/routing"

// Destination is the resolved route's destination, structured for the
// frontend badge it renders: an empty URL always pairs with the "No
// destination configured" label, so the frontend can tell "nothing
// resolved" apart from a real destination without a separate enum.
type Destination struct {
	URL   string `json:"url"`
	Label string `json:"label"`
}

// Response is POST .../routing/simulate's body. It reshapes
// routing.Explanation for the frontend's SimulateResult contract
// (formerly lib/routing-simulate.ts, retired by this phase in favor of
// these types going over the wire directly): MatchedStreamSet nests the
// full evaluation the frontend renders rather than just an id, and
// Destination is structured instead of folded into RouteResult.Reason's
// free-text summary. StreamSetEvaluations/FlowCandidates reuse
// routing's own JSON-tagged types unchanged — no separate DTO layer for
// shapes that already match field-for-field.
type Response struct {
	Request              map[string]string             `json:"request"`
	StreamSetEvaluations []routing.StreamSetEvaluation `json:"streamSetEvaluations"`
	MatchedStreamSet     *routing.StreamSetEvaluation  `json:"matchedStreamSet"`
	FlowCandidates       []routing.FlowCandidate       `json:"flowCandidates"`
	Destination          Destination                   `json:"destination"`
	StickyNote           string                        `json:"stickyNote"`
}
