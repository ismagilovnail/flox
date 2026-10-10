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

// cappedSource supplies the offers at their daily cap (internal/offercap);
// the simulator passes it to the engine so a simulated route matches what the
// tracker would do right now.
type cappedSource interface {
	Capped() map[string]struct{}
}

type Service struct {
	store  *routingstore.Store
	router explainer
	caps   cappedSource
}

func NewService(store *routingstore.Store, router explainer) *Service {
	return &Service{store: store, router: router}
}

// WithCaps makes simulations honor daily offer caps. Without it (nil
// monitor, ClickHouse down) no offer is ever treated as capped.
func (s *Service) WithCaps(caps cappedSource) *Service {
	s.caps = caps
	return s
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

	var capped map[string]struct{}
	if s.caps != nil {
		capped = s.caps.Capped()
	}
	result, explanation, err := s.router.Explain(ctx, routing.RequestContext{
		Attributes:   attrs,
		Config:       cr.Config,
		Sticky:       nil,
		VisitKey:     deriveVisitKey(attributes),
		CappedOffers: capped,
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

// deriveVisitKey builds the simulator's visit key: sorted non-empty
// field=value pairs joined by "|", so the same attributes always give the
// same pick.
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
