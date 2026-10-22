package analytics

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/push"
)

// OwnerSource is each campaign's owner — every campaign of the
// organization, archived ones included, "" for an unowned one
// (*campaign.Repository).
type OwnerSource interface {
	Owners(ctx context.Context, organizationID string) (map[string]string, error)
	// OwnerNames is every member who owns a campaign, with their name.
	OwnerNames(ctx context.Context, organizationID string) ([]Buyer, error)
}

// Buyer is a member who owns campaigns, for the buyer dimension's labels.
type Buyer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Buyers names the buyer dimension's values — readable with
// analytics.read, unlike the team list (team.read), so every role that
// sees a report by buyer sees who the buyers are.
func (s *Service) Buyers(ctx context.Context, organizationID string) ([]Buyer, error) {
	if s.owners == nil {
		return []Buyer{}, nil
	}
	b, err := s.owners.OwnerNames(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("analytics: reading buyer names: %w", err)
	}
	return b, nil
}

// WithOwners makes the buyer dimension answerable (audit TEAM-1). Without
// it a report by buyer is refused.
func (s *Service) WithOwners(o OwnerSource) *Service {
	s.owners = o
	return s
}

// buyerPlan is how a request that names the buyer dimension is answered.
// Events carry no buyer, so the query runs by campaign: a buyer filter
// becomes "campaign IN the campaigns that member owns", and a buyer
// grouping becomes a campaign grouping whose rows regroup adds up per
// current owner — before the derived metrics are computed, so CR, ROI and
// the rest come from the buyer's sums, never from averaging campaigns.
// Unique clicks add up: they are counted per campaign.
type buyerPlan struct {
	owners map[string]string // campaign -> owner
	// asked are the dimensions the caller asked for; query those run.
	asked, query []chstore.ReportDimension
	// grouped: the caller groups by buyer, so rows are regrouped.
	grouped bool
}

// planBuyer rewrites dims and filters when they name the buyer; nil when
// they do not, and the request runs as is.
func (s *Service) planBuyer(ctx context.Context, organizationID string, dims []chstore.ReportDimension, filters []chstore.ReportFilter) (*buyerPlan, []chstore.ReportFilter, error) {
	grouped := slices.Contains(dims, chstore.DimBuyer)
	filtered := slices.ContainsFunc(filters, func(f chstore.ReportFilter) bool { return f.Dimension == chstore.DimBuyer })
	if !grouped && !filtered {
		return nil, filters, nil
	}
	if s.owners == nil {
		return nil, nil, apierror.Validation("reports by buyer are not available", map[string]string{"dimensions": "buyer"})
	}
	owners, err := s.owners.Owners(ctx, organizationID)
	if err != nil {
		return nil, nil, fmt.Errorf("analytics: reading campaign owners: %w", err)
	}

	out := make([]chstore.ReportFilter, 0, len(filters))
	for _, f := range filters {
		if f.Dimension != chstore.DimBuyer {
			out = append(out, f)
			continue
		}
		in := []string{}
		for campaignID, owner := range owners {
			if owner == f.Value {
				in = append(in, campaignID)
			}
		}
		sort.Strings(in)
		out = append(out, chstore.ReportFilter{Dimension: chstore.DimCampaign, In: in})
	}

	p := &buyerPlan{owners: owners, asked: dims, grouped: grouped}
	for _, d := range dims {
		if d == chstore.DimBuyer {
			d = chstore.DimCampaign
		}
		if !slices.Contains(p.query, d) {
			p.query = append(p.query, d)
		}
	}
	return p, out, nil
}

