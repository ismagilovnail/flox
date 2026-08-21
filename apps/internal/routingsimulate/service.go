package routingsimulate

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/routing"
	"github.com/ismagilovnail/flox/apps/internal/routingstore"
)

// explainer is the one method this package actually needs from
// *routing.Engine. routing.Router (the interface §38 specifies) only
// requires Resolve — Explain is Explain's own, separate concrete method,
// since the hot path never needs the trace. Naming the dependency here,
// rather than depending on *routing.Engine directly, keeps this package
// testable without a real Postgres-backed routingstore.Store standing in
// for the routing decision itself.
type explainer interface {
	Explain(ctx context.Context, req routing.RequestContext) (routing.RouteResult, routing.Explanation, error)
}

type Service struct {
	store  *routingstore.Store
	router explainer
}

func NewService(store *routingstore.Store, router explainer) *Service {
	return &Service{store: store, router: router}
}

// Simulate evaluates the campaign's real routing configuration against a
// caller-supplied set of request attributes. Sticky is always nil — the
// simulator has no real sf_{campaignId} cookie to honor, and simulating
// one would be indistinguishable from inventing a fake visitor.
func (s *Service) Simulate(ctx context.Context, orgID, campaignID string, attributes map[string]string) (Response, error) {
	cr, err := s.store.LoadRoutingConfig(ctx, orgID, campaignID)
	if err != nil {
		if errors.Is(err, routingstore.ErrNotFound) {
			return Response{}, apierror.NotFound("campaign not found")
		}
		return Response{}, err
	}

	attrs := routing.Attributes{}
	for k, v := range attributes {
		attrs[routing.FilterField(k)] = v
	}

	result, explanation, err := s.router.Explain(ctx, routing.RequestContext{
		Attributes: attrs,
		Config:     cr.Config,
		Sticky:     nil,
		VisitKey:   deriveVisitKey(attributes),
	})
	if err != nil {
		if errors.Is(err, routing.ErrNoVisitKey) {
			return Response{}, apierror.Validation("invalid simulate request", map[string]string{
				"request": "the matched stream set has more than one weighted flow — fill in at least one request attribute so the pick is deterministic",
			})
		}
		return Response{}, err
	}

	var matched *routing.StreamSetEvaluation
	for i := range explanation.StreamSetEvaluations {
		if explanation.StreamSetEvaluations[i].StreamSetID == explanation.MatchedStreamSetID {
			matched = &explanation.StreamSetEvaluations[i]
			break
		}
	}

	// No stream set matched leaves explanation.FlowCandidates nil (no draw
	// ever happened) — encoded as JSON null, which crashes the frontend's
	// array methods (SimulatorResult calls .some() on it unconditionally).
	// The frontend always renders a real, if empty, list.
	flowCandidates := explanation.FlowCandidates
	if flowCandidates == nil {
		flowCandidates = []routing.FlowCandidate{}
	}

	return Response{
		Request:              attributes,
		StreamSetEvaluations: explanation.StreamSetEvaluations,
		MatchedStreamSet:     matched,
		FlowCandidates:       flowCandidates,
		Destination:          Destination{URL: result.Destination, Label: explanation.DestinationLabel},
		StickyNote:           stickyNote(cr.Config.StickyFlow),
	}, nil
}

func stickyNote(enabled bool) string {
	if enabled {
		return "This campaign has sticky flow enabled — a returning visitor's sf_{campaignId} cookie would override this pick if present. The simulator always evaluates fresh, as if no cookie existed."
	}
	return "Sticky flow isn't enabled on this campaign — every visit is evaluated fresh."
}

// deriveVisitKey mirrors the frontend's own deriveVisitKey exactly
// (formerly lib/routing-simulate.ts): sorted non-empty field=value pairs
// joined by "|". Kept identical on both sides isn't load-bearing for
// correctness (the server is now the only place a real pick happens),
// but it means a request that produced one pick against the old mock
// still produces the same key shape against the real engine.
func deriveVisitKey(attrs map[string]string) string {
	keys := make([]string, 0, len(attrs))
	for k, v := range attrs {
		if v != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + attrs[k]
	}
	return strings.Join(parts, "|")
}
