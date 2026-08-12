package routing

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sort"
)

// Engine is the concrete Router. Rand01, if nil, defaults to a real
// math/rand/v2 source — tests inject a fixed or seeded one so weighted
// picks are reproducible (exact-value assertions) or statistically
// checkable (distribution-over-N-trials assertions, §58).
type Engine struct {
	Rand01 func() float64
}

var _ Router = (*Engine)(nil)

func (e *Engine) rand01() float64 {
	if e.Rand01 != nil {
		return e.Rand01()
	}
	return rand.Float64()
}

// Resolve is §38's exact interface method — the production hot-path
// decision, returning only what apps/tracker/apps/worker need to act.
func (e *Engine) Resolve(ctx context.Context, req RequestContext) (RouteResult, error) {
	result, _ := e.resolve(req)
	return result, nil
}

// Explain runs the identical evaluation as Resolve — same function, same
// decision, so the two can never disagree — and additionally returns the
// full per-stream-set/per-flow trace §72 asks routing decisions to be able
// to produce. This is what the future /routing/simulate endpoint (Phase
// 27) and this package's own conformance tests use; Resolve alone is what
// ships in the hot path.
func (e *Engine) Explain(ctx context.Context, req RequestContext) (RouteResult, Explanation) {
	return e.resolve(req)
}

func (e *Engine) resolve(req RequestContext) (RouteResult, Explanation) {
	sticky, note := e.trySticky(req)
	if sticky != nil {
		return *sticky, Explanation{StickyNote: sticky.Reason}
	}
	return e.freshEvaluate(req, note)
}

// trySticky returns a non-nil RouteResult when an existing sticky
// assignment should be honored. When it returns (nil, ""), sticky isn't in
// play at all (disabled or no cookie). When it returns (nil, note), sticky
// was present but had to be dropped — note explains why, and the caller
// falls through to a fresh evaluation.
func (e *Engine) trySticky(req RequestContext) (*RouteResult, string) {
	cfg := req.Config
	if !cfg.StickyFlow || req.Sticky == nil {
		return nil, ""
	}

	var set *StreamSet
	for i := range cfg.StreamSets {
		if cfg.StreamSets[i].ID == req.Sticky.StreamSetID {
			set = &cfg.StreamSets[i]
			break
		}
	}
	if set == nil {
		return nil, "sticky cookie referenced a stream set that no longer exists; re-evaluated fresh"
	}

	var flow *Flow
	for i := range set.Flows {
		if set.Flows[i].ID == req.Sticky.FlowID {
			flow = &set.Flows[i]
			break
		}
	}
	if flow == nil {
		return nil, "sticky cookie referenced a flow that no longer exists; re-evaluated fresh"
	}

	eligible := set.Status == StreamSetActive && flow.Active
	if !eligible && !cfg.StickyFlowSkipInactive {
		return nil, "sticky flow is now inactive and stickyFlowSkipInactive is false; re-evaluated fresh"
	}

	destURL, destLabel := resolveDestination(flow, set.FallbackURL, cfg.FallbackURL)
	reason := fmt.Sprintf("sticky assignment honored (stream set %q, flow %q)", set.Name, flow.Name)
	if !eligible {
		reason = fmt.Sprintf("sticky assignment honored despite inactive flow (stickyFlowSkipInactive=true; stream set %q, flow %q)", set.Name, flow.Name)
	}
	if destLabel != "" {
		reason += "; " + destLabel
	}

	return &RouteResult{
		CampaignID:    cfg.CampaignID,
		StreamSetID:   set.ID,
		FlowID:        flow.ID,
		Destination:   destURL,
		Reason:        reason,
		StickyApplied: true,
		ConfigVersion: cfg.ConfigVersion,
	}, ""
}

func (e *Engine) freshEvaluate(req RequestContext, stickyNote string) (RouteResult, Explanation) {
	cfg := req.Config

	sorted := make([]StreamSet, len(cfg.StreamSets))
	copy(sorted, cfg.StreamSets)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Priority < sorted[j].Priority })

	evaluations := make([]StreamSetEvaluation, len(sorted))
	var matched *StreamSet
	for i := range sorted {
		set := &sorted[i]
		trace := set.RootFilter.evaluate(req.Attributes)
		active := set.Status == StreamSetActive
		isMatch := active && trace.Passed

		reasonNotMatched := ""
		switch {
		case isMatch:
		case !active:
			reasonNotMatched = "stream set is paused"
		default:
			reasonNotMatched = "filters didn't match"
		}

		evaluations[i] = StreamSetEvaluation{
			StreamSetID:      set.ID,
			Name:             set.Name,
			Priority:         set.Priority,
			Status:           set.Status,
			Matched:          isMatch,
			ReasonNotMatched: reasonNotMatched,
			Trace:            trace,
		}

		if isMatch && matched == nil {
			matched = set
		}
	}

	var flowCandidates []FlowCandidate
	var selectedFlow *Flow
	matchedStreamSetID := ""
	if matched != nil {
		matchedStreamSetID = matched.ID
		flowCandidates, selectedFlow = pickWeighted(matched.Flows, e.rand01)
	}

	streamSetFallback := ""
	if matched != nil {
		streamSetFallback = matched.FallbackURL
	}
	destURL, destLabel := resolveDestination(selectedFlow, streamSetFallback, cfg.FallbackURL)

	reason := buildReason(matched, selectedFlow, destLabel)

	streamSetID, flowID := "", ""
	if matched != nil {
		streamSetID = matched.ID
	}
	if selectedFlow != nil {
		flowID = selectedFlow.ID
	}

	result := RouteResult{
		CampaignID:    cfg.CampaignID,
		StreamSetID:   streamSetID,
		FlowID:        flowID,
		Destination:   destURL,
		Reason:        reason,
		StickyApplied: false,
		ConfigVersion: cfg.ConfigVersion,
	}
	explanation := Explanation{
		StreamSetEvaluations: evaluations,
		MatchedStreamSetID:   matchedStreamSetID,
		FlowCandidates:       flowCandidates,
		StickyNote:           stickyNote,
	}
	return result, explanation
}

func buildReason(matched *StreamSet, flow *Flow, destLabel string) string {
	switch {
	case matched == nil:
		return "no stream set matched; used campaign fallback"
	case flow == nil:
		return fmt.Sprintf("stream set %q matched (priority %d) but has no active flow; used fallback", matched.Name, matched.Priority)
	default:
		return fmt.Sprintf("stream set %q matched (priority %d); flow %q selected by weight; %s", matched.Name, matched.Priority, flow.Name, destLabel)
	}
}

// resolveDestination mirrors lib/routing-simulate.ts's resolveDestination,
// extended with the OfferActive check §58 requires (see types.go's
// Destination doc comment).
func resolveDestination(flow *Flow, streamSetFallback, campaignFallback string) (url, label string) {
	if flow != nil {
		d := flow.Destination
		switch {
		case d.Kind == DestinationRedirect && d.URL != "":
			return d.URL, "Redirect"
		case d.Kind == DestinationOffer && d.URL != "" && d.OfferActive:
			return d.URL, "Offer"
		}
	}
	if streamSetFallback != "" {
		return streamSetFallback, "Stream Set fallback"
	}
	if campaignFallback != "" {
		return campaignFallback, "Campaign fallback"
	}
	return "", "No destination configured"
}
