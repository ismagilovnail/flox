package routing

// pickWeighted mirrors lib/routing-simulate.ts's pickWeightedFlow: only
// Active flows compete, weight is a raw integer normalized to a percent of
// the active weight sum for display, and a flow with weight 0 (or all
// flows inactive/zero-weight) never gets selected. rand01 must return a
// value in [0, 1) — injected so tests can assert exact picks (a fixed
// value) and statistical distribution (a real PRNG over many trials, §58)
// without this package importing math/rand itself at the call site.
func pickWeighted(flows []Flow, rand01 func() float64) (candidates []FlowCandidate, selected *Flow) {
	var active []Flow
	weightSum := 0
	for _, f := range flows {
		if f.Active {
			active = append(active, f)
			weightSum += f.Weight
		}
	}

	candidates = make([]FlowCandidate, len(flows))
	if len(active) == 0 || weightSum <= 0 {
		for i, f := range flows {
			candidates[i] = FlowCandidate{FlowID: f.ID, Name: f.Name, Weight: f.Weight, NormalizedPercent: 0, Selected: false}
		}
		return candidates, nil
	}

	roll := rand01() * float64(weightSum)
	selectedID := active[len(active)-1].ID // float rounding fallback, matches the frontend
	remaining := roll
	for _, f := range active {
		if remaining < float64(f.Weight) {
			selectedID = f.ID
			break
		}
		remaining -= float64(f.Weight)
	}

	for i, f := range flows {
		pct := 0.0
		if f.Active {
			pct = float64(f.Weight) / float64(weightSum) * 100
		}
		candidates[i] = FlowCandidate{FlowID: f.ID, Name: f.Name, Weight: f.Weight, NormalizedPercent: pct, Selected: f.ID == selectedID}
		if f.ID == selectedID {
			selected = &flows[i]
		}
	}
	return candidates, selected
}
