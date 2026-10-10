package routing

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

// Engine is the concrete Router.
//
// It has no fields, and that is the point: the engine holds no state and no
// entropy. Every decision is a pure function of (configuration, attributes,
// sticky cookie, visit key), so two replicas resolve the same request the same
// way and a restart changes nothing (§38). The weighted draw is a hash of
// RequestContext's VisitKey, reproducible and uniform.
type Engine struct{}

var _ Router = (*Engine)(nil)

// Resolve is §38's exact interface method — the production hot-path
// decision, returning only what apps/tracker/apps/worker need to act.
// Timed for §53's routing_latency metric — Explain (below), used by
// /routing/simulate and this package's own conformance tests rather than
// the hot path, deliberately is not, so the metric reflects real
// production decisions only.
func (e *Engine) Resolve(ctx context.Context, req RequestContext) (RouteResult, error) {
	start := time.Now()
	result, _, err := e.resolve(req)
	metrics.RoutingLatencySeconds.Observe(time.Since(start).Seconds())
	return result, err
}

// Explain runs the identical evaluation as Resolve — same function, same
// decision, so the two can never disagree — and additionally returns the
// full per-stream-set/per-flow trace §72 asks routing decisions to be able
// to produce. This is what the future /routing/simulate endpoint (Phase
// 27) and this package's own conformance tests use; Resolve alone is what
// ships in the hot path.
func (e *Engine) Explain(ctx context.Context, req RequestContext) (RouteResult, Explanation, error) {
	return e.resolve(req)
}

func (e *Engine) resolve(req RequestContext) (RouteResult, Explanation, error) {
	// Sticky short-circuits before any draw happens, so an honored assignment
	// needs no VisitKey — the cookie already carries the answer the draw would
	// have produced (§39-STICKY).
	sticky, note, destLabel := e.trySticky(req)
	if sticky != nil {
		return *sticky, Explanation{StickyNote: sticky.Reason, DestinationLabel: destLabel}, nil
	}
	return e.freshEvaluate(req, note)
}

// trySticky returns a non-nil RouteResult when an existing sticky
// assignment should be honored. When it returns (nil, "", ""), sticky
// isn't in play at all (disabled or no cookie). When it returns (nil,
// note, ""), sticky was present but had to be dropped — note explains
// why, and the caller falls through to a fresh evaluation. The third
// return is the resolved destination's human label, only meaningful
// alongside a non-nil result.
func (e *Engine) trySticky(req RequestContext) (*RouteResult, string, string) {
	cfg := req.Config
	if !cfg.StickyFlow || req.Sticky == nil {
		return nil, "", ""
	}

	var set *StreamSet
	for i := range cfg.StreamSets {
		if cfg.StreamSets[i].ID == req.Sticky.StreamSetID {
			set = &cfg.StreamSets[i]
			break
		}
	}
	if set == nil {
		return nil, "sticky cookie referenced a stream set that no longer exists; re-evaluated fresh", ""
	}

	var flow *Flow
	for i := range set.Flows {
		if set.Flows[i].ID == req.Sticky.FlowID {
			flow = &set.Flows[i]
			break
		}
	}
	if flow == nil {
		return nil, "sticky cookie referenced a flow that no longer exists; re-evaluated fresh", ""
	}

	if offerCapped(req.CappedOffers, flow) {
		return nil, "sticky flow's offer reached its daily cap; re-evaluated fresh", ""
	}

	eligible := set.Status == StreamSetActive && flow.Active
	if !eligible && !cfg.StickyFlowSkipInactive {
		return nil, "sticky flow is now inactive and stickyFlowSkipInactive is false; re-evaluated fresh", ""
	}

	destURL, destLabel, offerID := resolveDestination(flow, set.FallbackURL, cfg.FallbackURL)
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
		OfferID:       offerID,
		Reason:        reason,
		StickyApplied: true,
		ConfigVersion: cfg.ConfigVersion,
	}, "", destLabel
}

func (e *Engine) freshEvaluate(req RequestContext, stickyNote string) (RouteResult, Explanation, error) {
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
	cappedNote := ""
	matchedStreamSetID := ""
	if matched != nil {
		matchedStreamSetID = matched.ID
		var err error
		flows, capped := applyCaps(matched.Flows, req.CappedOffers)
		flowCandidates, selectedFlow, err = pickWeighted(flows, req.VisitKey)
		if err != nil {
			return RouteResult{}, Explanation{}, err
		}
		for i := range flowCandidates {
			flowCandidates[i].Capped = capped[i]
		}
		if selectedFlow == nil && anyTrue(capped) {
			cappedNote = " (offers at their daily cap were skipped)"
		}
	}

	streamSetFallback := ""
	if matched != nil {
		streamSetFallback = matched.FallbackURL
	}
	destURL, destLabel, offerID := resolveDestination(selectedFlow, streamSetFallback, cfg.FallbackURL)

	reason := buildReason(matched, selectedFlow, destLabel) + cappedNote

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
		OfferID:       offerID,
		Reason:        reason,
		StickyApplied: false,
		ConfigVersion: cfg.ConfigVersion,
	}
	explanation := Explanation{
		StreamSetEvaluations: evaluations,
		MatchedStreamSetID:   matchedStreamSetID,
		FlowCandidates:       flowCandidates,
		StickyNote:           stickyNote,
		DestinationLabel:     destLabel,
	}
	return result, explanation, nil
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
//
// offerID is non-empty only on the "Offer" branch — see RouteResult.OfferID.
func resolveDestination(flow *Flow, streamSetFallback, campaignFallback string) (url, label, offerID string) {
	if flow != nil {
		d := flow.Destination
		switch {
		case d.Kind == DestinationRedirect && d.URL != "":
			return d.URL, "Redirect", ""
		case d.Kind == DestinationOffer && d.URL != "" && d.OfferActive:
			return d.URL, "Offer", d.OfferID
		}
	}
	if streamSetFallback != "" {
		return streamSetFallback, "Stream Set fallback", ""
	}
	if campaignFallback != "" {
		return campaignFallback, "Campaign fallback", ""
	}
	return "", "No destination configured", ""
}

// offerCapped reports whether flow sends traffic to an offer in capped.
func offerCapped(capped map[string]struct{}, flow *Flow) bool {
	if len(capped) == 0 || flow.Destination.Kind != DestinationOffer || flow.Destination.OfferID == "" {
		return false
	}
	_, ok := capped[flow.Destination.OfferID]
	return ok
}

// applyCaps returns flows with every capped-offer flow switched off, plus a
// parallel slice marking which ones were. The input is never modified (it
// belongs to the caller's config).
func applyCaps(flows []Flow, capped map[string]struct{}) ([]Flow, []bool) {
	marks := make([]bool, len(flows))
	if len(capped) == 0 {
		return flows, marks
	}
	out := make([]Flow, len(flows))
	copy(out, flows)
	for i := range out {
		if offerCapped(capped, &out[i]) {
			out[i].Active = false
			marks[i] = true
		}
	}
	return out, marks
}

func anyTrue(v []bool) bool {
	for _, b := range v {
		if b {
			return true
		}
	}
	return false
}
