package reportpresets

import (
	"context"
	"strings"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
)

const (
	nameMinLen      = 1
	nameMaxLen      = 80
	minRelativeDays = 1
	maxRelativeDays = 365
	dateLayout      = "2006-01-02"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// List ensures the org's system default preset exists before returning —
// see Repository.EnsureDefault's own doc comment for why this is done
// lazily here rather than at org-creation time.
func (s *Service) List(ctx context.Context, orgID string) ([]ReportPreset, error) {
	if err := s.repo.EnsureDefault(ctx, orgID); err != nil {
		return nil, err
	}
	return s.repo.List(ctx, orgID)
}

func (s *Service) Get(ctx context.Context, orgID, id string) (ReportPreset, error) {
	return s.repo.Get(ctx, orgID, id)
}

func (s *Service) Create(ctx context.Context, orgID, createdByUserID string, in CreateInput) (ReportPreset, error) {
	if err := validateMutable(in.Name, in.Dimensions, in.Metrics, in.Filters, in.ColumnOrder, in.ColumnWidths, in.HiddenColumns, in.GroupBy, &in.Period, in.Timezone); err != nil {
		return ReportPreset{}, err
	}
	return s.repo.Create(ctx, orgID, createdByUserID, in)
}

func (s *Service) Update(ctx context.Context, orgID, id string, in UpdateInput) (ReportPreset, error) {
	existing, err := s.repo.Get(ctx, orgID, id)
	if err != nil {
		return ReportPreset{}, err
	}
	if existing.IsDefault {
		return ReportPreset{}, apierror.Conflict("the default preset can't be renamed or edited")
	}

	// Validate the resulting merged state (existing fields overlaid with
	// whatever the caller is changing), not just the delta — a partial
	// update must never leave a preset in an invalid combined state.
	name := existing.Name
	if in.Name != nil {
		name = *in.Name
	}
	dims := existing.Dimensions
	if in.Dimensions != nil {
		dims = in.Dimensions
	}
	metrics := existing.Metrics
	if in.Metrics != nil {
		metrics = in.Metrics
	}
	filters := existing.Filters
	if in.Filters != nil {
		filters = in.Filters
	}
	columnOrder := existing.ColumnOrder
	if in.ColumnOrder != nil {
		columnOrder = in.ColumnOrder
	}
	columnWidths := existing.ColumnWidths
	if in.ColumnWidths != nil {
		columnWidths = in.ColumnWidths
	}
	hiddenColumns := existing.HiddenColumns
	if in.HiddenColumns != nil {
		hiddenColumns = in.HiddenColumns
	}
	groupBy := existing.GroupBy
	if in.GroupBy != nil {
		groupBy = *in.GroupBy
	}
	period := existing.Period
	if in.Period != nil {
		period = *in.Period
	}
	timezone := existing.Timezone
	if in.Timezone != nil {
		timezone = *in.Timezone
	}
	if err := validateMutable(name, dims, metrics, filters, columnOrder, columnWidths, hiddenColumns, groupBy, &period, timezone); err != nil {
		return ReportPreset{}, err
	}

	return s.repo.Update(ctx, orgID, id, in)
}

func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	existing, err := s.repo.Get(ctx, orgID, id)
	if err != nil {
		return err
	}
	if existing.IsDefault {
		return apierror.Conflict("the default preset can't be deleted")
	}
	return s.repo.Delete(ctx, orgID, id)
}