// regroup merges the accumulators of a by-campaign answer into the
// caller's dimensions: each row gets its campaign's owner as its buyer,
// and rows that then share every asked dimension (and day) add up.
func (p *buyerPlan) regroup(merged map[string]*rowAccumulator, order []string) (map[string]*rowAccumulator, []string) {
	if p == nil || !p.grouped {
		return merged, order
	}
	out := make(map[string]*rowAccumulator, len(merged))
	var outOrder []string
	for _, k := range order {
		src := merged[k]
		values := make(map[chstore.ReportDimension]string, len(p.asked))
		for _, d := range p.asked {
			if d == chstore.DimBuyer {
				values[d] = p.owners[src.dimValues[chstore.DimCampaign]]
			} else {
				values[d] = src.dimValues[d]
			}
		}
		key := rowKeyString(chstore.ReportKey{Day: src.day, DimValues: values}, p.asked)
		dst, ok := out[key]
		if !ok {
			dst = &rowAccumulator{day: src.day, dimValues: values}
			out[key] = dst
			outOrder = append(outOrder, key)
		}
		dst.add(src)
	}
	return out, outOrder
}

// add sums src into acc. A nil count or amount is unknown or unanswerable:
// it adds nothing, and the sum stays nil only when every part is nil — the
// same rule the campaign-less total applies to spend (CLAUDE.md #6: a
// buyer whose campaigns have no spend shows "—", one with some spend shows
// that spend).
func (acc *rowAccumulator) add(src *rowAccumulator) {
	acc.clicks += src.clicks
	acc.uniqueClicks += src.uniqueClicks
	acc.landViews += src.landViews
	acc.blocked += src.blocked
	acc.bots += src.bots
	acc.emptySubs += src.emptySubs
	acc.pwaInstalls += src.pwaInstalls
	acc.tgStarts += src.tgStarts
	acc.tgJoins += src.tgJoins
	acc.datedByConversion += src.datedByConversion
	acc.unpriced += src.unpriced
	acc.revenueUnknown = acc.revenueUnknown || src.revenueUnknown
	acc.conversions = addCount(acc.conversions, src.conversions)
	acc.registrations = addCount(acc.registrations, src.registrations)
	acc.purchases = addCount(acc.purchases, src.purchases)
	acc.repurchases = addCount(acc.repurchases, src.repurchases)
	acc.declines = addCount(acc.declines, src.declines)
	acc.trash = addCount(acc.trash, src.trash)
	acc.revenueUSD = addAmount(acc.revenueUSD, src.revenueUSD)
	acc.costUSD = addAmount(acc.costUSD, src.costUSD)
	if src.push != nil {
		if acc.push == nil {
			acc.push = &push.StatsCounts{}
		}
		acc.push.Sent += src.push.Sent
		acc.push.Delivered += src.push.Delivered
		acc.push.Failed += src.push.Failed
		acc.push.Unsubscribed += src.push.Unsubscribed
		acc.push.Opened += src.push.Opened
	}
}

func addCount(a, b *uint64) *uint64 {
	if b == nil {
		return a
	}
	sum := *b
	if a != nil {
		sum += *a
	}
	return &sum
}

func addAmount(a, b *float64) *float64 {
	if b == nil {
		return a
	}
	sum := *b
	if a != nil {
		sum += *a
	}
	return &sum
}

// buyerValues are the filter suggestions for the buyer dimension: every
// member who owns a campaign, and "" when some campaign has no owner.
func (s *Service) buyerValues(ctx context.Context, organizationID string) ([]string, error) {
	if s.owners == nil {
		return []string{}, nil
	}
	owners, err := s.owners.Owners(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("analytics: reading campaign owners: %w", err)
	}
	seen := map[string]bool{}
	out := []string{}
	for _, o := range owners {
		if !seen[o] {
			seen[o] = true
			out = append(out, o)
		}
	}
	sort.Strings(out)
	return out, nil
}

// campaignScope is the set of campaigns the filters allow, nil when they
// name no campaign — for spend, which Postgres keeps per campaign.
func campaignScope(filters []chstore.ReportFilter) map[string]bool {
	var allowed map[string]bool
	for _, f := range filters {
		if f.Dimension != chstore.DimCampaign {
			continue
		}
		set := map[string]bool{}
		if f.In == nil {
			set[f.Value] = true
		}
		for _, id := range f.In {
			set[id] = true
		}
		if allowed == nil {
			allowed = set
			continue
		}
		for id := range allowed { // AND of two campaign filters
			if !set[id] {
				delete(allowed, id)
			}
		}
	}
	return allowed
}
