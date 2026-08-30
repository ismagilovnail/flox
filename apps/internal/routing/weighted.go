package routing

import "errors"

// ErrNoVisitKey means a weighted draw was required but RequestContext.VisitKey
// was empty.
//
// This is a caller bug, not a routing outcome, and it is reported loudly on
// purpose. The alternative — hashing the empty string — would send every
// affected visit to whichever flow that one hash value lands in, silently
// routing 100% of traffic to one arm of an A/B test while every dashboard kept
// showing the configured split. A 502 on the first request is discovered in
// minutes; a corrupted experiment is discovered after the campaign is over.
var ErrNoVisitKey = errors.New("routing: weighted selection over several eligible flows needs a non-empty VisitKey")

// FNV-1a, 64-bit.
//
// Deliberately not hash/maphash: that one is seeded randomly per process, so
// two tracker replicas behind one load balancer would disagree about where the
// same visit belongs, and every restart would re-bucket every visitor — which
// is the whole failure this hash exists to prevent (§38). Not crypto either:
// nothing here is secret, and the hot path would notice.
const (
	fnvOffset64 = 14695981039346656037
	fnvPrime64  = 1099511628211
)

// VisitHash maps a visit key onto the 64-bit space the weighted draw divides
// up. Exported so the conformance fixture can assert the mapping is stable
// across builds — if this function's output ever changes, every returning
// visitor is silently re-bucketed.
func VisitHash(key string) uint64 {
	h := uint64(fnvOffset64)
	for i := 0; i < len(key); i++ {
		h ^= uint64(key[i])
		h *= fnvPrime64
	}
	return h
}

// pickWeighted selects one flow out of a stream set's flows, deterministically
// (§38).
//
// It backs both the tracker's live redirect decision and the routing
// simulator UI, which calls this same logic through the /routing/simulate
// API rather than reimplementing it in TypeScript (invariant #1 — single
// source of truth). The key itself is derived by the caller — the tracker
// fingerprints the visit, the simulate endpoint derives it from the request
// — because what identifies "the same visit" is an HTTP concern this package
// deliberately knows nothing about.
//
// Eligibility is decided BEFORE the draw: only flows that are both Active and
// carry a positive weight enter it, and the shares are relative to the sum of
// those weights rather than to 100. A paused or zero-weight flow therefore
// never wins and never absorbs share — pausing one arm of a split hands its
// traffic to the others immediately, without anyone having to re-balance the
// remaining weights first.
func pickWeighted(flows []Flow, key string) (candidates []FlowCandidate, selected *Flow, err error) {
	weightSum := 0
	eligible := 0
	for _, f := range flows {
		if f.Active && f.Weight > 0 {
			weightSum += f.Weight
			eligible++
		}
	}

	candidates = make([]FlowCandidate, len(flows))

	if weightSum == 0 {
		// Nothing can take traffic: every flow is paused, or all weights are
		// zero. Not an error — the caller falls back, and the trace shows why.
		for i, f := range flows {
			candidates[i] = FlowCandidate{FlowID: f.ID, Name: f.Name, Weight: f.Weight}
		}
		return candidates, nil, nil
	}

	// One candidate is not a draw, so it needs no key. This keeps the single
	// -flow case — by far the most common configuration — working for any
	// caller, while still refusing to guess when a real split is at stake.
	if eligible > 1 && key == "" {
		return nil, nil, ErrNoVisitKey
	}

	// Modulo bias: the hash spans 2^64 while weightSum is a sum of small
	// integers, so the final incomplete window is on the order of 2^-58 of the
	// space. It would take more clicks than this platform will ever serve for
	// that to become measurable against the ±2% the fixture allows.
	point := VisitHash(key) % uint64(weightSum)

	// Iterating `flows` in configuration order (rather than a filtered copy)
	// is what makes the mapping reproducible: the same config and the same key
	// always walk the same buckets in the same order.
	selectedID := ""
	var running uint64
	for _, f := range flows {
		if !f.Active || f.Weight <= 0 {
			continue
		}
		running += uint64(f.Weight)
		if point < running {
			selectedID = f.ID
			break
		}
	}

	for i, f := range flows {
		pct := 0.0
		if f.Active && f.Weight > 0 {
			pct = float64(f.Weight) / float64(weightSum) * 100
		}
		candidates[i] = FlowCandidate{
			FlowID:            f.ID,
			Name:              f.Name,
			Weight:            f.Weight,
			NormalizedPercent: pct,
			Selected:          f.ID == selectedID,
		}
		if f.ID == selectedID {
			selected = &flows[i]
		}
	}

	return candidates, selected, nil
}