func validateMutable(name string, dimensions, metrics []string, filters []Filter, columnOrder []string, columnWidths map[string]int, hiddenColumns []string, groupBy string, period *Period, timezone string) error {
	name = strings.TrimSpace(name)
	if len(name) < nameMinLen || len(name) > nameMaxLen {
		return apierror.Validation("name must be 1-80 characters", map[string]string{"name": "invalid length"})
	}

	if len(dimensions) == 0 || len(dimensions) > maxDimensions {
		return apierror.Validation("choose 1-4 dimensions", map[string]string{"dimensions": "invalid count"})
	}
	for _, d := range dimensions {
		// "day" is the report builder's per-day pseudo-dimension, not a column.
		if d != "day" && !chstore.ValidDimension(chstore.ReportDimension(d)) {
			return apierror.Validation("unknown dimension", map[string]string{"dimensions": d})
		}
	}

	if groupBy == "" || (groupBy != "day" && !chstore.ValidDimension(chstore.ReportDimension(groupBy))) {
		return apierror.Validation("unknown group-by dimension", map[string]string{"groupBy": groupBy})
	}

	if len(filters) > maxFilters {
		return apierror.Validation("too many filters", map[string]string{"filters": "too many"})
	}
	for _, f := range filters {
		// "day" is a bucket of time, not a value an event carries: not filterable.
		if f.Dimension == "day" || !chstore.ValidDimension(chstore.ReportDimension(f.Dimension)) {
			return apierror.Validation("unknown filter dimension", map[string]string{"filters": f.Dimension})
		}
		if len(f.Value) > maxFilterValueLen {
			return apierror.Validation("filter value too long", map[string]string{"filters": f.Dimension})
		}
	}

	if len(columnOrder) > maxColumnOrder {
		return apierror.Validation("too many columns in the order", map[string]string{"columnOrder": "too many"})
	}
	for _, id := range columnOrder {
		if id == "" || len(id) > maxColumnIDLen {
			return apierror.Validation("invalid column id in the order", map[string]string{"columnOrder": id})
		}
	}

	if len(hiddenColumns) > maxColumnOrder {
		return apierror.Validation("too many hidden columns", map[string]string{"hiddenColumns": "too many"})
	}
	for _, id := range hiddenColumns {
		if id == "" || len(id) > maxColumnIDLen {
			return apierror.Validation("invalid hidden column id", map[string]string{"hiddenColumns": id})
		}
	}

	if len(columnWidths) > maxColumnOrder {
		return apierror.Validation("too many column widths", map[string]string{"columnWidths": "too many"})
	}
	for id, w := range columnWidths {
		if id == "" || len(id) > maxColumnIDLen || w < minColumnWidth || w > maxColumnWidth {
			return apierror.Validation("invalid column width", map[string]string{"columnWidths": id})
		}
	}

	if len(metrics) == 0 {
		return apierror.Validation("choose at least one metric", map[string]string{"metrics": "empty"})
	}
	for _, m := range metrics {
		if !validMetrics[m] {
			return apierror.Validation("unknown metric", map[string]string{"metrics": m})
		}
	}

	if err := validatePeriod(period); err != nil {
		return err
	}

	if _, err := time.LoadLocation(timezone); err != nil {
		return apierror.Validation("invalid timezone", map[string]string{"timezone": timezone})
	}

	return nil
}

func validatePeriod(p *Period) error {
	if !p.Type.valid() {
		return apierror.Validation("invalid period type", map[string]string{"period.type": string(p.Type)})
	}
	switch p.Type {
	case PeriodRelative:
		if p.Days == nil || *p.Days < minRelativeDays || *p.Days > maxRelativeDays {
			return apierror.Validation("relative period must be 1-365 days", map[string]string{"period.days": "invalid"})
		}
	case PeriodCustom:
		if p.From == nil || p.To == nil {
			return apierror.Validation("custom period needs both from and to", map[string]string{"period": "incomplete"})
		}
		from, err := time.Parse(dateLayout, *p.From)
		if err != nil {
			return apierror.Validation("invalid period.from date", map[string]string{"period.from": *p.From})
		}
		to, err := time.Parse(dateLayout, *p.To)
		if err != nil {
			return apierror.Validation("invalid period.to date", map[string]string{"period.to": *p.To})
		}
		if from.After(to) {
			return apierror.Validation("period.from must not be after period.to", map[string]string{"period": "inverted range"})
		}
	}
	return nil
}
