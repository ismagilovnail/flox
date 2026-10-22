package analytics

import (
	"context"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/campaignscope"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
)

// readCampaigns is the campaigns the request may read (campaignscope,
// audit TEAM-1b): nil for every campaign, else exactly these — a Buyer's
// own, possibly none.
func readCampaigns(ctx context.Context) ([]string, error) {
	scope, err := campaignscope.From(ctx)
	if err != nil {
		return nil, err
	}
	if scope.All {
		return nil, nil
	}
	return scope.IDs, nil
}

// scopeFilters adds the request's campaign scope to filters: every report
// of a Buyer counts only their campaigns — totals, the Overview and
// spend included. Conversions with no campaign fall outside any scope.
func scopeFilters(ctx context.Context, filters []chstore.ReportFilter) ([]chstore.ReportFilter, error) {
	ids, err := readCampaigns(ctx)
	if err != nil || ids == nil {
		return filters, err
	}
	out := make([]chstore.ReportFilter, 0, len(filters)+1)
	out = append(out, filters...)
	return append(out, chstore.ReportFilter{Dimension: chstore.DimCampaign, In: ids}), nil
}

// requireCampaign answers 404 for a campaign outside the request's scope.
func requireCampaign(ctx context.Context, campaignID string) error {
	scope, err := campaignscope.From(ctx)
	if err != nil {
		return err
	}
	if !scope.Allows(campaignID) {
		return apierror.NotFound("campaign not found")
	}
	return nil
}
